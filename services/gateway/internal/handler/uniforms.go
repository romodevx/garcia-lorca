package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// UniformsService is the uniform shop as the edge needs it (consumer-defined, so
// the handler is testable without a Connect round-trip). The UniformsClient
// implements it.
type UniformsService interface {
	ListItems(ctx context.Context, f client.ListItemsFilter) ([]client.UniformItem, error)
	CreateItem(ctx context.Context, in client.CreateItemInput) (client.UniformItem, error)
	UpdateItem(ctx context.Context, in client.UpdateItemInput) (client.UniformItem, error)
	UpsertSizes(ctx context.Context, itemID string, specs []client.UniformSizeSpec) (client.UpsertSizesResult, error)
	ListStock(ctx context.Context, itemID string, onlyBelowMinimum bool) ([]client.UniformSize, error)
	ReceiveStock(ctx context.Context, lines []client.UniformReceiveLine, note, actorUserID string) (client.ReceiveStockResult, error)
	AdjustStock(ctx context.Context, stockID string, delta int32, note, actorUserID string) (client.UniformSize, error)
	CounterSale(ctx context.Context, in client.CounterSaleInput) (client.UniformOrder, error)
	ListOrders(ctx context.Context, f client.ListOrdersFilter) (client.UniformOrdersPage, error)
	GetOrder(ctx context.Context, id string, studentIDs []string) (client.UniformOrder, error)
	GetSettings(ctx context.Context) (client.UniformSettings, error)
	UpdateSettings(ctx context.Context, in client.UniformSettings) (client.UniformSettings, error)
}

// UniformsHandler exposes the shop's HTTP/JSON endpoints, proxying to the
// uniforms service.
//
// Every route runs behind the module guard (module_uniforms OFF ⇒ 404, so a
// disabled module is invisible rather than merely refused) plus RequireAuth and
// rotation-enforcement. The permission split is the interesting part: the
// catalogue and the storeroom need uniforms:stock, which resolves NO student
// data and is safe to grant; the orders list needs uniforms:orders, which
// carries every pupil's name and purchase history and is owner-only.
//
// The counter sale additionally needs finance:write, checked with
// RequireAllPermissions so the refusal names the missing permission — otherwise a
// cashier keys in six lines with a parent waiting and eats a bare 403.
type UniformsHandler struct {
	svc UniformsService
}

// NewUniformsHandler wires the handler to a uniforms service.
func NewUniformsHandler(svc UniformsService) *UniformsHandler {
	return &UniformsHandler{svc: svc}
}

// --- catalogue ---------------------------------------------------------------

// ListItems: GET /api/v1/uniforms/items?includeInactive&levelId
func (h *UniformsHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := h.svc.ListItems(r.Context(), client.ListItemsFilter{
		IncludeInactive: q.Get("includeInactive") == "true",
		LevelID:         q.Get("levelId"),
	})
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type createItemRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	LevelID     string `json:"levelId"` // a config level id; empty means every level
	PriceCents  int64  `json:"priceCents"`
	// Category is uniform, book, supply or other. Empty means uniform, which is
	// what everything in this module was before the school sold anything else.
	Category string `json:"category"`
}

// CreateItem: POST /api/v1/uniforms/items
func (h *UniformsHandler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var body createItemRequest
	if !decodeBody(w, r, &body) {
		return
	}
	item, err := h.svc.CreateItem(r.Context(), client.CreateItemInput{
		Code: body.Code, Name: body.Name, Description: body.Description,
		LevelID: body.LevelID, PriceCents: body.PriceCents, Category: body.Category,
	})
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

type updateItemRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	LevelID     string `json:"levelId"` // a config level id; empty means every level
	PriceCents  int64  `json:"priceCents"`
	Active      bool   `json:"active"`
	Category    string `json:"category"`
}

