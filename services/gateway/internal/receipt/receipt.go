// Package receipt builds the proof of payment the school hands over (task 12):
// one shape for a counter deposit and a uniforms sale, printed as an 80 mm
// ticket by the browser and as a PDF by this package.
package receipt

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// Kind says what a receipt proves.
type Kind string

// The two things the school takes money for at a counter.
const (
	KindPayment Kind = "payment"
	KindSale    Kind = "sale"
)

// Line is one concept on the receipt.
type Line struct {
	Description   string `json:"description"`
	AmountCents   int64  `json:"amountCents"`
	Surcharge     bool   `json:"surcharge,omitempty"`
	DiscountCents int64  `json:"discountCents,omitempty"`
}

// Receipt is everything a printed receipt says, already in the office's words.
type Receipt struct {
	Kind        Kind                 `json:"kind"`
	Folio       string               `json:"folio"`
	School      client.SchoolProfile `json:"school"`
	PaidOn      string               `json:"paidOn"`
	Buyer       string               `json:"buyer"`
	BuyerDetail string               `json:"buyerDetail,omitempty"`
	Lines       []Line               `json:"lines"`
	// The parts of the total a family asks about: late fees, discounts, and
	// money no concept claimed (their credit).
	SurchargeCents int64 `json:"surchargeCents"`
	DiscountCents  int64 `json:"discountCents"`
	CreditCents    int64 `json:"creditCents"`
	TotalCents     int64 `json:"totalCents"`
	// Method is how it was paid, in words: the one method, or every method
	// joined with " + " when several were combined (T8).
	Method    string `json:"method"`
	Reference string `json:"reference,omitempty"`
	// Tenders is the breakdown by method (T8): a credit note completed with
	// cash prints both, each with its amount.
	Tenders    []TenderLine `json:"tenders"`
	ReceivedBy string       `json:"receivedBy,omitempty"`
	Voided     bool         `json:"voided,omitempty"`
	VoidReason string       `json:"voidReason,omitempty"`
	// T9: a receipt replaced by its correction, and the correction's own
	// receipt naming the one it replaces. Both print again.
	Superseded        bool   `json:"superseded,omitempty"`
	SupersededByFolio string `json:"supersededByFolio,omitempty"`
	CorrectsFolio     string `json:"correctsFolio,omitempty"`
}

// TenderLine is one method of the payment, as the receipt prints it.
type TenderLine struct {
	// Code is the method as the ledger names it (cash, card, transfer,
	// credit_note), for a reader that must tell them apart; Method is the
	// office's words for it.
	Code        string `json:"code"`
	Method      string `json:"method"`
	AmountCents int64  `json:"amountCents"`
	Reference   string `json:"reference,omitempty"`
}

// tenderLines puts the tenders in the office's words.
func tenderLines(ts []client.Tender) []TenderLine {
	out := make([]TenderLine, 0, len(ts))
	for _, t := range ts {
		out = append(out, TenderLine{Code: t.Method, Method: methodLabel(t.Method, t.CardType), AmountCents: t.AmountCents, Reference: t.Reference})
	}
	return out
}

// methodsLabel is every method of the payment, joined: "Nota de crédito +
// Efectivo".
func methodsLabel(lines []TenderLine) string {
	labels := make([]string, 0, len(lines))
	for _, l := range lines {
		labels = append(labels, l.Method)
	}
	return strings.Join(labels, " + ")
}

// The reads a receipt needs, each defined here, where it is consumed.
type (
	DepositReader interface {
		DepositReceipt(ctx context.Context, depositID string) (client.DepositReceipt, error)
	}
	OrderReader interface {
		GetOrder(ctx context.Context, id string, studentIDs []string) (client.UniformOrder, error)
	}
	StudentReader interface {
		GetStudent(ctx context.Context, id string) (client.StudentResult, error)
	}
	AccountReader interface {
		GetAccount(ctx context.Context, studentID string) (client.AccountStatement, error)
	}
	ProfileReader interface {
		GetSchoolProfile(ctx context.Context) (client.SchoolProfile, bool, error)
	}
)

// counterSource is the order source of a counter sale.
const counterSource = "counter"

var errNotACounterSale = errors.New("only a counter sale has a sale receipt")

// Builder assembles receipts from the services that own their parts.
type Builder struct {
	deposits DepositReader
	orders   OrderReader
	students StudentReader
	accounts AccountReader
	profiles ProfileReader
}

// NewBuilder wires a Builder.
func NewBuilder(deposits DepositReader, orders OrderReader, students StudentReader, accounts AccountReader, profiles ProfileReader) *Builder {
	return &Builder{deposits: deposits, orders: orders, students: students, accounts: accounts, profiles: profiles}
}

