package importer

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParseCSV_HappyPath(t *testing.T) {
	csv := "name,hsn_sac_code,base_uom_code\nWidget,8471,PCS\nGadget,8472,BOX\n"
	rows, err := ParseCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Number != 1 || rows[0].Fields["name"] != "Widget" || rows[0].Fields["hsn_sac_code"] != "8471" {
		t.Fatalf("row 1 = %+v, want Number=1 name=Widget hsn_sac_code=8471", rows[0])
	}
	if rows[1].Number != 2 || rows[1].Fields["base_uom_code"] != "BOX" {
		t.Fatalf("row 2 = %+v, want Number=2 base_uom_code=BOX", rows[1])
	}
}

// TestParseCSV_RaggedRowsDoNotError checks brief §53's "never silently
// discard a malformed row": a row with fewer fields than the header must
// still come back as a Row (with the missing fields empty), so the
// caller's own validation gets a chance to report it — a parse-time
// hard failure would instead abort the ENTIRE import on one bad row.
func TestParseCSV_RaggedRowsDoNotError(t *testing.T) {
	csv := "name,hsn_sac_code,base_uom_code\nWidget\n" // second row has only 1 of 3 columns
	rows, err := ParseCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Fields["name"] != "Widget" {
		t.Fatalf("name = %q, want Widget", rows[0].Fields["name"])
	}
	if rows[0].Fields["hsn_sac_code"] != "" || rows[0].Fields["base_uom_code"] != "" {
		t.Fatalf("missing trailing fields should be empty strings, got %+v", rows[0].Fields)
	}
}

func TestParseCSV_EmptyFile(t *testing.T) {
	rows, err := ParseCSV(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseCSV(empty): %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows for an empty file, want 0", len(rows))
	}
}

func TestParseXLSX_HappyPath(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	mustSetCell(t, f, sheet, "A1", "party_type")
	mustSetCell(t, f, sheet, "B1", "legal_name")
	mustSetCell(t, f, sheet, "A2", "CUSTOMER")
	mustSetCell(t, f, sheet, "B2", "Acme Traders")

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}
	rows, err := ParseXLSX(buf)
	if err != nil {
		t.Fatalf("ParseXLSX: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Fields["party_type"] != "CUSTOMER" || rows[0].Fields["legal_name"] != "Acme Traders" {
		t.Fatalf("row = %+v, want party_type=CUSTOMER legal_name=Acme Traders", rows[0])
	}
}

func mustSetCell(t *testing.T, f *excelize.File, sheet, cell, value string) {
	t.Helper()
	if err := f.SetCellValue(sheet, cell, value); err != nil {
		t.Fatalf("SetCellValue(%s): %v", cell, err)
	}
}

func TestBuilder_Report_CountsEachOutcome(t *testing.T) {
	b := NewBuilder(false)
	b.Committed(1)
	b.Committed(2)
	b.Valid(3) // shouldn't normally happen with dryRun=false, but Report just counts whatever was recorded
	b.Error(4, "bad row: %s", "reason")
	b.Duplicate(5, "dup: %s", "name")

	rep := b.Report()
	if rep.Total != 5 {
		t.Fatalf("Total = %d, want 5", rep.Total)
	}
	if rep.Committed != 2 || rep.Valid != 1 || rep.Errors != 1 || rep.Duplicates != 1 {
		t.Fatalf("counts = %+v, want Committed=2 Valid=1 Errors=1 Duplicates=1", rep)
	}
	if rep.Results[3].Message != "bad row: reason" {
		t.Fatalf("error message = %q, want formatted message", rep.Results[3].Message)
	}
}
