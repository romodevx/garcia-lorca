package receipt

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/signintech/gopdf"
)

// Poppins, the web's display face (SIL Open Font License, fonts/OFL.txt). A
// TrueType font is embedded because the PDF must print "García" and "ñ" exactly,
// and gopdf has no built-in faces (ADR 0017).
var (
	//go:embed fonts/Poppins-Regular.ttf
	fontRegular []byte
	//go:embed fonts/Poppins-SemiBold.ttf
	fontBold []byte
)

const (
	familyRegular = "poppins"
	familyBold    = "poppins-bold"

	pageMargin   = 48.0
	pageWidth    = 612.0 // US Letter, in points
	contentWidth = pageWidth - 2*pageMargin
	amountWidth  = 110.0
	lineHeight   = 16.0
)

// PDF renders a receipt on a letter page.
func PDF(r Receipt) ([]byte, error) {
	w, err := newWriter()
	if err != nil {
		return nil, err
	}
	w.header(r)
	w.parties(r)
	w.lines(r)
	w.totals(r)
	w.foot(r)
	if w.err != nil {
		return nil, fmt.Errorf("rendering receipt %s: %w", r.Folio, w.err)
	}
	var out bytes.Buffer
	if _, err := w.pdf.WriteTo(&out); err != nil {
		return nil, fmt.Errorf("writing receipt %s: %w", r.Folio, err)
	}
	return out.Bytes(), nil
}

// writer keeps the first error, so the layout reads as a list of steps.
type writer struct {
	pdf *gopdf.GoPdf
	y   float64
	err error
}

func newWriter() (*writer, error) {
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeLetter})
	pdf.AddPage()
	if err := pdf.AddTTFFontData(familyRegular, fontRegular); err != nil {
		return nil, fmt.Errorf("loading the regular face: %w", err)
	}
	if err := pdf.AddTTFFontData(familyBold, fontBold); err != nil {
		return nil, fmt.Errorf("loading the bold face: %w", err)
	}
	return &writer{pdf: pdf, y: pageMargin}, nil
}

// text writes s at the left margin, wrapped to the page, and moves down.
func (w *writer) text(family string, size float64, s string) {
	w.y += w.wrapped(family, size, s, pageMargin, contentWidth) * (size + 6)
}

// wrapped writes s word-wrapped in a box of the given width, starting at the
// current height, and returns how many lines it took (0 for nothing). An
// address or a legend runs to hundreds of characters; one line would leave the
// page.
func (w *writer) wrapped(family string, size float64, s string, x, width float64) float64 {
	if w.err != nil || s == "" {
		return 0
	}
	if err := w.pdf.SetFont(family, "", size); err != nil {
		w.err = err
		return 0
	}
	lines, err := w.pdf.SplitTextWithWordWrap(s, width)
	if err != nil {
		w.err = err
		return 0
	}
	top := w.y
	for i, line := range lines {
		w.y = top + float64(i)*(size+6)
		w.textAt(family, size, line, x, width, gopdf.Left)
	}
	w.y = top
	return float64(len(lines))
}

// textAt writes s in a box of the given width, aligned, at the current height.
func (w *writer) textAt(family string, size float64, s string, x, width float64, align int) {
	if w.err != nil || s == "" {
		return
	}
	if err := w.pdf.SetFont(family, "", size); err != nil {
		w.err = err
		return
	}
	w.pdf.SetXY(x, w.y)
	w.err = w.pdf.CellWithOption(&gopdf.Rect{W: width, H: size + 2}, s, gopdf.CellOption{Align: align | gopdf.Top})
}

func (w *writer) rule() {
	w.y += 4
	w.pdf.SetLineWidth(0.6)
	w.pdf.Line(pageMargin, w.y, pageMargin+contentWidth, w.y)
	w.y += 10
}

func (w *writer) header(r Receipt) {
	w.text(familyBold, 16, r.School.Name)
	w.text(familyRegular, 9, r.School.LegalName)
	w.text(familyRegular, 9, joinNonEmpty(" · ", prefixed("RFC ", r.School.RFC), prefixed("CCT ", r.School.CCT)))
	w.text(familyRegular, 9, joinNonEmpty(" · ", r.School.Address, prefixed("Tel. ", r.School.Phone)))
	w.rule()

	title := "Comprobante de pago"
	if r.Kind == KindSale {
		title = "Comprobante de venta"
	}
	w.textAt(familyBold, 14, title, pageMargin, contentWidth, gopdf.Left)
	w.textAt(familyBold, 14, r.Folio, pageMargin, contentWidth, gopdf.Right)
	w.y += 22
	if r.Voided {
		w.text(familyBold, 13, "CANCELADO")
		w.text(familyRegular, 9, prefixed("Motivo: ", r.VoidReason))
	}
}

func (w *writer) parties(r Receipt) {
	w.text(familyRegular, 10, "Fecha de pago: "+r.PaidOn)
	w.text(familyRegular, 10, prefixed("Alumno: ", joinNonEmpty(" · ", r.Buyer, r.BuyerDetail)))
	w.text(familyRegular, 10, prefixed("Forma de pago: ", joinNonEmpty(" · ", r.Method, prefixed("ref. ", r.Reference))))
	w.text(familyRegular, 10, prefixed("Recibió: ", r.ReceivedBy))
	w.rule()
}

func (w *writer) lines(r Receipt) {
	w.textAt(familyBold, 10, "Concepto", pageMargin, contentWidth-amountWidth, gopdf.Left)
	w.textAt(familyBold, 10, "Importe", pageMargin+contentWidth-amountWidth, amountWidth, gopdf.Right)
	w.y += lineHeight
	for _, l := range r.Lines {
		description := l.Description
		if l.DiscountCents > 0 {
			description += " (descuento " + Money(l.DiscountCents) + ")"
		}
		w.textAt(familyRegular, 10, Money(l.AmountCents), pageMargin+contentWidth-amountWidth, amountWidth, gopdf.Right)
		// The description wraps inside its column, never into the amount's.
		taken := w.wrapped(familyRegular, 10, description, pageMargin, contentWidth-amountWidth-12)
		w.y += max(1, taken) * lineHeight
	}
	w.rule()
}

func (w *writer) totals(r Receipt) {
	row := func(family string, size float64, label string, cents int64) {
		w.textAt(family, size, label, pageMargin, contentWidth-amountWidth, gopdf.Right)
		w.textAt(family, size, Money(cents), pageMargin+contentWidth-amountWidth, amountWidth, gopdf.Right)
		w.y += size + 6
	}
	if r.SurchargeCents > 0 {
		row(familyRegular, 10, "Recargos incluidos", r.SurchargeCents)
	}
	if r.DiscountCents > 0 {
		row(familyRegular, 10, "Descuentos aplicados", r.DiscountCents)
	}
	if r.CreditCents > 0 {
		row(familyRegular, 10, "Saldo a favor", r.CreditCents)
	}
	row(familyBold, 13, "Total", r.TotalCents)
}

func (w *writer) foot(r Receipt) {
	w.y += 18
	w.text(familyRegular, 9, r.School.Legend)
}

// Money renders cents as the office writes pesos: $3,500.00.
func Money(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	pesos := fmt.Sprintf("%d", cents/100)
	var grouped strings.Builder
	for i, digit := range pesos {
		if i > 0 && (len(pesos)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	return fmt.Sprintf("%s$%s.%02d", sign, grouped.String(), cents%100)
}

func prefixed(prefix, value string) string {
	if value == "" {
		return ""
	}
	return prefix + value
}

func joinNonEmpty(sep string, parts ...string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}
