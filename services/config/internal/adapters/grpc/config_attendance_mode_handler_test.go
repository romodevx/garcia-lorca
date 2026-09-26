package grpcadapter_test

import (
	"testing"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// Every level read carries its attendance mode, and SetLevelAttendanceMode
// changes it and answers with the updated level.
func TestSetLevelAttendanceModeRoundTrip(t *testing.T) {
	h, _ := newHandler(t)
	id := seedLevel(t, h)

	got, err := h.GetLevel(ctx(), connect.NewRequest(&configv1.GetLevelRequest{Id: id}))
	if err != nil {
		t.Fatalf("GetLevel: %v", err)
	}
	if got.Msg.GetLevel().GetAttendanceMode() != configv1.AttendanceMode_ATTENDANCE_MODE_DAILY {
		t.Errorf("a new primary level must read back daily, got %v", got.Msg.GetLevel().GetAttendanceMode())
	}

	res, err := h.SetLevelAttendanceMode(ctx(), connect.NewRequest(&configv1.SetLevelAttendanceModeRequest{
		Id: id, AttendanceMode: configv1.AttendanceMode_ATTENDANCE_MODE_PER_CLASS,
	}))
	if err != nil {
		t.Fatalf("SetLevelAttendanceMode: %v", err)
	}
	if res.Msg.GetLevel().GetAttendanceMode() != configv1.AttendanceMode_ATTENDANCE_MODE_PER_CLASS || res.Msg.GetLevel().GetId() != id {
		t.Errorf("response level = %v", res.Msg.GetLevel())
	}
	list, err := h.ListLevels(ctx(), connect.NewRequest(&configv1.ListLevelsRequest{}))
	if err != nil {
		t.Fatalf("ListLevels: %v", err)
	}
	if list.Msg.GetLevels()[0].GetAttendanceMode() != configv1.AttendanceMode_ATTENDANCE_MODE_PER_CLASS {
		t.Errorf("list must carry the new mode, got %v", list.Msg.GetLevels()[0].GetAttendanceMode())
	}
}

func TestSetLevelAttendanceModeErrors(t *testing.T) {
	tests := []struct {
		name     string
		id       func(seeded string) string
		mode     configv1.AttendanceMode
		wantCode connect.Code
	}{
		{"unspecified mode", func(s string) string { return s }, configv1.AttendanceMode_ATTENDANCE_MODE_UNSPECIFIED, connect.CodeInvalidArgument},
		{"unknown enum value", func(s string) string { return s }, configv1.AttendanceMode(42), connect.CodeInvalidArgument},
		{"unknown level", func(string) string { return "missing" }, configv1.AttendanceMode_ATTENDANCE_MODE_DAILY, connect.CodeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newHandler(t)
			id := seedLevel(t, h)
			_, err := h.SetLevelAttendanceMode(ctx(), connect.NewRequest(&configv1.SetLevelAttendanceModeRequest{
				Id: tt.id(id), AttendanceMode: tt.mode,
			}))
			if connect.CodeOf(err) != tt.wantCode {
				t.Errorf("code = %v, want %v (err %v)", connect.CodeOf(err), tt.wantCode, err)
			}
		})
	}
}
