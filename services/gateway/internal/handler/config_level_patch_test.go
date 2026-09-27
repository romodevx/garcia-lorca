package handler_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// fakeScaleGuard answers how many captured grades a proposed scale refuses.
type fakeScaleGuard struct {
	violations int
	err        error
	calls      int
	gotLevel   string
	gotGrading client.LevelGrading
}

func (f *fakeScaleGuard) CountScaleViolations(_ context.Context, levelID string, g client.LevelGrading) (int, error) {
	f.calls++
	f.gotLevel, f.gotGrading = levelID, g
	return f.violations, f.err
}

const (
	validMode    = `"attendanceMode":"per_class"`
	validGrading = `"grading":{"enabled":true,"min":5,"max":10,"decimals":1,"passing":6}`
)

// PATCH /config/levels/{id} reaches config ONCE with every field, so mode and
// scale land together or not at all; a half-valid body reaches nobody.
func TestPatchLevelIsOneAtomicCall(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCode  int
		wantCalls int
	}{
		{"both fields in one call", `{` + validMode + `,` + validGrading + `}`, http.StatusOK, 1},
		{"valid mode + broken grading changes nothing", `{` + validMode + `,"grading":{"enabled":true,"min":5,"max":10,"decimals":1}}`, http.StatusBadRequest, 0},
		{"broken mode + valid grading changes nothing", `{"attendanceMode":"weekly",` + validGrading + `}`, http.StatusBadRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{}
			rec := patchLevel(handler.NewConfigHandler(svc).WithScaleGuard(&fakeScaleGuard{}), "lvl-1", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			assert.Equal(t, tt.wantCalls, svc.updateCalls)
		})
	}

	// Config refusing the scale refuses the whole patch: the mode is not
	// written on its own (the single call is config's transaction).
	svc := &fakeConfig{gradingErr: connect.NewError(connect.CodeInvalidArgument, errString("grading scale is inconsistent"))}
	rec := patchLevel(handler.NewConfigHandler(svc).WithScaleGuard(&fakeScaleGuard{}), "lvl-1", `{`+validMode+`,`+validGrading+`}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, svc.updateCalls, "one call, refused whole")
}

// Tightening (or disabling) a scale that captured grades of the active cycle
// would break is 409 SCALE_TIGHTENING and never reaches config; loosening, and
// a mode-only patch, pass.
func TestPatchLevelRefusesAScaleThatBreaksCapturedGrades(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		guard      *fakeScaleGuard
		wantCode   int
		wantHeader string
		wantCalls  int
		wantGuard  int
	}{
		{"violations", `{` + validGrading + `}`, &fakeScaleGuard{violations: 2}, http.StatusConflict, "SCALE_TIGHTENING", 0, 1},
		{"disable with grades", `{"grading":{"enabled":false,"min":5,"max":10,"decimals":1,"passing":6}}`, &fakeScaleGuard{violations: 1}, http.StatusConflict, "SCALE_TIGHTENING", 0, 1},
		{"loosen", `{` + validGrading + `}`, &fakeScaleGuard{}, http.StatusOK, "", 1, 1},
		{"mode only never asks grades", `{` + validMode + `}`, &fakeScaleGuard{violations: 9}, http.StatusOK, "", 1, 0},
		{"grades unreachable fails closed", `{` + validGrading + `}`, &fakeScaleGuard{err: connect.NewError(connect.CodeUnavailable, errors.New("down"))}, http.StatusInternalServerError, "", 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{}
			rec := patchLevel(handler.NewConfigHandler(svc).WithScaleGuard(tt.guard), "lvl-1", tt.body)
			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			assert.Equal(t, tt.wantHeader, rec.Header().Get("X-Error-Code"))
			assert.Equal(t, tt.wantCalls, svc.updateCalls)
			assert.Equal(t, tt.wantGuard, tt.guard.calls)
			if tt.wantCode == http.StatusConflict {
				assert.Contains(t, rec.Body.String(), "Ya hay calificaciones capturadas en este ciclo que no cumplirían la nueva escala")
				assert.Equal(t, "lvl-1", tt.guard.gotLevel)
			}
		})
	}
}
