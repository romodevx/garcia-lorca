package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestSetLevelGradingScale(t *testing.T) {
	custom, err := domain.NewGradingScale(true, 0, 10000, 7000, 2)
	require.NoError(t, err)
	tests := []struct {
		name     string
		scale    domain.GradingScale
		levelID  func(id string) string
		wantKind glerrors.Kind
	}{
		{name: "custom scale", scale: custom, levelID: same},
		{name: "zero scale", scale: domain.GradingScale{}, levelID: same, wantKind: glerrors.KindInvalid},
		{name: "unknown level", scale: custom, levelID: func(string) string { return "nope" }, wantKind: glerrors.KindNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newService(t)
			l := seedLevel(t, svc)
			got, err := svc.SetLevelGradingScale(ctx(), tt.levelID(l.ID()), tt.scale)
			if tt.wantKind != 0 {
				assert.Equal(t, tt.wantKind, glerrors.KindOf(err), "err = %v", err)
				assert.Equal(t, 1000, repo.levels[l.ID()].Grading().MaxHundredths(), "a rejected change was persisted")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.scale, got.Grading())
			assert.Equal(t, l.ID(), got.ID())
			assert.Equal(t, tt.scale, repo.levels[l.ID()].Grading())
		})
	}
}
