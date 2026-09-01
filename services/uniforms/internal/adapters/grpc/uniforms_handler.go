package grpcadapter

import (
	"context"
	"errors"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1/uniformsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

// Compile-time proof the adapter implements the generated Connect handler.
var _ uniformsv1connect.UniformsServiceHandler = (*UniformsHandler)(nil)

// dateLayout is the wire format for a calendar day: no time zone, because a
// pickup deadline is a day at the school, not an instant.
const dateLayout = "2006-01-02"

// UniformsHandler adapts the uniform use cases to the generated Connect service.
// It only translates: proto messages to use-case calls, domain entities back to
// proto, and domain error kinds to Connect codes.
type UniformsHandler struct {
	shop   *usecase.Service
	health *usecase.HealthService
}

// NewUniformsHandler wires the handler to the shop and health use cases.
func NewUniformsHandler(shop *usecase.Service, health *usecase.HealthService) *UniformsHandler {
	return &UniformsHandler{shop: shop, health: health}
}

// Health is the liveness probe present on every service.
func (h *UniformsHandler) Health(ctx context.Context, _ *connect.Request[uniformsv1.HealthRequest]) (*connect.Response[uniformsv1.HealthResponse], error) {
	status, err := h.health.Ping(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.HealthResponse{Status: status.Status}), nil
}

// --- catalogue ---------------------------------------------------------------

// CreateItem puts a garment on sale.
func (h *UniformsHandler) CreateItem(ctx context.Context, req *connect.Request[uniformsv1.CreateItemRequest]) (*connect.Response[uniformsv1.CreateItemResponse], error) {
	m := req.Msg
	item, err := h.shop.CreateItem(ctx, usecase.CreateItemInput{
		Code: m.GetCode(), Name: m.GetName(), Description: m.GetDescription(),
		Level: fromProtoLevel(m.GetLevel()), PriceCents: m.GetPriceCents(),
		Category: m.GetCategory(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.CreateItemResponse{Item: toProtoItem(item, nil)}), nil
}

// UpdateItem edits a garment.
func (h *UniformsHandler) UpdateItem(ctx context.Context, req *connect.Request[uniformsv1.UpdateItemRequest]) (*connect.Response[uniformsv1.UpdateItemResponse], error) {
	m := req.Msg
	item, err := h.shop.UpdateItem(ctx, usecase.UpdateItemInput{
		ID: m.GetId(), Name: m.GetName(), Description: m.GetDescription(),
		Level: fromProtoLevel(m.GetLevel()), PriceCents: m.GetPriceCents(), Active: m.GetActive(),
		Category: m.GetCategory(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.UpdateItemResponse{Item: toProtoItem(item, nil)}), nil
}

// ListItems returns the catalogue with each garment's size run.
func (h *UniformsHandler) ListItems(ctx context.Context, req *connect.Request[uniformsv1.ListItemsRequest]) (*connect.Response[uniformsv1.ListItemsResponse], error) {
	m := req.Msg
	items, err := h.shop.ListItems(ctx, ports.ListItemsFilter{
		IncludeInactive: m.GetIncludeInactive(),
		Level:           fromProtoLevel(m.GetLevel()),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*uniformsv1.Item, 0, len(items))
	for _, iws := range items {
		out = append(out, toProtoItem(iws.Item, iws.Sizes))
	}
	return connect.NewResponse(&uniformsv1.ListItemsResponse{Items: out}), nil
}

// UpsertSizes writes a garment's whole size run in one call.
func (h *UniformsHandler) UpsertSizes(ctx context.Context, req *connect.Request[uniformsv1.UpsertSizesRequest]) (*connect.Response[uniformsv1.UpsertSizesResponse], error) {
	m := req.Msg
	specs := make([]ports.SizeSpec, 0, len(m.GetSizes()))
	for _, s := range m.GetSizes() {
		specs = append(specs, ports.SizeSpec{
			SizeLabel: s.GetSizeLabel(), SizeOrder: int(s.GetSizeOrder()), MinStock: int(s.GetMinStock()),
		})
	}
	created, updated, sizes, err := h.shop.UpsertSizes(ctx, m.GetItemId(), specs)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.UpsertSizesResponse{
		CreatedCount: int32(created), UpdatedCount: int32(updated), Sizes: toProtoStockAll(sizes),
	}), nil
}

// --- inventory ---------------------------------------------------------------

// ListStock returns stock by garment and size.
func (h *UniformsHandler) ListStock(ctx context.Context, req *connect.Request[uniformsv1.ListStockRequest]) (*connect.Response[uniformsv1.ListStockResponse], error) {
	m := req.Msg
	stock, err := h.shop.ListStock(ctx, m.GetItemId(), m.GetOnlyBelowMinimum())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.ListStockResponse{Stock: toProtoStockAll(stock)}), nil
}

// ReceiveStock books a supplier delivery and allocates it to waiting lines.
func (h *UniformsHandler) ReceiveStock(ctx context.Context, req *connect.Request[uniformsv1.ReceiveStockRequest]) (*connect.Response[uniformsv1.ReceiveStockResponse], error) {
	m := req.Msg
	lines := make([]ports.ReceiveLine, 0, len(m.GetLines()))
	for _, l := range m.GetLines() {
		lines = append(lines, ports.ReceiveLine{StockID: l.GetStockId(), Qty: int(l.GetQty())})
	}
	stock, allocated, err := h.shop.ReceiveStock(ctx, lines, m.GetNote(), m.GetActorUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.ReceiveStockResponse{
		Stock: toProtoStockAll(stock), AllocatedLineIds: allocated,
	}), nil
}

// AdjustStock corrects a count.
func (h *UniformsHandler) AdjustStock(ctx context.Context, req *connect.Request[uniformsv1.AdjustStockRequest]) (*connect.Response[uniformsv1.AdjustStockResponse], error) {
	m := req.Msg
	stock, err := h.shop.AdjustStock(ctx, m.GetStockId(), int(m.GetQtyDelta()), m.GetNote(), m.GetActorUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.AdjustStockResponse{Stock: toProtoStock(stock)}), nil
}

// --- selling -----------------------------------------------------------------

// CounterSale sells and hands over in one act.
func (h *UniformsHandler) CounterSale(ctx context.Context, req *connect.Request[uniformsv1.CounterSaleRequest]) (*connect.Response[uniformsv1.CounterSaleResponse], error) {
	m := req.Msg
	lines := make([]usecase.SaleLineInput, 0, len(m.GetLines()))
	for _, l := range m.GetLines() {
		lines = append(lines, usecase.SaleLineInput{
			ItemID: l.GetItemId(), SizeLabel: l.GetSizeLabel(), Qty: int(l.GetQty()),
		})
	}
	sale, err := h.shop.CounterSale(ctx, usecase.CounterSaleInput{
		StudentID: m.GetStudentId(), Lines: lines, Method: m.GetMethod(),
		Reference: m.GetReference(), Note: m.GetNote(), ActorUserID: m.GetActorUserId(),
		IdempotencyKey: m.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.CounterSaleResponse{Order: toProtoOrder(sale)}), nil
}

// ListOrders is the panel's page.
func (h *UniformsHandler) ListOrders(ctx context.Context, req *connect.Request[uniformsv1.ListOrdersRequest]) (*connect.Response[uniformsv1.ListOrdersResponse], error) {
	m := req.Msg
	from, err := parseDate(m.GetFrom())
	if err != nil {
		return nil, connectErr(err)
	}
	to, err := parseDate(m.GetTo())
	if err != nil {
		return nil, connectErr(err)
	}
	statuses := make([]domain.FulfillmentStatus, 0, len(m.GetStatuses()))
	for _, s := range m.GetStatuses() {
		statuses = append(statuses, fromProtoFulfillmentStatus(s))
	}
	orders, next, total, err := h.shop.ListOrders(ctx, ports.ListOrdersFilter{
		StudentIDs: m.GetStudentIds(), Level: fromProtoLevel(m.GetLevel()), Query: m.GetQuery(),
		Statuses: statuses, OnlyRequiresAttention: m.GetOnlyRequiresAttention(),
		From: from, To: to, Sort: m.GetSort(), Descending: m.GetDescending(),
		PageSize: int(m.GetPageSize()), PageToken: m.GetPageToken(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*uniformsv1.Order, 0, len(orders))
	for _, agg := range orders {
		out = append(out, toProtoOrder(agg))
	}
	return connect.NewResponse(&uniformsv1.ListOrdersResponse{
		Orders: out, NextPageToken: next, TotalCount: int32(total),
	}), nil
}

// GetOrder reads one order, scoped to the caller's students when given.
func (h *UniformsHandler) GetOrder(ctx context.Context, req *connect.Request[uniformsv1.GetOrderRequest]) (*connect.Response[uniformsv1.GetOrderResponse], error) {
	m := req.Msg
	agg, err := h.shop.GetOrder(ctx, m.GetId(), m.GetStudentIds())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.GetOrderResponse{Order: toProtoOrder(agg)}), nil
}

// --- settings ----------------------------------------------------------------

// GetSettings reads the school's limits.
func (h *UniformsHandler) GetSettings(ctx context.Context, _ *connect.Request[uniformsv1.GetSettingsRequest]) (*connect.Response[uniformsv1.GetSettingsResponse], error) {
	settings, err := h.shop.GetSettings(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.GetSettingsResponse{Settings: toProtoSettings(settings)}), nil
}

// UpdateSettings replaces them.
func (h *UniformsHandler) UpdateSettings(ctx context.Context, req *connect.Request[uniformsv1.UpdateSettingsRequest]) (*connect.Response[uniformsv1.UpdateSettingsResponse], error) {
	s := req.Msg.GetSettings()
	// The wire carries numbers and no tenant, deliberately: the tenant is the
	// service's own. Validation happens in the use case, against the domain.
	saved, err := h.shop.UpdateSettings(ctx, usecase.UpdateSettingsInput{
		PickupWindowBusinessDays: int(s.GetPickupWindowBusinessDays()),
		AutoCancelEnabled:        s.GetAutoCancelEnabled(),
		MaxExtensions:            int(s.GetMaxExtensions()),
		MaxOpenOrdersPerStudent:  int(s.GetMaxOpenOrdersPerStudent()),
		MaxLinesPerOrder:         int(s.GetMaxLinesPerOrder()),
		MaxUnitsPerLine:          int(s.GetMaxUnitsPerLine()),
		MaxLineExpiries:          int(s.GetMaxLineExpiries()),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.UpdateSettingsResponse{Settings: toProtoSettings(saved)}), nil
}

// --- stage two ---------------------------------------------------------------
//
// The family ordering half of the shop lands in stage two. These answer
// Unimplemented explicitly rather than being absent or, worse, silently
// succeeding: a route that returns an empty success is indistinguishable from
// one that worked, and the first person to find out would be a parent whose
// order never appeared.

func (h *UniformsHandler) CreateOrder(context.Context, *connect.Request[uniformsv1.CreateOrderRequest]) (*connect.Response[uniformsv1.CreateOrderResponse], error) {
	return nil, notYet("placing an order from the family app")
}

func (h *UniformsHandler) PickLines(context.Context, *connect.Request[uniformsv1.PickLinesRequest]) (*connect.Response[uniformsv1.PickLinesResponse], error) {
	return nil, notYet("picking lines into a delivery")
}

func (h *UniformsHandler) DispatchFulfillment(context.Context, *connect.Request[uniformsv1.DispatchFulfillmentRequest]) (*connect.Response[uniformsv1.DispatchFulfillmentResponse], error) {
	return nil, notYet("dispatching a delivery")
}

func (h *UniformsHandler) CancelFulfillment(context.Context, *connect.Request[uniformsv1.CancelFulfillmentRequest]) (*connect.Response[uniformsv1.CancelFulfillmentResponse], error) {
	return nil, notYet("cancelling a delivery")
}

func (h *UniformsHandler) CancelOrderLines(context.Context, *connect.Request[uniformsv1.CancelOrderLinesRequest]) (*connect.Response[uniformsv1.CancelOrderLinesResponse], error) {
	return nil, notYet("cancelling order lines")
}

func (h *UniformsHandler) ExtendDeadline(context.Context, *connect.Request[uniformsv1.ExtendDeadlineRequest]) (*connect.Response[uniformsv1.ExtendDeadlineResponse], error) {
	return nil, notYet("extending a collection deadline")
}

func (h *UniformsHandler) ExpireDueFulfillments(context.Context, *connect.Request[uniformsv1.ExpireDueFulfillmentsRequest]) (*connect.Response[uniformsv1.ExpireDueFulfillmentsResponse], error) {
	return nil, notYet("the expiry sweep")
}

func (h *UniformsHandler) ResumeStuckPicking(context.Context, *connect.Request[uniformsv1.ResumeStuckPickingRequest]) (*connect.Response[uniformsv1.ResumeStuckPickingResponse], error) {
	return nil, notYet("resuming a stuck delivery")
}

func (h *UniformsHandler) DrainOutbox(context.Context, *connect.Request[uniformsv1.DrainOutboxRequest]) (*connect.Response[uniformsv1.DrainOutboxResponse], error) {
	return nil, notYet("draining the event outbox")
}

// notYet reports a stage-two route as unimplemented, naming what is missing so a
// caller reading the log knows whether to wait or to file a bug.
func notYet(what string) *connect.Error {
	return connect.NewError(connect.CodeUnimplemented,
		glerrors.New(glerrors.KindInvalid, what+" arrives with the family ordering slice"))
}

// --- translation -------------------------------------------------------------

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "dates are ISO days, YYYY-MM-DD")
	}
	return t, nil
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dateLayout)
}

func toProtoTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

var levelToProto = map[domain.Level]uniformsv1.Level{
	domain.LevelAny:          uniformsv1.Level_LEVEL_UNSPECIFIED,
	domain.LevelKinder:       uniformsv1.Level_LEVEL_KINDER,
	domain.LevelPrimaria:     uniformsv1.Level_LEVEL_PRIMARIA,
	domain.LevelSecundaria:   uniformsv1.Level_LEVEL_SECUNDARIA,
	domain.LevelPreparatoria: uniformsv1.Level_LEVEL_PREPARATORIA,
}

var levelFromProto = map[uniformsv1.Level]domain.Level{
	uniformsv1.Level_LEVEL_UNSPECIFIED:  domain.LevelAny,
	uniformsv1.Level_LEVEL_KINDER:       domain.LevelKinder,
	uniformsv1.Level_LEVEL_PRIMARIA:     domain.LevelPrimaria,
	uniformsv1.Level_LEVEL_SECUNDARIA:   domain.LevelSecundaria,
	uniformsv1.Level_LEVEL_PREPARATORIA: domain.LevelPreparatoria,
}

func toProtoLevel(l domain.Level) uniformsv1.Level { return levelToProto[l] }

func fromProtoLevel(l uniformsv1.Level) domain.Level { return levelFromProto[l] }

var statusToProto = map[domain.FulfillmentStatus]uniformsv1.FulfillmentStatus{
	domain.StatusPicking:        uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_PICKING,
	domain.StatusReadyForPickup: uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_READY_FOR_PICKUP,
	domain.StatusPaid:           uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_PAID,
	domain.StatusDispatched:     uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_DISPATCHED,
	domain.StatusExpired:        uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_EXPIRED,
	domain.StatusCancelled:      uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_CANCELLED,
	domain.StatusSoldAtCounter:  uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_SOLD_AT_COUNTER,
}

var statusFromProto = map[uniformsv1.FulfillmentStatus]domain.FulfillmentStatus{
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_PICKING:          domain.StatusPicking,
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_READY_FOR_PICKUP: domain.StatusReadyForPickup,
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_PAID:             domain.StatusPaid,
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_DISPATCHED:       domain.StatusDispatched,
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_EXPIRED:          domain.StatusExpired,
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_CANCELLED:        domain.StatusCancelled,
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_SOLD_AT_COUNTER:  domain.StatusSoldAtCounter,
}

func fromProtoFulfillmentStatus(s uniformsv1.FulfillmentStatus) domain.FulfillmentStatus {
	return statusFromProto[s]
}

var lineStatusToProto = map[domain.LineStatus]uniformsv1.LineStatus{
	domain.LineAwaitingStock: uniformsv1.LineStatus_LINE_STATUS_AWAITING_STOCK,
	domain.LineReserved:      uniformsv1.LineStatus_LINE_STATUS_RESERVED,
	domain.LineSold:          uniformsv1.LineStatus_LINE_STATUS_SOLD,
	domain.LineReleased:      uniformsv1.LineStatus_LINE_STATUS_RELEASED,
	domain.LineCancelled:     uniformsv1.LineStatus_LINE_STATUS_CANCELLED,
}

var labelToProto = map[domain.OrderLabel]uniformsv1.OrderLabel{
	domain.LabelAwaitingStock: uniformsv1.OrderLabel_ORDER_LABEL_AWAITING_STOCK,
	domain.LabelPartial:       uniformsv1.OrderLabel_ORDER_LABEL_PARTIAL,
	domain.LabelInProgress:    uniformsv1.OrderLabel_ORDER_LABEL_IN_PROGRESS,
	domain.LabelClosed:        uniformsv1.OrderLabel_ORDER_LABEL_CLOSED,
	domain.LabelClosedPartial: uniformsv1.OrderLabel_ORDER_LABEL_CLOSED_PARTIAL,
}

func toProtoItem(i domain.Item, sizes []domain.StockLevel) *uniformsv1.Item {
	return &uniformsv1.Item{
		Id: i.ID(), TenantId: i.TenantID(), Code: i.Code(), Name: i.Name(),
		Description: i.Description(), Level: toProtoLevel(i.Level()),
		PriceCents: i.PriceCents(), Active: i.Active(), Sizes: toProtoStockAll(sizes),
		Category: i.Category(),
	}
}

// toProtoStock fills `available` and `below_minimum` from the domain rather than
// leaving the client to compute them. Available is the only number a parent
// should ever be shown, and two clients deriving it separately is two chances to
// get it wrong.
func toProtoStock(s domain.StockLevel) *uniformsv1.StockLevel {
	return &uniformsv1.StockLevel{
		Id: s.ID(), ItemId: s.ItemID(), SizeLabel: s.SizeLabel(), SizeOrder: int32(s.SizeOrder()),
		OnHand: int32(s.OnHand()), Reserved: int32(s.Reserved()), Available: int32(s.Available()),
		MinStock: int32(s.MinStock()), BelowMinimum: s.BelowMinimum(),
	}
}

func toProtoStockAll(levels []domain.StockLevel) []*uniformsv1.StockLevel {
	out := make([]*uniformsv1.StockLevel, 0, len(levels))
	for _, s := range levels {
		out = append(out, toProtoStock(s))
	}
	return out
}

func toProtoLine(l domain.OrderLine) *uniformsv1.OrderLine {
	return &uniformsv1.OrderLine{
		Id: l.ID(), ItemId: l.ItemID(), ItemName: l.ItemName(), SizeLabel: l.SizeLabel(),
		Qty: int32(l.Qty()), UnitPriceCents: l.UnitPriceCents(),
		Status: lineStatusToProto[l.Status()], FulfillmentId: l.FulfillmentID(),
	}
}

func toProtoFulfillment(f domain.Fulfillment, orderFolio string) *uniformsv1.Fulfillment {
	return &uniformsv1.Fulfillment{
		Id: f.ID(), OrderId: f.OrderID(), Seq: int32(f.Seq()), Folio: f.Folio(orderFolio),
		Status: statusToProto[f.Status()], AmountCents: f.AmountCents(),
		FinanceChargeId: f.FinanceChargeID(),
		ReadyAt:         toProtoTime(f.ReadyAt()), PickupDeadline: formatDate(f.PickupDeadline()),
		DeadlineEstimated: f.DeadlineEstimated(), ExtendedDays: int32(f.ExtensionSeq()),
		RequiresAttention: f.RequiresAttention(), AttentionReason: f.AttentionReason(),
		PaidAt: toProtoTime(f.PaidAt()), DispatchedAt: toProtoTime(f.DispatchedAt()),
		CancelReason: f.CancelReason(),
	}
}

// toProtoOrder assembles an order with its lines and deliveries, and computes
// the DERIVED label here rather than storing one. A stored label would be a
// second truth to keep in step with the lines and deliveries it describes.
func toProtoOrder(agg ports.OrderAggregate) *uniformsv1.Order {
	lines := make([]*uniformsv1.OrderLine, 0, len(agg.Lines))
	statuses := make([]domain.LineStatus, 0, len(agg.Lines))
	var total int64
	for _, l := range agg.Lines {
		lines = append(lines, toProtoLine(l))
		statuses = append(statuses, l.Status())
		total += l.SubtotalCents()
	}
	fulfillments := make([]*uniformsv1.Fulfillment, 0, len(agg.Fulfillments))
	live := false
	for _, f := range agg.Fulfillments {
		fulfillments = append(fulfillments, toProtoFulfillment(f, agg.Order.Folio()))
		if f.HoldsStock() {
			live = true
		}
	}
	return &uniformsv1.Order{
		Id: agg.Order.ID(), TenantId: agg.Order.TenantID(), Folio: agg.Order.Folio(),
		StudentId: agg.Order.StudentID(), StudentName: agg.Order.StudentName(),
		StudentLevel:      toProtoLevel(agg.Order.StudentLevel()),
		StudentGradeGroup: agg.Order.StudentGradeGroup(), CycleId: agg.Order.CycleID(),
		Source:     string(agg.Order.Source()),
		Label:      labelToProto[domain.DeriveOrderLabel(statuses, live)],
		TotalCents: total, Lines: lines, Fulfillments: fulfillments,
		CreatedAt: toProtoTime(agg.Order.CreatedAt()), ClosedAt: toProtoTime(agg.Order.ClosedAt()),
	}
}

func toProtoSettings(s domain.Settings) *uniformsv1.Settings {
	return &uniformsv1.Settings{
		PickupWindowBusinessDays: int32(s.PickupWindowBusinessDays()),
		AutoCancelEnabled:        s.AutoCancelEnabled(),
		MaxExtensions:            int32(s.MaxExtensions()),
		MaxOpenOrdersPerStudent:  int32(s.MaxOpenOrdersPerStudent()),
		MaxLinesPerOrder:         int32(s.MaxLinesPerOrder()),
		MaxUnitsPerLine:          int32(s.MaxUnitsPerLine()),
		MaxLineExpiries:          int32(s.MaxLineExpiries()),
	}
}

// connectErr maps a domain error onto the Connect code that means the same thing
// and a message that is safe to send.
//
// Every code goes through glerrors.SafeMessage, not just the internal one. The
// kind decides the STATUS; it does not decide whether the wrapped cause is the
// caller's business, and it never is. A conflict wrapped around a Postgres
// unique violation carries the constraint name and the SQLSTATE, and Error()
// concatenates the lot — so passing the error straight through ships all of it to
// whoever called. The sentence we wrote survives; everything under it does not.
func connectErr(err error) *connect.Error {
	safe := errors.New(glerrors.SafeMessage(err))

	// Running out of stock is a conflict, but not the same conflict as a
	// duplicate code, and the edge has to show a different sentence for each.
	// FailedPrecondition says "the world is not in the state this needs", which
	// is exactly right and stays a distinct code all the way to the panel.
	if errors.Is(err, domain.ErrInsufficientStock) {
		return connect.NewError(connect.CodeFailedPrecondition, safe)
	}
	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, safe)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, safe)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, safe)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, safe)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, safe)
	default:
		// An unrecognised kind is not ours, so it says nothing at all.
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}
