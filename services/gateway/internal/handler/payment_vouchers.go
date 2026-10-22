package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// Edge bounds of the payment vouchers (ADR 0040), mirroring finance's domain.
const (
	maxVoucherBytes = 5 << 20
	// maxVoucherBody leaves room for the multipart framing and the two small
	// fields around a 5 MB file.
	maxVoucherBody = maxVoucherBytes + 64<<10
	// maxVoucherField bounds amountCents and paidOn: a number or a date.
	maxVoucherField     = 32
	minVoucherReason    = 3
	maxVoucherReason    = 300
	maxVoucherListLimit = 500
	voucherDayLayout    = "2006-01-02"
)

var voucherStatuses = map[string]bool{"": true, "pending": true, "applied": true, "rejected": true}

// PaymentVoucherService is the finance surface of the payment vouchers
// (consumer-defined; the Connect FinanceClient implements it).
type PaymentVoucherService interface {
	CreatePaymentVoucher(ctx context.Context, in client.VoucherUpload) (client.PaymentVoucher, bool, error)
	ListPaymentVouchers(ctx context.Context, scope client.VoucherScope, status string, limit int) ([]client.PaymentVoucher, error)
	GetPaymentVoucherFile(ctx context.Context, id string, scope client.VoucherScope) ([]byte, string, error)
	RejectPaymentVoucher(ctx context.Context, id, reason string, by client.Receiver) (client.PaymentVoucher, bool, error)
}

// PaymentVouchersHandler serves the payment vouchers (ADR 0040): the family
// sends one for its OWN child and reads them; the office reads the queue, the
// file, and rejects. Applying one is the counter's deposit (FinanceHandler).
type PaymentVouchersHandler struct {
	svc   PaymentVoucherService
	links GuardianLinkReader
	names UserNamer
}

// NewPaymentVouchersHandler wires the handler. names may be nil (the token's
// email names the caller).
func NewPaymentVouchersHandler(svc PaymentVoucherService, links GuardianLinkReader, names UserNamer) *PaymentVouchersHandler {
	return &PaymentVouchersHandler{svc: svc, links: links, names: names}
}

// familyVoucherResponse is a voucher as the family sees it: what it sent and
// what the office did, never who in the office.
type familyVoucherResponse struct {
	ID                 string `json:"id"`
	Status             string `json:"status"`
	ClaimedAmountCents int64  `json:"claimedAmountCents,omitempty"`
	ClaimedPaidOn      string `json:"claimedPaidOn,omitempty"`
	FileContentType    string `json:"fileContentType,omitempty"`
	DepositFolio       string `json:"depositFolio,omitempty"`
	RejectionReason    string `json:"rejectionReason,omitempty"`
	CreatedAt          string `json:"createdAt"`
	DecidedAt          string `json:"decidedAt,omitempty"`
}

func toFamilyVoucher(v client.PaymentVoucher) familyVoucherResponse {
	return familyVoucherResponse{
		ID: v.ID, Status: v.Status, ClaimedAmountCents: v.ClaimedAmountCents, ClaimedPaidOn: v.ClaimedPaidOn,
		FileContentType: v.FileContentType, DepositFolio: v.DepositFolio, RejectionReason: v.RejectionReason,
		CreatedAt: v.CreatedAt, DecidedAt: v.DecidedAt,
	}
}

func (h *PaymentVouchersHandler) caller(r *http.Request) client.Receiver {
	p, _ := middleware.PrincipalFrom(r.Context())
	return client.Receiver{UserID: p.UserID, Name: callerName(r.Context(), h.names, p)}
}

// ownChild answers 404 (and false) unless {studentId} is the caller's child.
func (h *PaymentVouchersHandler) ownChild(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	studentID := r.PathValue("studentId")
	owned, err := ownsChild(r.Context(), h.links, p.UserID, studentID)
	if err != nil {
		writeConnectError(w, err)
		return "", false
	}
	if !owned {
		writeNotFound(w)
		return "", false
	}
	return studentID, true
}

// --- family ----------------------------------------------------------------

// FamilyUpload handles POST /api/v1/family/students/{studentId}/payment-vouchers:
// a multipart form with the file and, optionally, amountCents and paidOn.
// What the file IS (JPEG/PNG/PDF) is decided by finance from its bytes.
func (h *PaymentVouchersHandler) FamilyUpload(w http.ResponseWriter, r *http.Request) {
	studentID, ok := h.ownChild(w, r)
	if !ok {
		return
	}
	form, status, msg := readVoucherForm(w, r)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	form.StudentID, form.By = studentID, h.caller(r)
	v, replayed, err := h.svc.CreatePaymentVoucher(r.Context(), form)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	code := http.StatusCreated
	if replayed {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"voucher": toFamilyVoucher(v), "replayed": replayed})
}

// readVoucherForm reads the upload. A non-zero status is the refusal: 413 for
// a file (or body) past the cap, 400 for anything else off the contract.
func readVoucherForm(w http.ResponseWriter, r *http.Request) (client.VoucherUpload, int, string) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return client.VoucherUpload{}, http.StatusBadRequest, "the voucher is sent as multipart/form-data"
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxVoucherBody)
	parts, err := r.MultipartReader()
	if err != nil {
		return client.VoucherUpload{}, http.StatusBadRequest, "invalid multipart body"
	}
	var (
		out     client.VoucherUpload
		hasFile bool
	)
	for {
		part, err := parts.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return client.VoucherUpload{}, bodyErrorStatus(err), "invalid multipart body"
		}
		status, msg := readVoucherPart(part, &out, &hasFile)
		_ = part.Close() // the part is fully read or abandoned; closing cannot fail the request
		if status != 0 {
			return client.VoucherUpload{}, status, msg
		}
	}
	if !hasFile {
		return client.VoucherUpload{}, http.StatusBadRequest, "file is required"
	}
	return out, 0, ""
}

