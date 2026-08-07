package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// FinanceService is the finance/debt surface the edge needs (consumer-defined,
// so the handler is testable with a fake). The Connect FinanceClient implements
// it.
type FinanceService interface {
	CreateConcept(ctx context.Context, name string, defaultAmountCents int64, recurrence string) (client.ChargeConcept, error)
	ListConcepts(ctx context.Context, includeInactive bool) ([]client.ChargeConcept, error)
	UpdateConcept(ctx context.Context, id, name string, defaultAmountCents int64, recurrence string, active bool) (client.ChargeConcept, error)
	CreateCharge(ctx context.Context, studentID, cycleID, conceptID string, amountCentsOverride int64, descriptionOverride, dueDate string) (client.Charge, error)
	GenerateCharges(ctx context.Context, conceptID, cycleID, groupID string, amountCentsOverride int64, dueDate string) (int32, int32, error)
	ListCharges(ctx context.Context, studentID, cycleID string) ([]client.Charge, error)
	ApplyDiscount(ctx context.Context, chargeID string, discountCents int64, reason string) (client.Charge, error)
	CancelCharge(ctx context.Context, chargeID string) (client.Charge, error)
	RegisterPayment(ctx context.Context, chargeID string, amountCents int64, method, reference string) (client.Payment, client.Charge, error)
	GetAccount(ctx context.Context, studentID string) (client.AccountStatement, error)
	ListOverdue(ctx context.Context, cycleID string) ([]client.Charge, error)
}

// FinanceHandler exposes the front-facing (HTTP/JSON) finance endpoints,
// proxying to the finance service. It translates JSON ↔ the FinanceClient and
// Connect error codes ↔ HTTP status. Every route must run behind the module
// guard (module_finance) plus RequireAuth + rotation-enforcement, with mutations
// gated on finance:write and reads on finance:read.
//
// MONEY IS ALWAYS INTEGER CENTS (int64) on the wire: every amount field is
// named …Cents and is a JSON integer, never a float currency value.
type FinanceHandler struct {
	svc FinanceService
}

// NewFinanceHandler wires the handler to a finance service.
func NewFinanceHandler(svc FinanceService) *FinanceHandler {
	return &FinanceHandler{svc: svc}
}

// --- request bodies --------------------------------------------------------

type conceptRequest struct {
	Name               string `json:"name"`
	DefaultAmountCents int64  `json:"defaultAmountCents"`
	Recurrence         string `json:"recurrence"`
	// Active applies to the update only; create always yields an active concept.
	Active bool `json:"active"`
}

type createChargeRequest struct {
	StudentID string `json:"studentId"`
	CycleID   string `json:"cycleId"`
	ConceptID string `json:"conceptId"`
	// AmountCentsOverride 0 ⇒ the concept's default amount.
	AmountCentsOverride int64 `json:"amountCentsOverride"`
	// DescriptionOverride "" ⇒ the concept's name.
	DescriptionOverride string `json:"descriptionOverride"`
	DueDate             string `json:"dueDate"`
}

type generateChargesRequest struct {
	ConceptID string `json:"conceptId"`
	CycleID   string `json:"cycleId"`
	// GroupID "" ⇒ the whole cycle.
	GroupID             string `json:"groupId"`
	AmountCentsOverride int64  `json:"amountCentsOverride"`
	DueDate             string `json:"dueDate"`
}

type applyDiscountRequest struct {
	DiscountCents int64  `json:"discountCents"`
	Reason        string `json:"reason"`
}

type registerPaymentRequest struct {
	ChargeID    string `json:"chargeId"`
	AmountCents int64  `json:"amountCents"`
	Method      string `json:"method"`
	Reference   string `json:"reference"`
}

// --- response bodies -------------------------------------------------------

type conceptResponse struct {
	ID                 string `json:"id"`
	TenantID           string `json:"tenantId"`
	Name               string `json:"name"`
	DefaultAmountCents int64  `json:"defaultAmountCents"`
	Recurrence         string `json:"recurrence"`
	Active             bool   `json:"active"`
}

func toConceptResponse(c client.ChargeConcept) conceptResponse {
	return conceptResponse{
		ID:                 c.ID,
		TenantID:           c.TenantID,
		Name:               c.Name,
		DefaultAmountCents: c.DefaultAmountCents,
		Recurrence:         c.Recurrence,
		Active:             c.Active,
	}
}

type chargeResponse struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenantId"`
	StudentID     string `json:"studentId"`
	CycleID       string `json:"cycleId"`
	ConceptID     string `json:"conceptId"`
	Description   string `json:"description"`
	AmountCents   int64  `json:"amountCents"`
	DiscountCents int64  `json:"discountCents"`
	DueDate       string `json:"dueDate"`
	Status        string `json:"status"`
	// PaidCents and RemainingCents are derived server-side, never client input.
	PaidCents      int64 `json:"paidCents"`
	RemainingCents int64 `json:"remainingCents"`
}

func toChargeResponse(c client.Charge) chargeResponse {
	return chargeResponse{
		ID:             c.ID,
		TenantID:       c.TenantID,
		StudentID:      c.StudentID,
		CycleID:        c.CycleID,
		ConceptID:      c.ConceptID,
		Description:    c.Description,
		AmountCents:    c.AmountCents,
		DiscountCents:  c.DiscountCents,
		DueDate:        c.DueDate,
		Status:         c.Status,
		PaidCents:      c.PaidCents,
		RemainingCents: c.RemainingCents,
	}
}

