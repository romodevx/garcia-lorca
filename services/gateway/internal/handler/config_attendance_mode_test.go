package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

func patchLevel(h *handler.ConfigHandler, id, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/config/levels/"+id, strings.NewReader(body))
	r.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.PatchLevel(rec, r)
	return rec
}

func TestConfigSetLevelAttendanceMode(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantMode string
	}{
		{"per class", `{"attendanceMode":"per_class"}`, http.StatusOK, "per_class"},
		{"daily", `{"attendanceMode":"daily"}`, http.StatusOK, "daily"},
		{"unknown mode", `{"attendanceMode":"weekly"}`, http.StatusBadRequest, ""},
		{"missing mode", `{}`, http.StatusBadRequest, ""},
		{"malformed body", `{`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{}
			rec := patchLevel(handler.NewConfigHandler(svc), "lvl-1", tt.body)
			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			if tt.wantCode != http.StatusOK {
				assert.Empty(t, svc.modeLevelID, "a refused change must not reach config")
				return
			}
			assert.Equal(t, "lvl-1", svc.modeLevelID)
			assert.JSONEq(t, `{"id":"lvl-1","name":"Secundaria","sepCode":"","enrollmentGranularity":"cycle","attendanceMode":"`+tt.wantMode+`",
				"grading":{"enabled":false,"min":0,"max":0,"decimals":0,"passing":0}}`, rec.Body.String())
		})
	}
}

func TestConfigSetLevelAttendanceModeUnknownLevelIs404(t *testing.T) {
	svc := &fakeConfig{err: connect.NewError(connect.CodeNotFound, nil)}
	rec := patchLevel(handler.NewConfigHandler(svc), "ghost", `{"attendanceMode":"daily"}`)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// Every level read carries its attendance mode.
func TestConfigListLevelsCarriesAttendanceMode(t *testing.T) {
	svc := &fakeConfig{levels: []client.LevelResult{{ID: "lvl-2", Name: "Secundaria", Granularity: "cycle", AttendanceMode: "per_class"}}}
	rec := doReq(t, handler.NewConfigHandler(svc).ListLevels, http.MethodGet, "/api/v1/config/levels", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"attendanceMode":"per_class"`)
}
