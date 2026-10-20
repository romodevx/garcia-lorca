package grpcadapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/uniforms/internal/adapters/grpc"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

const tenant = "11111111-1111-1111-1111-111111111111"

var now = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

// --- doubles -----------------------------------------------------------------

// stubRepo answers what each test needs and nothing else. The handler is a
// translator, so the interesting assertions are about shapes and codes.
type stubRepo struct {
	ports.Repository
	items    []ports.ItemWithSizes
	order    ports.OrderAggregate
	settings domain.Settings
	err      error
}

func (r *stubRepo) ListItems(context.Context, ports.ListItemsFilter) ([]ports.ItemWithSizes, error) {
	return r.items, r.err
}

func (r *stubRepo) GetOrder(context.Context, string, []string) (ports.OrderAggregate, error) {
	if r.err != nil {
		return ports.OrderAggregate{}, r.err
	}
	return r.order, nil
}

func (r *stubRepo) GetSettings(context.Context) (domain.Settings, error) {
	if r.settings.TenantID() == "" {
		return domain.DefaultSettings(tenant), nil
	}
	return r.settings, nil
}

func (r *stubRepo) UpdateSettings(_ context.Context, s domain.Settings) (domain.Settings, error) {
	return s, r.err
}

func handlerWith(repo ports.Repository) *grpcadapter.UniformsHandler {
	svc := usecase.NewService(repo, nil, nil, nil, tenant,
		func() string { return "id-1" }, func() time.Time { return now }, time.UTC)
	return grpcadapter.NewUniformsHandler(svc, usecase.NewHealthService("uniforms"))
}

// --- error mapping -----------------------------------------------------------

