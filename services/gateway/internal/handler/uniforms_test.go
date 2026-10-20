package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// fakeUniforms records what the handler asked for and answers what the test
// wants, so each route is exercised without a Connect round-trip.
type fakeUniforms struct {
	items    []client.UniformItem
	stock    []client.UniformSize
	order    client.UniformOrder
	page     client.UniformOrdersPage
	settings client.UniformSettings
	err      error

	lastItemsFilter  client.ListItemsFilter
	lastOrdersFilter client.ListOrdersFilter
	lastSale         client.CounterSaleInput
	lastReturn       client.ReturnSaleInput
	lastAdjustActor  string
	lastReceiveActor string
	lastSizes        []client.UniformSizeSpec
}

func (f *fakeUniforms) ListItems(_ context.Context, filter client.ListItemsFilter) ([]client.UniformItem, error) {
	f.lastItemsFilter = filter
	return f.items, f.err
}

func (f *fakeUniforms) CreateItem(context.Context, client.CreateItemInput) (client.UniformItem, error) {
	if f.err != nil {
		return client.UniformItem{}, f.err
	}
	return client.UniformItem{ID: "it-1", Name: "Playera"}, nil
}

func (f *fakeUniforms) UpdateItem(context.Context, client.UpdateItemInput) (client.UniformItem, error) {
	return client.UniformItem{ID: "it-1"}, f.err
}

func (f *fakeUniforms) UpsertSizes(_ context.Context, _ string, specs []client.UniformSizeSpec) (client.UpsertSizesResult, error) {
	f.lastSizes = specs
	return client.UpsertSizesResult{CreatedCount: int32(len(specs))}, f.err
}

func (f *fakeUniforms) ListStock(context.Context, string, bool) ([]client.UniformSize, error) {
	return f.stock, f.err
}

func (f *fakeUniforms) ReceiveStock(_ context.Context, _ []client.UniformReceiveLine, _, actor string) (client.ReceiveStockResult, error) {
	f.lastReceiveActor = actor
	return client.ReceiveStockResult{Stock: f.stock}, f.err
}

func (f *fakeUniforms) AdjustStock(_ context.Context, _ string, _ int32, _, actor string) (client.UniformSize, error) {
	f.lastAdjustActor = actor
	return client.UniformSize{ID: "st-1"}, f.err
}

func (f *fakeUniforms) CounterSale(_ context.Context, in client.CounterSaleInput) (client.UniformOrder, error) {
	f.lastSale = in
	return f.order, f.err
}

func (f *fakeUniforms) ReturnCounterSale(_ context.Context, in client.ReturnSaleInput) (client.UniformOrder, error) {
	f.lastReturn = in
	return f.order, f.err
}

func (f *fakeUniforms) ListOrders(_ context.Context, filter client.ListOrdersFilter) (client.UniformOrdersPage, error) {
	f.lastOrdersFilter = filter
	return f.page, f.err
}

func (f *fakeUniforms) GetOrder(context.Context, string, []string) (client.UniformOrder, error) {
	return f.order, f.err
}

func (f *fakeUniforms) GetSettings(context.Context) (client.UniformSettings, error) {
	return f.settings, f.err
}

func (f *fakeUniforms) UpdateSettings(_ context.Context, in client.UniformSettings) (client.UniformSettings, error) {
	return in, f.err
}