// UpdateItem: PUT /api/v1/uniforms/items/{id}
func (h *UniformsHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	var body updateItemRequest
	if !decodeBody(w, r, &body) {
		return
	}
	item, err := h.svc.UpdateItem(r.Context(), client.UpdateItemInput{
		ID: r.PathValue("id"), Name: body.Name, Description: body.Description,
		LevelID: body.LevelID, PriceCents: body.PriceCents, Active: body.Active,
		Category: body.Category,
	})
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type upsertSizesRequest struct {
	Sizes []client.UniformSizeSpec `json:"sizes"`
}

// UpsertSizes: POST /api/v1/uniforms/items/{id}/sizes
func (h *UniformsHandler) UpsertSizes(w http.ResponseWriter, r *http.Request) {
	var body upsertSizesRequest
	if !decodeBody(w, r, &body) {
		return
	}
	res, err := h.svc.UpsertSizes(r.Context(), r.PathValue("id"), body.Sizes)
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// --- inventory ---------------------------------------------------------------

// ListStock: GET /api/v1/uniforms/stock?itemId&onlyBelowMinimum
func (h *UniformsHandler) ListStock(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	stock, err := h.svc.ListStock(r.Context(), q.Get("itemId"), q.Get("onlyBelowMinimum") == "true")
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stock": stock})
}

type receiveStockRequest struct {
	Lines []client.UniformReceiveLine `json:"lines"`
	Note  string                      `json:"note"`
}

// ReceiveStock: POST /api/v1/uniforms/stock/receive
func (h *UniformsHandler) ReceiveStock(w http.ResponseWriter, r *http.Request) {
	var body receiveStockRequest
	if !decodeBody(w, r, &body) {
		return
	}
	res, err := h.svc.ReceiveStock(r.Context(), body.Lines, body.Note, actorOf(r))
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type adjustStockRequest struct {
	StockID  string `json:"stockId"`
	QtyDelta int32  `json:"qtyDelta"`
	Note     string `json:"note"`
}

// AdjustStock: POST /api/v1/uniforms/stock/adjust
func (h *UniformsHandler) AdjustStock(w http.ResponseWriter, r *http.Request) {
	var body adjustStockRequest
	if !decodeBody(w, r, &body) {
		return
	}
	stock, err := h.svc.AdjustStock(r.Context(), body.StockID, body.QtyDelta, body.Note, actorOf(r))
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stock)
}

// --- the counter sale --------------------------------------------------------

type counterSaleRequest struct {
	StudentID string                   `json:"studentId"`
	Lines     []client.UniformSaleLine `json:"lines"`
	Method    string                   `json:"method"`
	Reference string                   `json:"reference"`
	Note      string                   `json:"note"`
}

// CounterSale: POST /api/v1/uniforms/counter-sale
//
// The Idempotency-Key header is REQUIRED and is rejected here rather than
// downstream, so the refusal names the header instead of arriving as a generic
// invalid-argument from two services away. It is minted when the till drawer
// opens, not when the button is pressed: a retry of the same sale carries the
// same key, while a genuine second sale to the next parent carries a new one.
func (h *UniformsHandler) CounterSale(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid",
			"Falta el encabezado Idempotency-Key", map[string]string{"header": "Idempotency-Key"})
		return
	}
	var body counterSaleRequest
	if !decodeBody(w, r, &body) {
		return
	}
	order, err := h.svc.CounterSale(r.Context(), client.CounterSaleInput{
		StudentID: body.StudentID, Lines: body.Lines, Method: body.Method,
		Reference: body.Reference, Note: body.Note,
		ActorUserID: actorOf(r), IdempotencyKey: key,
	})
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

// --- orders ------------------------------------------------------------------

// ListOrders: GET /api/v1/uniforms/orders?levelId&q&status&from&to&sort&desc&pageSize&pageToken
func (h *UniformsHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.ListOrders(r.Context(), client.ListOrdersFilter{
		LevelID:               q.Get("levelId"),
		Query:                 q.Get("q"),
		Statuses:              q["status"],
		OnlyRequiresAttention: q.Get("requiresAttention") == "true",
		From:                  q.Get("from"),
		To:                    q.Get("to"),
		Sort:                  q.Get("sort"),
		// Newest first unless asked otherwise: the panel is a work queue, and
		// the work that matters arrived most recently.
		Descending: q.Get("desc") != "false",
		PageSize:   int32(atoiOr(q.Get("pageSize"), 0)),
		PageToken:  q.Get("pageToken"),
	})
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// GetOrder: GET /api/v1/uniforms/orders/{id}
func (h *UniformsHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.GetOrder(r.Context(), r.PathValue("id"), nil)
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// --- settings ----------------------------------------------------------------

// GetSettings: GET /api/v1/uniforms/settings
func (h *UniformsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.svc.GetSettings(r.Context())
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// UpdateSettings: PUT /api/v1/uniforms/settings
func (h *UniformsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body client.UniformSettings
	if !decodeBody(w, r, &body) {
		return
	}
	settings, err := h.svc.UpdateSettings(r.Context(), body)
	if err != nil {
		writeUniformsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// --- shared ------------------------------------------------------------------

// decodeBody reads a JSON body and answers the shared envelope on malformed
// input. It returns false when it has already written a response.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid",
			"No pudimos leer la información enviada", nil)
		return false
	}
	return true
}

// actorOf is who is doing this, for the stock ledger. An adjustment without an
// actor is an adjustment nobody has to explain.
func actorOf(r *http.Request) string {
	if p, ok := middleware.PrincipalFrom(r.Context()); ok {
		return p.UserID
	}
	return ""
}

func atoiOr(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

// uniformsErrors maps a Connect code onto the status, the machine-readable code
// the UI branches on, and the sentence a person reads.
//
// The messages are Spanish and the codes are not: the code is for the panel's
// switch statement, the message is for the human in front of it, and conflating
// them is how a UI ends up branching on prose.
var uniformsErrors = map[connect.Code]struct {
	status  int
	code    string
	message string
}{
	connect.CodeInvalidArgument: {http.StatusBadRequest, "invalid",
		"Revisa la información enviada"},
	connect.CodeNotFound: {http.StatusNotFound, "not_found",
		"No encontramos lo que buscas"},
	connect.CodeAlreadyExists: {http.StatusConflict, "conflict",
		"Eso ya existe"},
	// The one a parent causes by being unlucky rather than wrong. It gets its own
	// code and its own sentence precisely because the panel must not blame the
	// cashier for an empty shelf.
	connect.CodeFailedPrecondition: {http.StatusConflict, "insufficient_stock",
		"Esa talla ya no está disponible, actualizamos la lista"},
	connect.CodePermissionDenied: {http.StatusForbidden, "forbidden",
		"Tu cuenta no tiene permiso para esta acción"},
	connect.CodeUnauthenticated: {http.StatusUnauthorized, "unauthorized",
		"Tu sesión expiró"},
	connect.CodeUnimplemented: {http.StatusNotImplemented, "not_implemented",
		"Esta función aún no está disponible"},
	connect.CodeUnavailable: {http.StatusServiceUnavailable, "upstream_unavailable",
		"El servicio no está disponible, intenta de nuevo"},
}

// writeUniformsError answers the shared envelope.
//
// The service's own message rides along in details.reason when it is safe and
// specific — "there are not enough of Suéter verde in size M" tells the cashier
// WHICH garment, which the generic sentence cannot. It is already sanitised at
// the service boundary, so nothing from a driver can reach here.
func writeUniformsError(w http.ResponseWriter, err error) {
	mapped, ok := uniformsErrors[connect.CodeOf(err)]
	if !ok {
		middleware.WriteError(w, http.StatusInternalServerError, "internal",
			"Algo salió mal. Ya lo estamos revisando", nil)
		return
	}
	var details map[string]string
	if reason := connectMessage(err); reason != "" {
		details = map[string]string{"reason": reason}
	}
	middleware.WriteError(w, mapped.status, mapped.code, mapped.message, details)
}

// connectMessage pulls the service's message out of a Connect error, without the
// code prefix Connect prepends to Error().
func connectMessage(err error) string {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr.Message()
	}
	return ""
}
