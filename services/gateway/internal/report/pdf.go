package report

import (
	"bytes"
	"fmt"

	"github.com/signintech/gopdf"

	"github.com/garcia-lorca/services/gateway/internal/pdffont"
)

// Letter page geometry, in points (the receipts' page).
const (
	pageWidth    = 612.0
	pageHeight   = 792.0
	pageMargin   = 48.0
	contentWidth = pageWidth - 2*pageMargin
	pageBottom   = pageHeight - pageMargin
	bodySize     = 9.0
	lineHeight   = bodySize + 6
	cellGutter   = 6.0
	tableGap     = 14.0
)

// PDF renders the document on letter pages. A table runs onto as many pages
// as it needs, and each page repeats its column heads.
func PDF(doc Document) ([]byte, error) {
	w, err := newPDFWriter()
	if err != nil {
		return nil, err
	}
	w.heading(doc)
	for _, t := range doc.Tables {
		w.table(t)
	}
	if w.err != nil {
		return nil, fmt.Errorf("rendering %q: %w", doc.Title, w.err)
	}
	var out bytes.Buffer
	if _, err := w.pdf.WriteTo(&out); err != nil {
		return nil, fmt.Errorf("writing %q: %w", doc.Title, err)
	}
	return out.Bytes(), nil
}

// pdfWriter keeps the first error, so the layout reads as a list of steps.
type pdfWriter struct {
	pdf *gopdf.GoPdf
	y   float64
	err error
}

func newPDFWriter() (*pdfWriter, error) {
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeLetter})
	pdf.AddPage()
	if err := pdffont.Register(pdf); err != nil {
		return nil, err
	}
	return &pdfWriter{pdf: pdf, y: pageMargin}, nil
}

func (w *pdfWriter) heading(doc Document) {
	for i, line := range doc.Header {
		size, family := 9.0, pdffont.Regular
		if i == 0 {
			size, family = 15, pdffont.Bold
		}
		w.line(family, size, line, pageMargin, contentWidth, gopdf.Left)
		w.y += size + 6
	}
	w.rule()
	w.line(pdffont.Bold, 14, doc.Title, pageMargin, contentWidth, gopdf.Left)
	w.y += 20
	if doc.Subtitle != "" {
		w.line(pdffont.Regular, 10, doc.Subtitle, pageMargin, contentWidth, gopdf.Left)
		w.y += 16
	}
	w.y += 4
}

// widths shares the content width among the columns by weight.
func widths(cols []Column) []float64 {
	total := 0.0
	for _, c := range cols {
		total += weight(c)
	}
	out := make([]float64, len(cols))
	for i, c := range cols {
		out[i] = contentWidth * weight(c) / total
	}
	return out
}

func weight(c Column) float64 {
	if c.Weight <= 0 {
		return 1
	}
	return c.Weight
}

func align(k ColumnKind) int {
	if k == Text {
		return gopdf.Left
	}
	return gopdf.Right
}

func (w *pdfWriter) table(t Table) {
	if w.err != nil || len(t.Columns) == 0 {
		return
	}
	ws := widths(t.Columns)
	if w.y+3*lineHeight > pageBottom {
		w.newPage()
	}
	w.line(pdffont.Bold, 11, t.Title, pageMargin, contentWidth, gopdf.Left)
	w.y += 18
	w.heads(t.Columns, ws)
	for _, row := range t.Rows {
		w.row(t.Columns, ws, row, pdffont.Regular)
	}
	if len(t.Total) > 0 {
		w.rule()
		w.row(t.Columns, ws, t.Total, pdffont.Bold)
	}
	w.y += tableGap
}

func (w *pdfWriter) heads(cols []Column, ws []float64) {
	x := pageMargin
	for i, c := range cols {
		w.line(pdffont.Bold, bodySize, c.Header, x, ws[i]-cellGutter, align(c.Kind))
		x += ws[i]
	}
	w.y += lineHeight
}

func (w *pdfWriter) newPage() {
	w.pdf.AddPage()
	w.y = pageMargin
}

// row writes one row, wrapping text cells, on a new page (heads repeated)
// when it would not fit on this one.
func (w *pdfWriter) row(cols []Column, ws []float64, cells []Cell, family string) {
	if w.err != nil {
		return
	}
	split := make([][]string, len(cols))
	tallest := 1
	for i, c := range cols {
		var cell Cell
		if i < len(cells) {
			cell = cells[i]
		}
		split[i] = w.wrap(family, display(c.Kind, cell), ws[i]-cellGutter)
		tallest = max(tallest, len(split[i]))
	}
	if w.y+float64(tallest)*lineHeight > pageBottom {
		w.newPage()
		w.heads(cols, ws)
	}
	top := w.y
	x := pageMargin
	for i, c := range cols {
		for j, piece := range split[i] {
			w.y = top + float64(j)*lineHeight
			w.line(family, bodySize, piece, x, ws[i]-cellGutter, align(c.Kind))
		}
		x += ws[i]
	}
	w.y = top + float64(tallest)*lineHeight
}

func (w *pdfWriter) wrap(family, s string, width float64) []string {
	if w.err != nil || s == "" {
		return nil
	}
	if err := w.pdf.SetFont(family, "", bodySize); err != nil {
		w.err = err
		return nil
	}
	lines, err := w.pdf.SplitTextWithWordWrap(s, width)
	if err != nil {
		w.err = err
		return nil
	}
	return lines
}

// line writes s in a box at the current height.
func (w *pdfWriter) line(family string, size float64, s string, x, width float64, a int) {
	if w.err != nil || s == "" {
		return
	}
	if err := w.pdf.SetFont(family, "", size); err != nil {
		w.err = err
		return
	}
	w.pdf.SetXY(x, w.y)
	w.err = w.pdf.CellWithOption(&gopdf.Rect{W: width, H: size + 2}, s, gopdf.CellOption{Align: a | gopdf.Top})
}

func (w *pdfWriter) rule() {
	w.y += 2
	w.pdf.SetLineWidth(0.6)
	w.pdf.Line(pageMargin, w.y, pageMargin+contentWidth, w.y)
	w.y += 8
}
