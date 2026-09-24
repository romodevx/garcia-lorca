package client

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1/uniformsv1connect"
)

// UniformsClient is the gateway's typed client for the uniform shop. Handlers
// depend on its methods and the plain structs below, never on the generated
// proto types — the edge's JSON shape and the service's contract are allowed to
// drift apart, and this is where that is absorbed.
type UniformsClient struct {
	rpc uniformsv1connect.UniformsServiceClient
}

// NewUniformsClient builds the client against the uniforms base URL.
func NewUniformsClient(baseURL string, httpClient connect.HTTPClient) *UniformsClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &UniformsClient{rpc: uniformsv1connect.NewUniformsServiceClient(httpClient, baseURL)}
}

// UniformSize is one garment in one size as the edge shows it.
//
// Available is what a parent may take — the shelf less what is already promised —
// and it arrives computed by the service. The edge does NOT recompute it: two
// places deriving the same number is two chances to disagree, and the one a
// family sees has to be the one the till enforces.
type UniformSize struct {
	ID           string `json:"id"`
	ItemID       string `json:"itemId"`
	SizeLabel    string `json:"sizeLabel"`
	SizeOrder    int32  `json:"sizeOrder"`
	OnHand       int32  `json:"onHand"`
	Reserved     int32  `json:"reserved"`
	Available    int32  `json:"available"`
	MinStock     int32  `json:"minStock"`
	BelowMinimum bool   `json:"belowMinimum"`
}

