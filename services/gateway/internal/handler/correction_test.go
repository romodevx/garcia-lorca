package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

const correctedDeposit = "62000000-0000-0000-0000-000000000001"

func correctionRequest(body string) *http.Request {
	r := notesRequest(http.MethodPost, "/api/v1/finance/deposits/"+correctedDeposit+"/correct", body, "payments:edit")
	r.SetPathValue("id", correctedDeposit)
	return r
}

// T9: the edge forwards what the office typed, and stamps who corrected it
// from the session — never from the body.
func TestCorrectDepositForwardsTheCorrectionAndTheCaller(t *testing.T) {
	svc := &fakeFinance{correctionResult: client.CorrectionResult{
		Original:   client.Deposit{ID: correctedDeposit, SupersededByFolio: "P-2026-000002"},
		Deposit:    client.Deposit{ID: "dep-2", CorrectsFolio: "P-2026-000001"},
		Correction: client.DepositCorrection{Reason: "monto"},
	}}
	h := handler.NewFinanceHandler(svc, handler.WithNames(fakeNamer{user: client.User{DisplayName: "Directora Ruiz"}}))
	rec := httptest.NewRecorder()

	h.CorrectDeposit(rec, correctionRequest(`{"amountCents":100000,"paidAt":"2026-09-09","reason":"monto",
		"tenders":[{"method":"cash","amountCents":100000}],"chargeIds":["61000000-0000-0000-0000-000000000001"],
		"idempotencyKey":"k-1","correctedBy":"u-other","correctedByName":"Otra"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	in := svc.correction
	if in.DepositID != correctedDeposit || in.AmountCents != 100000 || in.PaidAt.Format("2006-01-02") != "2026-09-09" ||
		in.Reason != "monto" || in.IdempotencyKey != "k-1" || len(in.Tenders) != 1 || len(in.ChargeIDs) != 1 {
		t.Errorf("forwarded = %+v", in)
	}
	if in.By != (client.Receiver{UserID: "u-me", Name: "Directora Ruiz"}) {
		t.Errorf("corrected by = %+v, want the signed-in caller", in.By)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["deposit"].(map[string]any)["correctsFolio"] != "P-2026-000001" || body["original"].(map[string]any)["supersededByFolio"] != "P-2026-000002" {
		t.Errorf("body = %v", body)
	}
}

// What the contract refuses never reaches finance.
func TestCorrectDepositRefusesAtTheEdge(t *testing.T) {
	cases := map[string]string{
		"no reason":           `{"amountCents":100,"reason":"  ","tenders":[{"method":"cash","amountCents":100}]}`,
		"reason too long":     `{"amountCents":100,"reason":"` + strings.Repeat("a", 501) + `","tenders":[{"method":"cash","amountCents":100}]}`,
		"no tenders":          `{"amountCents":100,"reason":"x"}`,
		"bad tender":          `{"amountCents":100,"reason":"x","tenders":[{"method":"cheque","amountCents":100}]}`,
		"no amount":           `{"amountCents":0,"reason":"x","tenders":[{"method":"cash","amountCents":100}]}`,
		"malformed date":      `{"amountCents":100,"paidAt":"09/09/2026","reason":"x","tenders":[{"method":"cash","amountCents":100}]}`,
		"a charge that isn't": `{"amountCents":100,"reason":"x","tenders":[{"method":"cash","amountCents":100}],"chargeIds":["x"]}`,
		// CC-205: the bank's cut separates debit from credit.
		"a card with no type": `{"amountCents":100,"reason":"x","tenders":[{"method":"card","amountCents":100}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeFinance{}
			rec := httptest.NewRecorder()
			handler.NewFinanceHandler(svc).CorrectDeposit(rec, correctionRequest(body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if svc.correctCalls != 0 {
				t.Error("finance was asked anyway")
			}
		})
	}
}

func TestCorrectDepositOfAMalformedIdIsNotFound(t *testing.T) {
	svc := &fakeFinance{}
	r := correctionRequest(`{"amountCents":100,"reason":"x","tenders":[{"method":"cash","amountCents":100}]}`)
	r.SetPathValue("id", "../x")
	rec := httptest.NewRecorder()
	handler.NewFinanceHandler(svc).CorrectDeposit(rec, r)
	if rec.Code != http.StatusNotFound || svc.correctCalls != 0 {
		t.Errorf("status = %d, calls = %d; want 404 and none", rec.Code, svc.correctCalls)
	}
}

func TestDepositCorrectionsListsTheHistory(t *testing.T) {
	svc := &fakeFinance{history: []client.DepositCorrection{{ID: "c-1", Reason: "monto", CorrectedByName: "Directora"}}}
	r := correctionRequest("")
	rec := httptest.NewRecorder()
	handler.NewFinanceHandler(svc).DepositCorrections(rec, r)
	if rec.Code != http.StatusOK || svc.historyOf != correctedDeposit {
		t.Fatalf("status = %d, asked about %q", rec.Code, svc.historyOf)
	}
	var body struct {
		Corrections []client.DepositCorrection `json:"corrections"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Corrections) != 1 || body.Corrections[0].Reason != "monto" {
		t.Errorf("body = %s (%v)", rec.Body.String(), err)
	}
}
