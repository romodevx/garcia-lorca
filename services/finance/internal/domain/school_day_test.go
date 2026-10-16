package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// The school's day is Mexico City's wall-clock day, never the UTC one (ADR
// 0035). Mexico has kept UTC-6 all year since 2022, but the offset comes from
// the tz database, never from a constant.
func TestSchoolDateIsTheLocalCalendarDay(t *testing.T) {
	mexico, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	oct15 := time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)
	oct16 := time.Date(2026, time.October, 16, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		instant time.Time
		zone    *time.Location
		want    time.Time
	}{
		{"19:00 local on the 15th is still the 15th", time.Date(2026, time.October, 16, 1, 0, 0, 0, time.UTC), mexico, oct15},
		{"23:59 local on the 15th is still the 15th", time.Date(2026, time.October, 16, 5, 59, 59, 0, time.UTC), mexico, oct15},
		{"00:00 local on the 16th is the 16th", time.Date(2026, time.October, 16, 6, 0, 0, 0, time.UTC), mexico, oct16},
		{"an instant given in local time reads the same", time.Date(2026, time.October, 15, 19, 0, 0, 0, mexico), mexico, oct15},
		{"a UTC zone keeps the UTC day", time.Date(2026, time.October, 16, 1, 0, 0, 0, time.UTC), time.UTC, oct16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.SchoolDate(tt.instant, tt.zone)
			assert.True(t, got.Equal(tt.want), "SchoolDate = %s, want %s", got, tt.want)
			// The representation every stored finance date uses: midnight UTC.
			assert.Equal(t, time.UTC, got.Location())
			assert.Equal(t, tt.want.Format("2006-01-02"), got.Format("2006-01-02"))
		})
	}
}
