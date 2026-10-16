package usecase_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// The service's one clock answers two different questions: the instant, to
// stamp what happened, and the school's day, to decide what is due.
func TestSchoolClockTodayIsTheSchoolsDay(t *testing.T) {
	tests := []struct {
		name    string
		instant time.Time
		today   string
	}{
		{"19:00 in Morelia on the 15th", time.Date(2026, time.October, 16, 1, 0, 0, 0, time.UTC), "2026-10-15"},
		{"one second before local midnight", time.Date(2026, time.October, 16, 5, 59, 59, 0, time.UTC), "2026-10-15"},
		{"local midnight", time.Date(2026, time.October, 16, 6, 0, 0, 0, time.UTC), "2026-10-16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := usecase.NewSchoolClock(func() time.Time { return tt.instant }, schoolZone)
			assert.True(t, clock.Now().Equal(tt.instant), "Now must be the instant, untouched")
			assert.Equal(t, tt.today, clock.Today().Format("2006-01-02"))
			assert.Equal(t, time.UTC, clock.Today().Location())
		})
	}
}

// A clock wired without its zone is a composition-root bug: it must not
// quietly fall back to the UTC day this clock exists to replace.
func TestSchoolClockRefusesAMissingPiece(t *testing.T) {
	assert.Panics(t, func() { usecase.NewSchoolClock(time.Now, nil) })
	assert.Panics(t, func() { usecase.NewSchoolClock(nil, schoolZone) })
}
