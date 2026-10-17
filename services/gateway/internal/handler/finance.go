package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
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
	ListCharges(ctx context.Context, studentID, cycleID string, page client.PageRequest) (client.ChargePage, error)
	ApplyDiscount(ctx context.Context, chargeID string, discountCents int64, reason string) (client.Charge, error)
	CancelCharge(ctx context.Context, chargeID string) (client.Charge, error)
	RegisterTenderedPayment(ctx context.Context, in client.TenderedPayment) ([]client.Payment, client.Charge, error)
	GetAccount(ctx context.Context, studentID string) (client.AccountStatement, error)
	RegisterDeposit(ctx context.Context, studentID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time, grantEarlyPayment bool, cardType string, receiver client.Receiver, chargeIDs []string, tenders []client.Tender) (client.DepositResult, error)
	ExpectedToday(ctx context.Context, studentID, on string, withBalance bool) (client.Expected, error)
	PreviewDeposit(ctx context.Context, studentID string, amountCents int64, paidAt time.Time, chargeIDs []string) (client.DepositResult, error)
	ListDeposits(ctx context.Context, studentID string) ([]client.Deposit, error)
	DailyCashUp(ctx context.Context, day string) (client.CashUp, error)
	IncomeByConcept(ctx context.Context, from, to string) (client.Income, error)
	VoidDeposit(ctx context.Context, id, reason string) error
	CorrectDeposit(ctx context.Context, in client.CorrectionInput) (client.CorrectionResult, error)
	DepositCorrections(ctx context.Context, depositID string) ([]client.DepositCorrection, error)
	ListOverdue(ctx context.Context, cycleID string, page client.PageRequest) (client.ChargePage, error)
}

// FinanceHandler exposes the front-facing (HTTP/JSON) finance endpoints,
// proxying to the finance service. It translates JSON ↔ the FinanceClient and
// Connect error codes ↔ HTTP status. Every route must run behind the module
// guard (module_finance) plus RequireAuth + rotation-enforcement, with mutations
// gated on finance:write and reads on finance:read. The counter's routes also
// answer to collections:read and payments:write (the Cobranza role, ADR 0015);
// cmd/main.go is where each route's permission is set.
//
// MONEY IS ALWAYS INTEGER CENTS (int64) on the wire: every amount field is
// named …Cents and is a JSON integer, never a float currency value.
type FinanceHandler struct {
	svc   FinanceService
	names UserNamer
	// cutFlags decides whether the cash-up may carry T10's expenses and
	// closures (module_expenses); nil never shows them.
	cutFlags featureflag.Checker
}

// FinanceOption configures a FinanceHandler.
type FinanceOption func(*FinanceHandler)

// WithNames names the cashier who takes money from iam's directory (task 7).
// Without it the name falls back to the token's email.
func WithNames(names UserNamer) FinanceOption {
	return func(h *FinanceHandler) { h.names = names }
}

// WithCutExtras lets the cash-up carry the day's expenses, the expected cash
// and the closures (T10) — to a caller holding expenses:read, while
// module_expenses is on. Everyone else gets the takings alone: the cashier
// (collections:read) reads the counter, not what the school paid out.
func WithCutExtras(flags featureflag.Checker) FinanceOption {
	return func(h *FinanceHandler) { h.cutFlags = flags }
}

