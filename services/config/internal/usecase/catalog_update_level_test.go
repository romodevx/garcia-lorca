package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// UpdateLevel validates every present field before writing anything: a body
// whose mode is valid but whose scale is not must leave the level untouched.
func TestUpdateLevel(t *testing.T) {
	perClass, daily := domain.AttendancePerClass, domain.AttendanceDaily
	bad := domain.AttendanceModeUnspecified
	custom, err := domain.NewGradingScale(true, 0, 10000, 7000, 2)
	require.NoError(t, err)
	zero := domain.GradingScale{}
	tests := []struct {
		name      string
		patch     ports.LevelPatch
		levelID   func(string) string
		wantKind  glerrors.Kind
		wantMode  domain.AttendanceMode
		wantScale func(before domain.GradingScale) domain.GradingScale
	}{
		{name: "both fields", patch: ports.LevelPatch{AttendanceMode: &daily, Grading: &custom}, levelID: same,
			wantMode: daily, wantScale: func(domain.GradingScale) domain.GradingScale { return custom }},
		{name: "mode only keeps the scale", patch: ports.LevelPatch{AttendanceMode: &daily}, levelID: same,
			wantMode: daily, wantScale: func(b domain.GradingScale) domain.GradingScale { return b }},
		{name: "scale only keeps the mode", patch: ports.LevelPatch{Grading: &custom}, levelID: same,
			wantMode: perClass, wantScale: func(domain.GradingScale) domain.GradingScale { return custom }},
		{name: "valid mode + invalid scale changes nothing", patch: ports.LevelPatch{AttendanceMode: &daily, Grading: &zero},
			levelID: same, wantKind: glerrors.KindInvalid},
		{name: "invalid mode + valid scale changes nothing", patch: ports.LevelPatch{AttendanceMode: &bad, Grading: &custom},
			levelID: same, wantKind: glerrors.KindInvalid},
		{name: "empty patch", patch: ports.LevelPatch{}, levelID: same, wantKind: glerrors.KindInvalid},
		{name: "unknown level", patch: ports.LevelPatch{AttendanceMode: &daily}, levelID: func(string) string { return "nope" },
			wantKind: glerrors.KindNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newService(t)
			l := seedLevel(t, svc)
			l, err := svc.SetLevelAttendanceMode(ctx(), l.ID(), perClass)
			require.NoError(t, err)
			before := repo.levels[l.ID()]

			got, err := svc.UpdateLevel(ctx(), tt.levelID(l.ID()), tt.patch)
			if tt.wantKind != 0 {
				assert.Equal(t, tt.wantKind, glerrors.KindOf(err), "err = %v", err)
				assert.Equal(t, before, repo.levels[l.ID()], "a refused patch must change nothing")
				if tt.wantKind == glerrors.KindInvalid {
					assert.Equal(t, 0, repo.levelPatches, "an invalid patch must not reach the repository")
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantMode, got.AttendanceMode())
			assert.Equal(t, tt.wantScale(before.Grading()), got.Grading())
			assert.Equal(t, 1, repo.levelPatches, "both fields must be written in ONE repository call")
		})
	}
}
