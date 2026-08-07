package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// fakeFinance is a hand-written double for the finance surface the handler
// needs. It records the last input and returns canned results/errors so each
// handler is exercised in isolation (no Connect round-trip).
type fakeFinance struct {
	conceptName       string
	conceptAmount     int64
	conceptRecurrence string
	conceptID         string
	conceptActive     bool
	includeInactive   bool
	concepts          []client.ChargeConcept

	chargeStudent     string
	chargeCycle       string
	chargeConcept     string
	chargeAmount      int64
	chargeDescription string
	chargeDueDate     string

	genGroup string
	created  int32
	skipped  int32

	listStudent string
	listCycle   string
	charges     []client.Charge

	discountChargeID string
	discountCents    int64
	discountReason   string

	cancelChargeID string

	payChargeID string
	payAmount   int64
	payMethod   string
	payRef      string
	payKey      string
	payment     client.Payment

	accountStudent string
	account        client.AccountStatement

	overdueCycle string
	overdue      []client.Charge

	charge client.Charge
	err    error
}

func (f *fakeFinance) CreateConcept(_ context.Context, name string, defaultAmountCents int64, recurrence string) (client.ChargeConcept, error) {
	f.conceptName, f.conceptAmount, f.conceptRecurrence = name, defaultAmountCents, recurrence
	if f.err != nil {
		return client.ChargeConcept{}, f.err
	}
	return client.ChargeConcept{ID: "cpt-1", TenantID: "t1", Name: name, DefaultAmountCents: defaultAmountCents, Recurrence: recurrence, Active: true}, nil
}

func (f *fakeFinance) ListConcepts(_ context.Context, includeInactive bool) ([]client.ChargeConcept, error) {
	f.includeInactive = includeInactive
	return f.concepts, f.err
}

func (f *fakeFinance) UpdateConcept(_ context.Context, id, name string, defaultAmountCents int64, recurrence string, active bool) (client.ChargeConcept, error) {
	f.conceptID, f.conceptName, f.conceptAmount, f.conceptRecurrence, f.conceptActive = id, name, defaultAmountCents, recurrence, active
	if f.err != nil {
		return client.ChargeConcept{}, f.err
	}
	return client.ChargeConcept{ID: id, TenantID: "t1", Name: name, DefaultAmountCents: defaultAmountCents, Recurrence: recurrence, Active: active}, nil
}

func (f *fakeFinance) CreateCharge(_ context.Context, studentID, cycleID, conceptID string, amountCentsOverride int64, descriptionOverride, dueDate string) (client.Charge, error) {
	f.chargeStudent, f.chargeCycle, f.chargeConcept = studentID, cycleID, conceptID
	f.chargeAmount, f.chargeDescription, f.chargeDueDate = amountCentsOverride, descriptionOverride, dueDate
	if f.err != nil {
		return client.Charge{}, f.err
	}
	return f.charge, nil
}

func (f *fakeFinance) GenerateCharges(_ context.Context, conceptID, cycleID, groupID string, amountCentsOverride int64, dueDate string) (int32, int32, error) {
	f.chargeConcept, f.chargeCycle, f.genGroup = conceptID, cycleID, groupID
	f.chargeAmount, f.chargeDueDate = amountCentsOverride, dueDate
	if f.err != nil {
		return 0, 0, f.err
	}
	return f.created, f.skipped, nil
}

func (f *fakeFinance) ListCharges(_ context.Context, studentID, cycleID string) ([]client.Charge, error) {
	f.listStudent, f.listCycle = studentID, cycleID
	return f.charges, f.err
}

func (f *fakeFinance) ApplyDiscount(_ context.Context, chargeID string, discountCents int64, reason string) (client.Charge, error) {
	f.discountChargeID, f.discountCents, f.discountReason = chargeID, discountCents, reason
	if f.err != nil {
		return client.Charge{}, f.err
	}
	return f.charge, nil
}

func (f *fakeFinance) CancelCharge(_ context.Context, chargeID string) (client.Charge, error) {
	f.cancelChargeID = chargeID
	if f.err != nil {
		return client.Charge{}, f.err
	}
	return f.charge, nil
}

func (f *fakeFinance) RegisterPayment(_ context.Context, chargeID string, amountCents int64, method, reference, idempotencyKey string) (client.Payment, client.Charge, error) {
	f.payChargeID, f.payAmount, f.payMethod, f.payRef = chargeID, amountCents, method, reference
	f.payKey = idempotencyKey
	if f.err != nil {
		return client.Payment{}, client.Charge{}, f.err
	}
	return f.payment, f.charge, nil
}

