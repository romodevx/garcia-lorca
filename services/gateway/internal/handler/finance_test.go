package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// fakeFinance is a hand-written double for the finance surface the handler
// needs. It records the last input and returns canned results/errors so each
// handler is exercised in isolation (no Connect round-trip).
type fakeFinance struct {
	// T9: the correction asked for, and what the service answers.
	correction       client.CorrectionInput
	correctCalls     int
	correctionResult client.CorrectionResult
	history          []client.DepositCorrection
	historyOf        string

	depositChargeIDs []string
	depositReceiver  client.Receiver
	payReceiver      client.Receiver
	planIn           client.TuitionPlanInput
	plans            []client.TuitionPlan
	plansCycle       string
	run              client.BillingRun
	runCycle         string
	runAsOf          string

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
	listPage    client.PageRequest
	// nextPageToken is what both paged reads answer as the next page's token.
	nextPageToken string
	charges       []client.Charge

	discountChargeID string
	discountCents    int64
	discountReason   string

	cancelChargeID string

	payChargeID string
	payAmount   int64
	payMethod   string
	payRef      string
	payKey      string
	payPaidAt   time.Time
	payment     client.Payment
	payTenders  []client.Tender
	payments    []client.Payment

	accountStudent string
	account        client.AccountStatement

	overdueCycle string
	overduePage  client.PageRequest
	overdue      []client.Charge

	depositStudent  string
	depositAmount   int64
	depositMethod   string
	depositKey      string
	depositPaidAt   time.Time
	depositResult   client.DepositResult
	deposits        []client.Deposit
	expectedStudent string
	expectedOn      string
	expectedBalance bool
	expected        client.Expected
	depositGranted  bool
	depositCardType string
	depositTenders  []client.Tender
	incomeFrom      string
	incomeTo        string
	income          client.Income
	cashUpDay       string
	cashUp          client.CashUp
	voidedDeposit   string
	voidReason      string

	charge client.Charge
	err    error
}

func (f *fakeFinance) ExpectedToday(_ context.Context, studentID, on string, withBalance bool) (client.Expected, error) {
	f.expectedStudent, f.expectedOn, f.expectedBalance = studentID, on, withBalance
	return f.expected, f.err
}

func (f *fakeFinance) RegisterDeposit(_ context.Context, studentID string, amountCents int64, method, reference, idempotencyKey string, paidAt time.Time, grantEarlyPayment bool, cardType string, receiver client.Receiver, chargeIDs []string, tenders []client.Tender) (client.DepositResult, error) {
	f.depositCardType = cardType
	f.depositTenders = tenders
	f.depositChargeIDs = chargeIDs
	f.depositReceiver = receiver
	f.depositStudent, f.depositAmount, f.depositMethod = studentID, amountCents, method
	f.depositKey, f.depositPaidAt = idempotencyKey, paidAt
	f.depositGranted = grantEarlyPayment
	if f.err != nil {
		return client.DepositResult{}, f.err
	}
	return f.depositResult, nil
}

func (f *fakeFinance) PreviewDeposit(_ context.Context, studentID string, amountCents int64, paidAt time.Time, chargeIDs []string) (client.DepositResult, error) {
	f.depositStudent, f.depositAmount, f.depositPaidAt = studentID, amountCents, paidAt
	f.depositChargeIDs = chargeIDs
	return f.depositResult, f.err
}

func (f *fakeFinance) ListDeposits(_ context.Context, studentID string) ([]client.Deposit, error) {
	f.depositStudent = studentID
	return f.deposits, f.err
}

func (f *fakeFinance) DailyCashUp(_ context.Context, day string) (client.CashUp, error) {
	f.cashUpDay = day
	return f.cashUp, f.err
}

func (f *fakeFinance) IncomeByConcept(_ context.Context, from, to string) (client.Income, error) {
	f.incomeFrom, f.incomeTo = from, to
	return f.income, f.err
}

