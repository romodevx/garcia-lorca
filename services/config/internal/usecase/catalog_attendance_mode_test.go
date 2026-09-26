package usecase_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestSetLevelAttendanceMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     domain.AttendanceMode
		levelID  func(id string) string
		wantKind glerrors.Kind
		wantMode domain.AttendanceMode
	}{
		{name: "switch to per class", mode: domain.AttendancePerClass, levelID: same, wantMode: domain.AttendancePerClass},
		{name: "keep daily", mode: domain.AttendanceDaily, levelID: same, wantMode: domain.AttendanceDaily},
		{name: "unspecified mode", mode: domain.AttendanceModeUnspecified, levelID: same, wantKind: glerrors.KindInvalid},
		{name: "unknown level", mode: domain.AttendancePerClass, levelID: func(string) string { return "nope" }, wantKind: glerrors.KindNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newService(t)
			l := seedLevel(t, svc) // Primaria ⇒ daily
			got, err := svc.SetLevelAttendanceMode(ctx(), tt.levelID(l.ID()), tt.mode)
			if tt.wantKind != 0 {
				if glerrors.KindOf(err) != tt.wantKind {
					t.Fatalf("want %v, got %v", tt.wantKind, err)
				}
				if repo.levels[l.ID()].AttendanceMode() != domain.AttendanceDaily {
					t.Errorf("a rejected change was persisted")
				}
				return
			}
			if err != nil {
				t.Fatalf("SetLevelAttendanceMode: %v", err)
			}
			if got.AttendanceMode() != tt.wantMode || got.ID() != l.ID() || got.Name() != l.Name() {
				t.Errorf("returned level = %+v", got)
			}
			if repo.levels[l.ID()].AttendanceMode() != tt.wantMode {
				t.Errorf("persisted mode = %v, want %v", repo.levels[l.ID()].AttendanceMode(), tt.wantMode)
			}
		})
	}
}

func same(id string) string { return id }
