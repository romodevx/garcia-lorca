package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/grades/internal/domain"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func trimesters() []domain.EvaluationPeriod {
	return []domain.EvaluationPeriod{
		{ID: "p1", GradeLevelID: "g1", Ordinal: 1, Name: "1.er trimestre", StartsOn: date(2026, 8, 24), EndsOn: date(2026, 11, 30)},
		{ID: "p2", GradeLevelID: "g1", Ordinal: 2, Name: "2.º trimestre", StartsOn: date(2026, 12, 7), EndsOn: date(2027, 3, 26)},
		{ID: "p3", GradeLevelID: "g1", Ordinal: 3, Name: "3.er trimestre", StartsOn: date(2027, 4, 5), EndsOn: date(2027, 7, 9)},
	}
}

// The school's day is read on its own calendar (America/Mexico_City), never
// the UTC day: at 20:00 local on Nov 30 it is still Nov 30.
func TestSchoolDay(t *testing.T) {
	mx, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	evening := time.Date(2026, 12, 1, 2, 0, 0, 0, time.UTC) // 20:00 Nov 30 in Mexico City
	assert.Equal(t, date(2026, 11, 30), domain.SchoolDay(evening, mx))
	assert.Equal(t, date(2026, 12, 1), domain.SchoolDay(evening, time.UTC))
}

func TestPeriodOn(t *testing.T) {
	ps := trimesters()
	p, ok := domain.PeriodOn(ps, date(2026, 11, 30))
	require.True(t, ok)
	assert.Equal(t, "p1", p.ID, "both ends are inclusive")
	p, ok = domain.PeriodOn(ps, date(2026, 12, 7))
	require.True(t, ok)
	assert.Equal(t, "p2", p.ID)
	_, ok = domain.PeriodOn(ps, date(2026, 12, 3))
	assert.False(t, ok, "a day between two periods belongs to none")
}

// The sheet opens on the period of today; between periods, on the last one
// that already started; before the first, on the first.
func TestDefaultPeriod(t *testing.T) {
	ps := trimesters()
	cases := []struct {
		name string
		day  time.Time
		want string
	}{
		{"inside a period", date(2027, 1, 15), "p2"},
		{"in the gap after the first", date(2026, 12, 3), "p1"},
		{"before the cycle", date(2026, 7, 1), "p1"},
		{"after the last", date(2027, 8, 1), "p3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, ok := domain.DefaultPeriod(ps, c.day)
			require.True(t, ok)
			assert.Equal(t, c.want, p.ID)
		})
	}
	_, ok := domain.DefaultPeriod(nil, date(2027, 1, 1))
	assert.False(t, ok)
}

func TestEvaluationPeriodStartedAndEnded(t *testing.T) {
	p := trimesters()[1]
	assert.False(t, p.StartedBy(date(2026, 12, 6)))
	assert.True(t, p.StartedBy(date(2026, 12, 7)))
	assert.False(t, p.EndedBefore(date(2027, 3, 26)))
	assert.True(t, p.EndedBefore(date(2027, 3, 27)))
}

func TestFindPeriod(t *testing.T) {
	p, ok := domain.FindPeriod(trimesters(), "p3")
	require.True(t, ok)
	assert.Equal(t, 3, p.Ordinal)
	_, ok = domain.FindPeriod(trimesters(), "nope")
	assert.False(t, ok)
}
