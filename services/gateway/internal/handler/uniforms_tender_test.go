package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// T8: a counter sale paid with a credit note and cash carries its tenders to
// the shop, which checks them against the total it prices.
func TestCounterSaleCarriesTenders(t *testing.T) {
	fake := &fakeUniforms{order: client.UniformOrder{ID: "ord-1"}}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	h.CounterSale(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/counter-sale",
		`{"lines":[{"itemId":"it-1","sizeLabel":"8","qty":1}],"tenders":[{"method":"credit_note","amountCents":20000,"reference":"NC-12"},{"method":"cash","amountCents":12000}]}`,
		map[string]string{"Idempotency-Key": "till-t8"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	got := fake.lastSale.Tenders
	if len(got) != 2 || got[0] != (client.Tender{Method: "credit_note", AmountCents: 20000, Reference: "NC-12"}) ||
		got[1] != (client.Tender{Method: "cash", AmountCents: 12000}) {
		t.Errorf("tenders = %+v", got)
	}

	fake.lastSale = client.CounterSaleInput{}
	rec = httptest.NewRecorder()
	h.CounterSale(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/counter-sale",
		`{"lines":[{"itemId":"it-1","sizeLabel":"8","qty":1}],"tenders":[{"method":"vale","amountCents":1}]}`,
		map[string]string{"Idempotency-Key": "till-t8b"}))
	if rec.Code != http.StatusBadRequest || fake.lastSale.IdempotencyKey != "" {
		t.Errorf("an invalid tender: status %d, reached the shop: %v", rec.Code, fake.lastSale.IdempotencyKey != "")
	}
}