// bodyErrorStatus is 413 for a body past the cap, 400 for any other read error.
func bodyErrorStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func readVoucherPart(part *multipart.Part, out *client.VoucherUpload, hasFile *bool) (int, string) {
	switch part.FormName() {
	case "file":
		if *hasFile {
			return http.StatusBadRequest, "send one file"
		}
		data, err := io.ReadAll(io.LimitReader(part, maxVoucherBytes+1))
		if err != nil {
			return bodyErrorStatus(err), "invalid multipart body"
		}
		if len(data) > maxVoucherBytes {
			return http.StatusRequestEntityTooLarge, "the voucher is larger than 5 MB"
		}
		out.Data, *hasFile = data, true
	case "amountCents":
		raw, status := readVoucherField(part)
		if status != 0 {
			return status, "amountCents must be a whole number of cents"
		}
		cents, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || cents <= 0 {
			return http.StatusBadRequest, "amountCents must be a whole number of cents above zero"
		}
		out.ClaimedAmountCents = cents
	case "paidOn":
		raw, status := readVoucherField(part)
		if status != 0 {
			return status, "paidOn must be a date like 2026-10-20"
		}
		if _, err := time.Parse(voucherDayLayout, raw); err != nil {
			return http.StatusBadRequest, "paidOn must be a date like 2026-10-20"
		}
		out.ClaimedPaidOn = raw
	}
	return 0, ""
}

// readVoucherField reads a small text field; an empty one is "not said".
func readVoucherField(part io.Reader) (string, int) {
	raw, err := io.ReadAll(io.LimitReader(part, maxVoucherField+1))
	if err != nil {
		return "", bodyErrorStatus(err)
	}
	if len(raw) > maxVoucherField {
		return "", http.StatusBadRequest
	}
	return strings.TrimSpace(string(raw)), 0
}

// FamilyList handles GET /api/v1/family/students/{studentId}/payment-vouchers.
func (h *PaymentVouchersHandler) FamilyList(w http.ResponseWriter, r *http.Request) {
	studentID, ok := h.ownChild(w, r)
	if !ok {
		return
	}
	vs, err := h.svc.ListPaymentVouchers(r.Context(), client.VoucherScope{StudentIDs: []string{studentID}}, "", 0)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]familyVoucherResponse, 0, len(vs))
	for _, v := range vs {
		out = append(out, toFamilyVoucher(v))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"vouchers": out})
}

// --- the office ------------------------------------------------------------

// Queue handles GET /api/v1/finance/payment-vouchers?status&studentId&limit:
// the school's queue, newest first (the 500 cap cuts the oldest).
func (h *PaymentVouchersHandler) Queue(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if !voucherStatuses[status] {
		http.Error(w, "status must be pending, applied or rejected", http.StatusBadRequest)
		return
	}
	scope := client.VoucherScope{SchoolQueue: true}
	if studentID := q.Get("studentId"); studentID != "" {
		if !uuidShape.MatchString(studentID) {
			http.Error(w, "studentId must be a student id", http.StatusBadRequest)
			return
		}
		scope.StudentIDs = []string{studentID}
	}
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxVoucherListLimit {
			http.Error(w, "limit must be between 1 and 500", http.StatusBadRequest)
			return
		}
		limit = n
	}
	vs, err := h.svc.ListPaymentVouchers(r.Context(), scope, status, limit)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if vs == nil {
		vs = []client.PaymentVoucher{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"vouchers": vs})
}

// voucherIDOf is the path's id when it has an id's shape; otherwise 404.
func voucherIDOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		writeNotFound(w)
		return "", false
	}
	return id, true
}

// File handles GET /api/v1/finance/payment-vouchers/{id}/file: the family's
// upload with the type its bytes proved to be, never sniffed, never cached,
// sandboxed, and as an attachment — never rendered as a page of the origin.
func (h *PaymentVouchersHandler) File(w http.ResponseWriter, r *http.Request) {
	id, ok := voucherIDOf(w, r)
	if !ok {
		return
	}
	data, contentType, err := h.svc.GetPaymentVoucherFile(r.Context(), id, client.VoucherScope{SchoolQueue: true})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	extension := map[string]string{"application/pdf": "pdf", "image/png": "png", "image/jpeg": "jpg"}[contentType]
	w.Header().Set("Content-Type", contentType)
	// attachment, not inline: a family's upload is never rendered as a page of
	// the app's origin; the web previews it from the Blob it fetched.
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "comprobante-pago."+extension))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", receiptSandboxCSP)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // the status is sent; a client that hung up is not an error to report
}

type rejectVoucherRequest struct {
	Reason string `json:"reason"`
}

// Reject handles POST /api/v1/finance/payment-vouchers/{id}/reject.
func (h *PaymentVouchersHandler) Reject(w http.ResponseWriter, r *http.Request) {
	id, ok := voucherIDOf(w, r)
	if !ok {
		return
	}
	var req rejectVoucherRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if n := utf8.RuneCountInString(reason); n < minVoucherReason || n > maxVoucherReason {
		http.Error(w, "el motivo del rechazo debe tener entre 3 y 300 caracteres", http.StatusBadRequest)
		return
	}
	v, replayed, err := h.svc.RejectPaymentVoucher(r.Context(), id, reason, h.caller(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"voucher": v, "replayed": replayed})
}
