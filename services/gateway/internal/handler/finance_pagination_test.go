package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type pagedRoute struct {
	name    string
	target  string
	listKey string
	serve   func(h *handler.FinanceHandler) http.HandlerFunc
	sent    func(f *fakeFinance) client.PageRequest
}

var pagedRoutes = []pagedRoute{
	{
		name: "charges", target: "/api/v1/finance/charges?cycleId=cyc-1", listKey: "charges",
		serve: func(h *handler.FinanceHandler) http.HandlerFunc { return h.ListCharges },
		sent:  func(f *fakeFinance) client.PageRequest { return f.listPage },
	},
	{
		name: "overdue", target: "/api/v1/finance/overdue?cycleId=cyc-1", listKey: "overdue",
		serve: func(h *handler.FinanceHandler) http.HandlerFunc { return h.ListOverdue },
		sent:  func(f *fakeFinance) client.PageRequest { return f.overduePage },
	},
}

// T14: the edge forwards the cursor and the page size, and answers the token of
// the next page.
func TestPagedFinanceReadsForwardTheCursor(t *testing.T) {
	for _, route := range pagedRoutes {
		t.Run(route.name, func(t *testing.T) {
			svc := &fakeFinance{
				charges: []client.Charge{sampleCharge}, overdue: []client.Charge{sampleCharge},
				nextPageToken: "tok-2",
			}
			h := handler.NewFinanceHandler(svc)

			rec := doReq(t, route.serve(h), http.MethodGet, route.target+"&pageToken=tok-1&pageSize=200", "")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
			}
			if got := route.sent(svc); got != (client.PageRequest{Token: "tok-1", Size: 200}) {
				t.Errorf("forwarded page = %+v, want tok-1 / 200", got)
			}
			body := decodeObject(t, rec)
			if body["nextPageToken"] != "tok-2" {
				t.Errorf("nextPageToken = %v, want tok-2", body["nextPageToken"])
			}
			if rows, _ := body[route.listKey].([]any); len(rows) != 1 {
				t.Errorf("%s = %v, want one row", route.listKey, body[route.listKey])
			}
		})
	}
}

// The last page says so by leaving the token out; no page size asked means the
// service's default (0 on the wire).
func TestPagedFinanceReadsOmitTheTokenOnTheLastPage(t *testing.T) {
	for _, route := range pagedRoutes {
		t.Run(route.name, func(t *testing.T) {
			svc := &fakeFinance{charges: []client.Charge{sampleCharge}, overdue: []client.Charge{sampleCharge}}
			h := handler.NewFinanceHandler(svc)

			rec := doReq(t, route.serve(h), http.MethodGet, route.target, "")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := route.sent(svc); got != (client.PageRequest{}) {
				t.Errorf("forwarded page = %+v, want the zero page (service default)", got)
			}
			if _, ok := decodeObject(t, rec)["nextPageToken"]; ok {
				t.Error("nextPageToken is present on the last page, want it omitted")
			}
		})
	}
}

// pageSize is validated at the boundary: 1..500 or a 400 in the envelope, and
// finance is never asked.
func TestPagedFinanceReadsRejectABadPageSize(t *testing.T) {
	for _, route := range pagedRoutes {
		for _, size := range []string{"0", "501", "-1", "abc", "1.5", "", "99999999999"} {
			t.Run(route.name+"/"+size, func(t *testing.T) {
				svc := &fakeFinance{}
				h := handler.NewFinanceHandler(svc)

				rec := doReq(t, route.serve(h), http.MethodGet, route.target+"&pageSize="+size, "")

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400", rec.Code)
				}
				var envelope struct {
					Error struct {
						Code    string            `json:"code"`
						Details map[string]string `json:"details"`
					} `json:"error"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
					t.Fatalf("decode envelope: %v", err)
				}
				if envelope.Error.Code != "invalid" || envelope.Error.Details["field"] != "pageSize" {
					t.Errorf("envelope = %+v, want code invalid on field pageSize", envelope.Error)
				}
				if svc.listCycle != "" || svc.overdueCycle != "" {
					t.Error("finance was called with an invalid page size")
				}
			})
		}
	}
}

func decodeObject(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}