func toChargeResponses(in []client.Charge) []chargeResponse {
	out := make([]chargeResponse, 0, len(in))
	for _, c := range in {
		out = append(out, toChargeResponse(c))
	}
	return out
}

type paymentResponse struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenantId"`
	ChargeID    string `json:"chargeId"`
	AmountCents int64  `json:"amountCents"`
	Method      string `json:"method"`
	Reference   string `json:"reference"`
	PaidAt      string `json:"paidAt"`
}

func toPaymentResponse(p client.Payment) paymentResponse {
	return paymentResponse{
		ID:          p.ID,
		TenantID:    p.TenantID,
		ChargeID:    p.ChargeID,
		AmountCents: p.AmountCents,
		Method:      p.Method,
		Reference:   p.Reference,
		PaidAt:      p.PaidAt,
	}
}

// --- concepts --------------------------------------------------------------

// CreateConcept handles POST /api/v1/finance/concepts.
func (h *FinanceHandler) CreateConcept(w http.ResponseWriter, r *http.Request) {
	var req conceptRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateConcept(r.Context(), req.Name, req.DefaultAmountCents, req.Recurrence)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toConceptResponse(res))
}

// UpdateConcept handles PUT /api/v1/finance/concepts/{id}: a full replace of the
// concept's mutable fields, including active.
func (h *FinanceHandler) UpdateConcept(w http.ResponseWriter, r *http.Request) {
	var req conceptRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.UpdateConcept(r.Context(), r.PathValue("id"), req.Name, req.DefaultAmountCents, req.Recurrence, req.Active)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toConceptResponse(res))
}

// ListConcepts handles GET /api/v1/finance/concepts?includeInactive=true.
// Anything other than "true" keeps the default: active concepts only.
func (h *FinanceHandler) ListConcepts(w http.ResponseWriter, r *http.Request) {
	includeInactive := r.URL.Query().Get("includeInactive") == "true"
	res, err := h.svc.ListConcepts(r.Context(), includeInactive)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]conceptResponse, 0, len(res))
	for _, c := range res {
		out = append(out, toConceptResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"concepts": out})
}

// --- charges ---------------------------------------------------------------

// CreateCharge handles POST /api/v1/finance/charges: issue one charge against a
// student's enrollment in a cycle.
func (h *FinanceHandler) CreateCharge(w http.ResponseWriter, r *http.Request) {
	var req createChargeRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateCharge(r.Context(), req.StudentID, req.CycleID, req.ConceptID,
		req.AmountCentsOverride, req.DescriptionOverride, req.DueDate)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toChargeResponse(res))
}

// GenerateCharges handles POST /api/v1/finance/charges/generate: bulk-issue the
// concept's charge across a cycle (optionally one group). The response is a
// summary, not the charges: skipped counts the idempotent re-run.
func (h *FinanceHandler) GenerateCharges(w http.ResponseWriter, r *http.Request) {
	var req generateChargesRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	created, skipped, err := h.svc.GenerateCharges(r.Context(), req.ConceptID, req.CycleID, req.GroupID,
		req.AmountCentsOverride, req.DueDate)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "skipped": skipped})
}

// ListCharges handles GET /api/v1/finance/charges?studentId=...&cycleId=...
func (h *FinanceHandler) ListCharges(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.ListCharges(r.Context(), q.Get("studentId"), q.Get("cycleId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"charges": toChargeResponses(res)})
}

// ApplyDiscount handles POST /api/v1/finance/charges/{id}/discount. The
// remaining balance and status come back recomputed.
func (h *FinanceHandler) ApplyDiscount(w http.ResponseWriter, r *http.Request) {
	var req applyDiscountRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.ApplyDiscount(r.Context(), r.PathValue("id"), req.DiscountCents, req.Reason)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toChargeResponse(res))
}

// CancelCharge handles POST /api/v1/finance/charges/{id}/cancel. It carries no
// body: the charge id in the path is the whole request.
func (h *FinanceHandler) CancelCharge(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.CancelCharge(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toChargeResponse(res))
}

// --- payments + account ----------------------------------------------------

// RegisterPayment handles POST /api/v1/finance/payments. The response carries
// both the payment and the charge with its recomputed paid/remaining/status, so
// the caller needs no follow-up read.
func (h *FinanceHandler) RegisterPayment(w http.ResponseWriter, r *http.Request) {
	var req registerPaymentRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	payment, charge, err := h.svc.RegisterPayment(r.Context(), req.ChargeID, req.AmountCents, req.Method, req.Reference)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"payment": toPaymentResponse(payment),
		"charge":  toChargeResponse(charge),
	})
}

// GetAccount handles GET /api/v1/finance/accounts/{studentId}: the student's
// full statement (charges, payments, outstanding balance).
func (h *FinanceHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.GetAccount(r.Context(), r.PathValue("studentId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	payments := make([]paymentResponse, 0, len(res.Payments))
	for _, p := range res.Payments {
		payments = append(payments, toPaymentResponse(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"charges":           toChargeResponses(res.Charges),
		"payments":          payments,
		"totalBalanceCents": res.TotalBalanceCents,
	})
}

// ListOverdue handles GET /api/v1/finance/overdue?cycleId=... — charges past
// their due date with a remaining balance.
func (h *FinanceHandler) ListOverdue(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListOverdue(r.Context(), r.URL.Query().Get("cycleId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"overdue": toChargeResponses(res)})
}
