package receipt

import (
	"bytes"
	"fmt"

	"github.com/signintech/gopdf"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// The statement's columns, in points: date, concept, charge, credit, balance.
const (
	pageHeight     = 792.0 // US Letter
	pageBottom     = pageHeight - pageMargin
	dateColumn     = 64.0
	ledgerAmount   = 78.0
	conceptColumn  = contentWidth - dateColumn - 3*ledgerAmount
	conceptGutter  = 8.0
	statementTitle = "Estado de cuenta"
)

// StatementPDF renders a student's account statement on letter pages (T7),
// with the same engine and faces as the receipts. The ledger runs onto as many
// pages as it needs; each one repeats the column heads.
func StatementPDF(doc StatementDoc) ([]byte, error) {
	w, err := newWriter()
	if err != nil {
		return nil, err
	}
	w.statementHeader(doc)
	w.ledgerHead()
	for _, m := range doc.Statement.Movements {
		w.ledgerRow(m)
	}
	w.rule()
	w.statementTotals(doc.Statement)
	if w.err != nil {
		return nil, fmt.Errorf("rendering the statement of %s: %w", doc.Pupil, w.err)
	}
	var out bytes.Buffer
	if _, err := w.pdf.WriteTo(&out); err != nil {
		return nil, fmt.Errorf("writing the statement of %s: %w", doc.Pupil, err)
	}
	return out.Bytes(), nil
}

func (w *writer) statementHeader(doc StatementDoc) {
	w.text(familyBold, 16, doc.School.Name)
	w.text(familyRegular, 9, doc.School.LegalName)
	w.text(familyRegular, 9, joinNonEmpty(" · ", prefixed("RFC ", doc.School.RFC), prefixed("CCT ", doc.School.CCT)))
	w.text(familyRegular, 9, joinNonEmpty(" · ", doc.School.Address, prefixed("Tel. ", doc.School.Phone)))
	w.rule()
	w.textAt(familyBold, 14, statementTitle, pageMargin, contentWidth, gopdf.Left)
	w.textAt(familyRegular, 10, prefixed("Al ", doc.Statement.AsOf), pageMargin, contentWidth, gopdf.Right)
	w.y += 22
	w.text(familyRegular, 10, prefixed("Alumno: ", joinNonEmpty(" · ", doc.Pupil, doc.PupilDetail)))
	w.rule()
}

// ledgerHead writes the column heads.
func (w *writer) ledgerHead() {
	x := pageMargin
	w.textAt(familyBold, 9, "Fecha", x, dateColumn, gopdf.Left)
	x += dateColumn
	w.textAt(familyBold, 9, "Concepto", x, conceptColumn, gopdf.Left)
	x += conceptColumn
	for _, head := range []string{"Cargo", "Abono", "Saldo"} {
		w.textAt(familyBold, 9, head, x, ledgerAmount, gopdf.Right)
		x += ledgerAmount
	}
	w.y += lineHeight
}

// ledgerRow writes one movement, moving to a new page first when it would not
// fit on this one.
func (w *writer) ledgerRow(m client.StatementMovement) {
	if w.err != nil {
		return
	}
	lines, err := w.measure(familyRegular, 9, movementText(m), conceptColumn-conceptGutter)
	if err != nil {
		w.err = err
		return
	}
	if w.y+lines*lineHeight > pageBottom {
		w.pdf.AddPage()
		w.y = pageMargin
		w.ledgerHead()
	}
	x := pageMargin
	w.textAt(familyRegular, 9, m.Date, x, dateColumn, gopdf.Left)
	x += dateColumn + conceptColumn
	amount := Money(m.AmountCents)
	if m.Voided || m.NotYetDue {
		amount = "(" + amount + ")"
	}
	if isDebit(m.Kind) {
		w.textAt(familyRegular, 9, amount, x, ledgerAmount, gopdf.Right)
	} else {
		w.textAt(familyRegular, 9, amount, x+ledgerAmount, ledgerAmount, gopdf.Right)
	}
	w.textAt(familyRegular, 9, Money(m.BalanceCents), x+2*ledgerAmount, ledgerAmount, gopdf.Right)
	w.wrapped(familyRegular, 9, movementText(m), pageMargin+dateColumn, conceptColumn-conceptGutter)
	w.y += max(1, lines) * lineHeight
}

// measure is how many lines s takes in a column of the given width.
func (w *writer) measure(family string, size float64, s string, width float64) (float64, error) {
	if err := w.pdf.SetFont(family, "", size); err != nil {
		return 0, err
	}
	lines, err := w.pdf.SplitTextWithWordWrap(s, width)
	if err != nil {
		return 0, err
	}
	return float64(len(lines)), nil
}

func (w *writer) statementTotals(s client.Statement) {
	row := func(family string, size float64, label string, cents int64) {
		if w.y+size+6 > pageBottom {
			w.pdf.AddPage()
			w.y = pageMargin
		}
		w.textAt(family, size, label, pageMargin, contentWidth-amountWidth, gopdf.Right)
		w.textAt(family, size, Money(cents), pageMargin+contentWidth-amountWidth, amountWidth, gopdf.Right)
		w.y += size + 6
	}
	t := s.Totals
	row(familyRegular, 10, "Total cargado", t.ChargedCents)
	if t.DiscountCents > 0 {
		row(familyRegular, 10, "Descuentos y becas", t.DiscountCents)
	}
	row(familyRegular, 10, "Total pagado", t.PaidCents)
	if t.BalanceCents < 0 {
		row(familyBold, 13, "Saldo a favor", -t.BalanceCents)
	} else {
		row(familyBold, 13, "Saldo pendiente", t.BalanceCents)
	}
	row(familyRegular, 10, joinNonEmpty(" ", "A pagar hoy", prefixed("al ", s.AsOf)), s.DueTodayCents)
	if t.NotYetDueCents > 0 {
		row(familyRegular, 10, "Aún no exigible (fuera del saldo)", t.NotYetDueCents)
	}
}

// movementText is the concept column: the description and what qualifies it.
func movementText(m client.StatementMovement) string {
	text := m.Description
	if m.Kind == "payment" || m.Kind == "credit_note" {
		text = joinNonEmpty(" · ", text, m.Folio, methodLabel(m.Method, m.CardType), prefixed("ref. ", m.Reference))
	}
	switch {
	case m.Voided:
		text += " — Cancelado" + prefixed(": ", m.VoidReason)
	case m.Superseded:
		text += " — Sustituido" + prefixed(" por ", m.SupersededByFolio)
	case m.CorrectsFolio != "":
		text += " — Corrige a " + m.CorrectsFolio
	case m.NotYetDue:
		text += " — Aún no exigible"
	case m.Accrued:
		text += " — acumulado a la fecha"
	}
	return text
}

// isDebit reports the kinds that raise the balance.
func isDebit(kind string) bool { return kind == "charge" || kind == "surcharge" }
