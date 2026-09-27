package grpcadapter_test

import (
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// UpdateLevel applies mode and scale together; a half-valid request is refused
// whole and the level reads back unchanged.
func TestUpdateLevelHandler(t *testing.T) {
	daily := configv1.AttendanceMode_ATTENDANCE_MODE_DAILY
	unspecified := configv1.AttendanceMode_ATTENDANCE_MODE_UNSPECIFIED
	valid := &configv1.GradingScale{Enabled: true, MinHundredths: 0, MaxHundredths: 10000, Decimals: 2, PassingHundredths: 7000}
	broken := &configv1.GradingScale{Enabled: true, MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 1100}
	tests := []struct {
		name     string
		mode     *configv1.AttendanceMode
		grading  *configv1.GradingScale
		wantCode connect.Code
	}{
		{name: "both", mode: &daily, grading: valid},
		{name: "mode only", mode: &daily},
		{name: "scale only", grading: valid},
		{name: "valid mode, broken scale", mode: &daily, grading: broken, wantCode: connect.CodeInvalidArgument},
		{name: "unspecified mode, valid scale", mode: &unspecified, grading: valid, wantCode: connect.CodeInvalidArgument},
		{name: "nothing", wantCode: connect.CodeInvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newHandler(t)
			id := seedLevel(t, h)
			before, err := h.GetLevel(ctx(), connect.NewRequest(&configv1.GetLevelRequest{Id: id}))
			require.NoError(t, err)

			res, err := h.UpdateLevel(ctx(), connect.NewRequest(&configv1.UpdateLevelRequest{Id: id, AttendanceMode: tt.mode, Grading: tt.grading}))
			if tt.wantCode != 0 {
				assert.Equal(t, tt.wantCode, connect.CodeOf(err), "err = %v", err)
				after, err := h.GetLevel(ctx(), connect.NewRequest(&configv1.GetLevelRequest{Id: id}))
				require.NoError(t, err)
				assert.Equal(t, before.Msg.GetLevel().String(), after.Msg.GetLevel().String(), "a refused request must change nothing")
				return
			}
			require.NoError(t, err)
			got := res.Msg.GetLevel()
			if tt.mode != nil {
				assert.Equal(t, *tt.mode, got.GetAttendanceMode())
			} else {
				assert.Equal(t, before.Msg.GetLevel().GetAttendanceMode(), got.GetAttendanceMode())
			}
			if tt.grading != nil {
				assert.Equal(t, tt.grading.String(), got.GetGrading().String())
			} else {
				assert.Equal(t, before.Msg.GetLevel().GetGrading().String(), got.GetGrading().String())
			}
		})
	}
}

func TestUpdateLevelHandlerUnknownLevel(t *testing.T) {
	h, _ := newHandler(t)
	daily := configv1.AttendanceMode_ATTENDANCE_MODE_DAILY
	_, err := h.UpdateLevel(ctx(), connect.NewRequest(&configv1.UpdateLevelRequest{Id: "missing", AttendanceMode: &daily}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