func uniformsRequest(method, target, body string, headers map[string]string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r.WithContext(middleware.WithPrincipal(r.Context(), middleware.Principal{UserID: "usr-7"}))
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details"`
} {
	t.Helper()
	var body struct {
		Error struct {
			Code    string            `json:"code"`
			Message string            `json:"message"`
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response is not the shared envelope: %v (%s)", err, rec.Body.String())
	}
	return body.Error
}

// --- the counter sale --------------------------------------------------------

// TestCounterSaleRequiresTheIdempotencyHeader rejects at the EDGE, naming the
// header. Letting it through would surface as a generic invalid-argument from two
// services away, and a cashier reading that has no idea what to do.
func TestCounterSaleRequiresTheIdempotencyHeader(t *testing.T) {
	h := handler.NewUniformsHandler(&fakeUniforms{})
	rec := httptest.NewRecorder()
	h.CounterSale(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/counter-sale",
		`{"lines":[{"itemId":"it-1","sizeLabel":"8","qty":1}],"method":"cash"}`, nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	e := decodeError(t, rec)
	if e.Details["header"] != "Idempotency-Key" {
		t.Errorf("the refusal does not name the header: %+v", e)
	}
}

// TestCounterSalePassesTheKeyAndTheActor: the key makes the sale idempotent, and
// the actor is who the stock ledger records. An adjustment without an actor is an
// adjustment nobody has to explain.
func TestCounterSalePassesTheKeyAndTheActor(t *testing.T) {
	fake := &fakeUniforms{order: client.UniformOrder{ID: "ord-1", Folio: "U-2026-000001"}}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	h.CounterSale(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/counter-sale",
		`{"lines":[{"itemId":"it-1","sizeLabel":"8","qty":2}],"method":"cash"}`,
		map[string]string{"Idempotency-Key": "till-abc"}))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if fake.lastSale.IdempotencyKey != "till-abc" {
		t.Errorf("key = %q", fake.lastSale.IdempotencyKey)
	}
	if fake.lastSale.ActorUserID != "usr-7" {
		t.Errorf("actor = %q; the ledger needs to know who sold it", fake.lastSale.ActorUserID)
	}
}

// TestCounterSaleBodyCarriesNoPrice is a shape assertion, and the shape IS the
// enforcement: there is nowhere in the request for a client to put an amount, so
// a crafted body cannot buy a jumper for one peso.
func TestCounterSaleBodyCarriesNoPrice(t *testing.T) {
	fake := &fakeUniforms{order: client.UniformOrder{ID: "ord-1"}}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	h.CounterSale(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/counter-sale",
		`{"lines":[{"itemId":"it-1","sizeLabel":"8","qty":1,"unitPriceCents":1,"priceCents":1}],"method":"cash"}`,
		map[string]string{"Idempotency-Key": "till-abc"}))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	// The extra fields were parsed into nothing; the line carries only what it
	// is allowed to carry.
	line := fake.lastSale.Lines[0]
	if line.ItemID != "it-1" || line.SizeLabel != "8" || line.Qty != 1 {
		t.Errorf("line = %+v", line)
	}
}

// --- error translation -------------------------------------------------------

// TestOutOfStockGetsItsOwnCodeAndSentence is the copy the design review asked
// for. An empty shelf is not the cashier's fault, and it must not read like it.
func TestOutOfStockGetsItsOwnCodeAndSentence(t *testing.T) {
	fake := &fakeUniforms{err: connect.NewError(connect.CodeFailedPrecondition,
		errors.New("there are not enough of Suéter verde in size M"))}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	h.CounterSale(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/counter-sale",
		`{"lines":[],"method":"cash"}`, map[string]string{"Idempotency-Key": "k"}))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	e := decodeError(t, rec)
	if e.Code != "insufficient_stock" {
		t.Errorf("code = %q; the panel branches on this and must not confuse it with a duplicate", e.Code)
	}
	if !strings.Contains(e.Message, "Esa talla ya no está disponible") {
		t.Errorf("message = %q", e.Message)
	}
	// The specific garment rides along, because "which one" is what the cashier
	// actually needs and the generic sentence cannot say it.
	if !strings.Contains(e.Details["reason"], "Suéter verde") {
		t.Errorf("details = %v; the cashier needs to know WHICH garment", e.Details)
	}
}

func TestErrorCodesMapToStatusAndCode(t *testing.T) {
	cases := []struct {
		connectCode connect.Code
		wantStatus  int
		wantCode    string
	}{
		{connect.CodeInvalidArgument, http.StatusBadRequest, "invalid"},
		{connect.CodeNotFound, http.StatusNotFound, "not_found"},
		{connect.CodeAlreadyExists, http.StatusConflict, "conflict"},
		{connect.CodeFailedPrecondition, http.StatusConflict, "insufficient_stock"},
		{connect.CodePermissionDenied, http.StatusForbidden, "forbidden"},
		{connect.CodeUnauthenticated, http.StatusUnauthorized, "unauthorized"},
		{connect.CodeUnimplemented, http.StatusNotImplemented, "not_implemented"},
		{connect.CodeUnavailable, http.StatusServiceUnavailable, "upstream_unavailable"},
		{connect.CodeInternal, http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.wantCode, func(t *testing.T) {
			h := handler.NewUniformsHandler(&fakeUniforms{
				err: connect.NewError(tc.connectCode, errors.New("boom")),
			})
			rec := httptest.NewRecorder()
			h.ListItems(rec, uniformsRequest(http.MethodGet, "/api/v1/uniforms/items", "", nil))
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := decodeError(t, rec).Code; got != tc.wantCode {
				t.Errorf("code = %q, want %q", got, tc.wantCode)
			}
		})
	}
}

