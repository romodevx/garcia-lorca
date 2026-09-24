package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
)

// maxPhotoBytes mirrors the students domain's ceiling (2 MB). The edge refuses
// a larger body before a byte of it reaches the service.
const maxPhotoBytes = 2 << 20

// PhotoService is the pupil-photo surface the edge needs.
type PhotoService interface {
	UploadStudentPhoto(ctx context.Context, studentID string, data []byte) (contentType string, err error)
	GetStudentPhoto(ctx context.Context, studentID string) (data []byte, contentType string, err error)
	DeleteStudentPhoto(ctx context.Context, studentID string) error
}

// PhotoHandler serves a pupil's photo. The bucket behind it is private and has
// no public URL: these routes ARE the only way to the bytes, after the route's
// permission check (read: students:read; write/delete: students:update).
type PhotoHandler struct {
	svc PhotoService
}

// NewPhotoHandler wires the handler to the students service.
func NewPhotoHandler(svc PhotoService) *PhotoHandler {
	return &PhotoHandler{svc: svc}
}

// Put handles PUT /api/v1/students/{id}/photo with the image as the raw body.
// What the image IS (JPEG/PNG) is decided by the service from its bytes, not
// from this request's Content-Type.
func (h *PhotoHandler) Put(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPhotoBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "the photo is larger than 2 MB", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	contentType, err := h.svc.UploadStudentPhoto(r.Context(), r.PathValue("id"), data)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contentType": contentType, "sizeBytes": len(data)})
}

// Get handles GET /api/v1/students/{id}/photo: the image bytes, never cached by
// a shared computer or a proxy, never re-sniffed into another type.
func (h *PhotoHandler) Get(w http.ResponseWriter, r *http.Request) {
	data, contentType, err := h.svc.GetStudentPhoto(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // the status is sent; a client that hung up is not an error to report
}

// Delete handles DELETE /api/v1/students/{id}/photo.
func (h *PhotoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteStudentPhoto(r.Context(), r.PathValue("id")); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