func (f *fakeFinance) CorrectDeposit(_ context.Context, in client.CorrectionInput) (client.CorrectionResult, error) {
	f.correction = in
	f.correctCalls++
	return f.correctionResult, f.err
}

func (f *fakeFinance) DepositCorrections(_ context.Context, id string) ([]client.DepositCorrection, error) {
	f.historyOf = id
	return f.history, f.err
}

func (f *fakeFinance) VoidDeposit(_ context.Context, id, reason string) error {
	f.voidedDeposit, f.voidReason = id, reason
	return f.err
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

func (f *fakeFinance) ListCharges(_ context.Context, studentID, cycleID string, page client.PageRequest) (client.ChargePage, error) {
	f.listStudent, f.listCycle, f.listPage = studentID, cycleID, page
	return client.ChargePage{Charges: f.charges, NextPageToken: f.nextPageToken}, f.err
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

func (f *fakeFinance) RegisterTenderedPayment(_ context.Context, in client.TenderedPayment) ([]client.Payment, client.Charge, error) {
	f.payChargeID, f.payAmount = in.ChargeID, in.AmountCents
	f.payTenders = in.Tenders
	if len(in.Tenders) > 0 {
		f.payMethod, f.payRef = in.Tenders[0].Method, in.Tenders[0].Reference
	}
	f.payReceiver = in.Receiver
	f.payKey, f.payPaidAt = in.IdempotencyKey, in.PaidAt
	if f.err != nil {
		return nil, client.Charge{}, f.err
	}
	if len(f.payments) > 0 {
		return f.payments, f.charge, nil
	}
	return []client.Payment{f.payment}, f.charge, nil
}

func (f *fakeFinance) GetAccount(_ context.Context, studentID string) (client.AccountStatement, error) {
	f.accountStudent = studentID
	return f.account, f.err
}

func (f *fakeFinance) ListOverdue(_ context.Context, cycleID string, page client.PageRequest) (client.ChargePage, error) {
	f.overdueCycle, f.overduePage = cycleID, page
	return client.ChargePage{Charges: f.overdue, NextPageToken: f.nextPageToken}, f.err
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
			Method: "cash", PaidAt: "2026-09-01T10:00:00Z", DepositID: "dep-1"}},
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
	// T9: which receipt each payment came from, for the correction drawer.
	if out.Payments[0]["depositId"] != "dep-1" {
		t.Errorf("payment deposit = %v", out.Payments[0])
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

// --- tuition (M15) ----------------------------------------------------------

func (f *fakeFinance) UpsertTuitionPlan(_ context.Context, in client.TuitionPlanInput) (client.TuitionPlan, error) {
	f.planIn = in
	if f.err != nil {
		return client.TuitionPlan{}, f.err
	}
	return client.TuitionPlan{
		ID: "tp-1", CycleID: in.CycleID, LevelID: in.LevelID,
		EnrollmentCents: in.EnrollmentCents, MonthlyCents: in.MonthlyCents,
		Months: in.Months, FirstBilledMonth: in.FirstBilledMonth,
		DueDay: in.DueDay, LateFeeCents: in.LateFeeCents, Active: in.Active,
	}, nil
}

func (f *fakeFinance) ListTuitionPlans(_ context.Context, cycleID string) ([]client.TuitionPlan, error) {
	f.plansCycle = cycleID
	if f.err != nil {
		return nil, f.err
	}
	return f.plans, nil
}

func (f *fakeFinance) RunBilling(_ context.Context, cycleID, asOfDate string) (client.BillingRun, error) {
	f.runCycle, f.runAsOf = cycleID, asOfDate
	if f.err != nil {
		return client.BillingRun{}, f.err
	}
	return f.run, nil
}

func TestFinanceUpsertTuitionPlanForwardsTheWholeTable(t *testing.T) {
	svc := &fakeFinance{}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.UpsertTuitionPlan, http.MethodPut, "/api/v1/finance/tuition-plans",
		`{"cycleId":"cyc-1","levelId":"lv-1","enrollmentCents":50000,"monthlyCents":320000,
		  "months":10,"firstBilledMonth":9,"dueDay":10,"lateFeeCents":15000,"active":true}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Every field, because a table that arrives with a field dropped silently
	// prices a level wrong for a year.
	got := svc.planIn
	if got.CycleID != "cyc-1" || got.LevelID != "lv-1" || got.EnrollmentCents != 50000 ||
		got.MonthlyCents != 320000 || got.Months != 10 || got.FirstBilledMonth != 9 ||
		got.DueDay != 10 || got.LateFeeCents != 15000 || !got.Active {
		t.Errorf("forwarded input = %+v", got)
	}
}

func TestFinanceRunBillingReportsWhatItDid(t *testing.T) {
	svc := &fakeFinance{run: client.BillingRun{
		TuitionCreated: 30, EnrollmentCreated: 10, LateFeesCreated: 4, Skipped: 6,
		LevelsWithoutPlan: []string{"Kínder"},
	}}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.RunBilling, http.MethodPost, "/api/v1/finance/billing-runs",
		`{"cycleId":"cyc-1","asOfDate":"2026-11-15"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.runCycle != "cyc-1" || svc.runAsOf != "2026-11-15" {
		t.Errorf("forwarded %q %q", svc.runCycle, svc.runAsOf)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// skipped is the one a person reads twice: on a second press it is the whole
	// total, and that is the proof nobody was charged again.
	if out["tuitionCreated"] != float64(30) || out["skipped"] != float64(6) {
		t.Errorf("body = %v", out)
	}
	levels, ok := out["levelsWithoutPlan"].([]any)
	if !ok || len(levels) != 1 || levels[0] != "Kínder" {
		t.Errorf("levels without a plan = %v; a level nobody priced must reach the screen", out["levelsWithoutPlan"])
	}
}

// --- deposits (M16) --------------------------------------------------------

// The counter sends an amount and a date, never a list of what it covers. The
// response has to carry the split, or the cashier cannot print a receipt
// without a second round trip while a parent waits.
func TestFinanceRegisterDepositReturns201AndTheSplit(t *testing.T) {
	svc := &fakeFinance{depositResult: client.DepositResult{
		Deposit:          client.Deposit{ID: "dep-1", AmountCents: 670000},
		UnallocatedCents: 0,
	}}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.RegisterDeposit, http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":670000,"method":"cash","paidAt":"2026-10-10","idempotencyKey":"k1"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.depositStudent != "stu-1" || svc.depositAmount != 670000 || svc.depositKey != "k1" {
		t.Errorf("forwarded input = %q %d %q", svc.depositStudent, svc.depositAmount, svc.depositKey)
	}
	if svc.depositPaidAt.Format("2006-01-02") != "2026-10-10" {
		t.Errorf("paidAt = %v, want 2026-10-10", svc.depositPaidAt)
	}
}

// A cashier keying "10/10/2026" must be told, not have the money silently
// stamped today — which would put it on the wrong side of a due date.
func TestFinanceRegisterDepositRejectsAMalformedDate(t *testing.T) {
	h := handler.NewFinanceHandler(&fakeFinance{})

	rec := doReq(t, h.RegisterDeposit, http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":100000,"method":"cash","paidAt":"10/10/2026"}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// A mistyped year (1999-12-31) is a well-formed date finance refuses as invalid
// input (the folio counter starts at 2000): the cashier gets a 400 with the
// reason, not a 500.
func TestFinanceRegisterDepositDatedBefore2000Is400(t *testing.T) {
	const reason = "la fecha del pago es anterior al año 2000; revisa el año"
	svc := &fakeFinance{err: connect.NewError(connect.CodeInvalidArgument, errString(reason))}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.RegisterDeposit, http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":100000,"method":"cash","paidAt":"1999-12-31","idempotencyKey":"k1"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "anterior al año 2000") {
		t.Errorf("body = %q, want the reason", rec.Body.String())
	}
	if svc.depositPaidAt.Format("2006-01-02") != "1999-12-31" {
		t.Errorf("paidAt forwarded = %v, want 1999-12-31", svc.depositPaidAt)
	}
}

// Cancelling without a reason leaves an unexplained hole in the day's takings,
// so the edge refuses it before the money service is ever asked.
func TestFinanceVoidDepositNeedsAReason(t *testing.T) {
	svc := &fakeFinance{}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.VoidDeposit, http.MethodPost, "/api/v1/finance/deposits/dep-1/void", `{"reason":"   "}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if svc.voidedDeposit != "" {
		t.Errorf("the service was asked to cancel %q despite the missing reason", svc.voidedDeposit)
	}
}

// A period with a missing or malformed bound is refused at the edge. A report
// over "everything" reads like a real number and is nobody's question.
func TestFinanceIncomeNeedsBothDates(t *testing.T) {
	svc := &fakeFinance{}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.IncomeByConcept, http.MethodGet, "/api/v1/finance/income?from=2026-10-01", "")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if svc.incomeFrom != "" {
		t.Error("the service was asked for an unbounded period")
	}
}

// The bug this pins: client.Payment and client.Charge carry no JSON tags, so
// writing them straight through put "AmountCents" on the wire where the rest of
// finance puts "amountCents". Everything compiled, every unit test passed, and
// the counter showed blank amounts. Only running it found it.
func TestFinanceDepositAllocationsUseTheSameKeysAsEveryOtherEndpoint(t *testing.T) {
	svc := &fakeFinance{depositResult: client.DepositResult{
		Deposit:     client.Deposit{ID: "dep-1", AmountCents: 670000},
		Allocations: []client.Payment{{ID: "p1", ChargeID: "ch1", AmountCents: 320000, Method: "cash"}},
		Surcharges:  []client.Charge{{ID: "ch2", Description: "Recargo", AmountCents: 30000}},
	}}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.RegisterDeposit, http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":670000,"method":"cash"}`)

	var body struct {
		Allocations []map[string]any `json:"allocations"`
		Surcharges  []map[string]any `json:"surcharges"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Allocations) != 1 {
		t.Fatalf("allocations = %d, want 1", len(body.Allocations))
	}
	if body.Allocations[0]["amountCents"] != float64(320000) {
		t.Errorf("allocation keys are wrong: %v", body.Allocations[0])
	}
	if body.Allocations[0]["chargeId"] != "ch1" {
		t.Errorf("chargeId missing from the allocation: %v", body.Allocations[0])
	}
	if len(body.Surcharges) != 1 || body.Surcharges[0]["amountCents"] != float64(30000) {
		t.Errorf("surcharge keys are wrong: %v", body.Surcharges)
	}
}

// Same for the preview, which the counter calls on every keystroke.
func TestFinancePreviewUsesTheSameKeys(t *testing.T) {
	svc := &fakeFinance{depositResult: client.DepositResult{
		Allocations: []client.Payment{{ID: "p1", ChargeID: "ch1", AmountCents: 320000, Method: "cash"}},
	}}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.PreviewDeposit, http.MethodPost, "/api/v1/finance/deposits/preview",
		`{"studentId":"stu-1","amountCents":670000}`)

	var body struct {
		Allocations []map[string]any `json:"allocations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Allocations) != 1 || body.Allocations[0]["chargeId"] != "ch1" {
		t.Errorf("preview allocation keys are wrong: %v", body.Allocations)
	}
}

// Task 7: who took the money is the signed-in caller, named from iam — never a
// field of the request body.
func TestFinanceRecordsWhoTookTheMoney(t *testing.T) {
	svc := &fakeFinance{depositResult: client.DepositResult{Deposit: client.Deposit{ID: "dep-1"}}}
	h := handler.NewFinanceHandler(svc, handler.WithNames(fakeNamer{user: client.User{DisplayName: "Laura Caja"}}))
	want := client.Receiver{UserID: "u-me", Name: "Laura Caja"}

	h.RegisterDeposit(httptest.NewRecorder(), notesRequest(http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":1000,"method":"cash","receivedBy":"u-other","receivedByName":"Otra"}`, "payments:write"))
	if svc.depositReceiver != want {
		t.Errorf("deposit receiver = %+v, want %+v", svc.depositReceiver, want)
	}

	// A single-charge payment stores the account alone: no name is kept for
	// it, so iam is not asked (a round trip on the collection path for nothing).
	names := &countingNamer{}
	h = handler.NewFinanceHandler(svc, handler.WithNames(names))
	h.RegisterPayment(httptest.NewRecorder(), notesRequest(http.MethodPost, "/api/v1/finance/payments",
		`{"chargeId":"ch-1","amountCents":1000,"method":"cash"}`, "finance:write"))
	if svc.payReceiver != (client.Receiver{UserID: "u-me"}) || names.calls != 0 {
		t.Errorf("payment receiver = %+v after %d name lookups, want the account alone and none", svc.payReceiver, names.calls)
	}
}

type countingNamer struct{ calls int }

func (c *countingNamer) GetUser(context.Context, string) (client.User, error) {
	c.calls++
	return client.User{}, nil
}

// Task 8: the cashier's choice of charges reaches finance, on the deposit and
// on its preview; the quote answers the debt concept by concept.
func TestFinanceForwardsTheChosenCharges(t *testing.T) {
	svc := &fakeFinance{depositResult: client.DepositResult{Deposit: client.Deposit{ID: "dep-1"}}}
	h := handler.NewFinanceHandler(svc)

	doReq(t, h.RegisterDeposit, http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":1000,"method":"cash","chargeIds":["61000000-0000-0000-0000-00000000000a","61000000-0000-0000-0000-00000000000b"]}`)
	if strings.Join(svc.depositChargeIDs, ",") != "61000000-0000-0000-0000-00000000000a,61000000-0000-0000-0000-00000000000b" {
		t.Errorf("deposit chargeIds = %v", svc.depositChargeIDs)
	}
	svc.depositChargeIDs = nil
	doReq(t, h.PreviewDeposit, http.MethodPost, "/api/v1/finance/deposits/preview",
		`{"studentId":"stu-1","amountCents":1000,"chargeIds":["61000000-0000-0000-0000-00000000000a"]}`)
	if strings.Join(svc.depositChargeIDs, ",") != "61000000-0000-0000-0000-00000000000a" {
		t.Errorf("preview chargeIds = %v", svc.depositChargeIDs)
	}
}

func TestFinanceExpectedAnswersItsLines(t *testing.T) {
	svc := &fakeFinance{expected: client.Expected{AmountCents: 350000, Lines: []client.ExpectedLine{{
		ChargeID: "ch-sep", Description: "Colegiatura septiembre", DueDate: "2026-09-10",
		OwedCents: 320000, SurchargeCents: 30000, TotalCents: 350000, Due: true,
	}}}}
	h := handler.NewFinanceHandler(svc)
	rec := doReq(t, h.ExpectedToday, http.MethodGet, "/api/v1/finance/expected?studentId=stu-1", "")
	body := rec.Body.String()
	for _, want := range []string{`"lines"`, `"chargeId":"ch-sep"`, `"surchargeCents":30000`, `"totalCents":350000`, `"due":true`, `"dueDate":"2026-09-10"`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body lacks %s: %s", want, body)
		}
	}
}

// D1 (T5 review): the balance it holds travels in the JSON, and the search
// may leave the balance out with withBalance=false (default: included).
func TestFinanceExpectedBalanceIsOptional(t *testing.T) {
	svc := &fakeFinance{expected: client.Expected{OnTimeBalanceCents: 300000, Lines: []client.ExpectedLine{{
		ChargeID: "ch-sep", InBalance: true,
	}}}}
	h := handler.NewFinanceHandler(svc)

	rec := doReq(t, h.ExpectedToday, http.MethodGet, "/api/v1/finance/expected?studentId=stu-1", "")
	body := rec.Body.String()
	for _, want := range []string{`"onTimeBalanceCents":300000`, `"inBalance":true`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body lacks %s: %s", want, body)
		}
	}
	if !svc.expectedBalance {
		t.Error("the balance is included by default")
	}

	doReq(t, h.ExpectedToday, http.MethodGet, "/api/v1/finance/expected?studentId=stu-1&withBalance=false", "")
	if svc.expectedBalance {
		t.Error("withBalance=false still asked for the balance")
	}

	if rec := doReq(t, h.ExpectedToday, http.MethodGet, "/api/v1/finance/expected?studentId=stu-1&withBalance=maybe", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("withBalance=maybe: status %d, want 400", rec.Code)
	}
}

// chargeIds is validated at the edge, against the contract: uuids, and a
// bounded list — never forwarded as whatever a client sends.
func TestFinanceRefusesChosenChargesThatAreNotCharges(t *testing.T) {
	svc := &fakeFinance{}
	h := handler.NewFinanceHandler(svc)
	many := make([]string, 501)
	for i := range many {
		many[i] = fmt.Sprintf(`"00000000-0000-0000-0000-%012d"`, i)
	}
	cases := map[string]string{
		"not a uuid": `{"studentId":"stu-1","amountCents":1000,"method":"cash","chargeIds":["abc"]}`,
		"too many":   `{"studentId":"stu-1","amountCents":1000,"method":"cash","chargeIds":[` + strings.Join(many, ",") + `]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			svc.depositChargeIDs = nil
			if rec := doReq(t, h.RegisterDeposit, http.MethodPost, "/api/v1/finance/deposits", body); rec.Code != http.StatusBadRequest {
				t.Errorf("deposit: status %d, want 400", rec.Code)
			}
			if rec := doReq(t, h.PreviewDeposit, http.MethodPost, "/api/v1/finance/deposits/preview", body); rec.Code != http.StatusBadRequest {
				t.Errorf("preview: status %d, want 400", rec.Code)
			}
			if svc.depositChargeIDs != nil {
				t.Error("an invalid choice reached finance")
			}
		})
	}
}

// Task 9: granting the early-payment reward after its window is a discount, and
// discounts stay finance:write's (ADR 0015). A cashier's deposit that asks for
// it is refused before finance is called; the owner's goes through.
func TestFinanceOnlyFinanceWriteGrantsTheLateReward(t *testing.T) {
	body := `{"studentId":"stu-1","amountCents":1000,"method":"cash","grantEarlyPayment":true}`

	svc := &fakeFinance{}
	h := handler.NewFinanceHandler(svc)
	rec := httptest.NewRecorder()
	h.RegisterDeposit(rec, notesRequest(http.MethodPost, "/api/v1/finance/deposits", body, "payments:write", "collections:read"))
	if rec.Code != http.StatusForbidden || svc.depositStudent != "" {
		t.Errorf("cashier: status %d, finance called with %q; want 403 and no call", rec.Code, svc.depositStudent)
	}

	svc = &fakeFinance{depositResult: client.DepositResult{Deposit: client.Deposit{ID: "dep-1"}}}
	h = handler.NewFinanceHandler(svc)
	rec = httptest.NewRecorder()
	h.RegisterDeposit(rec, notesRequest(http.MethodPost, "/api/v1/finance/deposits", body, "finance:write"))
	if rec.Code != http.StatusCreated || !svc.depositGranted {
		t.Errorf("owner: status %d, granted %v; want 201 and the reward granted", rec.Code, svc.depositGranted)
	}
}
