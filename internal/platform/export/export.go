// Package export provides shared, reusable table-to-file writers (CSV,
// XLSX, JSON, PDF) for the reporting module's export formats (brief §54).
// Every report's export handler converts its own typed rows into a plain
// []string-per-row table once, then calls one of these — the format
// mechanism itself is written once, not copy-pasted per report.
//
// brief §54 also asks for a background job + expiring download link for
// large exports. That needs a scheduled-job/outbox mechanism
// (docs/architecture.md §2, brief §34) that does not exist yet — Stage 9
// territory. Exports here are synchronous HTTP responses; large-export
// async delivery is a documented follow-up, not faked with a job queue
// that doesn't actually run anything in the background.
package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

// Table is a header row plus data rows, the common shape every writer in
// this package accepts. A report handler builds this once from its own
// typed row slice and reuses it across every requested format.
type Table struct {
	Title   string
	Headers []string
	Rows    [][]string
}

func WriteCSV(w io.Writer, t Table) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(t.Headers); err != nil {
		return fmt.Errorf("export: writing CSV header: %w", err)
	}
	for _, row := range t.Rows {
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("export: writing CSV row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

func WriteJSON(w io.Writer, t Table) error {
	type jsonRow map[string]string
	rows := make([]jsonRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		jr := make(jsonRow, len(t.Headers))
		for i, h := range t.Headers {
			if i < len(r) {
				jr[h] = r[i]
			}
		}
		rows = append(rows, jr)
	}
	enc := json.NewEncoder(w)
	return enc.Encode(struct {
		Title string    `json:"title"`
		Rows  []jsonRow `json:"rows"`
	}{Title: t.Title, Rows: rows})
}

func WriteXLSX(w io.Writer, t Table) error {
	f := excelize.NewFile()
	defer f.Close()
	const sheet = "Sheet1"
	for col, h := range t.Headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("export: XLSX header cell: %w", err)
		}
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return fmt.Errorf("export: writing XLSX header: %w", err)
		}
	}
	for rowIdx, row := range t.Rows {
		for col, v := range row {
			cell, err := excelize.CoordinatesToCellName(col+1, rowIdx+2)
			if err != nil {
				return fmt.Errorf("export: XLSX data cell: %w", err)
			}
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return fmt.Errorf("export: writing XLSX cell: %w", err)
			}
		}
	}
	if err := f.SetSheetName("Sheet1", "Sheet1"); err != nil {
		return fmt.Errorf("export: naming XLSX sheet: %w", err)
	}
	return f.Write(w)
}

// WriteTablePDF renders a simple grid table (title, header row, data
// rows) as a PDF — the generic report-export analogue of
// internal/modules/sales/printing's per-document-type renderer. It never
// computes any figure itself, only lays out strings the caller already
// formatted.
func WriteTablePDF(w io.Writer, t Table) error {
	pdf := fpdf.New("L", "mm", "A4", "") // landscape — most reports are wider than tall
	pdf.SetAutoPageBreak(true, 10)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 14)
	pdf.CellFormat(0, 10, t.Title, "", 1, "L", false, 0, "")
	pdf.Ln(2)

	if len(t.Headers) == 0 {
		return pdf.Output(w)
	}
	pageW, _ := pdf.GetPageSize()
	marginL, _, marginR, _ := pdf.GetMargins()
	colW := (pageW - marginL - marginR) / float64(len(t.Headers))

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(230, 230, 230)
	for _, h := range t.Headers {
		pdf.CellFormat(colW, 8, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 8)
	for _, row := range t.Rows {
		for i := range t.Headers {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			pdf.CellFormat(colW, 7, v, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}
	return pdf.Output(w)
}
