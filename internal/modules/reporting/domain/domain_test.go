package domain

import "testing"

func TestValidGroupDimension(t *testing.T) {
	valid := []GroupDimension{GroupByDay, GroupByMonth, GroupByCustomer, GroupBySupplier,
		GroupByProduct, GroupByCategory, GroupBySalesperson, GroupByBranch, GroupByWarehouse}
	for _, g := range valid {
		if !ValidGroupDimension(g) {
			t.Errorf("ValidGroupDimension(%q) = false, want true", g)
		}
	}

	invalid := []GroupDimension{"", "DROP TABLE users;--", "week", "year", "unknown"}
	for _, g := range invalid {
		if ValidGroupDimension(g) {
			t.Errorf("ValidGroupDimension(%q) = true, want false", g)
		}
	}
}
