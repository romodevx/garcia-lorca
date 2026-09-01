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
	ID             string
	TenantID       string
	StudentID      string
	CycleID        string
	ConceptID      string
	Description    string
	AmountCents    int64
	DiscountCents  int64
	DueDate        string
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
	return c.RegisterPaymentAt(ctx, chargeID, amountCents, method, reference, idempotencyKey, time.Time{})
}

// RegisterPaymentAt carries the date the money changed hands. A zero paidAt is
// omitted from the request, so the service stamps its own clock exactly as
// before.
func (c *FinanceClient) RegisterPaymentAt(ctx context.Context, chargeID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time) (Payment, Charge, error) {
	req := &financev1.RegisterPaymentRequest{
		ChargeId:       chargeID,
		AmountCents:    amountCents,
		Method:         method,
		Reference:      reference,
		IdempotencyKey: idempotencyKey,
	}
	if !paidAt.IsZero() {
		req.PaidAt = timestamppb.New(paidAt)
	}
	res, err := c.rpc.RegisterPayment(ctx, connect.NewRequest(req))
	if err != nil {
		return Payment{}, Charge{}, err
	}
	return toPayment(res.Msg.GetPayment()), toCharge(res.Msg.GetCharge()), nil
}

// GetAccount returns a student's account statement: every charge, every
// payment, and the total outstanding balance.
func (c *FinanceClient) GetAccount(ctx context.Context, studentID string) (AccountStatement, error) {
	res, err := c.rpc.GetAccount(ctx, connect.NewRequest(&financev1.GetAccountRequest{
		StudentId: studentID,
	}))
	if err != nil {
		return AccountStatement{}, err
	}
	payments := make([]Payment, 0, len(res.Msg.GetPayments()))
	for _, p := range res.Msg.GetPayments() {
		payments = append(payments, toPayment(p))
	}
	return AccountStatement{
		Charges:           toCharges(res.Msg.GetCharges()),
		Payments:          payments,
		TotalBalanceCents: res.Msg.GetTotalBalanceCents(),
		UnallocatedCents:  res.Msg.GetUnallocatedCents(),
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
type Deposit struct {
	ID              string `json:"id"`
	StudentID       string `json:"studentId"`
	AmountCents     int64  `json:"amountCents"`
	Method          string `json:"method"`
	Reference       string `json:"reference"`
	PaidAt          string `json:"paidAt"`
	CalendarVersion string `json:"calendarVersion"`
	VoidedAt        string `json:"voidedAt,omitempty"`
	VoidReason      string `json:"voidReason,omitempty"`
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
func (c *FinanceClient) RegisterDeposit(ctx context.Context, studentID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time) (DepositResult, error) {
	req := &financev1.RegisterDepositRequest{
		StudentId:      studentID,
		AmountCents:    amountCents,
		Method:         method,
		Reference:      reference,
		IdempotencyKey: idempotencyKey,
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
func (c *FinanceClient) PreviewDeposit(ctx context.Context, studentID string, amountCents int64, paidAt time.Time) (DepositResult, error) {
	req := &financev1.PreviewDepositRequest{StudentId: studentID, AmountCents: amountCents}
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
	Deposits      []Deposit `json:"deposits"`
	Cancelled     []Deposit `json:"cancelled"`
	CashCents     int64     `json:"cashCents"`
	CardCents     int64     `json:"cardCents"`
	TransferCents int64     `json:"transferCents"`
	TotalCents    int64     `json:"totalCents"`
}

// DailyCashUp returns what came in on one day, and in what form.
func (c *FinanceClient) DailyCashUp(ctx context.Context, day string) (CashUp, error) {
	res, err := c.rpc.DailyCashUp(ctx, connect.NewRequest(&financev1.DailyCashUpRequest{Day: day}))
	if err != nil {
		return CashUp{}, err
	}
	return CashUp{
		Deposits:      toDeposits(res.Msg.GetDeposits()),
		Cancelled:     toDeposits(res.Msg.GetCancelled()),
		CashCents:     res.Msg.GetCashCents(),
		CardCents:     res.Msg.GetCardCents(),
		TransferCents: res.Msg.GetTransferCents(),
		TotalCents:    res.Msg.GetTotalCents(),
	}, nil
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
	return Income{Lines: lines, TotalCents: res.Msg.GetTotalCents()}, nil
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
		ID:              d.GetId(),
		StudentID:       d.GetStudentId(),
		AmountCents:     d.GetAmountCents(),
		Method:          d.GetMethod(),
		Reference:       d.GetReference(),
		CalendarVersion: d.GetCalendarVersion(),
		VoidReason:      d.GetVoidReason(),
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
}

// BillingRun is what a run did, as the panel reports it back to the person who
// pressed the button.
type BillingRun struct {
	TuitionCreated    int      `json:"tuitionCreated"`
	EnrollmentCreated int      `json:"enrollmentCreated"`
	LateFeesCreated   int      `json:"lateFeesCreated"`
	Skipped           int      `json:"skipped"`
	LevelsWithoutPlan []string `json:"levelsWithoutPlan"`
}

// UpsertTuitionPlan sets what one level of one cycle costs.
func (c *FinanceClient) UpsertTuitionPlan(ctx context.Context, in TuitionPlanInput) (TuitionPlan, error) {
	res, err := c.rpc.UpsertTuitionPlan(ctx, connect.NewRequest(&financev1.UpsertTuitionPlanRequest{
		CycleId:          in.CycleID,
		LevelId:          in.LevelID,
		EnrollmentCents:  in.EnrollmentCents,
		MonthlyCents:     in.MonthlyCents,
		Months:           int32(in.Months),
		FirstBilledMonth: int32(in.FirstBilledMonth),
		DueDay:           int32(in.DueDay),
		LateFeeCents:     in.LateFeeCents,
		Active:           in.Active,
		Policy: &financev1.CollectionPolicy{
			DailyMode:              in.Policy.DailyMode,
			LateFeeDailyCents:      in.Policy.LateFeeDailyCents,
			SurchargeCountsAllDays: in.Policy.SurchargeCountsAllDays,
			EarlyPaymentCents:      in.Policy.EarlyPaymentCents,
			EarlyPaymentDays:       int32(in.Policy.EarlyPaymentDays),
		},
	}))
	if err != nil {
		return TuitionPlan{}, err
	}
	return toTuitionPlan(res.Msg.GetPlan()), nil
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
	m := res.Msg
	levels := m.GetLevelsWithoutPlan()
	if levels == nil {
		// A JSON null would make the panel test for it; an empty list reads the
		// same to a person and to a `for`.
		levels = []string{}
	}
	return BillingRun{
		TuitionCreated:    int(m.GetTuitionCreated()),
		EnrollmentCreated: int(m.GetEnrollmentCreated()),
		LateFeesCreated:   int(m.GetLateFeesCreated()),
		Skipped:           int(m.GetSkipped()),
		LevelsWithoutPlan: levels,
	}, nil
}

func toTuitionPlan(p *financev1.TuitionPlan) TuitionPlan {
	return TuitionPlan{
		ID:               p.GetId(),
		CycleID:          p.GetCycleId(),
		LevelID:          p.GetLevelId(),
		EnrollmentCents:  p.GetEnrollmentCents(),
		MonthlyCents:     p.GetMonthlyCents(),
		Months:           int(p.GetMonths()),
		FirstBilledMonth: int(p.GetFirstBilledMonth()),
		DueDay:           int(p.GetDueDay()),
		LateFeeCents:     p.GetLateFeeCents(),
		Active:           p.GetActive(),
		Policy: CollectionPolicy{
			DailyMode:              p.GetPolicy().GetDailyMode(),
			LateFeeDailyCents:      p.GetPolicy().GetLateFeeDailyCents(),
			SurchargeCountsAllDays: p.GetPolicy().GetSurchargeCountsAllDays(),
			EarlyPaymentCents:      p.GetPolicy().GetEarlyPaymentCents(),
			EarlyPaymentDays:       int(p.GetPolicy().GetEarlyPaymentDays()),
		},
	}
}
