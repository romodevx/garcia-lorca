package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// SchoolDay is the one rule attendance asks before taking a list (T14): a
// weekday inside the group's cycle that no closed period covers.
func TestSchoolDayIsAWeekdayOfTheGroupsCycleNotClosed(t *testing.T) {
	f, _, _, _ := anaWeek(t)
	f.closeDays(t, "Día del maestro", d(2026, time.October, 7), d(2026, time.October, 7))
	s := newScheduleService(f, duringCycle())

	tests := []struct {
		name string
		date time.Time
		want usecase.SchoolDayStatus
	}{
		{"a plain Monday", mon5, usecase.SchoolDayStatus{Open: true}},
		{"a Saturday", d(2026, time.October, 10), usecase.SchoolDayStatus{}},
		{"a Sunday", d(2026, time.October, 11), usecase.SchoolDayStatus{}},
		{"a closed weekday", d(2026, time.October, 7), usecase.SchoolDayStatus{Closed: "Día del maestro"}},
		{"a Monday before the cycle", d(2026, time.August, 17), usecase.SchoolDayStatus{}},
		{"a Monday after the cycle", d(2027, time.July, 12), usecase.SchoolDayStatus{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.SchoolDay(context.Background(), gr3A, tt.date)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSchoolDayNeedsAKnownGroup(t *testing.T) {
	f, _, _, _ := anaWeek(t)
	s := newScheduleService(f, duringCycle())

	_, err := s.SchoolDay(context.Background(), " ", mon5)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))

	_, err = s.SchoolDay(context.Background(), "ghost", mon5)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}
