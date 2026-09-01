package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

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
	UpsertTuitionPlan(ctx context.Context, in client.TuitionPlanInput) (client.TuitionPlan, error)
	ListTuitionPlans(ctx context.Context, cycleID string) ([]client.TuitionPlan, error)
	RunBilling(ctx context.Context, cycleID, asOfDate string) (client.BillingRun, error)
	ListCharges(ctx context.Context, studentID, cycleID string) ([]client.Charge, error)
	ApplyDiscount(ctx context.Context, chargeID string, discountCents int64, reason string) (client.Charge, error)
	CancelCharge(ctx context.Context, chargeID string) (client.Charge, error)
	RegisterPaymentAt(ctx context.Context, chargeID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time) (client.Payment, client.Charge, error)
	GetAccount(ctx context.Context, studentID string) (client.AccountStatement, error)
	RegisterDeposit(ctx context.Context, studentID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time, grantEarlyPayment bool) (client.DepositResult, error)
	ExpectedToday(ctx context.Context, studentID, on string) (client.Expected, error)
	PreviewDeposit(ctx context.Context, studentID string, amountCents int64, paidAt time.Time) (client.DepositResult, error)
	ListDeposits(ctx context.Context, studentID string) ([]client.Deposit, error)
	DailyCashUp(ctx context.Context, day string) (client.CashUp, error)
	IncomeByConcept(ctx context.Context, from, to string) (client.Income, error)
	VoidDeposit(ctx context.Context, id, reason string) error
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
	// IdempotencyKey "" ⇒ no deduplication; a repeat under the same key returns
	// the original payment rather than collecting twice.
	IdempotencyKey string `json:"idempotencyKey"`
	// PaidAt "" ⇒ now. A date like "2026-09-04": the counter records the DAY the
	// money changed hands, which is not always the day it was captured.
	PaidAt string `json:"paidAt"`
}

// --- response bodies -------------------------------------------------------

type conceptResponse struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	DefaultAmountCents int64  `json:"defaultAmountCents"`
	Recurrence         string `json:"recurrence"`
	Active             bool   `json:"active"`
}

func toConceptResponse(c client.ChargeConcept) conceptResponse {
	return conceptResponse{
		ID:                 c.ID,
		Name:               c.Name,
		DefaultAmountCents: c.DefaultAmountCents,
		Recurrence:         c.Recurrence,
		Active:             c.Active,
	}
}

type chargeResponse struct {
	ID            string `json:"id"`
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
	ChargeID    string `json:"chargeId"`
	AmountCents int64  `json:"amountCents"`
	Method      string `json:"method"`
	Reference   string `json:"reference"`
	PaidAt      string `json:"paidAt"`
}

// toPaymentResponses maps a list of payments to the camelCase shape the front
// reads.
func toPaymentResponses(in []client.Payment) []paymentResponse {
	out := make([]paymentResponse, 0, len(in))
	for _, p := range in {
		out = append(out, toPaymentResponse(p))
	}
	return out
}

func toPaymentResponse(p client.Payment) paymentResponse {
	return paymentResponse{
		ID:          p.ID,
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
// the caller needs no follow-up read. An idempotencyKey in the body makes the
// endpoint safe to retry: the repeat returns the original payment.
func (h *FinanceHandler) RegisterPayment(w http.ResponseWriter, r *http.Request) {
	var req registerPaymentRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// paidAt is optional and dated-only ("2026-09-04"): the counter records a
	// day, not an instant, and a time zone on a cashier's clock is a bug waiting
	// to move a payment across a due date.
	paidAt, perr := parseCounterDate(req.PaidAt)
	if perr != nil {
		http.Error(w, "paidAt must be a date like 2026-09-04", http.StatusBadRequest)
		return
	}
	payment, charge, err := h.svc.RegisterPaymentAt(r.Context(), req.ChargeID, req.AmountCents, req.Method, req.Reference, req.IdempotencyKey, paidAt)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"payment": toPaymentResponse(payment),
		"charge":  toChargeResponse(charge),
	})
}

