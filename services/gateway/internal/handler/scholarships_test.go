package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

const (
	becaID      = "5c100000-0000-0000-0000-000000000001"
	becaStudent = "5c100000-0000-0000-0000-00000000005a"
	becaConcept = "5c100000-0000-0000-0000-0000000000c1"
)

type fakeScholarships struct {
	input    client.ScholarshipInput
	assigned []string
	removed  []string
	err      error
}

func (f *fakeScholarships) ListScholarships(context.Context, bool) ([]client.Scholarship, error) {
	return nil, f.err
}

func (f *fakeScholarships) GetScholarship(_ context.Context, id string) (client.Scholarship, error) {
	return client.Scholarship{ID: id}, f.err
}

func (f *fakeScholarships) CreateScholarship(_ context.Context, in client.ScholarshipInput) (client.Scholarship, error) {
	f.input = in
	return client.Scholarship{ID: becaID, ScholarshipInput: in}, f.err
}

func (f *fakeScholarships) UpdateScholarship(_ context.Context, id string, in client.ScholarshipInput) (client.Scholarship, error) {
	f.input = in
	return client.Scholarship{ID: id, ScholarshipInput: in}, f.err
}

func (f *fakeScholarships) ListScholarshipTargets(context.Context) ([]client.ScholarshipTarget, error) {
	return nil, f.err
}

func (f *fakeScholarships) ListScholarshipHolders(context.Context, string) ([]client.ScholarshipHolder, error) {
	return nil, f.err
}

func (f *fakeScholarships) AssignScholarship(_ context.Context, _ string, ids []string) ([]client.ScholarshipHolder, error) {
	f.assigned = ids
	return []client.ScholarshipHolder{{StudentID: ids[0], StartsOn: "2026-10-02", Overlaps: []client.ScholarshipOverlap{}}}, f.err
}

func (f *fakeScholarships) UnassignScholarship(_ context.Context, _ string, ids []string) (int32, error) {
	f.removed = ids
	return int32(len(ids)), f.err
}

func scholarshipMux(f *fakeScholarships) *http.ServeMux {
	h := handler.NewScholarshipsHandler(f)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/scholarships", h.List)
	mux.HandleFunc("POST /api/v1/scholarships", h.Create)
	mux.HandleFunc("GET /api/v1/scholarships/targets", h.Targets)
	mux.HandleFunc("GET /api/v1/scholarships/{id}", h.Get)
	mux.HandleFunc("PUT /api/v1/scholarships/{id}", h.Update)
	mux.HandleFunc("GET /api/v1/scholarships/{id}/students", h.Holders)
	mux.HandleFunc("POST /api/v1/scholarships/{id}/students", h.Assign)
	mux.HandleFunc("POST /api/v1/scholarships/{id}/students/remove", h.Unassign)
	return mux
}

func doScholarship(t *testing.T, mux http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func validBeca() map[string]any {
	return map[string]any{"name": " Hermanos ", "kind": "percent", "value": 5000, "targets": []string{"tuition", "concept:" + becaConcept}}
}

func TestScholarshipsHandlerCreatesAndDefaultsActive(t *testing.T) {
	f := &fakeScholarships{}
	rec := doScholarship(t, scholarshipMux(f), http.MethodPost, "/api/v1/scholarships", validBeca())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if f.input.Name != "Hermanos" || !f.input.Active || f.input.Value != 5000 || len(f.input.Targets) != 2 {
		t.Fatalf("input = %+v", f.input)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["id"] != becaID || got["kind"] != "percent" {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestScholarshipsHandlerRefusesBadBodies(t *testing.T) {
	cases := map[string]func(map[string]any){
		"no name":              func(b map[string]any) { b["name"] = " " },
		"long name":            func(b map[string]any) { b["name"] = strings.Repeat("a", 121) },
		"unknown kind":         func(b map[string]any) { b["kind"] = "gift" },
		"percent over 100":     func(b map[string]any) { b["value"] = 10001 },
		"percent zero":         func(b map[string]any) { b["value"] = 0 },
		"fixed off zero":       func(b map[string]any) { b["kind"], b["value"] = "fixed_off", 0 },
		"negative price":       func(b map[string]any) { b["kind"], b["value"] = "fixed_price", -1 },
		"no targets":           func(b map[string]any) { b["targets"] = []string{} },
		"target not a uuid":    func(b map[string]any) { b["targets"] = []string{"concept:abc"} },
		"unknown target":       func(b map[string]any) { b["targets"] = []string{"uniforms"} },
		"repeated target":      func(b map[string]any) { b["targets"] = []string{"tuition", "tuition"} },
		"too many targets":     func(b map[string]any) { b["targets"] = make([]string, 51) },
		"amount above the cap": func(b map[string]any) { b["kind"], b["value"] = "fixed_off", int64(1_000_000_000_000_001) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			body := validBeca()
			mutate(body)
			if rec := doScholarship(t, scholarshipMux(&fakeScholarships{}), http.MethodPost, "/api/v1/scholarships", body); rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d", rec.Code)
			}
		})
	}
	if rec := doScholarship(t, scholarshipMux(&fakeScholarships{}), http.MethodGet, "/api/v1/scholarships/nope", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("a malformed id is 404, got %d", rec.Code)
	}
}

func TestScholarshipsHandlerAssignsAndRemoves(t *testing.T) {
	f := &fakeScholarships{}
	mux := scholarshipMux(f)
	path := "/api/v1/scholarships/" + becaID + "/students"
	rec := doScholarship(t, mux, http.MethodPost, path, map[string]any{"studentIds": []string{becaStudent}})
	if rec.Code != http.StatusOK || len(f.assigned) != 1 || !strings.Contains(rec.Body.String(), `"overlaps":[]`) {
		t.Fatalf("assign %d %s", rec.Code, rec.Body)
	}
	rec = doScholarship(t, mux, http.MethodPost, path+"/remove", map[string]any{"studentIds": []string{becaStudent}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"removed":1`) {
		t.Fatalf("remove %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []map[string]any{{"studentIds": []string{}}, {"studentIds": []string{"x"}}, {"studentIds": make([]string, 201)}} {
		if rec := doScholarship(t, mux, http.MethodPost, path, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("%v: status %d", bad, rec.Code)
		}
	}
	list := doScholarship(t, mux, http.MethodGet, path, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"students":[]`) {
		t.Fatalf("holders %d %s", list.Code, list.Body)
	}
	targets := doScholarship(t, mux, http.MethodGet, "/api/v1/scholarships/targets", nil)
	if targets.Code != http.StatusOK || !strings.Contains(targets.Body.String(), `"targets":[]`) {
		t.Fatalf("targets %d %s", targets.Code, targets.Body)
	}
}

func TestScholarshipsHandlerMapsConflicts(t *testing.T) {
	f := &fakeScholarships{err: connect.NewError(connect.CodeAlreadyExists, nil)}
	rec := doScholarship(t, scholarshipMux(f), http.MethodPost, "/api/v1/scholarships", validBeca())
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d", rec.Code)
	}
}
