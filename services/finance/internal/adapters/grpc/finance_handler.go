// Package grpcadapter adapts the finance use cases to the generated
// garcialorca.finance.v1 Connect service. It only translates: proto messages ↔
// use-case calls, domain entities ↔ proto messages, and domain error Kinds ↔
// Connect codes. All logic lives in usecase/domain — GORM and transport details
// never leak inward.
package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/finance/v1/financev1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// Compile-time proof the adapter implements the generated Connect handler.
var _ financev1connect.FinanceServiceHandler = (*FinanceHandler)(nil)

// dueDateLayout is the wire format for a charge's due date: a plain calendar
// day, no time zone (mirrors the use case's parse side).
const dueDateLayout = "2006-01-02"

// FinanceHandler adapts the finance use cases to the generated Connect service.
type FinanceHandler struct {
	finance *usecase.FinanceService
	health  *usecase.HealthService
}

// NewFinanceHandler wires the handler to the finance + health use cases.
func NewFinanceHandler(svc *usecase.FinanceService, health *usecase.HealthService) *FinanceHandler {
	return &FinanceHandler{finance: svc, health: health}
}

// Health is the liveness/ping probe present on every service.
func (h *FinanceHandler) Health(ctx context.Context, _ *connect.Request[financev1.HealthRequest]) (*connect.Response[financev1.HealthResponse], error) {
	status, err := h.health.Ping(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.HealthResponse{Status: status.Status}), nil
}

// CreateConcept registers a charge concept in the tenant's catalog.
func (h *FinanceHandler) CreateConcept(ctx context.Context, req *connect.Request[financev1.CreateConceptRequest]) (*connect.Response[financev1.CreateConceptResponse], error) {
	m := req.Msg
	concept, err := h.finance.CreateConcept(ctx, m.GetName(), m.GetDefaultAmountCents(), m.GetRecurrence())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CreateConceptResponse{Concept: toProtoConcept(concept)}), nil
}

// ListConcepts returns the catalog; deactivated entries appear only when
// include_inactive is set.
func (h *FinanceHandler) ListConcepts(ctx context.Context, req *connect.Request[financev1.ListConceptsRequest]) (*connect.Response[financev1.ListConceptsResponse], error) {
	concepts, err := h.finance.ListConcepts(ctx, req.Msg.GetIncludeInactive())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.ChargeConcept, 0, len(concepts))
	for _, c := range concepts {
		out = append(out, toProtoConcept(c))
	}
	return connect.NewResponse(&financev1.ListConceptsResponse{Concepts: out}), nil
}

// UpdateConcept replaces the mutable fields of a catalog entry.
func (h *FinanceHandler) UpdateConcept(ctx context.Context, req *connect.Request[financev1.UpdateConceptRequest]) (*connect.Response[financev1.UpdateConceptResponse], error) {
	m := req.Msg
	concept, err := h.finance.UpdateConcept(ctx, m.GetId(), m.GetName(), m.GetDefaultAmountCents(), m.GetRecurrence(), m.GetActive())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.UpdateConceptResponse{Concept: toProtoConcept(concept)}), nil
}