// TestErrorKindsBecomeTheRightConnectCodes is the translation nobody notices
// until it is wrong: a not-found answered as internal turns a missing garment
// into a page that says "something went wrong", and a conflict answered as
// invalid tells a cashier to fix input that was never the problem.
func TestErrorKindsBecomeTheRightConnectCodes(t *testing.T) {
	cases := []struct {
		kind glerrors.Kind
		want connect.Code
	}{
		{glerrors.KindInvalid, connect.CodeInvalidArgument},
		{glerrors.KindUnauthorized, connect.CodeUnauthenticated},
		{glerrors.KindForbidden, connect.CodePermissionDenied},
		{glerrors.KindNotFound, connect.CodeNotFound},
		{glerrors.KindConflict, connect.CodeAlreadyExists},
		{glerrors.KindInternal, connect.CodeInternal},
	}
	for _, tc := range cases {
		t.Run(string(rune('a'+int(tc.kind))), func(t *testing.T) {
			h := handlerWith(&stubRepo{err: glerrors.New(tc.kind, "boom")})
			_, err := h.ListItems(context.Background(), connect.NewRequest(&uniformsv1.ListItemsRequest{}))
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := connect.CodeOf(err); got != tc.want {
				t.Errorf("kind %v became %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

// TestInternalErrorsSayNothing: whatever an internal failure was carrying — a
// table name, a driver message, a query — is not the caller's business.
func TestInternalErrorsSayNothing(t *testing.T) {
	h := handlerWith(&stubRepo{err: glerrors.New(glerrors.KindInternal,
		`pq: relation "uniform_stock" does not exist`)})
	_, err := h.ListItems(context.Background(), connect.NewRequest(&uniformsv1.ListItemsRequest{}))
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); got != "internal: internal error" && !contains(got, "internal error") {
		t.Errorf("the internal detail leaked to the caller: %q", got)
	}
	if contains(err.Error(), "uniform_stock") {
		t.Errorf("a table name leaked to the caller: %q", err.Error())
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// --- derived state on the wire ----------------------------------------------

// TestOrderLabelIsDerivedOnEveryRead is why the wire carries a label the server
// computed. The alternative — storing one — is a second truth to keep in step
// with the lines and deliveries it describes.
func TestOrderLabelIsDerivedOnEveryRead(t *testing.T) {
	order, err := domain.NewOrder("ord-1", tenant, "U-2026-000123", domain.OrderStudent{
		ID: "stu-1", FullName: "Zoe Villalobos", Level: domain.Level("c1000000-0000-0000-0000-000000000002"), LevelName: "Primaria", CycleID: "cyc-1",
	}, domain.SourceFamily, "usr-1", now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	sold := mustLine(t, "ln-1", domain.LineSold)
	lost := mustLine(t, "ln-2", domain.LineReleased)

	h := handlerWith(&stubRepo{order: ports.OrderAggregate{
		Order: order, Lines: []domain.OrderLine{sold, lost},
	}})
	got, err := h.GetOrder(context.Background(), connect.NewRequest(&uniformsv1.GetOrderRequest{Id: "ord-1"}))
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	// One garment delivered, one lost: the honest ending, and NOT plain closed.
	if got.Msg.GetOrder().GetLabel() != uniformsv1.OrderLabel_ORDER_LABEL_CLOSED_PARTIAL {
		t.Errorf("label = %v, want CLOSED_PARTIAL", got.Msg.GetOrder().GetLabel())
	}
	// The student's level travels as the config level it was at order time.
	o := got.Msg.GetOrder()
	if o.GetStudentLevelId() != "c1000000-0000-0000-0000-000000000002" || o.GetStudentLevelName() != "Primaria" {
		t.Errorf("student level = (%q, %q)", o.GetStudentLevelId(), o.GetStudentLevelName())
	}
	// T14-019: who made it travels too — the sale receipt names the seller.
	if o.GetCreatedByUserId() != "usr-1" {
		t.Errorf("created by = %q, want usr-1", o.GetCreatedByUserId())
	}
}

// TestOrderTotalIsSummedFromTheLines: the wire total is derived too, so it can
// never disagree with the lines beside it.
func TestOrderTotalIsSummedFromTheLines(t *testing.T) {
	order, err := domain.NewOrder("ord-1", tenant, "U-2026-000123", domain.OrderStudent{},
		domain.SourceCounter, "usr-1", now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	h := handlerWith(&stubRepo{order: ports.OrderAggregate{
		Order: order,
		Lines: []domain.OrderLine{mustLine(t, "ln-1", domain.LineSold), mustLine(t, "ln-2", domain.LineSold)},
	}})
	got, err := h.GetOrder(context.Background(), connect.NewRequest(&uniformsv1.GetOrderRequest{Id: "ord-1"}))
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if got.Msg.GetOrder().GetTotalCents() != 64000 {
		t.Errorf("total = %d, want 64000", got.Msg.GetOrder().GetTotalCents())
	}
}

// TestStockCarriesAvailableAndBelowMinimum: available is the only number a parent
// should ever see, and two clients deriving it separately is two chances to get
// it wrong.
func TestStockCarriesAvailableAndBelowMinimum(t *testing.T) {
	item, err := domain.NewItem("it-1", tenant, "Playera", "", domain.LevelAny, 32000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	stock, err := domain.NewStockLevel("st-1", tenant, "it-1", "8", 1, 5, 4, 2)
	if err != nil {
		t.Fatalf("NewStockLevel: %v", err)
	}

	h := handlerWith(&stubRepo{items: []ports.ItemWithSizes{{Item: item, Sizes: []domain.StockLevel{stock}}}})
	got, err := h.ListItems(context.Background(), connect.NewRequest(&uniformsv1.ListItemsRequest{}))
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	size := got.Msg.GetItems()[0].GetSizes()[0]
	if size.GetAvailable() != 1 {
		t.Errorf("available = %d, want 1 — five on hand, four promised", size.GetAvailable())
	}
	if !size.GetBelowMinimum() {
		t.Error("one free against a minimum of two is below minimum; the shelf being full is not the question")
	}
}

// --- stage two ---------------------------------------------------------------

// TestStageTwoRoutesAnswerUnimplemented: a route that is not built yet must say
// so. An empty success is indistinguishable from one that worked, and the first
// person to find out would be a parent whose order never appeared.
func TestStageTwoRoutesAnswerUnimplemented(t *testing.T) {
	h := handlerWith(&stubRepo{})
	ctx := context.Background()

	cases := map[string]func() error{
		"CreateOrder": func() error {
			_, err := h.CreateOrder(ctx, connect.NewRequest(&uniformsv1.CreateOrderRequest{}))
			return err
		},
		"PickLines": func() error {
			_, err := h.PickLines(ctx, connect.NewRequest(&uniformsv1.PickLinesRequest{}))
			return err
		},
		"DispatchFulfillment": func() error {
			_, err := h.DispatchFulfillment(ctx, connect.NewRequest(&uniformsv1.DispatchFulfillmentRequest{}))
			return err
		},
		"CancelFulfillment": func() error {
			_, err := h.CancelFulfillment(ctx, connect.NewRequest(&uniformsv1.CancelFulfillmentRequest{}))
			return err
		},
		"CancelOrderLines": func() error {
			_, err := h.CancelOrderLines(ctx, connect.NewRequest(&uniformsv1.CancelOrderLinesRequest{}))
			return err
		},
		"ExtendDeadline": func() error {
			_, err := h.ExtendDeadline(ctx, connect.NewRequest(&uniformsv1.ExtendDeadlineRequest{}))
			return err
		},
		"ExpireDueFulfillments": func() error {
			_, err := h.ExpireDueFulfillments(ctx, connect.NewRequest(&uniformsv1.ExpireDueFulfillmentsRequest{}))
			return err
		},
		"ResumeStuckPicking": func() error {
			_, err := h.ResumeStuckPicking(ctx, connect.NewRequest(&uniformsv1.ResumeStuckPickingRequest{}))
			return err
		},
		"DrainOutbox": func() error {
			_, err := h.DrainOutbox(ctx, connect.NewRequest(&uniformsv1.DrainOutboxRequest{}))
			return err
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil {
				t.Fatal("a route that is not built answered success")
			}
			if got := connect.CodeOf(err); got != connect.CodeUnimplemented {
				t.Errorf("code = %v, want Unimplemented", got)
			}
		})
	}
}

// --- settings ----------------------------------------------------------------

// TestUpdateSettingsValidatesBeforeWriting: the wire is a bag of numbers, and a
// pickup window of zero working days would make every hold expire the instant it
// was created.
func TestUpdateSettingsValidatesBeforeWriting(t *testing.T) {
	h := handlerWith(&stubRepo{})
	_, err := h.UpdateSettings(context.Background(), connect.NewRequest(&uniformsv1.UpdateSettingsRequest{
		Settings: &uniformsv1.Settings{PickupWindowBusinessDays: 0},
	}))
	if err == nil {
		t.Fatal("a zero-day pickup window was accepted")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestHealth(t *testing.T) {
	h := handlerWith(&stubRepo{})
	got, err := h.Health(context.Background(), connect.NewRequest(&uniformsv1.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if got.Msg.GetStatus() != "SERVING" {
		t.Errorf("status = %q, want SERVING", got.Msg.GetStatus())
	}
}

// --- helpers -----------------------------------------------------------------

func mustLine(t *testing.T, id string, status domain.LineStatus) domain.OrderLine {
	t.Helper()
	line, err := domain.NewOrderLine(id, tenant, "ord-1", "it-1", "Playera", "8", 1, 32000, status)
	if err != nil {
		t.Fatalf("NewOrderLine: %v", err)
	}
	return line
}

// TestOutOfStockIsItsOwnCode is the distinction the panel's copy depends on.
// Both are conflicts; only one is the cashier's fault. Answering the same code
// for a duplicate garment code and an empty shelf forces the UI to show one
// sentence for both, and it would be the wrong sentence half the time.
func TestOutOfStockIsItsOwnCode(t *testing.T) {
	outOfStock := glerrors.Wrap(glerrors.KindConflict,
		"no quedan suficientes Suéter verde en talla M", domain.ErrInsufficientStock)
	duplicate := glerrors.New(glerrors.KindConflict, "a garment with that code already exists")

	h := handlerWith(&stubRepo{err: outOfStock})
	_, err := h.ListItems(context.Background(), connect.NewRequest(&uniformsv1.ListItemsRequest{}))
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Errorf("out of stock answered %v, want FailedPrecondition", got)
	}

	h = handlerWith(&stubRepo{err: duplicate})
	_, err = h.ListItems(context.Background(), connect.NewRequest(&uniformsv1.ListItemsRequest{}))
	if got := connect.CodeOf(err); got != connect.CodeAlreadyExists {
		t.Errorf("a duplicate answered %v, want AlreadyExists", got)
	}
}

// TestNoErrorLeaksTheDriversWords is the wider version of the internal-error
// test, and it was written after finding the leak: a CONFLICT was shipping the
// Postgres constraint name and SQLSTATE to whoever called. The kind decides the
// status; it does not decide whether the cause is the caller's business, and it
// never is.
func TestNoErrorLeaksTheDriversWords(t *testing.T) {
	driver := `ERROR: duplicate key value violates unique constraint "uniform_items_tenant_id_code_key" (SQLSTATE 23505)`
	cases := map[string]error{
		"conflict": glerrors.Wrap(glerrors.KindConflict, "a garment with that code already exists", errors.New(driver)),
		"invalid":  glerrors.Wrap(glerrors.KindInvalid, "that size is not on this garment", errors.New(driver)),
		"notFound": glerrors.Wrap(glerrors.KindNotFound, "no such garment", errors.New(driver)),
		"internal": glerrors.Wrap(glerrors.KindInternal, "reading the catalogue", errors.New(driver)),
	}
	for name, injected := range cases {
		t.Run(name, func(t *testing.T) {
			h := handlerWith(&stubRepo{err: injected})
			_, err := h.ListItems(context.Background(), connect.NewRequest(&uniformsv1.ListItemsRequest{}))
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, secret := range []string{"SQLSTATE", "uniform_items_tenant_id_code_key", "duplicate key"} {
				if contains(err.Error(), secret) {
					t.Errorf("%q reached the caller: %s", secret, err.Error())
				}
			}
		})
	}
}

// TestUsefulMessagesStillReachTheCaller: sanitising must not turn every refusal
// into "internal error". The sentence we wrote is the one a cashier reads.
func TestUsefulMessagesStillReachTheCaller(t *testing.T) {
	h := handlerWith(&stubRepo{err: glerrors.Wrap(glerrors.KindConflict,
		"no quedan suficientes Suéter verde en talla M",
		errors.New("ERROR: something internal (SQLSTATE 23514)"))})
	_, err := h.ListItems(context.Background(), connect.NewRequest(&uniformsv1.ListItemsRequest{}))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !contains(err.Error(), "no quedan suficientes Suéter verde en talla M") {
		t.Errorf("the useful part was thrown away too: %s", err.Error())
	}
}

// C2-015: a return without a reason is the caller's mistake, refused before
// any money or garment moves.
func TestReturnCounterSaleWithoutAReasonIsInvalid(t *testing.T) {
	h := handlerWith(&stubRepo{})

	_, err := h.ReturnCounterSale(context.Background(), connect.NewRequest(&uniformsv1.ReturnCounterSaleRequest{
		OrderId: "0f000000-0000-4000-8000-000000000001",
	}))

	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

// C2-015 review: a return's decision is stored before the money moves; the
// wire shows a return only once the delivery is returned.
func TestOnlyAReturnedDeliveryCarriesItsReturn(t *testing.T) {
	order, err := domain.NewOrder("ord-1", tenant, "U-2026-000002", domain.OrderStudent{}, domain.SourceCounter, "usr-1", now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	sold, err := domain.NewCounterSaleFulfillment("ful-1", tenant, "ord-1", 1, 25000, now)
	if err != nil {
		t.Fatalf("fulfillment: %v", err)
	}
	returned, err := sold.ReturnAtCounter()
	if err != nil {
		t.Fatalf("ReturnAtCounter: %v", err)
	}
	rec := map[string]ports.CounterSaleReturn{"ful-1": {Reason: "Talla", RefundMode: ports.RefundVoid, RefundStatus: ports.RefundDone}}

	for _, tc := range []struct {
		name string
		f    domain.Fulfillment
		want bool
	}{{"decision only", sold, false}, {"returned", returned, true}} {
		t.Run(tc.name, func(t *testing.T) {
			h := handlerWith(&stubRepo{order: ports.OrderAggregate{Order: order, Fulfillments: []domain.Fulfillment{tc.f}, Returns: rec}})
			got, err := h.GetOrder(context.Background(), connect.NewRequest(&uniformsv1.GetOrderRequest{Id: "ord-1"}))
			if err != nil {
				t.Fatalf("GetOrder: %v", err)
			}
			if has := got.Msg.GetOrder().GetFulfillments()[0].GetCounterSaleReturn() != nil; has != tc.want {
				t.Errorf("carries a return = %v, want %v", has, tc.want)
			}
		})
	}
}
