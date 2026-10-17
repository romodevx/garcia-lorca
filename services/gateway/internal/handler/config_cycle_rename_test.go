package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

const cycleA = "00000000-0000-4000-8000-0000000000c1"

func (f *fakeConfig) RenameCycle(_ context.Context, id, name string) (client.CycleResult, error) {
	f.renamedCycle, f.renamedCycleTo = id, name
	if f.err != nil {
		return client.CycleResult{}, f.err
	}
	return client.CycleResult{ID: id, Name: name, Status: "draft"}, nil
}

// --- PATCH /api/v1/config/cycles/{id} (CC-103) ---------------------------------

func TestRenameCycleForwardsAndAnswersTheCycle(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)

	rec := doPathReq(t, h.RenameCycle, http.MethodPatch, "/api/v1/config/cycles/"+cycleA, cycleA, `{"name":"2027-2028"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, cycleA, svc.renamedCycle)
	assert.Equal(t, "2027-2028", svc.renamedCycleTo)
	var got map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "2027-2028", got["name"])
	assert.Equal(t, cycleA, got["id"])
}

func TestRenameCycleRefusesBadShapesBeforeCallingConfig(t *testing.T) {
	cases := map[string]struct{ id, body string }{
		"malformed id":   {"not-a-uuid", `{"name":"x"}`},
		"malformed body": {cycleA, `{"name":`},
		"no name":        {cycleA, `{"name":"  "}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeConfig{}
			h := handler.NewConfigHandler(svc)
			rec := doPathReq(t, h.RenameCycle, http.MethodPatch, "/api/v1/config/cycles/"+tc.id, tc.id, tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, svc.renamedCycle, "config must not be called")
		})
	}
}

func TestRenameCycleToATakenNameIsConflictWithTheServiceSentence(t *testing.T) {
	svc := &fakeConfig{err: connect.NewError(connect.CodeAlreadyExists, errors.New("Ya existe un ciclo con ese nombre"))}
	h := handler.NewConfigHandler(svc)
	rec := doPathReq(t, h.RenameCycle, http.MethodPatch, "/api/v1/config/cycles/"+cycleA, cycleA, `{"name":"2026-2027"}`)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "Ya existe un ciclo con ese nombre")
}

// CC-101: the overlap refusal reaches the screen with its code and sentence.
func TestCreateCycleOverlapIsConflictWithItsCode(t *testing.T) {
	ce := connect.NewError(connect.CodeAlreadyExists, errors.New(`Las fechas se traslapan con el ciclo "2026-2027"`))
	ce.Meta().Set("X-Error-Code", "CYCLE_OVERLAP")
	svc := &fakeConfig{err: ce}
	h := handler.NewConfigHandler(svc)

	rec := doPathReq(t, h.CreateCycle, http.MethodPost, "/api/v1/config/cycles", "", `{"name":"2027","startsOn":"2027-01-10","endsOn":"2027-12-20"}`)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "CYCLE_OVERLAP", rec.Header().Get("X-Error-Code"))
	assert.Contains(t, rec.Body.String(), `"2026-2027"`)
}