// --- deposits (M16) --------------------------------------------------------

// registerDepositRequest is a deposit as the counter reports it: how much, how,
// and when. What it covers is deliberately not asked for.
type registerDepositRequest struct {
	StudentID      string `json:"studentId"`
	AmountCents    int64  `json:"amountCents"`
	Method         string `json:"method"`
	Reference      string `json:"reference"`
	PaidAt         string `json:"paidAt"`
	IdempotencyKey string `json:"idempotencyKey"`
	// GrantEarlyPayment honours the reward after the window closed, because the
	// school decided to. Recorded as a decision, not as the rule firing.
	GrantEarlyPayment bool `json:"grantEarlyPayment"`
}

type voidDepositRequest struct {
	Reason string `json:"reason"`
}

// RegisterDeposit handles POST /api/v1/finance/deposits. The response carries
// the split, so the cashier can print a receipt without a follow-up read: which
// months the money landed on, which surcharges it made real, and what is left
// over as the family's credit.
func (h *FinanceHandler) RegisterDeposit(w http.ResponseWriter, r *http.Request) {
	var req registerDepositRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	paidAt, err := parseCounterDate(req.PaidAt)
	if err != nil {
		http.Error(w, "paidAt must be a date like 2026-09-04", http.StatusBadRequest)
		return
	}
	got, derr := h.svc.RegisterDeposit(r.Context(), req.StudentID, req.AmountCents,
		req.Method, req.Reference, req.IdempotencyKey, paidAt, req.GrantEarlyPayment)
	if derr != nil {
		writeConnectError(w, derr)
		return
	}
	// Mapped, not written straight through: client.Payment and client.Charge
	// carry no JSON tags, so serialising them directly puts Go field names
	// ("AmountCents") on the wire where every other finance endpoint puts
	// "amountCents". The front would read every allocation as blank.
	writeJSON(w, http.StatusCreated, map[string]any{
		"deposit":          got.Deposit,
		"allocations":      toPaymentResponses(got.Allocations),
		"surcharges":       toChargeResponses(got.Surcharges),
		"unallocatedCents": got.UnallocatedCents,
		"replayed":         got.Replayed,
	})
}

// PreviewDeposit handles POST /api/v1/finance/deposits/preview: what an amount
// WOULD cover. It writes nothing, so the cashier can say it out loud before the
// money changes hands.
func (h *FinanceHandler) PreviewDeposit(w http.ResponseWriter, r *http.Request) {
	var req registerDepositRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	paidAt, err := parseCounterDate(req.PaidAt)
	if err != nil {
		http.Error(w, "paidAt must be a date like 2026-09-04", http.StatusBadRequest)
		return
	}
	got, perr := h.svc.PreviewDeposit(r.Context(), req.StudentID, req.AmountCents, paidAt)
	if perr != nil {
		writeConnectError(w, perr)
		return
	}
	// Only the three fields a preview HAS. Returning the deposit shape would put
	// an empty deposit on the wire, and an empty deposit is a thing that reads
	// like a real one.
	writeJSON(w, http.StatusOK, map[string]any{
		"allocations":      toPaymentResponses(got.Allocations),
		"surcharges":       toChargeResponses(got.Surcharges),
		"unallocatedCents": got.UnallocatedCents,
	})
}

// ExpectedToday handles GET /api/v1/finance/expected?studentId=…&on=… — what a
// family must hand over today to be current. The counter shows it as "Monto
// esperado" and never lets anybody edit it.
func (h *FinanceHandler) ExpectedToday(w http.ResponseWriter, r *http.Request) {
	on := strings.TrimSpace(r.URL.Query().Get("on"))
	if on != "" {
		if _, err := time.Parse("2006-01-02", on); err != nil {
			http.Error(w, "on must be a date like 2026-10-10", http.StatusBadRequest)
			return
		}
	}
	expected, err := h.svc.ExpectedToday(r.Context(), r.URL.Query().Get("studentId"), on)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, expected)
}

