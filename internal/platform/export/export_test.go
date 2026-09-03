package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func sampleTable() Table {
	return Table{
		Title:   "Test Report",
		Headers: []string{"Name", "Amount"},
		Rows: [][]string{
			{"Alpha", "100.00"},
			{"Beta", "250.50"},
		},
	}
}

func TestWriteCSV_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, sampleTable()); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("re-reading CSV: %v", err)
	}
	if len(records) != 3 { // header + 2 rows
		t.Fatalf("got %d records, want 3", len(records))
	}
	if records[0][0] != "Name" || records[0][1] != "Amount" {
		t.Fatalf("header row = %v", records[0])
	}
	if records[1][0] != "Alpha" || records[1][1] != "100.00" {
		t.Fatalf("row 1 = %v", records[1])
	}
}

func TestWriteJSON_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, sampleTable()); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var decoded struct {
		Title   string     `json:"title"`
		Headers []string   `json:"headers"`
		Rows    [][]string `json:"rows"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decoding JSON: %v", err)
	}
	if decoded.Title != "Test Report" {
		t.Fatalf("title = %q", decoded.Title)
	}
	if len(decoded.Headers) != 2 || decoded.Headers[0] != "Name" || decoded.Headers[1] != "Amount" {
		t.Fatalf("headers = %v", decoded.Headers)
	}
	if len(decoded.Rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(decoded.Rows))
	}
	if decoded.Rows[0][0] != "Alpha" || decoded.Rows[0][1] != "100.00" {
		t.Fatalf("row 0 = %v", decoded.Rows[0])
	}
}

// TestWriteJSON_PreservesColumnOrder is a regression test for a real bug:
// an earlier map[string]string-per-row implementation passed
// TestWriteJSON_RoundTrips (which only checked 2 columns, alphabetically
// adjacent) while silently reordering columns whose alphabetical order
// differs from Headers — exactly apps/web's ReportTable's real-world
// case ("Key" before "Grand Total" in Headers, but "G" < "K"
// alphabetically).
func TestWriteJSON_PreservesColumnOrder(t *testing.T) {
	table := Table{
		Title:   "Order Test",
		Headers: []string{"Key", "Documents", "Taxable", "Tax", "Grand Total"},
		Rows:    [][]string{{"2026-09-01", "3", "1000.00", "180.00", "1180.00"}},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, table); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var decoded struct {
		Headers []string   `json:"headers"`
		Rows    [][]string `json:"rows"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decoding JSON: %v", err)
	}
	for i, h := range table.Headers {
		if decoded.Headers[i] != h {
			t.Fatalf("headers[%d] = %q, want %q (order not preserved)", i, decoded.Headers[i], h)
		}
	}
	for i, v := range table.Rows[0] {
		if decoded.Rows[0][i] != v {
			t.Fatalf("rows[0][%d] = %q, want %q (order not preserved)", i, decoded.Rows[0][i], v)
		}
	}
}

func TestWriteXLSX_ProducesReadableWorkbook(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteXLSX(&buf, sampleTable()); err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatalf("re-opening XLSX: %v", err)
	}
	defer f.Close()
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("reading rows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[1][0] != "Alpha" || rows[1][1] != "100.00" {
		t.Fatalf("row 1 = %v", rows[1])
	}
}

func TestWriteTablePDF_ProducesNonEmptyValidPDF(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteTablePDF(&buf, sampleTable()); err != nil {
		t.Fatalf("WriteTablePDF: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("PDF output is empty")
	}
	if !strings.HasPrefix(buf.String(), "%PDF-") {
		t.Fatal("output does not start with the PDF magic header %PDF-")
	}
}

func TestWriteTablePDF_EmptyRowsStillProducesValidPDF(t *testing.T) {
	var buf bytes.Buffer
	empty := Table{Title: "Empty Report", Headers: []string{"A", "B"}}
	if err := WriteTablePDF(&buf, empty); err != nil {
		t.Fatalf("WriteTablePDF with no rows: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "%PDF-") {
		t.Fatal("output does not start with the PDF magic header %PDF-")
	}
}
