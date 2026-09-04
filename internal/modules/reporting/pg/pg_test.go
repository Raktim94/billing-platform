package pg

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"rechvix/internal/modules/reporting/domain"
)

// TestGroupExpr_OnlyEmitsFixedLiteralExpressions is the SQL-injection
// defense check for the GROUP BY dimension mechanism (brief §62): every
// branch of groupExpr must return one of a small fixed set of hardcoded
// SQL fragments, never anything derived from caller input. There is no
// caller-supplied string anywhere in this function's inputs (the
// GroupDimension itself is validated against domain.ValidGroupDimension
// before this is ever called — app/service.go — so this test exists to
// pin the fixed-expression-set property itself, not to fuzz arbitrary
// input through it).
func TestGroupExpr_OnlyEmitsFixedLiteralExpressions(t *testing.T) {
	cases := []struct {
		dim           domain.GroupDimension
		wantSelectHas string
		wantJoinHas   string
	}{
		{domain.GroupByDay, "issue_date::text", ""},
		{domain.GroupByMonth, "to_char(sd.issue_date", ""},
		{domain.GroupByCustomer, "p.trade_name", ""},
		{domain.GroupByProduct, "pr.name", "product_variants"},
		{domain.GroupByCategory, "c.name", "categories"},
		{domain.GroupBySalesperson, "u.legal_name", "users"},
		{domain.GroupByBranch, "br.name", "branches"},
		{domain.GroupByWarehouse, "wh.name", "warehouses"},
	}
	for _, c := range cases {
		selectExpr, groupByExpr, join := groupExpr(c.dim, "sd", "sdl", "p", "issue_date")
		if !strings.Contains(selectExpr, c.wantSelectHas) {
			t.Errorf("%s: selectExpr = %q, want substring %q", c.dim, selectExpr, c.wantSelectHas)
		}
		if selectExpr != groupByExpr {
			t.Errorf("%s: selectExpr (%q) and groupByExpr (%q) must match — the GROUP BY must group by exactly what SELECT projects", c.dim, selectExpr, groupByExpr)
		}
		if c.wantJoinHas != "" && !strings.Contains(join, c.wantJoinHas) {
			t.Errorf("%s: join = %q, want substring %q", c.dim, join, c.wantJoinHas)
		}
	}
}

func TestCondAppend_EmptyForOrgOnlyFilter(t *testing.T) {
	w := newWhere(uuid.Must(uuid.NewV7()), "sb")
	if got := condAppend(w); got != "" {
		t.Errorf("condAppend with only the organisation_id clause = %q, want empty", got)
	}
}

func TestCondAppend_IncludesAdditionalClauses(t *testing.T) {
	w := newWhere(uuid.Must(uuid.NewV7()), "sb")
	wh := uuid.Must(uuid.NewV7())
	w.addOptionalUUID("sb.warehouse_id", &wh)
	got := condAppend(w)
	if !strings.HasPrefix(got, " AND ") {
		t.Errorf("condAppend = %q, want it to start with \" AND \"", got)
	}
	if !strings.Contains(got, "sb.warehouse_id = $2") {
		t.Errorf("condAppend = %q, want it to reference the second bind parameter", got)
	}
}

func TestWhereBuilder_BindParametersStayInOrder(t *testing.T) {
	orgID := uuid.Must(uuid.NewV7())
	w := newWhere(orgID, "sd")
	w.add("status", "FINALIZED")
	if len(w.args) != 2 {
		t.Fatalf("got %d args, want 2", len(w.args))
	}
	if w.args[0] != orgID {
		t.Errorf("args[0] = %v, want organisation id %v", w.args[0], orgID)
	}
	if w.args[1] != "FINALIZED" {
		t.Errorf("args[1] = %v, want \"FINALIZED\"", w.args[1])
	}
	if !strings.Contains(w.sql(), "$1") || !strings.Contains(w.sql(), "$2") {
		t.Errorf("sql() = %q, want both $1 and $2 placeholders", w.sql())
	}
}