// ListDeposits handles GET /api/v1/finance/deposits?studentId=…
func (h *FinanceHandler) ListDeposits(w http.ResponseWriter, r *http.Request) {
	deposits, err := h.svc.ListDeposits(r.Context(), r.URL.Query().Get("studentId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deposits": deposits})
}

// DailyCashUp handles GET /api/v1/finance/cash-up?day=2026-10-10 — what came in
// that day, split by how it arrived. Empty day means today.
func (h *FinanceHandler) DailyCashUp(w http.ResponseWriter, r *http.Request) {
	day := strings.TrimSpace(r.URL.Query().Get("day"))
	if day != "" {
		if _, err := time.Parse("2006-01-02", day); err != nil {
			http.Error(w, "day must be a date like 2026-10-10", http.StatusBadRequest)
			return
		}
	}
	cut, err := h.svc.DailyCashUp(r.Context(), day)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cut)
}

// IncomeByConcept handles GET /api/v1/finance/income?from=…&to=… — what the
// school earned in a period, by what it charged for. Both dates are required:
// a report over "everything since the beginning" is almost never what was
// meant, and it is the kind of mistake that gets read as a real number.
func (h *FinanceHandler) IncomeByConcept(w http.ResponseWriter, r *http.Request) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	for _, raw := range []string{from, to} {
		if _, err := time.Parse("2006-01-02", raw); err != nil {
			http.Error(w, "from and to must be dates like 2026-10-01", http.StatusBadRequest)
			return
		}
	}
	income, err := h.svc.IncomeByConcept(r.Context(), from, to)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, income)
}

// VoidDeposit handles POST /api/v1/finance/deposits/{id}/void. The reason is
// required: a cancelled deposit without one is an unexplained hole in the day's
// takings.
func (h *FinanceHandler) VoidDeposit(w http.ResponseWriter, r *http.Request) {
	var req voidDepositRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		http.Error(w, "reason is required to cancel a deposit", http.StatusBadRequest)
		return
	}
	if err := h.svc.VoidDeposit(r.Context(), r.PathValue("id"), req.Reason); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseCounterDate reads an optional dated-only value ("2026-09-04"). The
// counter records a DAY, not an instant: a time zone on a cashier's clock is a
// bug waiting to move a payment across a due date. Empty means "now".
func parseCounterDate(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, nil
	}
	return time.Parse("2006-01-02", trimmed)
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
		"unallocatedCents":  res.UnallocatedCents,
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

// --- tuition (M15) ----------------------------------------------------------

// UpsertTuitionPlan: PUT /api/v1/finance/tuition-plans
//
// A PUT and not a POST: the school states what a level costs, and stating it
// twice must mean the same thing as stating it once. There is exactly one table
// per (cycle, level), so there is nothing to create a second of.
func (h *FinanceHandler) UpsertTuitionPlan(w http.ResponseWriter, r *http.Request) {
	var body client.TuitionPlanInput
	if !decodeBody(w, r, &body) {
		return
	}
	plan, err := h.svc.UpsertTuitionPlan(r.Context(), body)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// ListTuitionPlans: GET /api/v1/finance/tuition-plans?cycleId=…
func (h *FinanceHandler) ListTuitionPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.ListTuitionPlans(r.Context(), r.URL.Query().Get("cycleId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
}

// RunBilling: POST /api/v1/finance/billing-runs
//
// Idempotent downstream, which is what makes it safe to expose as a button the
// school can press twice: the second press reports everything as skipped rather
// than charging anybody again.
func (h *FinanceHandler) RunBilling(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CycleID  string `json:"cycleId"`
		AsOfDate string `json:"asOfDate"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	run, err := h.svc.RunBilling(r.Context(), body.CycleID, body.AsOfDate)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