// NewFinanceHandler wires the handler to a finance service.
func NewFinanceHandler(svc FinanceService, opts ...FinanceOption) *FinanceHandler {
	h := &FinanceHandler{svc: svc}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// maxChosenCharges bounds the counter's choice: a family has at most a year of
// charges, far fewer than this.
const maxChosenCharges = 500

var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// invalidChoice checks chargeIds against the contract at the edge (task 8):
// uuids, a bounded list. "" means it is fine.
func invalidChoice(ids []string) string {
	if len(ids) > maxChosenCharges {
		return "chargeIds: too many charges"
	}
	for _, id := range ids {
		if !uuidShape.MatchString(id) {
			return "chargeIds must be charge ids"
		}
	}
	return ""
}

// accountOf is the authenticated caller's account alone: a single-charge payment
// keeps no name, so iam is not asked for one.
func accountOf(r *http.Request) client.Receiver {
	p, _ := middleware.PrincipalFrom(r.Context())
	return client.Receiver{UserID: p.UserID}
}

// receiverOf is who takes the money: the authenticated caller, never a field
// of the request body (task 7), with their name for the cash-up.
func (h *FinanceHandler) receiverOf(r *http.Request) client.Receiver {
	p, _ := middleware.PrincipalFrom(r.Context())
	return client.Receiver{UserID: p.UserID, Name: callerName(r.Context(), h.names, p)}
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
	// Tenders combine methods (T8); empty means the single method above.
	Tenders []client.Tender `json:"tenders"`
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
	// StartsOn is the first day the charge opens to the family (T5); empty
	// when it is open from issue.
	StartsOn string `json:"startsOn,omitempty"`
	Status   string `json:"status"`
	// PaidCents and RemainingCents are derived server-side, never client input.
	PaidCents      int64 `json:"paidCents"`
	RemainingCents int64 `json:"remainingCents"`
	// ScholarshipCents is the scholarship's part (T18), already out of
	// RemainingCents.
	ScholarshipCents int64 `json:"scholarshipCents"`
}

func toChargeResponse(c client.Charge) chargeResponse {
	return chargeResponse{
		ID:               c.ID,
		StudentID:        c.StudentID,
		CycleID:          c.CycleID,
		ConceptID:        c.ConceptID,
		Description:      c.Description,
		AmountCents:      c.AmountCents,
		DiscountCents:    c.DiscountCents,
		DueDate:          c.DueDate,
		StartsOn:         c.StartsOn,
		Status:           c.Status,
		PaidCents:        c.PaidCents,
		RemainingCents:   c.RemainingCents,
		ScholarshipCents: c.ScholarshipCents,
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
	// PaidOn is the school-local day (ADR 0035).
	PaidOn   string `json:"paidOn,omitempty"`
	CardType string `json:"cardType,omitempty"`
	// DepositID is the receipt this payment is a slice of (T9).
	DepositID string `json:"depositId,omitempty"`
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
		PaidOn:      p.PaidOn,
		CardType:    p.CardType,
		DepositID:   p.DepositID,
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
// &pageToken=...&pageSize=... — one page; nextPageToken is absent on the last.
func (h *FinanceHandler) ListCharges(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, ok := readPageRequest(w, q)
	if !ok {
		return
	}
	res, err := h.svc.ListCharges(r.Context(), q.Get("studentId"), q.Get("cycleId"), page)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pagedChargesBody("charges", res))
}

// maxFinancePageSize is the largest page the finance reads serve. It mirrors
// finance's ports.MaxChargePageSize (and the contract's pageSize maximum); the
// gateway cannot import another module's internal package, so a change there
// must be made here too.
const maxFinancePageSize = 500

// readPageRequest reads pageToken and pageSize. pageSize, when present, must be
// a whole number in 1..maxFinancePageSize; anything else answers 400 in the
// envelope and reports false. Absent means the service default.
func readPageRequest(w http.ResponseWriter, q url.Values) (client.PageRequest, bool) {
	page := client.PageRequest{Token: q.Get("pageToken")}
	if !q.Has("pageSize") {
		return page, true
	}
	size, err := strconv.Atoi(q.Get("pageSize"))
	if err != nil || size < 1 || size > maxFinancePageSize {
		middleware.WriteError(w, http.StatusBadRequest, "invalid",
			fmt.Sprintf("pageSize debe ser un entero entre 1 y %d", maxFinancePageSize), map[string]string{"field": "pageSize"})
		return client.PageRequest{}, false
	}
	page.Size = int32(size)
	return page, true
}

// pagedChargesBody is a page of charges under listKey, with nextPageToken only
// when another page exists.
func pagedChargesBody(listKey string, page client.ChargePage) map[string]any {
	body := map[string]any{listKey: toChargeResponses(page.Charges)}
	if page.NextPageToken != "" {
		body["nextPageToken"] = page.NextPageToken
	}
	return body
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
	if msg := invalidTenders(req.Tenders); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	tenders := req.Tenders
	if len(tenders) == 0 {
		tenders = []client.Tender{{Method: req.Method, AmountCents: req.AmountCents, Reference: req.Reference}}
	}
	payments, charge, err := h.svc.RegisterTenderedPayment(r.Context(), client.TenderedPayment{
		ChargeID: req.ChargeID, AmountCents: req.AmountCents, Tenders: tenders,
		IdempotencyKey: req.IdempotencyKey, PaidAt: paidAt, Receiver: accountOf(r),
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"payment":  toPaymentResponse(payments[0]),
		"payments": toPaymentResponses(payments),
		"charge":   toChargeResponse(charge),
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
	// CardType is debit or credit, only meaningful when method is card.
	CardType string `json:"cardType"`
	// ChargeIDs are the charges the money may pay, chosen at the counter
	// (task 8). Empty means every charge, oldest first.
	ChargeIDs []string `json:"chargeIds"`
	// Tenders combine methods (T8) and replace method, reference and cardType.
	Tenders []client.Tender `json:"tenders"`
}

// The contract's bounds on a combined payment (Tender in openapi.yaml).
const (
	maxTenders         = 4
	maxTenderReference = 120
)

// tenderMethods is the contract's method vocabulary for a tender.
var tenderMethods = map[string]bool{"cash": true, "card": true, "transfer": true, "credit_note": true}

// invalidTenders checks the counter's tenders against the contract at the
// edge: at most four, a known method, a positive amount, a card type only on a
// card, a bounded reference.
// Whether they add up to the amount is finance's rule, and finance enforces it.
// Empty means no tenders were sent, which is valid.
func invalidTenders(ts []client.Tender) string {
	if len(ts) > maxTenders {
		return "a payment combines at most four methods"
	}
	for _, t := range ts {
		switch {
		case !tenderMethods[t.Method]:
			return "tender method must be cash, card, transfer or credit_note"
		case t.AmountCents <= 0:
			return "every tender needs an amount greater than zero"
		case t.CardType != "" && (t.Method != "card" || (t.CardType != "debit" && t.CardType != "credit")):
			return "cardType is debit or credit, and only on a card"
		case len([]rune(t.Reference)) > maxTenderReference:
			return "a tender reference is at most 120 characters"
		}
	}
	return ""
}

// grantsDiscounts is the permission that may lower what a family owes.
const grantsDiscounts = "finance:write"

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
	if msg := invalidChoice(req.ChargeIDs); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if msg := invalidTenders(req.Tenders); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	// The late reward is a discount the school grants, and discounts are
	// finance:write's: payments:write alone takes money at the listed price
	// (ADR 0015).
	if req.GrantEarlyPayment && !callerHolds(r, grantsDiscounts) {
		http.Error(w, "granting the early-payment discount requires "+grantsDiscounts, http.StatusForbidden)
		return
	}
	got, derr := h.svc.RegisterDeposit(r.Context(), req.StudentID, req.AmountCents,
		req.Method, req.Reference, req.IdempotencyKey, paidAt, req.GrantEarlyPayment, req.CardType, h.receiverOf(r), req.ChargeIDs, req.Tenders)
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
	if msg := invalidChoice(req.ChargeIDs); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	got, perr := h.svc.PreviewDeposit(r.Context(), req.StudentID, req.AmountCents, paidAt, req.ChargeIDs)
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

// ExpectedToday handles GET /api/v1/finance/expected?studentId=…&on=…&withBalance=…
// — what a family must hand over today to be current. The counter shows it as
// "Monto esperado" and never lets anybody edit it. withBalance=false (the
// search list) leaves the year's balance out; it is included by default.
func (h *FinanceHandler) ExpectedToday(w http.ResponseWriter, r *http.Request) {
	on := strings.TrimSpace(r.URL.Query().Get("on"))
	if on != "" {
		if _, err := time.Parse("2006-01-02", on); err != nil {
			http.Error(w, "on must be a date like 2026-10-10", http.StatusBadRequest)
			return
		}
	}
	withBalance := true
	if raw := r.URL.Query().Get("withBalance"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "withBalance must be true or false", http.StatusBadRequest)
			return
		}
		withBalance = parsed
	}
	expected, err := h.svc.ExpectedToday(r.Context(), r.URL.Query().Get("studentId"), on, withBalance)
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
	if !h.mayReadCutExtras(r) {
		cut = takingsOnly(cut)
	}
	writeJSON(w, http.StatusOK, cut)
}

// mayReadCutExtras: module_expenses on and the caller holds expenses:read.
func (h *FinanceHandler) mayReadCutExtras(r *http.Request) bool {
	if h.cutFlags == nil || !h.cutFlags.Enabled(r.Context(), featureflag.Key("expenses")) {
		return false
	}
	p, _ := middleware.PrincipalFrom(r.Context())
	return slices.Contains(p.Permissions, "expenses:read")
}

// takingsOnly drops T10's expense and closure fields from a cut.
func takingsOnly(cut client.CashUp) client.CashUp {
	cut.Expenses = []client.Expense{}
	cut.ExpensesByMethod = client.MethodTotals{}
	cut.ExpectedCashCents = 0
	cut.Closure = nil
	cut.ModifiedAfterClose = false
	cut.Reopened = []client.CashCutClosure{}
	return cut
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
		// The same figures the family's own statement carries, so the office and
		// the parent are never quoting two different numbers.
		"dueTodayCents":      res.DueTodayCents,
		"surchargeCents":     res.SurchargeCents,
		"earlyDiscountCents": res.EarlyDiscountCents,
		"asOf":               res.AsOf,
	})
}

// ListOverdue handles GET /api/v1/finance/overdue?cycleId=... — charges past
// their due date with a remaining balance, one page at a time (&pageToken,
// &pageSize). A page may be short or empty while nextPageToken is present.
func (h *FinanceHandler) ListOverdue(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, ok := readPageRequest(w, q)
	if !ok {
		return
	}
	res, err := h.svc.ListOverdue(r.Context(), q.Get("cycleId"), page)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pagedChargesBody("overdue", res))
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
