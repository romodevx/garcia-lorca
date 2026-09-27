package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestNewGradingScale(t *testing.T) {
	tests := []struct {
		name                     string
		enabled                  bool
		min, max, passing, decim int
		wantErr                  bool
	}{
		{"default SEP scale", true, 500, 1000, 600, 1, false},
		{"passing equals max", true, 500, 1000, 1000, 0, false},
		{"zero based, hundred top", true, 0, 10000, 7000, 2, false},
		{"disabled keeps the numbers", false, 500, 1000, 600, 1, false},
		{"min equals passing", true, 600, 1000, 600, 1, true},
		{"passing above max", true, 500, 1000, 1100, 1, true},
		{"negative min", true, -100, 1000, 600, 1, true},
		{"max above one hundred", true, 500, 10100, 600, 1, true},
		{"three decimals", true, 500, 1000, 600, 3, true},
		{"negative decimals", true, 500, 1000, 600, -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := domain.NewGradingScale(tt.enabled, tt.min, tt.max, tt.passing, tt.decim)
			if tt.wantErr {
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "err = %v", err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.enabled, s.Enabled())
			assert.Equal(t, tt.min, s.MinHundredths())
			assert.Equal(t, tt.max, s.MaxHundredths())
			assert.Equal(t, tt.passing, s.PassingHundredths())
			assert.Equal(t, tt.decim, s.Decimals())
		})
	}
}

// A new level grades on the school's default scale (5–10, one decimal, 6 to
// pass) unless its name says it is an early-years level, which takes no numeric
// capture. The name is matched without case, accents or padding.
func TestNewLevelDefaultsGradingByName(t *testing.T) {
	tests := []struct {
		name        string
		wantEnabled bool
	}{
		{"Preescolar", false},
		{" PREESCOLAR ", false},
		{"Kinder", false},
		{"Kínder", false},
		{"Maternal", false},
		{"Primaria", true},
		{"Secundaria", true},
		{"Preparatoria", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := domain.NewLevel("l1", "t1", tt.name, "", domain.GranularityCycle)
			require.NoError(t, err)
			g := l.Grading()
			assert.Equal(t, tt.wantEnabled, g.Enabled())
			assert.Equal(t, 500, g.MinHundredths())
			assert.Equal(t, 1000, g.MaxHundredths())
			assert.Equal(t, 600, g.PassingHundredths())
			assert.Equal(t, 1, g.Decimals())
		})
	}
}

func TestLevelWithGrading(t *testing.T) {
	l, err := domain.NewLevel("l1", "t1", "Primaria", "", domain.GranularityCycle)
	require.NoError(t, err)
	scale, err := domain.NewGradingScale(true, 0, 10000, 7000, 2)
	require.NoError(t, err)

	changed, err := l.WithGrading(scale)
	require.NoError(t, err)
	assert.Equal(t, scale, changed.Grading())
	assert.Equal(t, 1000, l.Grading().MaxHundredths(), "the receiver must not change")

	_, err = l.WithGrading(domain.GradingScale{})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "a zero scale is not a scale")
}