// Payment is the receipt of a counter deposit.
func (b *Builder) Payment(ctx context.Context, depositID string) (Receipt, error) {
	got, err := b.deposits.DepositReceipt(ctx, depositID)
	if err != nil {
		return Receipt{}, err
	}
	school, err := b.school(ctx)
	if err != nil {
		return Receipt{}, err
	}
	pupil, err := b.students.GetStudent(ctx, got.Deposit.StudentID)
	if err != nil {
		return Receipt{}, fmt.Errorf("reading the pupil of deposit %s: %w", depositID, err)
	}
	d := got.Deposit
	r := Receipt{
		Kind: KindPayment, Folio: d.Folio, School: school, PaidOn: dateOf(d.PaidAt),
		Buyer:       pupil.FullName(),
		BuyerDetail: pupil.GradeGroup,
		Lines:       make([]Line, 0, len(got.Lines)),
		CreditCents: got.UnallocatedCents, TotalCents: d.AmountCents,
		Reference: d.Reference, ReceivedBy: d.ReceivedByName,
		Voided: d.VoidedAt != "", VoidReason: d.VoidReason,
		Superseded: d.SupersededBy != "", SupersededByFolio: d.SupersededByFolio,
		CorrectsFolio: d.CorrectsFolio,
	}
	tenders := d.Tenders
	if len(tenders) == 0 {
		// A finance from before T8 sends the single method only.
		tenders = []client.Tender{{Method: d.Method, AmountCents: d.AmountCents, Reference: d.Reference, CardType: d.CardType}}
	}
	r.Tenders = tenderLines(tenders)
	r.Method = methodsLabel(r.Tenders)
	for _, l := range got.Lines {
		r.Lines = append(r.Lines, Line{Description: l.Description, AmountCents: l.AmountCents, Surcharge: l.Surcharge, DiscountCents: l.DiscountCents})
		// Every line's discount counts, so the lines and the totals agree.
		r.DiscountCents += l.DiscountCents
		if l.Surcharge {
			r.SurchargeCents += l.AmountCents
		}
	}
	return r, nil
}

// Sale is the receipt of a uniforms sale. How it was paid is finance's to
// know: the payment on the delivery's charge.
func (b *Builder) Sale(ctx context.Context, orderID string) (Receipt, error) {
	order, err := b.orders.GetOrder(ctx, orderID, nil)
	if err != nil {
		return Receipt{}, err
	}
	// A sale receipt proves a counter sale: goods handed over and paid at once.
	// A family order (unpaid, waiting, cancelled) is not one.
	if order.Source != counterSource {
		return Receipt{}, connect.NewError(connect.CodeNotFound, errNotACounterSale)
	}
	school, err := b.school(ctx)
	if err != nil {
		return Receipt{}, err
	}
	r := Receipt{
		Kind: KindSale, Folio: order.Folio, School: school, PaidOn: dateOf(order.CreatedAt),
		Buyer: buyerOf(order), BuyerDetail: order.StudentGradeGroup,
		Lines: make([]Line, 0, len(order.Lines)), TotalCents: order.TotalCents,
		Tenders: []TenderLine{},
	}
	for _, l := range order.Lines {
		r.Lines = append(r.Lines, Line{
			Description: fmt.Sprintf("%s · talla %s × %d", l.ItemName, l.SizeLabel, l.Qty),
			AmountCents: l.UnitPriceCents * int64(l.Qty),
		})
	}
	if order.StudentID != "" {
		tenders, err := b.paymentOf(ctx, order)
		if err != nil {
			return Receipt{}, err
		}
		r.Tenders = tenderLines(tenders)
		r.Method = methodsLabel(r.Tenders)
		if len(tenders) == 1 {
			r.Reference = tenders[0].Reference
		}
	}
	return r, nil
}

func (b *Builder) school(ctx context.Context) (client.SchoolProfile, error) {
	return schoolOf(ctx, b.profiles)
}

// schoolOf reads the school's details for a printed document. A school that
// has not filled them in still gets its document.
func schoolOf(ctx context.Context, profiles ProfileReader) (client.SchoolProfile, error) {
	p, found, err := profiles.GetSchoolProfile(ctx)
	if err != nil {
		return client.SchoolProfile{}, fmt.Errorf("reading the school's details: %w", err)
	}
	if !found {
		return client.SchoolProfile{}, nil
	}
	return p, nil
}

// paymentOf finds how a sale was paid: every payment on its deliveries'
// charges, one per method it combined (T8). Empty when finance never recorded
// it (a flagged delivery).
func (b *Builder) paymentOf(ctx context.Context, order client.UniformOrder) ([]client.Tender, error) {
	charges := map[string]bool{}
	for _, f := range order.Fulfillments {
		if f.FinanceChargeID != "" {
			charges[f.FinanceChargeID] = true
		}
	}
	if len(charges) == 0 {
		return nil, nil
	}
	account, err := b.accounts.GetAccount(ctx, order.StudentID)
	if err != nil {
		return nil, fmt.Errorf("reading how sale %s was paid: %w", order.Folio, err)
	}
	var out []client.Tender
	for _, p := range account.Payments {
		if charges[p.ChargeID] {
			out = append(out, client.Tender{Method: p.Method, AmountCents: p.AmountCents, Reference: p.Reference, CardType: p.CardType})
		}
	}
	return out, nil
}

func buyerOf(order client.UniformOrder) string {
	if strings.TrimSpace(order.StudentName) == "" {
		return "Venta de mostrador"
	}
	return order.StudentName
}

// dateOf keeps the calendar day of an RFC 3339 instant ("2026-10-10T…").
func dateOf(instant string) string {
	if len(instant) < len("2006-01-02") {
		return instant
	}
	return instant[:len("2006-01-02")]
}

// methodLabel says how the money came, as the office says it.
func methodLabel(method, cardType string) string {
	switch method {
	case "cash":
		return "Efectivo"
	case "transfer":
		return "Transferencia"
	case "credit_note":
		return "Nota de crédito"
	case "mixed":
		return "Combinado"
	case "card":
		switch cardType {
		case "debit":
			return "Tarjeta de débito"
		case "credit":
			return "Tarjeta de crédito"
		}
		return "Tarjeta"
	}
	return method
}
