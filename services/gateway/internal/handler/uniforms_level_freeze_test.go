package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// fakeLevelReader answers config's GetLevel for the uniforms level guard.
type fakeLevelReader struct {
	levels map[string]client.LevelResult
	calls  int
}

func (f *fakeLevelReader) GetLevel(_ context.Context, id string) (client.LevelResult, error) {
	f.calls++
	l, ok := f.levels[id]
	if !ok {
		return client.LevelResult{}, connect.NewError(connect.CodeNotFound, nil)
	}
	return l, nil
}

// recordingUniforms counts the writes that reached the uniforms service.
type recordingUniforms struct {
	fakeUniforms
	writes int
}

func (f *recordingUniforms) CreateItem(ctx context.Context, in client.CreateItemInput) (client.UniformItem, error) {
	f.writes++
	return f.fakeUniforms.CreateItem(ctx, in)
}

func (f *recordingUniforms) UpdateItem(ctx context.Context, in client.UpdateItemInput) (client.UniformItem, error) {
	f.writes++
	return f.fakeUniforms.UpdateItem(ctx, in)
}

// Confirmation review (gap 3): uniforms never looks a level up itself, so the
// edge — the only caller that writes garments — refuses to scope one to a level
// whose deletion runs (409 LEVEL_BEING_DELETED). A garment without a level, or
// on an active one, goes through; nothing reaches uniforms on a refusal.
func TestUniformItemWritesRefuseALevelBeingDeleted(t *testing.T) {
	const (
		deleting = "11111111-1111-4111-8111-111111111111"
		active   = "22222222-2222-4222-8222-222222222222"
	)
	levels := map[string]client.LevelResult{
		deleting: {ID: deleting, Name: "Primaria", Deleting: true},
		active:   {ID: active, Name: "Secundaria"},
	}
	cases := []struct {
		name      string
		call      func(h *handler.UniformsHandler, rec *httptest.ResponseRecorder, body string)
		okStatus  int
		levelBody func(level string) string
	}{
		{"create", func(h *handler.UniformsHandler, rec *httptest.ResponseRecorder, body string) {
			h.CreateItem(rec, uniformsRequest(http.MethodPost, "/api/v1/uniforms/items", body, nil))
		}, http.StatusCreated, func(level string) string {
			return `{"name":"Playera","priceCents":32000,"levelId":"` + level + `"}`
		}},
		{"update", func(h *handler.UniformsHandler, rec *httptest.ResponseRecorder, body string) {
			r := uniformsRequest(http.MethodPut, "/api/v1/uniforms/items/it-1", body, nil)
			r.SetPathValue("id", "it-1")
			h.UpdateItem(rec, r)
		}, http.StatusOK, func(level string) string {
			return `{"name":"Playera","priceCents":32000,"active":true,"levelId":"` + level + `"}`
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := &recordingUniforms{}
			h := handler.NewUniformsHandler(svc).WithLevelGuard(&fakeLevelReader{levels: levels})

			rec := httptest.NewRecorder()
			c.call(h, rec, c.levelBody(deleting))
			if rec.Code != http.StatusConflict {
				t.Fatalf("deleting level: status = %d, want 409 (%s)", rec.Code, rec.Body.String())
			}
			if got := decodeError(t, rec).Code; got != "LEVEL_BEING_DELETED" {
				t.Errorf("code = %q, want LEVEL_BEING_DELETED", got)
			}
			if got := rec.Header().Get("X-Error-Code"); got != "LEVEL_BEING_DELETED" {
				t.Errorf("X-Error-Code = %q", got)
			}
			if svc.writes != 0 {
				t.Errorf("a refused write reached uniforms (%d)", svc.writes)
			}

			for _, level := range []string{active, ""} {
				rec = httptest.NewRecorder()
				c.call(h, rec, c.levelBody(level))
				if rec.Code != c.okStatus {
					t.Errorf("level %q: status = %d, want %d (%s)", level, rec.Code, c.okStatus, rec.Body.String())
				}
			}
		})
	}
}
