// Package receipt builds the proof of payment the school hands over (task 12):
// one shape for a counter deposit and a uniforms sale, printed as an 80 mm
// ticket by the browser and as a PDF by this package.
package receipt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	// The scholarship's part of the concept and its name (T18).
	ScholarshipCents int64  `json:"scholarshipCents,omitempty"`
	ScholarshipName  string `json:"scholarshipName,omitempty"`
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
	// ScholarshipCents is what scholarships took off the concepts paid (T18).
	ScholarshipCents int64 `json:"scholarshipCents"`
	CreditCents      int64 `json:"creditCents"`
	TotalCents       int64 `json:"totalCents"`
	// PendingCents is what the charges this payment paid still owed right
	// after it (T14-020): an abono says how much is left. Zero on a sale, on a
	// settled payment, and when the account could not be read.
	PendingCents int64 `json:"pendingCents,omitempty"`
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
	// GroupLabeler names the group a pupil was in on a school day (YYYY-MM-DD)
	// as every screen does ("1° PRIMARIA · A"); found is false for a pupil
	// with no enrollment.
	GroupLabeler interface {
		StudentGroupLabel(ctx context.Context, studentID, day string) (label string, found bool, err error)
	}
	// UserNamer names the staff member who made a counter sale.
	UserNamer interface {
		GetUser(ctx context.Context, id string) (client.User, error)
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
	// groups names the pupil's group in full; nil keeps the stored letter.
	groups GroupLabeler
	// users names who made a counter sale; nil leaves "Recibió" out.
	users UserNamer
	// zone is the school's time zone, whose calendar day a sale prints
	// (D8). UTC unless WithSchoolLocation says otherwise.
	zone *time.Location
}

// Option configures a Builder.
type Option func(*Builder)

// WithSchoolLocation sets the school's time zone: a sale's receipt prints the
// day its instant falls on there, not the UTC date.
func WithSchoolLocation(location *time.Location) Option {
	return func(b *Builder) {
		if location != nil {
			b.zone = location
		}
	}
}

// WithGroupLabeler names the pupil's group of the payment's day with its grade
// and level (ISSUE-017) instead of the bare letter the pupil's row stores.
func WithGroupLabeler(groups GroupLabeler) Option {
	return func(b *Builder) { b.groups = groups }
}

// WithUserNamer names who made a counter sale on its receipt (T14-019), the
// "Recibió" a payment's receipt already prints.
func WithUserNamer(users UserNamer) Option {
	return func(b *Builder) { b.users = users }
}

// NewBuilder wires a Builder.
func NewBuilder(deposits DepositReader, orders OrderReader, students StudentReader, accounts AccountReader, profiles ProfileReader, opts ...Option) *Builder {
	b := &Builder{deposits: deposits, orders: orders, students: students, accounts: accounts, profiles: profiles, zone: time.UTC}
	for _, opt := range opts {
		opt(b)
	}
	return b
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
		Kind: KindPayment, Folio: d.Folio, School: school, PaidOn: depositDay(d),
		Buyer:       pupil.FullName(),
		BuyerDetail: b.groupOf(ctx, got.Deposit.StudentID, depositDay(got.Deposit), pupil.GradeGroup),
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
		r.Lines = append(r.Lines, Line{Description: l.Description, AmountCents: l.AmountCents, Surcharge: l.Surcharge,
			DiscountCents: l.DiscountCents, ScholarshipCents: l.ScholarshipCents, ScholarshipName: l.ScholarshipName})
		// Every line's discount counts, so the lines and the totals agree.
		r.DiscountCents += l.DiscountCents
		r.ScholarshipCents += l.ScholarshipCents
		if l.Surcharge {
			r.SurchargeCents += l.AmountCents
		}
	}
	if !r.Voided {
		r.PendingCents = b.pendingAfter(ctx, got)
	}
	return r, nil
}