// CreateCharge issues one charge against a student's cycle.
func (h *FinanceHandler) CreateCharge(ctx context.Context, req *connect.Request[financev1.CreateChargeRequest]) (*connect.Response[financev1.CreateChargeResponse], error) {
	m := req.Msg
	charge, err := h.finance.CreateCharge(ctx, usecase.CreateChargeInput{
		StudentID:           m.GetStudentId(),
		CycleID:             m.GetCycleId(),
		ConceptID:           m.GetConceptId(),
		DueDate:             m.GetDueDate(),
		AmountCentsOverride: m.GetAmountCentsOverride(),
		DescriptionOverride: m.GetDescriptionOverride(),
		ExternalRef:         m.GetExternalRef(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CreateChargeResponse{Charge: toProtoCharge(charge)}), nil
}

// GenerateCharges issues the concept to every active enrollment of a cycle (or
// of one group within it), skipping students who already owe it.
func (h *FinanceHandler) GenerateCharges(ctx context.Context, req *connect.Request[financev1.GenerateChargesRequest]) (*connect.Response[financev1.GenerateChargesResponse], error) {
	m := req.Msg
	created, skipped, err := h.finance.GenerateCharges(ctx, m.GetConceptId(), m.GetCycleId(), m.GetGroupId(),
		m.GetAmountCentsOverride(), m.GetDueDate())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.GenerateChargesResponse{
		CreatedCount: int32(created),
		SkippedCount: int32(skipped),
	}), nil
}

// ListCharges lists charges, optionally filtered by student and/or cycle.
func (h *FinanceHandler) ListCharges(ctx context.Context, req *connect.Request[financev1.ListChargesRequest]) (*connect.Response[financev1.ListChargesResponse], error) {
	m := req.Msg
	charges, err := h.finance.ListCharges(ctx, m.GetStudentId(), m.GetCycleId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ListChargesResponse{Charges: toProtoCharges(charges)}), nil
}

// ApplyDiscount sets the discount on a charge and returns it re-derived.
func (h *FinanceHandler) ApplyDiscount(ctx context.Context, req *connect.Request[financev1.ApplyDiscountRequest]) (*connect.Response[financev1.ApplyDiscountResponse], error) {
	m := req.Msg
	charge, err := h.finance.ApplyDiscount(ctx, m.GetChargeId(), m.GetDiscountCents(), m.GetReason())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ApplyDiscountResponse{Charge: toProtoCharge(charge)}), nil
}

// CancelCharge voids a charge.
func (h *FinanceHandler) CancelCharge(ctx context.Context, req *connect.Request[financev1.CancelChargeRequest]) (*connect.Response[financev1.CancelChargeResponse], error) {
	charge, err := h.finance.CancelCharge(ctx, req.Msg.GetChargeId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CancelChargeResponse{Charge: toProtoCharge(charge)}), nil
}

// CancelChargeIfUnpaid voids a charge only if nothing was collected against it.
// The guarded variant automated callers use; see the RPC comment in the proto.
func (h *FinanceHandler) CancelChargeIfUnpaid(ctx context.Context, req *connect.Request[financev1.CancelChargeIfUnpaidRequest]) (*connect.Response[financev1.CancelChargeIfUnpaidResponse], error) {
	charge, err := h.finance.CancelChargeIfUnpaid(ctx, req.Msg.GetChargeId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CancelChargeIfUnpaidResponse{Charge: toProtoCharge(charge)}), nil
}

// RegisterPayment applies a payment to a charge and returns the payment plus
// the charge with its recomputed paid/remaining/status. An idempotency key on
// the request makes a retry return the original payment instead of collecting
// again; an empty one keeps the historical behaviour.
func (h *FinanceHandler) RegisterPayment(ctx context.Context, req *connect.Request[financev1.RegisterPaymentRequest]) (*connect.Response[financev1.RegisterPaymentResponse], error) {
	m := req.Msg
	payment, charge, err := h.finance.RegisterPayment(ctx, m.GetChargeId(), m.GetAmountCents(), m.GetMethod(), m.GetReference(), m.GetIdempotencyKey())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.RegisterPaymentResponse{
		Payment: toProtoPayment(payment),
		Charge:  toProtoCharge(charge),
	}), nil
}

// GetAccount returns a student's account statement.
func (h *FinanceHandler) GetAccount(ctx context.Context, req *connect.Request[financev1.GetAccountRequest]) (*connect.Response[financev1.GetAccountResponse], error) {
	statement, err := h.finance.GetAccount(ctx, req.Msg.GetStudentId())
	if err != nil {
		return nil, connectErr(err)
	}
	payments := make([]*financev1.Payment, 0, len(statement.Payments))
	for _, p := range statement.Payments {
		payments = append(payments, toProtoPayment(p))
	}
	return connect.NewResponse(&financev1.GetAccountResponse{
		Charges:           toProtoCharges(statement.Charges),
		Payments:          payments,
		TotalBalanceCents: statement.TotalBalanceCents,
	}), nil
}

