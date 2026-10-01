package client

import (
	"context"
	"net/http"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/finance/v1/financev1connect"
)

// FinanceClient is the gateway's typed client for the finance service over the
// generated Connect client. Handlers depend on its small methods and the plain
// structs below, never on the generated proto types.
//
// MONEY IS ALWAYS INTEGER CENTS (int64) — at the edge exactly as in the
// contract. No float ever touches an amount.
type FinanceClient struct {
	rpc financev1connect.FinanceServiceClient
}

// ChargeConcept is the edge's view of a school-defined billable concept
// (colegiatura, inscripción, uniformes, …). Recurrence is "once" or "monthly".
type ChargeConcept struct {
	ID                 string
	TenantID           string
	Name               string
	DefaultAmountCents int64
	Recurrence         string
	Active             bool
}

// Charge is the edge's view of one debt owed by a student for a concept in a
// cycle. PaidCents and RemainingCents are DERIVED server-side and never
// client-supplied. Status is pending | partially_paid | paid | cancelled;
// DueDate is an ISO date, "YYYY-MM-DD".
type Charge struct {
	ID            string
	TenantID      string
	StudentID     string
	CycleID       string
	ConceptID     string
	Description   string
	AmountCents   int64
	DiscountCents int64
	DueDate       string
	// StartsOn is the first day the charge opens to the family (YYYY-MM-DD,
	// T5); empty when it is open from issue.
	StartsOn       string
	Status         string
	PaidCents      int64
	RemainingCents int64
}

// Payment is the edge's view of one amount applied to a charge. Method is
// cash | card | transfer; Reference is the folio/voucher/transfer id. PaidAt is
// rendered RFC3339 so the edge never leaks a proto timestamp.
type Payment struct {
	ID          string
	TenantID    string
	ChargeID    string
	AmountCents int64
	Method      string
	Reference   string
	PaidAt      string
	// CardType is debit or credit on a card payment (T8), empty otherwise.
	CardType string
	// DepositID is the deposit this payment is a slice of (T9), empty for a
	// payment made straight against its charge.
	DepositID string
}

// AccountStatement is a student's full financial picture: every charge, every
// payment, and the outstanding balance across non-cancelled charges.
type AccountStatement struct {
	Charges           []Charge
	Payments          []Payment
	TotalBalanceCents int64
	// UnallocatedCents is money the family has handed over that no charge has
	// claimed — their credit with the school.
	UnallocatedCents int64
	// DueTodayCents is what is being asked for NOW, already carrying the accrued
	// surcharge and net of the early-payment reward. TotalBalanceCents above is
	// what the YEAR still owes; before this field the parents' app showed that
	// one and the counter showed this one.
	DueTodayCents int64
	// The parts of it, so the statement can explain its own number.
	SurchargeCents     int64
	EarlyDiscountCents int64
	// AsOf is the day those three were worked out for, YYYY-MM-DD.
	AsOf string
}

// NewFinanceClient builds a FinanceClient targeting the finance base URL over
// httpClient.
func NewFinanceClient(baseURL string, httpClient connect.HTTPClient) *FinanceClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &FinanceClient{rpc: financev1connect.NewFinanceServiceClient(httpClient, baseURL)}
}

// CreateConcept registers a charge concept in the tenant's catalog.
func (c *FinanceClient) CreateConcept(ctx context.Context, name string, defaultAmountCents int64, recurrence string) (ChargeConcept, error) {
	res, err := c.rpc.CreateConcept(ctx, connect.NewRequest(&financev1.CreateConceptRequest{
		Name:               name,
		DefaultAmountCents: defaultAmountCents,
		Recurrence:         recurrence,
	}))
	if err != nil {
		return ChargeConcept{}, err
	}
	return toChargeConcept(res.Msg.GetConcept()), nil
}

