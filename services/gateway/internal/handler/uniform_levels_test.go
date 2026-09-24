package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakeLevelLister struct {
	levels []client.LevelResult
	err    error
}

func (f fakeLevelLister) ListLevels(context.Context) ([]client.LevelResult, error) {
	return f.levels, f.err
}

// The inventory's level chips read the school's levels through the uniforms
// module itself, so a storeroom role needs no config:read to see them — and
// the answer carries only what a chip shows: an id and a name.
func TestUniformLevelsListsIDAndNameOnly(t *testing.T) {
	h := handler.NewUniformLevelsHandler(fakeLevelLister{levels: []client.LevelResult{
		{ID: "lvl-pre", Name: "Preescolar", SEPCode: "PRE", Granularity: "cycle"},
	}})
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/uniforms/levels", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Levels []map[string]any `json:"levels"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Levels) != 1 || body.Levels[0]["id"] != "lvl-pre" || body.Levels[0]["name"] != "Preescolar" {
		t.Fatalf("levels = %v", body.Levels)
	}
	if _, leaked := body.Levels[0]["sepCode"]; leaked {
		t.Error("only id and name belong in the inventory's view")
	}
}

func TestUniformLevelsUpstreamFailureIsNotA200(t *testing.T) {
	h := handler.NewUniformLevelsHandler(fakeLevelLister{err: errors.New("config down")})
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/uniforms/levels", nil))
	if rec.Code < 500 {
		t.Fatalf("status = %d, want a 5xx", rec.Code)
	}
}
