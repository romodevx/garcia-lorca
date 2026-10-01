package report_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"

	"github.com/garcia-lorca/services/gateway/internal/report"
)

func sampleDoc(rows int) report.Document {
	table := report.Table{
		Title: "Egresos",
		Columns: []report.Column{
			{Header: "Fecha", Kind: report.Text, Weight: 1},
			{Header: "Categoría", Kind: report.Text, Weight: 2},
			{Header: "Descripción", Kind: report.Text, Weight: 4},
			{Header: "Importe", Kind: report.Money, Weight: 1.5},
		},
	}
	for i := 0; i < rows; i++ {
		table.Rows = append(table.Rows, []report.Cell{
			report.TextCell("2026-10-01"), report.TextCell("Papelería"),
			report.TextCell("Hojas y tóner para dirección, con descripción larga que obliga a partir la línea en dos"),
			report.MoneyCell(125050),
		})
	}
	table.Total = []report.Cell{report.TextCell("Total"), {}, {}, report.MoneyCell(int64(rows) * 125050)}
	return report.Document{
		Title:    "Reporte de finanzas",
		Subtitle: "Octubre de 2026",
		Header:   []string{"COLEGIO GARCÍA LORCA", "RFC EGL150101AB1"},
		Tables: []report.Table{
			{Title: "Resumen", Columns: []report.Column{{Header: "Concepto", Kind: report.Text, Weight: 3}, {Header: "Monto", Kind: report.Money, Weight: 1}},
				Rows: [][]report.Cell{{report.TextCell("Ingresos"), report.MoneyCell(500000)}, {report.TextCell("Balance"), report.MoneyCell(-25000)}}},
			table,
		},
	}
}

func pdfText(t *testing.T, doc []byte) string {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("the PDF does not open: %v", err)
	}
	plain, err := r.GetPlainText()
	if err != nil {
		t.Fatalf("reading its text: %v", err)
	}
	var b bytes.Buffer
	if _, err := io.Copy(&b, plain); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestPDFSaysWhatTheDocumentSays(t *testing.T) {
	out, err := report.PDF(sampleDoc(3))
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("not a PDF")
	}
	text := pdfText(t, out)
	for _, want := range []string{"COLEGIO GARCÍA LORCA", "Reporte de finanzas", "Octubre de 2026", "Resumen", "Ingresos", "$5,000.00", "-$250.00", "Papelería", "$3,751.50", "Categoría"} {
		if !strings.Contains(text, want) {
			t.Errorf("the PDF does not say %q", want)
		}
	}
}

// A long table runs onto more pages and each repeats the column heads.
func TestPDFPaginates(t *testing.T) {
	out, err := report.PDF(sampleDoc(120))
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	r, err := pdf.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	if r.NumPage() < 3 {
		t.Fatalf("120 wrapped rows on %d pages", r.NumPage())
	}
	if n := strings.Count(pdfText(t, out), "Descripción"); n < r.NumPage()-1 {
		t.Fatalf("column heads printed %d times over %d pages", n, r.NumPage())
	}
}

func TestXLSXHoldsEveryTableAsNumbers(t *testing.T) {
	out, err := report.XLSX(sampleDoc(2))
	if err != nil {
		t.Fatalf("XLSX: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("the workbook does not open: %v", err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) != 2 || sheets[0] != "Resumen" || sheets[1] != "Egresos" {
		t.Fatalf("sheets = %v", sheets)
	}
	rows, err := f.GetRows("Egresos", excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, r := range rows {
		joined += strings.Join(r, "|") + "\n"
	}
	for _, want := range []string{"Reporte de finanzas", "Octubre de 2026", "Fecha|Categoría|Descripción|Importe", "1250.5", "2501"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the sheet does not hold %q:\n%s", want, joined)
		}
	}
	// Money is a number the school can add up, not text.
	cell := ""
	for i, r := range rows {
		if len(r) > 3 && r[3] == "1250.5" {
			cell, _ = excelize.CoordinatesToCellName(4, i+1)
			break
		}
	}
	typ, err := f.GetCellType("Egresos", cell)
	if err != nil || typ == excelize.CellTypeSharedString || typ == excelize.CellTypeInlineString {
		t.Fatalf("money cell %s is %v (%v), want a number", cell, typ, err)
	}
}

// A cell that looks like a formula stays text: an expense described as
// "=HYPERLINK(...)" must not become a live formula in the director's Excel.
func TestXLSXNeverWritesFormulas(t *testing.T) {
	doc := report.Document{Title: "x", Tables: []report.Table{{
		Title: "Datos", Columns: []report.Column{{Header: "Texto", Kind: report.Text}},
		Rows: [][]report.Cell{{report.TextCell(`=HYPERLINK("http://evil","x")`)}},
	}}}
	out, err := report.XLSX(doc)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := excelize.OpenReader(bytes.NewReader(out))
	defer func() { _ = f.Close() }()
	rows, _ := f.GetRows("Datos")
	for i := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if formula, _ := f.GetCellFormula("Datos", cell); formula != "" {
			t.Fatalf("cell %s became a formula: %q", cell, formula)
		}
	}
}