// TestInternalErrorsSayNothingSpecific: an unmapped failure gets a sentence a
// person can read and no detail at all — not even the service's own message,
// since an unmapped code means we do not know what it was carrying.
func TestInternalErrorsSayNothingSpecific(t *testing.T) {
	h := handler.NewUniformsHandler(&fakeUniforms{
		err: connect.NewError(connect.CodeInternal, errors.New("pq: relation uniform_stock does not exist")),
	})
	rec := httptest.NewRecorder()
	h.ListItems(rec, uniformsRequest(http.MethodGet, "/api/v1/uniforms/items", "", nil))

	e := decodeError(t, rec)
	if e.Details != nil {
		t.Errorf("an unmapped failure carried details: %v", e.Details)
	}
	if strings.Contains(rec.Body.String(), "uniform_stock") {
		t.Errorf("a table name reached the caller: %s", rec.Body.String())
	}
}

func TestMalformedBodyIsTheSharedEnvelope(t *testing.T) {
	h := handler.NewUniformsHandler(&fakeUniforms{})
	rec := httptest.NewRecorder()
	h.CreateItem(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/items", `{not json`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if decodeError(t, rec).Code != "invalid" {
		t.Errorf("code = %q", decodeError(t, rec).Code)
	}
}

// --- reads -------------------------------------------------------------------

// TestListOrdersDefaultsToNewestFirst: the panel is a work queue, and the work
// that matters arrived most recently.
func TestListOrdersDefaultsToNewestFirst(t *testing.T) {
	fake := &fakeUniforms{}
	h := handler.NewUniformsHandler(fake)

	rec := httptest.NewRecorder()
	h.ListOrders(rec, uniformsRequest(http.MethodGet, "/api/v1/uniforms/orders", "", nil))
	if !fake.lastOrdersFilter.Descending {
		t.Error("the default order is not newest first")
	}

	rec = httptest.NewRecorder()
	h.ListOrders(rec, uniformsRequest(http.MethodGet, "/api/v1/uniforms/orders?desc=false", "", nil))
	if fake.lastOrdersFilter.Descending {
		t.Error("desc=false was ignored")
	}
}

// TestListOrdersPassesEveryFilter guards against a filter being silently dropped
// on the way through — the kind of bug that shows up as a panel whose level tabs
// do nothing.
func TestListOrdersPassesEveryFilter(t *testing.T) {
	fake := &fakeUniforms{}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	h.ListOrders(rec, uniformsRequest(http.MethodGet,
		"/api/v1/uniforms/orders?levelId=lvl-primaria&q=Zoe&status=ready_for_pickup&status=paid"+
			"&requiresAttention=true&from=2026-09-01&to=2026-09-30&sort=deadline&pageSize=50&pageToken=20",
		"", nil))

	got := fake.lastOrdersFilter
	if got.LevelID != "lvl-primaria" || got.Query != "Zoe" || got.Sort != "deadline" {
		t.Errorf("filter = %+v", got)
	}
	if len(got.Statuses) != 2 {
		t.Errorf("statuses = %v; repeated params must all arrive", got.Statuses)
	}
	if !got.OnlyRequiresAttention {
		t.Error("requiresAttention was dropped")
	}
	if got.From != "2026-09-01" || got.To != "2026-09-30" {
		t.Errorf("dates = %q..%q", got.From, got.To)
	}
	if got.PageSize != 50 || got.PageToken != "20" {
		t.Errorf("paging = %d / %q", got.PageSize, got.PageToken)
	}
}

