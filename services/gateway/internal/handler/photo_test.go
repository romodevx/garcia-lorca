package handler_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakePhotos struct {
	gotID   string
	gotData []byte
	data    []byte
	ctype   string
	err     error
	deleted string
}

func (f *fakePhotos) UploadStudentPhoto(_ context.Context, id string, data []byte) (string, error) {
	f.gotID, f.gotData = id, data
	return "image/jpeg", f.err
}

func (f *fakePhotos) GetStudentPhoto(_ context.Context, id string) ([]byte, string, error) {
	f.gotID = id
	return f.data, f.ctype, f.err
}

func (f *fakePhotos) DeleteStudentPhoto(_ context.Context, id string) error {
	f.deleted = id
	return f.err
}

var jpeg = []byte{0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3}

func photoRequest(method string, body []byte) *http.Request {
	req := httptest.NewRequest(method, "/api/v1/students/stu-1/photo", bytes.NewReader(body))
	req.SetPathValue("id", "stu-1")
	return req
}

func TestPhotoUploadForwardsTheBytes(t *testing.T) {
	svc := &fakePhotos{}
	rec := httptest.NewRecorder()
	handler.NewPhotoHandler(svc).Put(rec, photoRequest(http.MethodPut, jpeg))
	if rec.Code != http.StatusOK || svc.gotID != "stu-1" || !bytes.Equal(svc.gotData, jpeg) {
		t.Errorf("status %d forwarded %q %v", rec.Code, svc.gotID, svc.gotData)
	}
}

// Over the ceiling is refused at the edge, before a byte reaches the service.
func TestPhotoUploadOverTwoMegabytesIs413(t *testing.T) {
	svc := &fakePhotos{}
	rec := httptest.NewRecorder()
	handler.NewPhotoHandler(svc).Put(rec, photoRequest(http.MethodPut, make([]byte, 2<<20+1)))
	if rec.Code != http.StatusRequestEntityTooLarge || svc.gotID != "" {
		t.Errorf("status %d, service called with %q", rec.Code, svc.gotID)
	}
}

func TestPhotoUploadNotAnImageIs400(t *testing.T) {
	svc := &fakePhotos{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	rec := httptest.NewRecorder()
	handler.NewPhotoHandler(svc).Put(rec, photoRequest(http.MethodPut, []byte("%PDF")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}

// The bytes come back with the type they proved to be, never cached, never
// sniffed into something else.
func TestPhotoGetServesTheImagePrivately(t *testing.T) {
	svc := &fakePhotos{data: jpeg, ctype: "image/jpeg"}
	rec := httptest.NewRecorder()
	handler.NewPhotoHandler(svc).Get(rec, photoRequest(http.MethodGet, nil))
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), jpeg) {
		t.Fatalf("status %d body %v", rec.Code, rec.Body.Bytes())
	}
	h := rec.Header()
	if h.Get("Content-Type") != "image/jpeg" || h.Get("Cache-Control") != "private, no-store" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers = %v", h)
	}
}

func TestPhotoGetWithoutOneIs404(t *testing.T) {
	svc := &fakePhotos{err: connect.NewError(connect.CodeNotFound, nil)}
	rec := httptest.NewRecorder()
	handler.NewPhotoHandler(svc).Get(rec, photoRequest(http.MethodGet, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

func TestPhotoDeleteIs204(t *testing.T) {
	svc := &fakePhotos{}
	rec := httptest.NewRecorder()
	handler.NewPhotoHandler(svc).Delete(rec, photoRequest(http.MethodDelete, nil))
	if rec.Code != http.StatusNoContent || svc.deleted != "stu-1" {
		t.Errorf("status %d deleted %q", rec.Code, svc.deleted)
	}
}