// UniformItem is one garment plus its size run. LevelID is the config level the
// garment is scoped to, empty when it applies to every level (the common case).
type UniformItem struct {
	ID          string        `json:"id"`
	Code        string        `json:"code"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	LevelID     string        `json:"levelId,omitempty"`
	PriceCents  int64         `json:"priceCents"`
	Active      bool          `json:"active"`
	Sizes       []UniformSize `json:"sizes"`
	// Category is uniform, book, supply or other. The module sells whatever the
	// school sells; only the vocabulary was ever about clothing.
	Category string `json:"category"`
}

// UniformOrderLine is one garment in one order.
type UniformOrderLine struct {
	ID             string `json:"id"`
	ItemID         string `json:"itemId"`
	ItemName       string `json:"itemName"`
	SizeLabel      string `json:"sizeLabel"`
	Qty            int32  `json:"qty"`
	UnitPriceCents int64  `json:"unitPriceCents"`
	Status         string `json:"status"`
	FulfillmentID  string `json:"fulfillmentId,omitempty"`
}

// UniformFulfillment is one delivery. Its folio carries the suffix, so the panel
// and the parent read the same string at the window.
type UniformFulfillment struct {
	ID                string `json:"id"`
	Seq               int32  `json:"seq"`
	Folio             string `json:"folio"`
	Status            string `json:"status"`
	AmountCents       int64  `json:"amountCents"`
	FinanceChargeID   string `json:"financeChargeId,omitempty"`
	PickupDeadline    string `json:"pickupDeadline,omitempty"`
	DeadlineEstimated bool   `json:"deadlineEstimated"`
	RequiresAttention bool   `json:"requiresAttention"`
	AttentionReason   string `json:"attentionReason,omitempty"`
}

// UniformOrder is what a family asked for, with everything hanging off it. Label
// is DERIVED by the service on every read, so it cannot drift from the lines and
// deliveries beside it.
type UniformOrder struct {
	ID                string               `json:"id"`
	Folio             string               `json:"folio"`
	StudentID         string               `json:"studentId,omitempty"`
	StudentName       string               `json:"studentName"`
	StudentLevel      string               `json:"studentLevel,omitempty"`   // the level's name at order time
	StudentLevelID    string               `json:"studentLevelId,omitempty"` // the config level id
	StudentGradeGroup string               `json:"studentGradeGroup,omitempty"`
	Source            string               `json:"source"`
	Label             string               `json:"label"`
	TotalCents        int64                `json:"totalCents"`
	Lines             []UniformOrderLine   `json:"lines"`
	Fulfillments      []UniformFulfillment `json:"fulfillments"`
	CreatedAt         string               `json:"createdAt,omitempty"`
}

// UniformSizeSpec is one row of a size run being created or updated.
type UniformSizeSpec struct {
	SizeLabel string `json:"sizeLabel"`
	SizeOrder int32  `json:"sizeOrder"`
	MinStock  int32  `json:"minStock"`
}

// UniformReceiveLine is one line of a goods receipt.
type UniformReceiveLine struct {
	StockID string `json:"stockId"`
	Qty     int32  `json:"qty"`
}

// UniformSaleLine is one garment being sold. There is no price field, and its
// absence is the enforcement: the amount always comes from the shop's catalogue.
type UniformSaleLine struct {
	ItemID    string `json:"itemId"`
	SizeLabel string `json:"sizeLabel"`
	Qty       int32  `json:"qty"`
}

// UniformSettings is the school's limits.
type UniformSettings struct {
	PickupWindowBusinessDays int32 `json:"pickupWindowBusinessDays"`
	AutoCancelEnabled        bool  `json:"autoCancelEnabled"`
	MaxExtensions            int32 `json:"maxExtensions"`
	MaxOpenOrdersPerStudent  int32 `json:"maxOpenOrdersPerStudent"`
	MaxLinesPerOrder         int32 `json:"maxLinesPerOrder"`
	MaxUnitsPerLine          int32 `json:"maxUnitsPerLine"`
	MaxLineExpiries          int32 `json:"maxLineExpiries"`
}

// UniformOrdersPage is one page of the panel's list.
type UniformOrdersPage struct {
	Orders        []UniformOrder `json:"orders"`
	NextPageToken string         `json:"nextPageToken,omitempty"`
	TotalCount    int32          `json:"totalCount"`
}

// ListItemsFilter narrows the catalogue read.
type ListItemsFilter struct {
	IncludeInactive bool
	LevelID         string
}

// ListItems returns the catalogue with each garment's sizes.
func (c *UniformsClient) ListItems(ctx context.Context, f ListItemsFilter) ([]UniformItem, error) {
	res, err := c.rpc.ListItems(ctx, connect.NewRequest(&uniformsv1.ListItemsRequest{
		IncludeInactive: f.IncludeInactive,
		LevelId:         f.LevelID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]UniformItem, 0, len(res.Msg.GetItems()))
	for _, item := range res.Msg.GetItems() {
		out = append(out, toUniformItem(item))
	}
	return out, nil
}

// CreateItemInput is a new garment.
type CreateItemInput struct {
	Code        string
	Name        string
	Description string
	LevelID     string // a config level id; empty means every level
	PriceCents  int64
	// Category is uniform, book, supply or other. Empty means uniform.
	Category string
}

// CreateItem puts a garment on sale.
func (c *UniformsClient) CreateItem(ctx context.Context, in CreateItemInput) (UniformItem, error) {
	res, err := c.rpc.CreateItem(ctx, connect.NewRequest(&uniformsv1.CreateItemRequest{
		Code: in.Code, Name: in.Name, Description: in.Description,
		LevelId: in.LevelID, PriceCents: in.PriceCents, Category: in.Category,
	}))
	if err != nil {
		return UniformItem{}, err
	}
	return toUniformItem(res.Msg.GetItem()), nil
}

// UpdateItemInput edits a garment. The code is not editable: it is how issued
// orders and any external stock list refer to it.
type UpdateItemInput struct {
	ID          string
	Name        string
	Description string
	LevelID     string // a config level id; empty means every level
	PriceCents  int64
	Active      bool
	Category    string
}

// UpdateItem edits a garment.
func (c *UniformsClient) UpdateItem(ctx context.Context, in UpdateItemInput) (UniformItem, error) {
	res, err := c.rpc.UpdateItem(ctx, connect.NewRequest(&uniformsv1.UpdateItemRequest{
		Id: in.ID, Name: in.Name, Description: in.Description,
		LevelId: in.LevelID, PriceCents: in.PriceCents, Active: in.Active,
		Category: in.Category,
	}))
	if err != nil {
		return UniformItem{}, err
	}
	return toUniformItem(res.Msg.GetItem()), nil
}

// UpsertSizesResult reports created and updated separately, so the panel can say
// "8 sizes created, 2 already existed" rather than upserting in silence.
type UpsertSizesResult struct {
	CreatedCount int32         `json:"createdCount"`
	UpdatedCount int32         `json:"updatedCount"`
	Sizes        []UniformSize `json:"sizes"`
}

// UpsertSizes writes a garment's whole size run in one call.
func (c *UniformsClient) UpsertSizes(ctx context.Context, itemID string, specs []UniformSizeSpec) (UpsertSizesResult, error) {
	sizes := make([]*uniformsv1.SizeSpec, 0, len(specs))
	for _, s := range specs {
		sizes = append(sizes, &uniformsv1.SizeSpec{
			SizeLabel: s.SizeLabel, SizeOrder: s.SizeOrder, MinStock: s.MinStock,
		})
	}
	res, err := c.rpc.UpsertSizes(ctx, connect.NewRequest(&uniformsv1.UpsertSizesRequest{
		ItemId: itemID, Sizes: sizes,
	}))
	if err != nil {
		return UpsertSizesResult{}, err
	}
	return UpsertSizesResult{
		CreatedCount: res.Msg.GetCreatedCount(),
		UpdatedCount: res.Msg.GetUpdatedCount(),
		Sizes:        toUniformSizes(res.Msg.GetSizes()),
	}, nil
}

// ListStock returns stock by garment and size.
func (c *UniformsClient) ListStock(ctx context.Context, itemID string, onlyBelowMinimum bool) ([]UniformSize, error) {
	res, err := c.rpc.ListStock(ctx, connect.NewRequest(&uniformsv1.ListStockRequest{
		ItemId: itemID, OnlyBelowMinimum: onlyBelowMinimum,
	}))
	if err != nil {
		return nil, err
	}
	return toUniformSizes(res.Msg.GetStock()), nil
}

// ReceiveStockResult is the shelf after a receipt, plus the waiting lines it
// satisfied on the way in.
type ReceiveStockResult struct {
	Stock            []UniformSize `json:"stock"`
	AllocatedLineIDs []string      `json:"allocatedLineIds"`
}

// ReceiveStock books a supplier delivery.
func (c *UniformsClient) ReceiveStock(ctx context.Context, lines []UniformReceiveLine, note, actorUserID string) (ReceiveStockResult, error) {
	rpcLines := make([]*uniformsv1.ReceiveStockLine, 0, len(lines))
	for _, l := range lines {
		rpcLines = append(rpcLines, &uniformsv1.ReceiveStockLine{StockId: l.StockID, Qty: l.Qty})
	}
	res, err := c.rpc.ReceiveStock(ctx, connect.NewRequest(&uniformsv1.ReceiveStockRequest{
		Lines: rpcLines, Note: note, ActorUserId: actorUserID,
	}))
	if err != nil {
		return ReceiveStockResult{}, err
	}
	return ReceiveStockResult{
		Stock:            toUniformSizes(res.Msg.GetStock()),
		AllocatedLineIDs: res.Msg.GetAllocatedLineIds(),
	}, nil
}

// AdjustStock corrects a count.
func (c *UniformsClient) AdjustStock(ctx context.Context, stockID string, delta int32, note, actorUserID string) (UniformSize, error) {
	res, err := c.rpc.AdjustStock(ctx, connect.NewRequest(&uniformsv1.AdjustStockRequest{
		StockId: stockID, QtyDelta: delta, Note: note, ActorUserId: actorUserID,
	}))
	if err != nil {
		return UniformSize{}, err
	}
	return toUniformSize(res.Msg.GetStock()), nil
}

// CounterSaleInput is the parent at the window.
type CounterSaleInput struct {
	StudentID      string
	Lines          []UniformSaleLine
	Method         string
	Reference      string
	Note           string
	ActorUserID    string
	IdempotencyKey string
}

// CounterSale sells and hands over in one act.
func (c *UniformsClient) CounterSale(ctx context.Context, in CounterSaleInput) (UniformOrder, error) {
	lines := make([]*uniformsv1.SaleLine, 0, len(in.Lines))
	for _, l := range in.Lines {
		lines = append(lines, &uniformsv1.SaleLine{ItemId: l.ItemID, SizeLabel: l.SizeLabel, Qty: l.Qty})
	}
	res, err := c.rpc.CounterSale(ctx, connect.NewRequest(&uniformsv1.CounterSaleRequest{
		StudentId: in.StudentID, Lines: lines, Method: in.Method, Reference: in.Reference,
		Note: in.Note, ActorUserId: in.ActorUserID, IdempotencyKey: in.IdempotencyKey,
	}))
	if err != nil {
		return UniformOrder{}, err
	}
	return toUniformOrder(res.Msg.GetOrder()), nil
}

// ListOrdersFilter is the panel's query. StudentIDs is security-bearing: when set,
// the SERVICE restricts the read to those pupils, so a caller cannot reach
// another family's order by id.
type ListOrdersFilter struct {
	StudentIDs            []string
	LevelID               string
	Query                 string
	Statuses              []string
	OnlyRequiresAttention bool
	From, To              string
	Sort                  string
	Descending            bool
	PageSize              int32
	PageToken             string
}

// ListOrders returns one page of the panel's list.
func (c *UniformsClient) ListOrders(ctx context.Context, f ListOrdersFilter) (UniformOrdersPage, error) {
	statuses := make([]uniformsv1.FulfillmentStatus, 0, len(f.Statuses))
	for _, s := range f.Statuses {
		statuses = append(statuses, fulfillmentStatusFromString(s))
	}
	res, err := c.rpc.ListOrders(ctx, connect.NewRequest(&uniformsv1.ListOrdersRequest{
		StudentIds: f.StudentIDs, StudentLevelId: f.LevelID, Query: f.Query,
		Statuses: statuses, OnlyRequiresAttention: f.OnlyRequiresAttention,
		From: f.From, To: f.To, Sort: f.Sort, Descending: f.Descending,
		PageSize: f.PageSize, PageToken: f.PageToken,
	}))
	if err != nil {
		return UniformOrdersPage{}, err
	}
	orders := make([]UniformOrder, 0, len(res.Msg.GetOrders()))
	for _, o := range res.Msg.GetOrders() {
		orders = append(orders, toUniformOrder(o))
	}
	return UniformOrdersPage{
		Orders: orders, NextPageToken: res.Msg.GetNextPageToken(), TotalCount: res.Msg.GetTotalCount(),
	}, nil
}

// GetOrder reads one order. studentIDs, when set, restricts what the caller may
// see at the SERVICE, not merely here.
func (c *UniformsClient) GetOrder(ctx context.Context, id string, studentIDs []string) (UniformOrder, error) {
	res, err := c.rpc.GetOrder(ctx, connect.NewRequest(&uniformsv1.GetOrderRequest{
		Id: id, StudentIds: studentIDs,
	}))
	if err != nil {
		return UniformOrder{}, err
	}
	return toUniformOrder(res.Msg.GetOrder()), nil
}

// GetSettings reads the school's limits.
func (c *UniformsClient) GetSettings(ctx context.Context) (UniformSettings, error) {
	res, err := c.rpc.GetSettings(ctx, connect.NewRequest(&uniformsv1.GetSettingsRequest{}))
	if err != nil {
		return UniformSettings{}, err
	}
	return toUniformSettings(res.Msg.GetSettings()), nil
}

// UpdateSettings replaces them.
func (c *UniformsClient) UpdateSettings(ctx context.Context, in UniformSettings) (UniformSettings, error) {
	res, err := c.rpc.UpdateSettings(ctx, connect.NewRequest(&uniformsv1.UpdateSettingsRequest{
		Settings: &uniformsv1.Settings{
			PickupWindowBusinessDays: in.PickupWindowBusinessDays,
			AutoCancelEnabled:        in.AutoCancelEnabled,
			MaxExtensions:            in.MaxExtensions,
			MaxOpenOrdersPerStudent:  in.MaxOpenOrdersPerStudent,
			MaxLinesPerOrder:         in.MaxLinesPerOrder,
			MaxUnitsPerLine:          in.MaxUnitsPerLine,
			MaxLineExpiries:          in.MaxLineExpiries,
		},
	}))
	if err != nil {
		return UniformSettings{}, err
	}
	return toUniformSettings(res.Msg.GetSettings()), nil
}

// --- translation -------------------------------------------------------------

var fulfillmentStatuses = map[string]uniformsv1.FulfillmentStatus{
	"picking":          uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_PICKING,
	"ready_for_pickup": uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_READY_FOR_PICKUP,
	"paid":             uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_PAID,
	"dispatched":       uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_DISPATCHED,
	"expired":          uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_EXPIRED,
	"cancelled":        uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_CANCELLED,
	"sold_at_counter":  uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_SOLD_AT_COUNTER,
}

var fulfillmentStatusNames = map[uniformsv1.FulfillmentStatus]string{
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_PICKING:          "picking",
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_READY_FOR_PICKUP: "ready_for_pickup",
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_PAID:             "paid",
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_DISPATCHED:       "dispatched",
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_EXPIRED:          "expired",
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_CANCELLED:        "cancelled",
	uniformsv1.FulfillmentStatus_FULFILLMENT_STATUS_SOLD_AT_COUNTER:  "sold_at_counter",
}

func fulfillmentStatusFromString(s string) uniformsv1.FulfillmentStatus {
	return fulfillmentStatuses[s]
}

var lineStatusNames = map[uniformsv1.LineStatus]string{
	uniformsv1.LineStatus_LINE_STATUS_AWAITING_STOCK: "awaiting_stock",
	uniformsv1.LineStatus_LINE_STATUS_RESERVED:       "reserved",
	uniformsv1.LineStatus_LINE_STATUS_SOLD:           "sold",
	uniformsv1.LineStatus_LINE_STATUS_RELEASED:       "released",
	uniformsv1.LineStatus_LINE_STATUS_CANCELLED:      "cancelled",
}

var orderLabelNames = map[uniformsv1.OrderLabel]string{
	uniformsv1.OrderLabel_ORDER_LABEL_AWAITING_STOCK: "awaiting_stock",
	uniformsv1.OrderLabel_ORDER_LABEL_PARTIAL:        "partial",
	uniformsv1.OrderLabel_ORDER_LABEL_IN_PROGRESS:    "in_progress",
	uniformsv1.OrderLabel_ORDER_LABEL_CLOSED:         "closed",
	uniformsv1.OrderLabel_ORDER_LABEL_CLOSED_PARTIAL: "closed_partial",
}

func toUniformSize(s *uniformsv1.StockLevel) UniformSize {
	return UniformSize{
		ID: s.GetId(), ItemID: s.GetItemId(), SizeLabel: s.GetSizeLabel(),
		SizeOrder: s.GetSizeOrder(), OnHand: s.GetOnHand(), Reserved: s.GetReserved(),
		Available: s.GetAvailable(), MinStock: s.GetMinStock(), BelowMinimum: s.GetBelowMinimum(),
	}
}

func toUniformSizes(sizes []*uniformsv1.StockLevel) []UniformSize {
	out := make([]UniformSize, 0, len(sizes))
	for _, s := range sizes {
		out = append(out, toUniformSize(s))
	}
	return out
}

func toUniformItem(i *uniformsv1.Item) UniformItem {
	return UniformItem{
		ID: i.GetId(), Code: i.GetCode(), Name: i.GetName(), Description: i.GetDescription(),
		LevelID: i.GetLevelId(), PriceCents: i.GetPriceCents(),
		Active: i.GetActive(), Sizes: toUniformSizes(i.GetSizes()),
		Category: i.GetCategory(),
	}
}

func toUniformOrder(o *uniformsv1.Order) UniformOrder {
	lines := make([]UniformOrderLine, 0, len(o.GetLines()))
	for _, l := range o.GetLines() {
		lines = append(lines, UniformOrderLine{
			ID: l.GetId(), ItemID: l.GetItemId(), ItemName: l.GetItemName(),
			SizeLabel: l.GetSizeLabel(), Qty: l.GetQty(), UnitPriceCents: l.GetUnitPriceCents(),
			Status: lineStatusNames[l.GetStatus()], FulfillmentID: l.GetFulfillmentId(),
		})
	}
	fulfillments := make([]UniformFulfillment, 0, len(o.GetFulfillments()))
	for _, f := range o.GetFulfillments() {
		fulfillments = append(fulfillments, UniformFulfillment{
			ID: f.GetId(), Seq: f.GetSeq(), Folio: f.GetFolio(),
			Status: fulfillmentStatusNames[f.GetStatus()], AmountCents: f.GetAmountCents(),
			FinanceChargeID: f.GetFinanceChargeId(), PickupDeadline: f.GetPickupDeadline(),
			DeadlineEstimated: f.GetDeadlineEstimated(),
			RequiresAttention: f.GetRequiresAttention(), AttentionReason: f.GetAttentionReason(),
		})
	}
	created := ""
	if ts := o.GetCreatedAt(); ts != nil {
		created = ts.AsTime().UTC().Format("2006-01-02T15:04:05Z")
	}
	return UniformOrder{
		ID: o.GetId(), Folio: o.GetFolio(), StudentID: o.GetStudentId(),
		StudentName: o.GetStudentName(), StudentLevel: o.GetStudentLevelName(),
		StudentLevelID:    o.GetStudentLevelId(),
		StudentGradeGroup: o.GetStudentGradeGroup(), Source: o.GetSource(),
		Label: orderLabelNames[o.GetLabel()], TotalCents: o.GetTotalCents(),
		Lines: lines, Fulfillments: fulfillments, CreatedAt: created,
	}
}

func toUniformSettings(s *uniformsv1.Settings) UniformSettings {
	return UniformSettings{
		PickupWindowBusinessDays: s.GetPickupWindowBusinessDays(),
		AutoCancelEnabled:        s.GetAutoCancelEnabled(),
		MaxExtensions:            s.GetMaxExtensions(),
		MaxOpenOrdersPerStudent:  s.GetMaxOpenOrdersPerStudent(),
		MaxLinesPerOrder:         s.GetMaxLinesPerOrder(),
		MaxUnitsPerLine:          s.GetMaxUnitsPerLine(),
		MaxLineExpiries:          s.GetMaxLineExpiries(),
	}
}
