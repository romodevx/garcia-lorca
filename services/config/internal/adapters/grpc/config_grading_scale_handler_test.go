package grpcadapter_test

import (
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// Every level read carries its grading scale, and SetLevelGradingScale replaces
// it and answers with the updated level.
func TestSetLevelGradingScaleRoundTrip(t *testing.T) {
	h, _ := newHandler(t)
	id := seedLevel(t, h)

	got, err := h.GetLevel(ctx(), connect.NewRequest(&configv1.GetLevelRequest{Id: id}))
	require.NoError(t, err)
	assert.Equal(t, &configv1.GradingScale{Enabled: true, MinHundredths: 500, MaxHundredths: 1000, Decimals: 0, PassingHundredths: 600},
		got.Msg.GetLevel().GetGrading(), "a new primary level grades 5–10, whole numbers, 6 to pass")

	want := &configv1.GradingScale{Enabled: true, MinHundredths: 0, MaxHundredths: 10000, Decimals: 2, PassingHundredths: 7000}
	res, err := h.SetLevelGradingScale(ctx(), connect.NewRequest(&configv1.SetLevelGradingScaleRequest{Id: id, Grading: want}))
	require.NoError(t, err)
	assert.Equal(t, id, res.Msg.GetLevel().GetId())
	assert.Equal(t, want.String(), res.Msg.GetLevel().GetGrading().String())

	list, err := h.ListLevels(ctx(), connect.NewRequest(&configv1.ListLevelsRequest{}))
	require.NoError(t, err)
	assert.Equal(t, want.String(), list.Msg.GetLevels()[0].GetGrading().String())
}

func TestSetLevelGradingScaleErrors(t *testing.T) {
	valid := &configv1.GradingScale{Enabled: true, MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}
	tests := []struct {
		name     string
		id       func(seeded string) string
		grading  *configv1.GradingScale
		wantCode connect.Code
	}{
		{"missing scale", func(s string) string { return s }, nil, connect.CodeInvalidArgument},
		{"passing above max", func(s string) string { return s }, &configv1.GradingScale{Enabled: true, MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 1100}, connect.CodeInvalidArgument},
		{"unknown level", func(string) string { return "missing" }, valid, connect.CodeNotFound},
		{"numbers and letters at once", func(s string) string { return s }, &configv1.GradingScale{Enabled: true, Letters: true, MinHundredths: 500, MaxHundredths: 1000, PassingHundredths: 600}, connect.CodeInvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newHandler(t)
			id := seedLevel(t, h)
			_, err := h.SetLevelGradingScale(ctx(), connect.NewRequest(&configv1.SetLevelGradingScaleRequest{Id: tt.id(id), Grading: tt.grading}))
			assert.Equal(t, tt.wantCode, connect.CodeOf(err), "err = %v", err)
		})
	}
}

// A level switched to letters reads back with letters and without numbers.
func TestSetLevelGradingScaleToLetters(t *testing.T) {
	h, _ := newHandler(t)
	id := seedLevel(t, h)
	want := &configv1.GradingScale{Letters: true, MinHundredths: 500, MaxHundredths: 1000, PassingHundredths: 600}
	res, err := h.SetLevelGradingScale(ctx(), connect.NewRequest(&configv1.SetLevelGradingScaleRequest{Id: id, Grading: want}))
	require.NoError(t, err)
	assert.Equal(t, want.String(), res.Msg.GetLevel().GetGrading().String())
}
