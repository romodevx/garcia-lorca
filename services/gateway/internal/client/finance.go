package client

import (
	"context"
	"net/http"
	"time"

	connect "connectrpc.com/connect"

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
// the charge with its recomputed paid/remaining/status.
func (c *FinanceClient) RegisterPayment(ctx context.Context, chargeID string, amountCents int64, method, reference string) (Payment, Charge, error) {
	res, err := c.rpc.RegisterPayment(ctx, connect.NewRequest(&financev1.RegisterPaymentRequest{
		ChargeId:    chargeID,
		AmountCents: amountCents,
		Method:      method,
		Reference:   reference,
	}))
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