// pendingAfter is what the charges a deposit paid still owed right after it:
// each one's balance today plus whatever later deposits paid on it, so a
// receipt printed again next month still says what it said the day it was
// handed over. A cancelled charge owes nothing. A courtesy, never a gate: an
// account that cannot be read prints the receipt without it (logged, ids only).
func (b *Builder) pendingAfter(ctx context.Context, got client.DepositReceipt) int64 {
	paid := make(map[string]bool, len(got.Lines))
	for _, l := range got.Lines {
		if l.ChargeID != "" {
			paid[l.ChargeID] = true
		}
	}
	if len(paid) == 0 {
		return 0
	}
	account, err := b.accounts.GetAccount(ctx, got.Deposit.StudentID)
	if err != nil {
		slog.WarnContext(ctx, "account read failed; the receipt prints without its pending balance",
			"deposit_id", got.Deposit.ID, "error", err)
		return 0
	}
	at, _ := time.Parse(time.RFC3339, got.Deposit.PaidAt)
	later := make(map[string]int64, len(paid))
	for _, p := range account.Payments {
		if paid[p.ChargeID] && p.DepositID != got.Deposit.ID && recordedAfter(p.PaidAt, at) {
			later[p.ChargeID] += p.AmountCents
		}
	}
	var pending int64
	for _, c := range account.Charges {
		if !paid[c.ID] || c.Status == chargeCancelled {
			continue
		}
		pending += max(0, c.RemainingCents+later[c.ID])
	}
	return pending
}

// chargeCancelled is the status of a charge nobody owes any more.
const chargeCancelled = "cancelled"

// recordedAfter says whether an RFC 3339 instant falls after `at`. One that
// does not parse is not counted: the receipt then shows today's balance.
func recordedAfter(instant string, at time.Time) bool {
	t, err := time.Parse(time.RFC3339, instant)
	return err == nil && !at.IsZero() && t.After(at)
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
		Kind: KindSale, Folio: order.Folio, School: school, PaidOn: b.schoolDayOf(order.CreatedAt),
		Buyer: buyerOf(order), BuyerDetail: b.groupOf(ctx, order.StudentID, b.schoolDayOf(order.CreatedAt), order.StudentGradeGroup),
		Lines: make([]Line, 0, len(order.Lines)), TotalCents: order.TotalCents,
		Tenders:    []TenderLine{},
		ReceivedBy: b.sellerOf(ctx, order.CreatedByUserID),
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

// groupOf is the pupil's group on the day the money came, as the receipt
// prints it. The full name is a courtesy, never a gate: no labeler, no
// enrollment or a catalog that cannot
// answer all keep the stored name, and the receipt still prints — logged (ids
// only) so a broken catalog does not hide behind bare letters.
func (b *Builder) groupOf(ctx context.Context, studentID, day, stored string) string {
	if b.groups == nil || studentID == "" {
		return stored
	}
	label, found, err := b.groups.StudentGroupLabel(ctx, studentID, day)
	if err != nil {
		slog.WarnContext(ctx, "group lookup failed; the receipt prints the stored group", "student_id", studentID, "error", err)
		return stored
	}
	if !found || label == "" {
		return stored
	}
	return label
}

// sellerOf is who made a counter sale, as the receipt prints it: the display
// name, else the email. Empty when unknown — a directory that cannot answer
// never stops the receipt (logged, ids only).
func (b *Builder) sellerOf(ctx context.Context, userID string) string {
	if b.users == nil || userID == "" {
		return ""
	}
	u, err := b.users.GetUser(ctx, userID)
	if err != nil {
		slog.WarnContext(ctx, "seller lookup failed; the sale receipt prints without Recibió", "user_id", userID, "error", err)
		return ""
	}
	if name := strings.TrimSpace(u.DisplayName); name != "" {
		return name
	}
	return strings.TrimSpace(u.Email)
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

// schoolDayOf is the school's calendar day of an RFC 3339 instant: a sale at
// 19:00 in Morelia on the 15th is 01:00 UTC on the 16th and prints the 15th.
// An instant that does not parse keeps its leading date, as before.
func (b *Builder) schoolDayOf(instant string) string {
	at, err := time.Parse(time.RFC3339, instant)
	if err != nil {
		return dateOf(instant)
	}
	return at.In(b.zone).Format(time.DateOnly)
}

// depositDay is the day a deposit's receipt prints: its school-local day
// (paid_on, ADR 0035). A finance that predates it sends none; its paid_at was
// then the day as the counter stored it.
func depositDay(d client.Deposit) string {
	if d.PaidOn != "" {
		return d.PaidOn
	}
	return dateOf(d.PaidAt)
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
