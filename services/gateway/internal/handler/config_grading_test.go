package handler_test

import (
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// PATCH /config/levels/{id} takes the grading scale in the units a person types
// (5, 9.5) and hands config exact hundredths; it may carry attendanceMode,
// grading or both.
func TestConfigPatchLevelGrading(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    int
		wantGrading client.LevelGrading
		wantMode    string
		wantMsg     string
	}{
		{name: "grading only", body: `{"grading":{"enabled":true,"letters":false,"min":5,"max":10,"decimals":1,"passing":6}}`, wantCode: http.StatusOK,
			wantGrading: client.LevelGrading{Enabled: true, Scale: client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}}},
		{name: "decimal marks", body: `{"grading":{"enabled":true,"letters":false,"min":0,"max":100,"decimals":2,"passing":69.5}}`, wantCode: http.StatusOK,
			wantGrading: client.LevelGrading{Enabled: true, Scale: client.GradeScale{MinHundredths: 0, MaxHundredths: 10000, Decimals: 2, PassingHundredths: 6950}}},
		{name: "both fields", body: `{"attendanceMode":"per_class","grading":{"enabled":false,"letters":false,"min":5,"max":10,"decimals":1,"passing":6}}`, wantCode: http.StatusOK,
			wantGrading: client.LevelGrading{Enabled: false, Scale: client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}}, wantMode: "per_class"},
		{name: "letters", body: `{"grading":{"enabled":false,"letters":true,"min":5,"max":10,"decimals":0,"passing":6}}`, wantCode: http.StatusOK,
			wantGrading: client.LevelGrading{Letters: true, Scale: client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, PassingHundredths: 600}}},
		{name: "missing field", body: `{"grading":{"enabled":true,"letters":false,"min":5,"max":10,"decimals":1}}`, wantCode: http.StatusBadRequest,
			wantMsg: "grading requiere enabled, min, max, decimals y passing"},
		{name: "string mark", body: `{"grading":{"enabled":true,"letters":false,"min":"5","max":10,"decimals":1,"passing":6}}`, wantCode: http.StatusBadRequest,
			wantMsg: "grading requiere enabled, min, max, decimals y passing"},
		{name: "three-decimal mark", body: `{"grading":{"enabled":true,"letters":false,"min":5.001,"max":10,"decimals":1,"passing":6}}`, wantCode: http.StatusBadRequest,
			wantMsg: "La escala admite hasta 2 decimales"},
		{name: "empty body", body: `{}`, wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{}
			rec := patchLevel(handler.NewConfigHandler(svc), "lvl-1", tt.body)
			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			if tt.wantCode != http.StatusOK {
				assert.Empty(t, svc.gradingLevelID, "a refused change must not reach config")
				assert.Empty(t, svc.modeLevelID)
				if tt.wantMsg != "" {
					assert.Contains(t, rec.Body.String(), tt.wantMsg)
				}
				return
			}
			assert.Equal(t, "lvl-1", svc.gradingLevelID)
			assert.Equal(t, tt.wantGrading, svc.gradingIn)
			assert.Equal(t, tt.wantMode, svc.modeIn)
		})
	}
}

func TestConfigPatchLevelGradingResponseIsInPersonUnits(t *testing.T) {
	svc := &fakeConfig{}
	rec := patchLevel(handler.NewConfigHandler(svc), "lvl-1", `{"grading":{"enabled":true,"letters":false,"min":5,"max":10,"decimals":1,"passing":6.5}}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"id":"lvl-1","name":"Primaria","sepCode":"","sortOrder":0,"enrollmentGranularity":"cycle","attendanceMode":"daily",
		"grading":{"enabled":true,"letters":false,"min":5,"max":10,"decimals":1,"passing":6.5}}`, rec.Body.String())
}

func TestConfigPatchLevelGradingRefusalCarriesConfigSentence(t *testing.T) {
	svc := &fakeConfig{gradingErr: connect.NewError(connect.CodeInvalidArgument, errString("La calificación mínima debe ser menor que la aprobatoria"))}
	rec := patchLevel(handler.NewConfigHandler(svc), "lvl-1", `{"grading":{"enabled":true,"letters":false,"min":6,"max":10,"decimals":1,"passing":6}}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "La calificación mínima debe ser menor que la aprobatoria")
}

// A PATCH that leaves "letters" out keeps what the level holds: an older
// screen saving the scale must not quietly drop preescolar's letters.
func TestConfigPatchLevelWithoutLettersKeepsThem(t *testing.T) {
	svc := &fakeConfig{levels: []client.LevelResult{{ID: "lvl-1", Name: "PREESCOLAR", Grading: client.LevelGrading{Letters: true}}}}
	rec := patchLevel(handler.NewConfigHandler(svc), "lvl-1", `{"grading":{"enabled":false,"min":5,"max":10,"decimals":0,"passing":6}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, svc.gradingIn.Letters, "absent keeps the stored letters")

	rec = patchLevel(handler.NewConfigHandler(svc), "lvl-1", `{"grading":{"enabled":true,"min":5,"max":10,"decimals":0,"passing":6}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.False(t, svc.gradingIn.Letters, "numbers on: never letters too")

	rec = patchLevel(handler.NewConfigHandler(svc), "lvl-1", `{"grading":{"enabled":false,"letters":false,"min":5,"max":10,"decimals":0,"passing":6}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.False(t, svc.gradingIn.Letters, "an explicit false turns letters off")
}