// ListConcepts lists the tenant's charge concepts; inactive ones are omitted
// unless includeInactive is set.
func (c *FinanceClient) ListConcepts(ctx context.Context, includeInactive bool) ([]ChargeConcept, error) {
	res, err := c.rpc.ListConcepts(ctx, connect.NewRequest(&financev1.ListConceptsRequest{
		IncludeInactive: includeInactive,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]ChargeConcept, 0, len(res.Msg.GetConcepts()))
	for _, cpt := range res.Msg.GetConcepts() {
		out = append(out, toChargeConcept(cpt))
	}
	return out, nil
}

// UpdateConcept replaces a concept's mutable fields (including active —
// deactivating hides it from new charges). Charges already issued are untouched.
func (c *FinanceClient) UpdateConcept(ctx context.Context, id, name string, defaultAmountCents int64, recurrence string, active bool) (ChargeConcept, error) {
	res, err := c.rpc.UpdateConcept(ctx, connect.NewRequest(&financev1.UpdateConceptRequest{
		Id:                 id,
		Name:               name,
		DefaultAmountCents: defaultAmountCents,
		Recurrence:         recurrence,
		Active:             active,
	}))
	if err != nil {
		return ChargeConcept{}, err
	}
	return toChargeConcept(res.Msg.GetConcept()), nil
}

// CreateCharge issues a single charge against a student's enrollment in a
// cycle. An override of 0 (amount) or "" (description) falls back to the
// concept's default server-side.
func (c *FinanceClient) CreateCharge(ctx context.Context, studentID, cycleID, conceptID string, amountCentsOverride int64, descriptionOverride, dueDate string) (Charge, error) {
	res, err := c.rpc.CreateCharge(ctx, connect.NewRequest(&financev1.CreateChargeRequest{
		StudentId:           studentID,
		CycleId:             cycleID,
		ConceptId:           conceptID,
		AmountCentsOverride: amountCentsOverride,
		DescriptionOverride: descriptionOverride,
		DueDate:             dueDate,
	}))
	if err != nil {
		return Charge{}, err
	}
	return toCharge(res.Msg.GetCharge()), nil
}

// GenerateCharges issues the concept's charge to every active enrollment in the
// cycle, optionally narrowed to one group. It is idempotent: already-charged
// enrollments come back in skipped, so a retry is safe.
func (c *FinanceClient) GenerateCharges(ctx context.Context, conceptID, cycleID, groupID string, amountCentsOverride int64, dueDate string) (int32, int32, error) {
	res, err := c.rpc.GenerateCharges(ctx, connect.NewRequest(&financev1.GenerateChargesRequest{
		ConceptId:           conceptID,
		CycleId:             cycleID,
		GroupId:             groupID,
		AmountCentsOverride: amountCentsOverride,
		DueDate:             dueDate,
	}))
	if err != nil {
		return 0, 0, err
	}
	return res.Msg.GetCreatedCount(), res.Msg.GetSkippedCount(), nil
}

// ListCharges lists charges filtered by any of studentID, cycleID ("" = no
// filter on that dimension).
func (c *FinanceClient) ListCharges(ctx context.Context, studentID, cycleID string) ([]Charge, error) {
	res, err := c.rpc.ListCharges(ctx, connect.NewRequest(&financev1.ListChargesRequest{
		StudentId: studentID,
		CycleId:   cycleID,
	}))
	if err != nil {
		return nil, err
	}
	return toCharges(res.Msg.GetCharges()), nil
}

// ApplyDiscount sets the discount on a charge (beca, convenio, ajuste); the
// remaining balance and status are recomputed server-side.
func (c *FinanceClient) ApplyDiscount(ctx context.Context, chargeID string, discountCents int64, reason string) (Charge, error) {
	res, err := c.rpc.ApplyDiscount(ctx, connect.NewRequest(&financev1.ApplyDiscountRequest{
		ChargeId:      chargeID,
		DiscountCents: discountCents,
		Reason:        reason,
	}))
	if err != nil {
		return Charge{}, err
	}
	return toCharge(res.Msg.GetCharge()), nil
}

// CancelCharge voids a charge: it stops counting toward the account balance and
// can no longer receive payments.
func (c *FinanceClient) CancelCharge(ctx context.Context, chargeID string) (Charge, error) {
	res, err := c.rpc.CancelCharge(ctx, connect.NewRequest(&financev1.CancelChargeRequest{
		ChargeId: chargeID,
	}))
	if err != nil {
		return Charge{}, err
	}
	return toCharge(res.Msg.GetCharge()), nil
}

// RegisterPayment applies a payment to a charge and returns the payment plus
// the charge with its recomputed paid/remaining/status. idempotencyKey is
// optional: a repeat under the same key returns the original payment instead of
// collecting again, which is what makes a retried request safe.
func (c *FinanceClient) RegisterPayment(ctx context.Context, chargeID string, amountCents int64, method, reference, idempotencyKey string) (Payment, Charge, error) {
	return c.RegisterPaymentAt(ctx, chargeID, amountCents, method, reference, idempotencyKey, time.Time{}, Receiver{})
}

// Tender is one method a payment came in by (T8). The same shape travels in
// requests and answers: a credit note completed with cash is two of them.
type Tender struct {
	Method      string `json:"method"`
	AmountCents int64  `json:"amountCents"`
	Reference   string `json:"reference,omitempty"`
	CardType    string `json:"cardType,omitempty"`
}

// TenderedPayment is a payment against one charge, possibly combining methods.
type TenderedPayment struct {
	ChargeID    string
	AmountCents int64
	// Tenders must add up to AmountCents; finance refuses them otherwise.
	Tenders        []Tender
	IdempotencyKey string
	// PaidAt zero is omitted, so the service stamps its own clock.
	PaidAt   time.Time
	Receiver Receiver
}

// RegisterPaymentAt carries the date the money changed hands, in one method.
func (c *FinanceClient) RegisterPaymentAt(ctx context.Context, chargeID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time, receiver Receiver) (Payment, Charge, error) {
	payments, charge, err := c.RegisterTenderedPayment(ctx, TenderedPayment{
		ChargeID: chargeID, AmountCents: amountCents, IdempotencyKey: idempotencyKey, PaidAt: paidAt, Receiver: receiver,
		Tenders: []Tender{{Method: method, AmountCents: amountCents, Reference: reference}},
	})
	if err != nil {
		return Payment{}, Charge{}, err
	}
	return payments[0], charge, nil
}

// RegisterTenderedPayment posts one payment row per tender against a charge,
// atomically (T8), and returns every row with the recomputed charge.
func (c *FinanceClient) RegisterTenderedPayment(ctx context.Context, in TenderedPayment) ([]Payment, Charge, error) {
	req := &financev1.RegisterPaymentRequest{
		ChargeId:       in.ChargeID,
		AmountCents:    in.AmountCents,
		IdempotencyKey: in.IdempotencyKey,
		ReceivedBy:     in.Receiver.UserID,
		Tenders:        toProtoTenders(in.Tenders),
	}
	if len(in.Tenders) == 1 {
		// Understood by a finance from before T8 too.
		req.Method, req.Reference = in.Tenders[0].Method, in.Tenders[0].Reference
	}
	if !in.PaidAt.IsZero() {
		req.PaidAt = timestamppb.New(in.PaidAt)
	}
	res, err := c.rpc.RegisterPayment(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, Charge{}, err
	}
	rows := res.Msg.GetPayments()
	if len(rows) == 0 {
		rows = []*financev1.Payment{res.Msg.GetPayment()}
	}
	payments := make([]Payment, 0, len(rows))
	for _, p := range rows {
		payments = append(payments, toPayment(p))
	}
	return payments, toCharge(res.Msg.GetCharge()), nil
}

func toProtoTenders(ts []Tender) []*financev1.Tender {
	out := make([]*financev1.Tender, 0, len(ts))
	for _, t := range ts {
		out = append(out, &financev1.Tender{
			Method: t.Method, AmountCents: t.AmountCents, Reference: t.Reference, CardType: t.CardType,
		})
	}
	return out
}

func fromProtoTenders(ts []*financev1.Tender) []Tender {
	out := make([]Tender, 0, len(ts))
	for _, t := range ts {
		out = append(out, Tender{
			Method: t.GetMethod(), AmountCents: t.GetAmountCents(), Reference: t.GetReference(), CardType: t.GetCardType(),
		})
	}
	return out
}

// GetAccount returns a student's account statement: every charge, every
// payment, and the total outstanding balance.
func (c *FinanceClient) GetAccount(ctx context.Context, studentID string) (AccountStatement, error) {
	return c.account(ctx, accountRequest(studentID, false))
}

// GetFamilyAccount is the statement a family is shown (T5): finance leaves out
// the charges that have not opened yet and works the totals out over the rest.
// Filtering here instead would leave the balance counting what it hid.
func (c *FinanceClient) GetFamilyAccount(ctx context.Context, studentID string) (AccountStatement, error) {
	return c.account(ctx, accountRequest(studentID, true))
}

// accountRequest is the statement read for one pupil, as the school or as the
// family sees it.
func accountRequest(studentID string, familyView bool) *financev1.GetAccountRequest {
	return &financev1.GetAccountRequest{StudentId: studentID, FamilyView: familyView}
}

func (c *FinanceClient) account(ctx context.Context, req *financev1.GetAccountRequest) (AccountStatement, error) {
	res, err := c.rpc.GetAccount(ctx, connect.NewRequest(req))
	if err != nil {
		return AccountStatement{}, err
	}
	payments := make([]Payment, 0, len(res.Msg.GetPayments()))
	for _, p := range res.Msg.GetPayments() {
		payments = append(payments, toPayment(p))
	}
	return AccountStatement{
		Charges:            toCharges(res.Msg.GetCharges()),
		Payments:           payments,
		TotalBalanceCents:  res.Msg.GetTotalBalanceCents(),
		DueTodayCents:      res.Msg.GetDueTodayCents(),
		SurchargeCents:     res.Msg.GetSurchargeCents(),
		EarlyDiscountCents: res.Msg.GetEarlyDiscountCents(),
		AsOf:               res.Msg.GetAsOf(),
		UnallocatedCents:   res.Msg.GetUnallocatedCents(),
	}, nil
}

// ListOverdue lists charges past their due date with a remaining balance,
// optionally filtered by cycle ("" = any).
func (c *FinanceClient) ListOverdue(ctx context.Context, cycleID string) ([]Charge, error) {
	res, err := c.rpc.ListOverdue(ctx, connect.NewRequest(&financev1.ListOverdueRequest{
		CycleId: cycleID,
	}))
	if err != nil {
		return nil, err
	}
	return toCharges(res.Msg.GetCharges()), nil
}

// --- deposits (M16) --------------------------------------------------------

// Deposit is money as it arrived, before it was decided what it pays.
// Receiver is who took the money: the authenticated caller and their name
// (task 7).
type Receiver struct {
	UserID string
	Name   string
}

type Deposit struct {
	ID              string `json:"id"`
	StudentID       string `json:"studentId"`
	AmountCents     int64  `json:"amountCents"`
	Method          string `json:"method"`
	Reference       string `json:"reference"`
	PaidAt          string `json:"paidAt"`
	CalendarVersion string `json:"calendarVersion"`
	// CardType is debit or credit on a card payment, empty otherwise.
	CardType   string `json:"cardType,omitempty"`
	VoidedAt   string `json:"voidedAt,omitempty"`
	VoidReason string `json:"voidReason,omitempty"`
	// Who took the money and when it was keyed (task 7). Empty on deposits
	// from before it was recorded.
	ReceivedByName string `json:"receivedByName,omitempty"`
	CapturedAt     string `json:"capturedAt,omitempty"`
	// Folio is the receipt number, P-<year>-<six digits> (task 12).
	Folio string `json:"folio,omitempty"`
	// Tenders are the methods the money came in by (T8); Method is their
	// summary ("mixed" when several).
	Tenders []Tender `json:"tenders"`
	// T9: a deposit replaced by its correction, and the deposit a correction
	// replaces, each with the other's folio for the receipt.
	SupersededAt      string `json:"supersededAt,omitempty"`
	SupersededBy      string `json:"supersededBy,omitempty"`
	SupersededByFolio string `json:"supersededByFolio,omitempty"`
	CorrectsDepositID string `json:"correctsDepositId,omitempty"`
	CorrectsFolio     string `json:"correctsFolio,omitempty"`
}

// ReceiptLine is one concept a deposit paid (task 12).
type ReceiptLine struct {
	ChargeID      string
	Description   string
	AmountCents   int64
	Surcharge     bool
	DiscountCents int64
}

// DepositReceipt is a deposit and what it paid, concept by concept.
type DepositReceipt struct {
	Deposit          Deposit
	Lines            []ReceiptLine
	UnallocatedCents int64
}

// DepositReceipt reads a deposit back for its receipt.
func (c *FinanceClient) DepositReceipt(ctx context.Context, depositID string) (DepositReceipt, error) {
	res, err := c.rpc.GetDepositReceipt(ctx, connect.NewRequest(&financev1.GetDepositReceiptRequest{DepositId: depositID}))
	if err != nil {
		return DepositReceipt{}, err
	}
	lines := make([]ReceiptLine, 0, len(res.Msg.GetLines()))
	for _, l := range res.Msg.GetLines() {
		lines = append(lines, ReceiptLine{
			ChargeID: l.GetChargeId(), Description: l.GetDescription(), AmountCents: l.GetAmountCents(),
			Surcharge: l.GetSurcharge(), DiscountCents: l.GetDiscountCents(),
		})
	}
	return DepositReceipt{Deposit: toDeposit(res.Msg.GetDeposit()), Lines: lines, UnallocatedCents: res.Msg.GetUnallocatedCents()}, nil
}

// DepositResult is what the money did, in the shape a receipt needs.
type DepositResult struct {
	Deposit          Deposit   `json:"deposit"`
	Allocations      []Payment `json:"allocations"`
	Surcharges       []Charge  `json:"surcharges"`
	UnallocatedCents int64     `json:"unallocatedCents"`
	Replayed         bool      `json:"replayed"`
}

// RegisterDeposit takes money without being told what it pays for and splits it
// across what the family owes, oldest first.
func (c *FinanceClient) RegisterDeposit(ctx context.Context, studentID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time, grantEarlyPayment bool, cardType string, receiver Receiver, chargeIDs []string, tenders []Tender) (DepositResult, error) {
	req := &financev1.RegisterDepositRequest{
		StudentId:         studentID,
		AmountCents:       amountCents,
		Method:            method,
		Reference:         reference,
		IdempotencyKey:    idempotencyKey,
		GrantEarlyPayment: grantEarlyPayment,
		CardType:          cardType,
		ReceivedBy:        receiver.UserID,
		ReceivedByName:    receiver.Name,
		ChargeIds:         chargeIDs,
		Tenders:           toProtoTenders(tenders),
	}
	if !paidAt.IsZero() {
		req.PaidAt = timestamppb.New(paidAt)
	}
	res, err := c.rpc.RegisterDeposit(ctx, connect.NewRequest(req))
	if err != nil {
		return DepositResult{}, err
	}
	allocations := make([]Payment, 0, len(res.Msg.GetAllocations()))
	for _, p := range res.Msg.GetAllocations() {
		allocations = append(allocations, toPayment(p))
	}
	return DepositResult{
		Deposit:          toDeposit(res.Msg.GetDeposit()),
		Allocations:      allocations,
		Surcharges:       toCharges(res.Msg.GetSurcharges()),
		UnallocatedCents: res.Msg.GetUnallocatedCents(),
		Replayed:         res.Msg.GetReplayed(),
	}, nil
}

// PreviewDeposit says what an amount would cover, without taking it.
func (c *FinanceClient) PreviewDeposit(ctx context.Context, studentID string, amountCents int64, paidAt time.Time, chargeIDs []string) (DepositResult, error) {
	req := &financev1.PreviewDepositRequest{StudentId: studentID, AmountCents: amountCents, ChargeIds: chargeIDs}
	if !paidAt.IsZero() {
		req.PaidAt = timestamppb.New(paidAt)
	}
	res, err := c.rpc.PreviewDeposit(ctx, connect.NewRequest(req))
	if err != nil {
		return DepositResult{}, err
	}
	allocations := make([]Payment, 0, len(res.Msg.GetAllocations()))
	for _, p := range res.Msg.GetAllocations() {
		allocations = append(allocations, toPayment(p))
	}
	return DepositResult{
		Allocations:      allocations,
		Surcharges:       toCharges(res.Msg.GetSurcharges()),
		UnallocatedCents: res.Msg.GetUnallocatedCents(),
	}, nil
}

// Expected is what a family must hand over today to be current.
type Expected struct {
	AmountCents        int64 `json:"amountCents"`
	SurchargeCents     int64 `json:"surchargeCents"`
	DiscountCents      int64 `json:"discountCents"`
	AnnualBalanceCents int64 `json:"annualBalanceCents"`
	// Lines is the debt concept by concept, oldest due date first (task 8).
	Lines []ExpectedLine `json:"lines"`
}

// ExpectedLine is one charge still owed, as of the day asked about.
type ExpectedLine struct {
	ChargeID       string `json:"chargeId"`
	Description    string `json:"description"`
	DueDate        string `json:"dueDate"`
	OwedCents      int64  `json:"owedCents"`
	SurchargeCents int64  `json:"surchargeCents"`
	DiscountCents  int64  `json:"discountCents"`
	TotalCents     int64  `json:"totalCents"`
	Due            bool   `json:"due"`
}

// ExpectedToday asks what is due as of a date ("" = today).
func (c *FinanceClient) ExpectedToday(ctx context.Context, studentID, on string) (Expected, error) {
	res, err := c.rpc.ExpectedToday(ctx, connect.NewRequest(&financev1.ExpectedTodayRequest{
		StudentId: studentID, On: on,
	}))
	if err != nil {
		return Expected{}, err
	}
	return Expected{
		AmountCents:        res.Msg.GetAmountCents(),
		SurchargeCents:     res.Msg.GetSurchargeCents(),
		DiscountCents:      res.Msg.GetDiscountCents(),
		AnnualBalanceCents: res.Msg.GetAnnualBalanceCents(),
		Lines:              toExpectedLines(res.Msg.GetLines()),
	}, nil
}

func toExpectedLines(lines []*financev1.ExpectedLine) []ExpectedLine {
	out := make([]ExpectedLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, ExpectedLine{
			ChargeID: l.GetChargeId(), Description: l.GetDescription(), DueDate: l.GetDueDate(),
			OwedCents: l.GetOwedCents(), SurchargeCents: l.GetSurchargeCents(), DiscountCents: l.GetDiscountCents(),
			TotalCents: l.GetTotalCents(), Due: l.GetDue(),
		})
	}
	return out
}