// TestListItemsLevelFilterReachesTheService is the family-facing narrowing: a
// pupil sees the garments for their level plus the ones for every level.
func TestListItemsLevelFilterReachesTheService(t *testing.T) {
	fake := &fakeUniforms{}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	h.ListItems(rec, uniformsRequest(http.MethodGet,
		"/api/v1/uniforms/items?levelId=lvl-kinder&includeInactive=true", "", nil))

	if fake.lastItemsFilter.LevelID != "lvl-kinder" || !fake.lastItemsFilter.IncludeInactive {
		t.Errorf("filter = %+v", fake.lastItemsFilter)
	}
}

// TestAdjustStockRecordsWhoDidIt: this is the one write that can conceal a
// shortfall, so the ledger has to name a person.
func TestAdjustStockRecordsWhoDidIt(t *testing.T) {
	fake := &fakeUniforms{}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	h.AdjustStock(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/stock/adjust",
		`{"stockId":"st-1","qtyDelta":-2,"note":"dos piezas dañadas"}`, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if fake.lastAdjustActor != "usr-7" {
		t.Errorf("actor = %q, want usr-7", fake.lastAdjustActor)
	}
}

func TestUpsertSizesPassesTheWholeRun(t *testing.T) {
	fake := &fakeUniforms{}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	h.UpsertSizes(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/items/it-1/sizes",
		`{"sizes":[{"sizeLabel":"4","sizeOrder":1},{"sizeLabel":"6","sizeOrder":2},{"sizeLabel":"8","sizeOrder":3,"minStock":5}]}`, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if len(fake.lastSizes) != 3 || fake.lastSizes[2].MinStock != 5 {
		t.Errorf("sizes = %+v", fake.lastSizes)
	}
}

// Task 3: an article has no clave. It is created without one, and neither the
// created article nor the catalogue carries a "code".
func TestCreateItemTakesNoCode(t *testing.T) {
	f := &fakeUniforms{items: []client.UniformItem{{ID: "it-1", Name: "Playera"}}}
	h := handler.NewUniformsHandler(f)

	rec := httptest.NewRecorder()
	h.CreateItem(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/items", `{"name":"Playera","priceCents":32000}`, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create without a code: status %d, body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"code"`) {
		t.Errorf("created article carries a code: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ListItems(rec, uniformsRequest(http.MethodGet, "/api/v1/uniforms/items", "", nil))
	if strings.Contains(rec.Body.String(), `"code"`) {
		t.Errorf("catalogue carries a code: %s", rec.Body.String())
	}
}

// C2-015: a return names the order in the path, the reason and the damaged
// lines in the body, and who returned it is the signed-in caller.
func TestReturnCounterSaleForwardsTheBodyAndTheCaller(t *testing.T) {
	fake := &fakeUniforms{order: client.UniformOrder{ID: "0f000000-0000-4000-8000-000000000001"}}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	r := uniformsRequest(http.MethodPost, "/api/v1/uniforms/orders/0f000000-0000-4000-8000-000000000001/return",
		`{"reason":"Talla equivocada","damagedLineIds":["ln-1"],"refundMethod":"cash","actorUserId":"usr-forged"}`, nil)
	r.SetPathValue("id", "0f000000-0000-4000-8000-000000000001")

	h.ReturnCounterSale(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	got := fake.lastReturn
	if got.OrderID != "0f000000-0000-4000-8000-000000000001" || got.Reason != "Talla equivocada" ||
		len(got.DamagedLineIDs) != 1 || got.RefundMethod != "cash" || got.ActorUserID != "usr-7" {
		t.Errorf("forwarded %+v", got)
	}
}

// An id that cannot be an order's is 404 at the edge, never a database error.
func TestReturnCounterSaleWithAMalformedIDIs404(t *testing.T) {
	fake := &fakeUniforms{}
	h := handler.NewUniformsHandler(fake)
	rec := httptest.NewRecorder()
	r := uniformsRequest(http.MethodPost, "/api/v1/uniforms/orders/nope/return", `{"reason":"x"}`, nil)
	r.SetPathValue("id", "nope")

	h.ReturnCounterSale(rec, r)

	if rec.Code != http.StatusNotFound || fake.lastReturn.OrderID != "" {
		t.Errorf("status = %d, forwarded %+v", rec.Code, fake.lastReturn)
	}
}