// ListOverdue lists charges past their due date that still owe money.
func (h *FinanceHandler) ListOverdue(ctx context.Context, req *connect.Request[financev1.ListOverdueRequest]) (*connect.Response[financev1.ListOverdueResponse], error) {
	charges, err := h.finance.ListOverdue(ctx, req.Msg.GetCycleId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ListOverdueResponse{Charges: toProtoCharges(charges)}), nil
}

// ExportContpaqi is the CONTPAQi XML export (spike S02). The contract reserves
// the RPC; the field mapping is a later phase, so the boundary answers
// Unimplemented rather than an empty document that could be mistaken for one.
func (h *FinanceHandler) ExportContpaqi(_ context.Context, _ *connect.Request[financev1.ExportContpaqiRequest]) (*connect.Response[financev1.ExportContpaqiResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("CONTPAQi export is not implemented yet"))
}

// toProtoConcept maps a domain ChargeConcept to its proto message.
func toProtoConcept(c domain.ChargeConcept) *financev1.ChargeConcept {
	return &financev1.ChargeConcept{
		Id:                 c.ID(),
		TenantId:           c.TenantID(),
		Name:               c.Name(),
		DefaultAmountCents: c.DefaultAmount().Int64(),
		Recurrence:         c.Recurrence(),
		Active:             c.Active(),
	}
}

// toProtoCharges maps a slice of charges with their collected totals.
func toProtoCharges(cs []ports.ChargeWithPaid) []*financev1.Charge {
	out := make([]*financev1.Charge, 0, len(cs))
	for _, c := range cs {
		out = append(out, toProtoCharge(c))
	}
	return out
}

// toProtoCharge maps a charge plus what has been collected against it, filling
// the DERIVED wire fields the domain deliberately does not store: paid_cents
// comes from the payments, and remaining_cents is the effective amount minus
// what is paid, floored at zero — an over-collected charge is a credit to
// reconcile, never a negative debt.
func toProtoCharge(cwp ports.ChargeWithPaid) *financev1.Charge {
	c := cwp.Charge
	return &financev1.Charge{
		Id:             c.ID(),
		TenantId:       c.TenantID(),
		StudentId:      c.StudentID(),
		CycleId:        c.CycleID(),
		ConceptId:      c.ConceptID(),
		Description:    c.Description(),
		AmountCents:    c.Amount().Int64(),
		DiscountCents:  c.Discount().Int64(),
		DueDate:        c.DueDate().Format(dueDateLayout),
		ExternalRef:    c.ExternalRef(),
		Status:         c.Status(),
		PaidCents:      cwp.PaidCents,
		RemainingCents: max(0, c.Effective().Int64()-cwp.PaidCents),
	}
}

// toProtoPayment maps a domain Payment to its proto message.
func toProtoPayment(p domain.Payment) *financev1.Payment {
	return &financev1.Payment{
		Id:          p.ID(),
		TenantId:    p.TenantID(),
		ChargeId:    p.ChargeID(),
		AmountCents: p.Amount().Int64(),
		Method:      p.Method(),
		Reference:   p.Reference(),
		PaidAt:      timestamppb.New(p.PaidAt()),
	}
}

// connectErr maps a domain error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks to the client);
// the other Kinds carry already-generic domain messages.
// Every code goes through glerrors.SafeMessage, not just the internal one. The
// kind decides the STATUS; it does not decide whether the wrapped cause is the
// caller's business, and it never is. Error() concatenates the cause, so handing
// the error straight to connect.NewError shipped Postgres constraint names and
// SQLSTATEs — and on some constraints the conflicting value — to whoever called.
func connectErr(err error) *connect.Error {
	safe := errors.New(glerrors.SafeMessage(err))

	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, safe)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, safe)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, safe)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, safe)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, safe)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}
