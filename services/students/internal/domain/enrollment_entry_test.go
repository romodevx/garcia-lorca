package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// T9. The alta's entry day: between the cycle's first day and the school's
// today, stored as 00:00 in the school's zone so every reader that takes the
// school day of it gets the day the office typed.
func TestEntryInstant(t *testing.T) {
	zone, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	cycleStart := day(2026, time.August, 24)
	today := day(2026, time.October, 16)

	for _, tc := range []struct {
		name string
		on   time.Time
		ok   bool
	}{
		{"the cycle's first day", day(2026, time.August, 24), true},
		{"a day in between", day(2026, time.September, 15), true},
		{"today", today, true},
		{"the day before the cycle", day(2026, time.August, 23), false},
		{"tomorrow", day(2026, time.October, 17), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.EntryInstant(tc.on, today, cycleStart, zone)
			if !tc.ok {
				require.Error(t, err)
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
				assert.Contains(t, err.Error(), "La fecha de ingreso debe estar entre el inicio del ciclo (24 ago 2026) y hoy.")
				return
			}
			require.NoError(t, err)
			want := time.Date(tc.on.Year(), tc.on.Month(), tc.on.Day(), 0, 0, 0, 0, zone)
			assert.True(t, got.Equal(want), "got %s, want %s", got, want)
			assert.Equal(t, tc.on.Format("2006-01-02"), got.In(zone).Format("2006-01-02"))
		})
	}
}

// The school's today is the Morelia date of the instant: 01:00 UTC on the 16th
// is still the 15th there.
func TestSchoolDate(t *testing.T) {
	zone, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	got := domain.SchoolDate(time.Date(2026, time.October, 16, 1, 0, 0, 0, time.UTC), zone)
	assert.Equal(t, day(2026, time.October, 15), got)
}

func TestShortSpanishDate(t *testing.T) {
	assert.Equal(t, "24 ago 2026", domain.ShortSpanishDate(day(2026, time.August, 24)))
	assert.True(t, strings.HasPrefix(domain.ShortSpanishDate(day(2027, time.January, 3)), "3 ene"))
}