func (f *fakeFinance) GetAccount(_ context.Context, studentID string) (client.AccountStatement, error) {
	f.accountStudent = studentID
	return f.account, f.err
}

func (f *fakeFinance) ListOverdue(_ context.Context, cycleID string) ([]client.Charge, error) {
	f.overdueCycle = cycleID
	return f.overdue, f.err
}

// sampleCharge is the canned charge the fake returns from every charge-mutating
// call. Money is integer cents everywhere — 150000 = $1,500.00 MXN.
var sampleCharge = client.Charge{
	ID: "chg-1", TenantID: "t1", StudentID: "stu-1", CycleID: "cyc-1", ConceptID: "cpt-1",
	Description: "Colegiatura", AmountCents: 150000, DiscountCents: 0,
	DueDate: "2026-09-05", Status: "pending", PaidCents: 0, RemainingCents: 150000,
}

// --- concepts --------------------------------------------------------------

func TestFinanceCreateConceptReturns201AndBody(t *testing.T) {
	svc := &fakeFinance{}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.CreateConcept, http.MethodPost, "/api/v1/finance/concepts",
		`{"name":"Colegiatura","defaultAmountCents":150000,"recurrence":"monthly"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.conceptName != "Colegiatura" || svc.conceptAmount != 150000 || svc.conceptRecurrence != "monthly" {
		t.Errorf("forwarded input = %q %d %q", svc.conceptName, svc.conceptAmount, svc.conceptRecurrence)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// Money is int64 cents on the wire: 150000 must survive verbatim.
	if out["id"] != "cpt-1" || out["defaultAmountCents"] != float64(150000) || out["active"] != true {
		t.Errorf("body = %v", out)
	}
}

func TestFinanceCreateConceptInvalidBodyIs400(t *testing.T) {
	h := handler.NewFinanceHandler(&fakeFinance{})
	rec := doReq(t, h.CreateConcept, http.MethodPost, "/api/v1/finance/concepts", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestFinanceCreateConceptInvalidArgumentIs400(t *testing.T) {
	svc := &fakeFinance{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewFinanceHandler(svc)
	rec := doReq(t, h.CreateConcept, http.MethodPost, "/api/v1/finance/concepts",
		`{"name":"","defaultAmountCents":-1,"recurrence":"weekly"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestFinanceListConceptsPassesIncludeInactive(t *testing.T) {
	svc := &fakeFinance{concepts: []client.ChargeConcept{
		{ID: "cpt-1", Name: "Colegiatura", DefaultAmountCents: 150000, Recurrence: "monthly", Active: true},
		{ID: "cpt-2", Name: "Inscripción", DefaultAmountCents: 500000, Recurrence: "once", Active: false},
	}}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.ListConcepts, http.MethodGet, "/api/v1/finance/concepts?includeInactive=true", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !svc.includeInactive {
		t.Error("includeInactive = false, want true")
	}
	var out struct {
		Concepts []map[string]any `json:"concepts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.Concepts) != 2 || out.Concepts[1]["defaultAmountCents"] != float64(500000) || out.Concepts[1]["active"] != false {
		t.Errorf("body = %v", out)
	}
}

func TestFinanceListConceptsDefaultsToActiveOnly(t *testing.T) {
	svc := &fakeFinance{includeInactive: true} // must be overwritten by the handler
	h := handler.NewFinanceHandler(svc)
	rec := doReq(t, h.ListConcepts, http.MethodGet, "/api/v1/finance/concepts", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.includeInactive {
		t.Error("includeInactive = true, want false when the query param is absent")
	}
}

func TestFinanceUpdateConceptUsesPathID(t *testing.T) {
	svc := &fakeFinance{}
	h := handler.NewFinanceHandler(svc)

	rec := doAdminReq(t, http.HandlerFunc(h.UpdateConcept), http.MethodPut, "/api/v1/finance/concepts/cpt-9",
		`{"name":"Colegiatura","defaultAmountCents":160000,"recurrence":"monthly","active":false}`, "id", "cpt-9")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.conceptID != "cpt-9" || svc.conceptAmount != 160000 || svc.conceptActive {
		t.Errorf("forwarded input = %q %d %v", svc.conceptID, svc.conceptAmount, svc.conceptActive)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["id"] != "cpt-9" || out["defaultAmountCents"] != float64(160000) || out["active"] != false {
		t.Errorf("body = %v", out)
	}
}

func TestFinanceUpdateConceptUnknownIs404(t *testing.T) {
	svc := &fakeFinance{err: connect.NewError(connect.CodeNotFound, nil)}
	h := handler.NewFinanceHandler(svc)
	rec := doAdminReq(t, http.HandlerFunc(h.UpdateConcept), http.MethodPut, "/api/v1/finance/concepts/ghost",
		`{"name":"x","defaultAmountCents":1,"recurrence":"once","active":true}`, "id", "ghost")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// --- charges ---------------------------------------------------------------

func TestFinanceCreateChargeReturns201AndBody(t *testing.T) {
	svc := &fakeFinance{charge: sampleCharge}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.CreateCharge, http.MethodPost, "/api/v1/finance/charges",
		`{"studentId":"stu-1","cycleId":"cyc-1","conceptId":"cpt-1","amountCentsOverride":150000,"descriptionOverride":"Colegiatura","dueDate":"2026-09-05"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.chargeStudent != "stu-1" || svc.chargeCycle != "cyc-1" || svc.chargeConcept != "cpt-1" ||
		svc.chargeAmount != 150000 || svc.chargeDescription != "Colegiatura" || svc.chargeDueDate != "2026-09-05" {
		t.Errorf("forwarded input = %q %q %q %d %q %q", svc.chargeStudent, svc.chargeCycle, svc.chargeConcept,
			svc.chargeAmount, svc.chargeDescription, svc.chargeDueDate)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// paidCents / remainingCents are derived server-side and surfaced as cents.
	if out["id"] != "chg-1" || out["amountCents"] != float64(150000) ||
		out["remainingCents"] != float64(150000) || out["status"] != "pending" {
		t.Errorf("body = %v", out)
	}
}

func TestFinanceCreateChargeUnknownStudentIs400(t *testing.T) {
	svc := &fakeFinance{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewFinanceHandler(svc)
	rec := doReq(t, h.CreateCharge, http.MethodPost, "/api/v1/finance/charges",
		`{"studentId":"ghost","cycleId":"cyc-1","conceptId":"cpt-1","dueDate":"2026-09-05"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestFinanceGenerateChargesReturnsCounts(t *testing.T) {
	svc := &fakeFinance{created: 24, skipped: 3}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.GenerateCharges, http.MethodPost, "/api/v1/finance/charges/generate",
		`{"conceptId":"cpt-1","cycleId":"cyc-1","groupId":"grp-1","amountCentsOverride":150000,"dueDate":"2026-09-05"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.chargeConcept != "cpt-1" || svc.chargeCycle != "cyc-1" || svc.genGroup != "grp-1" ||
		svc.chargeAmount != 150000 || svc.chargeDueDate != "2026-09-05" {
		t.Errorf("forwarded input = %q %q %q %d %q", svc.chargeConcept, svc.chargeCycle, svc.genGroup,
			svc.chargeAmount, svc.chargeDueDate)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// skipped counts the idempotent re-run: already-charged enrollments.
	if out["created"] != float64(24) || out["skipped"] != float64(3) {
		t.Errorf("body = %v", out)
	}
}

func TestFinanceGenerateChargesInvalidBodyIs400(t *testing.T) {
	h := handler.NewFinanceHandler(&fakeFinance{})
	rec := doReq(t, h.GenerateCharges, http.MethodPost, "/api/v1/finance/charges/generate", `{`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestFinanceListChargesForwardsFilters(t *testing.T) {
	svc := &fakeFinance{charges: []client.Charge{sampleCharge}}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.ListCharges, http.MethodGet, "/api/v1/finance/charges?studentId=stu-1&cycleId=cyc-1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.listStudent != "stu-1" || svc.listCycle != "cyc-1" {
		t.Errorf("forwarded filters = %q %q", svc.listStudent, svc.listCycle)
	}
	var out struct {
		Charges []map[string]any `json:"charges"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.Charges) != 1 || out.Charges[0]["amountCents"] != float64(150000) {
		t.Errorf("body = %v", out)
	}
}

// TestFinanceListChargesUnfilteredIs400 pins the edge's half of the unscoped-read
// blindaje: the handler forwards the two empty filters verbatim — it does not
// invent a scope — and the finance service's rejection of the whole-ledger read
// reaches the caller as 400, not as an empty 200 that looks like "no debts".
func TestFinanceListChargesUnfilteredIs400(t *testing.T) {
	svc := &fakeFinance{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.ListCharges, http.MethodGet, "/api/v1/finance/charges", "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if svc.listStudent != "" || svc.listCycle != "" {
		t.Errorf("forwarded filters = %q %q, want both empty", svc.listStudent, svc.listCycle)
	}
}

func TestFinanceApplyDiscountUsesPathID(t *testing.T) {
	discounted := sampleCharge
	discounted.DiscountCents = 50000
	discounted.RemainingCents = 100000
	svc := &fakeFinance{charge: discounted}
	h := handler.NewFinanceHandler(svc)

	rec := doAdminReq(t, http.HandlerFunc(h.ApplyDiscount), http.MethodPost, "/api/v1/finance/charges/chg-1/discount",
		`{"discountCents":50000,"reason":"beca 33%"}`, "id", "chg-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.discountChargeID != "chg-1" || svc.discountCents != 50000 || svc.discountReason != "beca 33%" {
		t.Errorf("forwarded input = %q %d %q", svc.discountChargeID, svc.discountCents, svc.discountReason)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["discountCents"] != float64(50000) || out["remainingCents"] != float64(100000) {
		t.Errorf("body = %v", out)
	}
}

// A discount above the charge amount is an InvalidArgument at the domain ⇒ 400.
func TestFinanceApplyDiscountAboveAmountIs400(t *testing.T) {
	svc := &fakeFinance{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewFinanceHandler(svc)
	rec := doAdminReq(t, http.HandlerFunc(h.ApplyDiscount), http.MethodPost, "/api/v1/finance/charges/chg-1/discount",
		`{"discountCents":999999999,"reason":"nope"}`, "id", "chg-1")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestFinanceCancelChargeUsesPathID(t *testing.T) {
	cancelled := sampleCharge
	cancelled.Status = "cancelled"
	cancelled.RemainingCents = 0
	svc := &fakeFinance{charge: cancelled}
	h := handler.NewFinanceHandler(svc)

	rec := doAdminReq(t, http.HandlerFunc(h.CancelCharge), http.MethodPost, "/api/v1/finance/charges/chg-1/cancel", "", "id", "chg-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.cancelChargeID != "chg-1" {
		t.Errorf("forwarded charge id = %q", svc.cancelChargeID)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["status"] != "cancelled" || out["remainingCents"] != float64(0) {
		t.Errorf("body = %v", out)
	}
}

func TestFinanceCancelChargeUnknownIs404(t *testing.T) {
	svc := &fakeFinance{err: connect.NewError(connect.CodeNotFound, nil)}
	h := handler.NewFinanceHandler(svc)
	rec := doAdminReq(t, http.HandlerFunc(h.CancelCharge), http.MethodPost, "/api/v1/finance/charges/ghost/cancel", "", "id", "ghost")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// --- payments + account ----------------------------------------------------

func TestFinanceRegisterPaymentReturnsPaymentAndCharge(t *testing.T) {
	paid := sampleCharge
	paid.PaidCents = 50000
	paid.RemainingCents = 100000
	paid.Status = "partially_paid"
	svc := &fakeFinance{
		payment: client.Payment{ID: "pay-1", TenantID: "t1", ChargeID: "chg-1", AmountCents: 50000,
			Method: "transfer", Reference: "SPEI-993", PaidAt: "2026-09-01T10:00:00Z"},
		charge: paid,
	}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.RegisterPayment, http.MethodPost, "/api/v1/finance/payments",
		`{"chargeId":"chg-1","amountCents":50000,"method":"transfer","reference":"SPEI-993"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.payChargeID != "chg-1" || svc.payAmount != 50000 || svc.payMethod != "transfer" || svc.payRef != "SPEI-993" {
		t.Errorf("forwarded input = %q %d %q %q", svc.payChargeID, svc.payAmount, svc.payMethod, svc.payRef)
	}
	var out struct {
		Payment map[string]any `json:"payment"`
		Charge  map[string]any `json:"charge"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// Money stays int64 cents on the wire — never a float amount.
	if out.Payment["amountCents"] != float64(50000) || out.Payment["method"] != "transfer" ||
		out.Payment["paidAt"] != "2026-09-01T10:00:00Z" {
		t.Errorf("payment = %v", out.Payment)
	}
	// The charge carries the recomputed paid/remaining/status.
	if out.Charge["paidCents"] != float64(50000) || out.Charge["remainingCents"] != float64(100000) ||
		out.Charge["status"] != "partially_paid" {
		t.Errorf("charge = %v", out.Charge)
	}
}

// The edge is where the browser's retry key enters the system; if the handler
// drops it, every layer below is deduplicating on an empty string and the whole
// feature is inert.
func TestFinanceRegisterPaymentForwardsTheIdempotencyKey(t *testing.T) {
	svc := &fakeFinance{payment: client.Payment{ID: "pay-1"}, charge: sampleCharge}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.RegisterPayment, http.MethodPost, "/api/v1/finance/payments",
		`{"chargeId":"chg-1","amountCents":50000,"method":"cash","reference":"","idempotencyKey":"attempt-1"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.payKey != "attempt-1" {
		t.Errorf("forwarded idempotency key = %q, want %q", svc.payKey, "attempt-1")
	}
}

// Overpaying a charge (or paying a cancelled one) is InvalidArgument ⇒ 400.
func TestFinanceRegisterPaymentOverpayIs400(t *testing.T) {
	svc := &fakeFinance{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewFinanceHandler(svc)
	rec := doReq(t, h.RegisterPayment, http.MethodPost, "/api/v1/finance/payments",
		`{"chargeId":"chg-1","amountCents":99999999,"method":"cash","reference":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestFinanceRegisterPaymentInvalidBodyIs400(t *testing.T) {
	h := handler.NewFinanceHandler(&fakeFinance{})
	rec := doReq(t, h.RegisterPayment, http.MethodPost, "/api/v1/finance/payments", `oops`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestFinanceGetAccountUsesPathStudentID(t *testing.T) {
	svc := &fakeFinance{account: client.AccountStatement{
		Charges: []client.Charge{sampleCharge},
		Payments: []client.Payment{{ID: "pay-1", ChargeID: "chg-1", AmountCents: 50000,
			Method: "cash", PaidAt: "2026-09-01T10:00:00Z"}},
		TotalBalanceCents: 100000,
	}}
	h := handler.NewFinanceHandler(svc)

	rec := doAdminReq(t, http.HandlerFunc(h.GetAccount), http.MethodGet, "/api/v1/finance/accounts/stu-1", "", "studentId", "stu-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.accountStudent != "stu-1" {
		t.Errorf("forwarded student id = %q", svc.accountStudent)
	}
	var out struct {
		Charges           []map[string]any `json:"charges"`
		Payments          []map[string]any `json:"payments"`
		TotalBalanceCents int64            `json:"totalBalanceCents"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.Charges) != 1 || len(out.Payments) != 1 || out.TotalBalanceCents != 100000 {
		t.Errorf("body = %+v", out)
	}
	if out.Payments[0]["amountCents"] != float64(50000) {
		t.Errorf("payment = %v", out.Payments[0])
	}
}

func TestFinanceGetAccountUnknownStudentIs404(t *testing.T) {
	svc := &fakeFinance{err: connect.NewError(connect.CodeNotFound, nil)}
	h := handler.NewFinanceHandler(svc)
	rec := doAdminReq(t, http.HandlerFunc(h.GetAccount), http.MethodGet, "/api/v1/finance/accounts/ghost", "", "studentId", "ghost")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestFinanceListOverdueForwardsCycleFilter(t *testing.T) {
	overdue := sampleCharge
	overdue.DueDate = "2026-08-05"
	svc := &fakeFinance{overdue: []client.Charge{overdue}}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.ListOverdue, http.MethodGet, "/api/v1/finance/overdue?cycleId=cyc-1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.overdueCycle != "cyc-1" {
		t.Errorf("forwarded cycle filter = %q", svc.overdueCycle)
	}
	var out struct {
		Overdue []map[string]any `json:"overdue"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.Overdue) != 1 || out.Overdue[0]["dueDate"] != "2026-08-05" ||
		out.Overdue[0]["remainingCents"] != float64(150000) {
		t.Errorf("body = %v", out)
	}
}

func TestFinanceListOverdueUpstreamFailureIs500(t *testing.T) {
	svc := &fakeFinance{err: connect.NewError(connect.CodeInternal, nil)}
	h := handler.NewFinanceHandler(svc)
	rec := doReq(t, h.ListOverdue, http.MethodGet, "/api/v1/finance/overdue", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}