// ListDeposits returns a student's deposits, newest first.
func (c *FinanceClient) ListDeposits(ctx context.Context, studentID string) ([]Deposit, error) {
	res, err := c.rpc.ListDeposits(ctx, connect.NewRequest(&financev1.ListDepositsRequest{
		StudentId: studentID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]Deposit, 0, len(res.Msg.GetDeposits()))
	for _, d := range res.Msg.GetDeposits() {
		out = append(out, toDeposit(d))
	}
	return out, nil
}

// CashUp is a day's takings: one number per method, the total, and the list
// behind them.
type CashUp struct {
	Deposits  []Deposit `json:"deposits"`
	Cancelled []Deposit `json:"cancelled"`
	// Superseded are the day's deposits replaced by a correction (T9): they
	// count for nothing and are listed.
	Superseded    []Deposit `json:"superseded"`
	CashCents     int64     `json:"cashCents"`
	CardCents     int64     `json:"cardCents"`
	TransferCents int64     `json:"transferCents"`
	TotalCents    int64     `json:"totalCents"`
	// CreditNoteCents is what credit notes settled that day (T8): not money,
	// so in neither the per-method figures nor TotalCents.
	CreditNoteCents int64 `json:"creditNoteCents"`
	// Payments taken outside a deposit (a counter sale's). The totals count
	// them: the money is in the drawer.
	Payments []CashUpPayment `json:"payments"`
}

// CashUpPayment is a payment taken outside a deposit, with what it paid for.
type CashUpPayment struct {
	ID          string `json:"id"`
	ChargeID    string `json:"chargeId"`
	AmountCents int64  `json:"amountCents"`
	Method      string `json:"method"`
	Reference   string `json:"reference"`
	PaidAt      string `json:"paidAt"`
	Description string `json:"description"`
	CardType    string `json:"cardType,omitempty"`
}

// DailyCashUp returns what came in on one day, and in what form.
func (c *FinanceClient) DailyCashUp(ctx context.Context, day string) (CashUp, error) {
	res, err := c.rpc.DailyCashUp(ctx, connect.NewRequest(&financev1.DailyCashUpRequest{Day: day}))
	if err != nil {
		return CashUp{}, err
	}
	return toCashUp(res.Msg), nil
}

func toCashUp(m *financev1.DailyCashUpResponse) CashUp {
	payments := make([]CashUpPayment, 0, len(m.GetPayments()))
	for _, p := range m.GetPayments() {
		paid := toPayment(p.GetPayment())
		payments = append(payments, CashUpPayment{
			ID: paid.ID, ChargeID: paid.ChargeID, AmountCents: paid.AmountCents, Method: paid.Method,
			Reference: paid.Reference, PaidAt: paid.PaidAt, Description: p.GetDescription(), CardType: paid.CardType,
		})
	}
	return CashUp{
		Deposits:        toDeposits(m.GetDeposits()),
		Cancelled:       toDeposits(m.GetCancelled()),
		Superseded:      toDeposits(m.GetSuperseded()),
		CashCents:       m.GetCashCents(),
		CardCents:       m.GetCardCents(),
		TransferCents:   m.GetTransferCents(),
		TotalCents:      m.GetTotalCents(),
		Payments:        payments,
		CreditNoteCents: m.GetCreditNoteCents(),
	}
}

func toDeposits(ds []*financev1.Deposit) []Deposit {
	out := make([]Deposit, 0, len(ds))
	for _, d := range ds {
		out = append(out, toDeposit(d))
	}
	return out
}

// IncomeLine is what one concept earned in a period.
type IncomeLine struct {
	ConceptID   string `json:"conceptId"`
	Concept     string `json:"concept"`
	AmountCents int64  `json:"amountCents"`
}

// Income is a period's takings, by what the school charged for.
type Income struct {
	Lines      []IncomeLine `json:"lines"`
	TotalCents int64        `json:"totalCents"`
	// CreditNoteCents is what credit notes settled in the period (T8): not
	// income, so in neither the lines nor TotalCents.
	CreditNoteCents int64 `json:"creditNoteCents"`
}

// IncomeByConcept returns what came in between two days, biggest line first.
func (c *FinanceClient) IncomeByConcept(ctx context.Context, from, to string) (Income, error) {
	res, err := c.rpc.IncomeByConcept(ctx, connect.NewRequest(&financev1.IncomeByConceptRequest{
		From: from, To: to,
	}))
	if err != nil {
		return Income{}, err
	}
	lines := make([]IncomeLine, 0, len(res.Msg.GetLines()))
	for _, l := range res.Msg.GetLines() {
		lines = append(lines, IncomeLine{
			ConceptID: l.GetConceptId(), Concept: l.GetConcept(), AmountCents: l.GetAmountCents(),
		})
	}
	return Income{Lines: lines, TotalCents: res.Msg.GetTotalCents(), CreditNoteCents: res.Msg.GetCreditNoteCents()}, nil
}

// VoidDeposit cancels a deposit and releases what it settled.
func (c *FinanceClient) VoidDeposit(ctx context.Context, id, reason string) error {
	_, err := c.rpc.VoidDeposit(ctx, connect.NewRequest(&financev1.VoidDepositRequest{
		Id: id, Reason: reason,
	}))
	return err
}

func toDeposit(d *financev1.Deposit) Deposit {
	out := Deposit{
		ID:                d.GetId(),
		StudentID:         d.GetStudentId(),
		AmountCents:       d.GetAmountCents(),
		Method:            d.GetMethod(),
		Reference:         d.GetReference(),
		CalendarVersion:   d.GetCalendarVersion(),
		CardType:          d.GetCardType(),
		VoidReason:        d.GetVoidReason(),
		ReceivedByName:    d.GetReceivedByName(),
		Folio:             d.GetFolio(),
		Tenders:           fromProtoTenders(d.GetTenders()),
		SupersededBy:      d.GetSupersededBy(),
		SupersededByFolio: d.GetSupersededByFolio(),
		CorrectsDepositID: d.GetCorrectsDepositId(),
		CorrectsFolio:     d.GetCorrectsFolio(),
	}
	if ts := d.GetSupersededAt(); ts != nil {
		out.SupersededAt = ts.AsTime().Format(time.RFC3339)
	}
	if d.GetCapturedAt() != nil {
		out.CapturedAt = d.GetCapturedAt().AsTime().Format(time.RFC3339)
	}
	if ts := d.GetPaidAt(); ts != nil {
		out.PaidAt = ts.AsTime().Format(time.RFC3339)
	}
	if ts := d.GetVoidedAt(); ts != nil {
		out.VoidedAt = ts.AsTime().Format(time.RFC3339)
	}
	return out
}

func toChargeConcept(c *financev1.ChargeConcept) ChargeConcept {
	return ChargeConcept{
		ID:                 c.GetId(),
		TenantID:           c.GetTenantId(),
		Name:               c.GetName(),
		DefaultAmountCents: c.GetDefaultAmountCents(),
		Recurrence:         c.GetRecurrence(),
		Active:             c.GetActive(),
	}
}

func toCharge(c *financev1.Charge) Charge {
	return Charge{
		ID:             c.GetId(),
		TenantID:       c.GetTenantId(),
		StudentID:      c.GetStudentId(),
		CycleID:        c.GetCycleId(),
		ConceptID:      c.GetConceptId(),
		Description:    c.GetDescription(),
		AmountCents:    c.GetAmountCents(),
		DiscountCents:  c.GetDiscountCents(),
		DueDate:        c.GetDueDate(),
		StartsOn:       c.GetStartsOn(),
		Status:         c.GetStatus(),
		PaidCents:      c.GetPaidCents(),
		RemainingCents: c.GetRemainingCents(),
	}
}

func toCharges(in []*financev1.Charge) []Charge {
	out := make([]Charge, 0, len(in))
	for _, c := range in {
		out = append(out, toCharge(c))
	}
	return out
}

// toPayment renders paid_at as RFC3339. An unset timestamp yields "" rather
// than the zero-time string, so the edge never reports a bogus instant.
func toPayment(p *financev1.Payment) Payment {
	paidAt := ""
	if ts := p.GetPaidAt(); ts != nil {
		paidAt = ts.AsTime().Format(time.RFC3339)
	}
	return Payment{
		ID:          p.GetId(),
		TenantID:    p.GetTenantId(),
		ChargeID:    p.GetChargeId(),
		AmountCents: p.GetAmountCents(),
		Method:      p.GetMethod(),
		Reference:   p.GetReference(),
		PaidAt:      paidAt,
		CardType:    p.GetCardType(),
		DepositID:   p.GetDepositId(),
	}
}

// --- tuition (M15) ----------------------------------------------------------

// TuitionPlan is the edge's view of what one level costs in one cycle.
type TuitionPlan struct {
	ID               string           `json:"id"`
	CycleID          string           `json:"cycleId"`
	LevelID          string           `json:"levelId"`
	EnrollmentCents  int64            `json:"enrollmentCents"`
	MonthlyCents     int64            `json:"monthlyCents"`
	Months           int              `json:"months"`
	FirstBilledMonth int              `json:"firstBilledMonth"`
	DueDay           int              `json:"dueDay"`
	LateFeeCents     int64            `json:"lateFeeCents"`
	Active           bool             `json:"active"`
	Policy           CollectionPolicy `json:"policy"`
	Schedule         []Installment    `json:"schedule"`
	// MonthlyDiscountCents comes off every month of this level.
	MonthlyDiscountCents int64 `json:"monthlyDiscountCents"`
	// Fees are the extras collected on top of the tuition, each split across
	// the months the school chose.
	Fees []Fee `json:"fees"`
	// EnrollmentDueOn is the deadline the school set for the inscription
	// (YYYY-MM-DD), empty when it set none.
	EnrollmentDueOn string `json:"enrollmentDueOn"`
	// EnrollmentStartsOn is the first day the inscription opens to families
	// (YYYY-MM-DD, T5), empty when it is open from issue.
	EnrollmentStartsOn string `json:"enrollmentStartsOn"`
	// EnrollmentConcepts are the lines the inscription is broken into, each with
	// its price ladder. Empty when the plan carries the single EnrollmentCents.
	EnrollmentConcepts []EnrollmentConcept `json:"enrollmentConcepts"`
}

// EnrollmentConcept is one line of the inscription, with what it costs depending
// on when the family settles it.
type EnrollmentConcept struct {
	Name  string      `json:"name"`
	Tiers []PriceTier `json:"tiers"`
}

// PriceTier is what a concept costs from a date onward (YYYY-MM-DD).
type PriceTier struct {
	EffectiveFrom string `json:"effectiveFrom"`
	// The last day the price applies, empty when the school set none.
	EffectiveTo string `json:"effectiveTo"`
	AmountCents int64  `json:"amountCents"`
}

// Fee is money the school collects on top of the monthly tuition, spread across
// the months it chooses.
//
// "La cuota de julio" is charged for a month the school year does not bill, so
// it is collected inside months that it does. AmountCents is the WHOLE fee: the
// school states what it collects, and dividing it is the server's job.
type Fee struct {
	Name        string `json:"name"`
	AmountCents int64  `json:"amountCents"`
	Months      []int  `json:"months"`
}

// Installment is one payment of the year: the month it falls in and how much of
// the tuition it collects. A school does not always divide the year into equal
// parts — "diciembre a la mitad y la otra mitad en marzo" is a real
// arrangement.
type Installment struct {
	Month       int   `json:"month"`
	AmountCents int64 `json:"amountCents"`
}

// CollectionPolicy is what a late payment costs and what an early one saves.
// It rides with the tuition plan because that is where a price lives.
type CollectionPolicy struct {
	// DailyMode picks the rule: false keeps the fixed monthly surcharge, true
	// charges LateFeeDailyCents for every day the money is late.
	DailyMode              bool  `json:"dailyMode"`
	LateFeeDailyCents      int64 `json:"lateFeeDailyCents"`
	SurchargeCountsAllDays bool  `json:"surchargeCountsAllDays"`
	EarlyPaymentCents      int64 `json:"earlyPaymentCents"`
	EarlyPaymentDays       int   `json:"earlyPaymentDays"`
}

// TuitionPlanInput is what the panel sends to price a level.
type TuitionPlanInput struct {
	CycleID              string           `json:"cycleId"`
	LevelID              string           `json:"levelId"`
	EnrollmentCents      int64            `json:"enrollmentCents"`
	MonthlyCents         int64            `json:"monthlyCents"`
	Months               int              `json:"months"`
	FirstBilledMonth     int              `json:"firstBilledMonth"`
	DueDay               int              `json:"dueDay"`
	LateFeeCents         int64            `json:"lateFeeCents"`
	Active               bool             `json:"active"`
	Policy               CollectionPolicy `json:"policy"`
	Schedule             []Installment    `json:"schedule"`
	MonthlyDiscountCents int64            `json:"monthlyDiscountCents"`
	Fees                 []Fee            `json:"fees"`
	EnrollmentDueOn      string           `json:"enrollmentDueOn"`
	EnrollmentStartsOn   string           `json:"enrollmentStartsOn"`
	// EnrollmentConcepts breaks the inscription into priced lines. Empty keeps
	// EnrollmentCents; the two are mutually exclusive (ADR 0011).
	EnrollmentConcepts []EnrollmentConcept `json:"enrollmentConcepts"`
}

// BillingRun is what a run did, as the panel reports it back to the person who
// pressed the button.
type BillingRun struct {
	TuitionCreated    int `json:"tuitionCreated"`
	EnrollmentCreated int `json:"enrollmentCreated"`
	LateFeesCreated   int `json:"lateFeesCreated"`
	FeesCreated       int `json:"feesCreated"`
	// EnrollmentMoved counts inscriptions whose deadline the run corrected.
	EnrollmentMoved int `json:"enrollmentMoved"`
	// EnrollmentRepriced counts unpaid inscriptions moved to the tier now in
	// force. Reported because this one moves money, not dates.
	EnrollmentRepriced int      `json:"enrollmentRepriced"`
	Skipped            int      `json:"skipped"`
	LevelsWithoutPlan  []string `json:"levelsWithoutPlan"`
}

// UpsertTuitionPlan sets what one level of one cycle costs.
func (c *FinanceClient) UpsertTuitionPlan(ctx context.Context, in TuitionPlanInput) (TuitionPlan, error) {
	res, err := c.rpc.UpsertTuitionPlan(ctx, connect.NewRequest(tuitionPlanRequest(in)))
	if err != nil {
		return TuitionPlan{}, err
	}
	return toTuitionPlan(res.Msg.GetPlan()), nil
}

// tuitionPlanRequest builds the wire message. Pulled out of the call so the
// mapping is testable on its own: it was silently missing two fields — they
// existed on the struct, carried JSON tags, and were never copied — and the
// screen saved a plan the API answered as zeros while everything compiled.
func tuitionPlanRequest(in TuitionPlanInput) *financev1.UpsertTuitionPlanRequest {
	return &financev1.UpsertTuitionPlanRequest{
		CycleId:              in.CycleID,
		LevelId:              in.LevelID,
		EnrollmentCents:      in.EnrollmentCents,
		MonthlyCents:         in.MonthlyCents,
		Months:               int32(in.Months),
		FirstBilledMonth:     int32(in.FirstBilledMonth),
		DueDay:               int32(in.DueDay),
		LateFeeCents:         in.LateFeeCents,
		Active:               in.Active,
		Schedule:             scheduleToProto(in.Schedule),
		MonthlyDiscountCents: in.MonthlyDiscountCents,
		Fees:                 feesToProto(in.Fees),
		EnrollmentDueOn:      in.EnrollmentDueOn,
		EnrollmentStartsOn:   in.EnrollmentStartsOn,
		EnrollmentConcepts:   enrollmentConceptsToProto(in.EnrollmentConcepts),
		Policy: &financev1.CollectionPolicy{
			DailyMode:              in.Policy.DailyMode,
			LateFeeDailyCents:      in.Policy.LateFeeDailyCents,
			SurchargeCountsAllDays: in.Policy.SurchargeCountsAllDays,
			EarlyPaymentCents:      in.Policy.EarlyPaymentCents,
			EarlyPaymentDays:       int32(in.Policy.EarlyPaymentDays),
		},
	}
}

// ListTuitionPlans returns the whole tuition table of a cycle.
func (c *FinanceClient) ListTuitionPlans(ctx context.Context, cycleID string) ([]TuitionPlan, error) {
	res, err := c.rpc.ListTuitionPlans(ctx, connect.NewRequest(&financev1.ListTuitionPlansRequest{
		CycleId: cycleID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]TuitionPlan, 0, len(res.Msg.GetPlans()))
	for _, p := range res.Msg.GetPlans() {
		out = append(out, toTuitionPlan(p))
	}
	return out, nil
}

// RunBilling generates what the school is owed as of a date.
func (c *FinanceClient) RunBilling(ctx context.Context, cycleID, asOfDate string) (BillingRun, error) {
	res, err := c.rpc.RunBilling(ctx, connect.NewRequest(&financev1.RunBillingRequest{
		CycleId: cycleID, AsOfDate: asOfDate,
	}))
	if err != nil {
		return BillingRun{}, err
	}
	return toBillingRun(res.Msg), nil
}

// toBillingRun reads what the run did. Pulled out of the call for the same
// reason tuitionPlanRequest was: a count that is added to the proto and not
// copied here never reaches the person who pressed the button.
func toBillingRun(m *financev1.RunBillingResponse) BillingRun {
	levels := m.GetLevelsWithoutPlan()
	if levels == nil {
		// A JSON null would make the panel test for it; an empty list reads the
		// same to a person and to a `for`.
		levels = []string{}
	}
	return BillingRun{
		TuitionCreated:     int(m.GetTuitionCreated()),
		EnrollmentCreated:  int(m.GetEnrollmentCreated()),
		LateFeesCreated:    int(m.GetLateFeesCreated()),
		FeesCreated:        int(m.GetFeesCreated()),
		EnrollmentMoved:    int(m.GetEnrollmentMoved()),
		EnrollmentRepriced: int(m.GetEnrollmentRepriced()),
		Skipped:            int(m.GetSkipped()),
		LevelsWithoutPlan:  levels,
	}
}

func feesToProto(in []Fee) []*financev1.Fee {
	out := make([]*financev1.Fee, 0, len(in))
	for _, f := range in {
		months := make([]int32, 0, len(f.Months))
		for _, m := range f.Months {
			months = append(months, int32(m))
		}
		out = append(out, &financev1.Fee{Name: f.Name, AmountCents: f.AmountCents, Months: months})
	}
	return out
}

func feesFromProto(in []*financev1.Fee) []Fee {
	out := make([]Fee, 0, len(in))
	for _, f := range in {
		months := make([]int, 0, len(f.GetMonths()))
		for _, m := range f.GetMonths() {
			months = append(months, int(m))
		}
		out = append(out, Fee{Name: f.GetName(), AmountCents: f.GetAmountCents(), Months: months})
	}
	return out
}

func scheduleToProto(in []Installment) []*financev1.Installment {
	out := make([]*financev1.Installment, 0, len(in))
	for _, i := range in {
		out = append(out, &financev1.Installment{Month: int32(i.Month), AmountCents: i.AmountCents})
	}
	return out
}

func scheduleFromProto(in []*financev1.Installment) []Installment {
	out := make([]Installment, 0, len(in))
	for _, i := range in {
		out = append(out, Installment{Month: int(i.GetMonth()), AmountCents: i.GetAmountCents()})
	}
	return out
}

func toTuitionPlan(p *financev1.TuitionPlan) TuitionPlan {
	return TuitionPlan{
		ID:                   p.GetId(),
		CycleID:              p.GetCycleId(),
		LevelID:              p.GetLevelId(),
		EnrollmentCents:      p.GetEnrollmentCents(),
		MonthlyCents:         p.GetMonthlyCents(),
		Months:               int(p.GetMonths()),
		FirstBilledMonth:     int(p.GetFirstBilledMonth()),
		DueDay:               int(p.GetDueDay()),
		LateFeeCents:         p.GetLateFeeCents(),
		Active:               p.GetActive(),
		Schedule:             scheduleFromProto(p.GetSchedule()),
		MonthlyDiscountCents: p.GetMonthlyDiscountCents(),
		Fees:                 feesFromProto(p.GetFees()),
		EnrollmentDueOn:      p.GetEnrollmentDueOn(),
		EnrollmentStartsOn:   p.GetEnrollmentStartsOn(),
		EnrollmentConcepts:   enrollmentConceptsFromProto(p.GetEnrollmentConcepts()),
		Policy: CollectionPolicy{
			DailyMode:              p.GetPolicy().GetDailyMode(),
			LateFeeDailyCents:      p.GetPolicy().GetLateFeeDailyCents(),
			SurchargeCountsAllDays: p.GetPolicy().GetSurchargeCountsAllDays(),
			EarlyPaymentCents:      p.GetPolicy().GetEarlyPaymentCents(),
			EarlyPaymentDays:       int(p.GetPolicy().GetEarlyPaymentDays()),
		},
	}
}

// enrollmentConceptsToProto and its twin carry the inscription's price ladder
// across the edge. They are separate from feesToProto on purpose: a fee is one
// amount split between months, a concept is one amount that CHANGES with the
// month, and an adapter that blurred them would make both unreadable.
func enrollmentConceptsToProto(in []EnrollmentConcept) []*financev1.EnrollmentConcept {
	if len(in) == 0 {
		return nil
	}
	out := make([]*financev1.EnrollmentConcept, 0, len(in))
	for _, c := range in {
		tiers := make([]*financev1.PriceTier, 0, len(c.Tiers))
		for _, t := range c.Tiers {
			tiers = append(tiers, &financev1.PriceTier{
				EffectiveFrom: t.EffectiveFrom,
				EffectiveTo:   t.EffectiveTo,
				AmountCents:   t.AmountCents,
			})
		}
		out = append(out, &financev1.EnrollmentConcept{Name: c.Name, Tiers: tiers})
	}
	return out
}

func enrollmentConceptsFromProto(in []*financev1.EnrollmentConcept) []EnrollmentConcept {
	if len(in) == 0 {
		return nil
	}
	out := make([]EnrollmentConcept, 0, len(in))
	for _, c := range in {
		tiers := make([]PriceTier, 0, len(c.GetTiers()))
		for _, t := range c.GetTiers() {
			tiers = append(tiers, PriceTier{
				EffectiveFrom: t.GetEffectiveFrom(),
				EffectiveTo:   t.GetEffectiveTo(),
				AmountCents:   t.GetAmountCents(),
			})
		}
		out = append(out, EnrollmentConcept{Name: c.GetName(), Tiers: tiers})
	}
	return out
}
