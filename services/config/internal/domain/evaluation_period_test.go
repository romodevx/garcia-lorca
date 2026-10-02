package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func testCycle(t *testing.T) domain.Cycle {
	t.Helper()
	c, err := domain.NewCycle("c1", "ten", "2026-2027", domain.CycleStatusActive, day(2026, time.August, 24), day(2027, time.July, 9))
	require.NoError(t, err)
	return c
}

func period(t *testing.T, id string, ordinal int, from, to time.Time) domain.EvaluationPeriod {
	t.Helper()
	p, err := domain.NewEvaluationPeriod(id, "ten", "c1", "g1", "Trimestre", ordinal, from, to)
	require.NoError(t, err)
	return p
}

func TestNewEvaluationPeriodValidates(t *testing.T) {
	from, to := day(2026, time.September, 1), day(2026, time.November, 30)
	tests := []struct {
		name     string
		id       string
		grade    string
		label    string
		ordinal  int
		from, to time.Time
		wantCode domain.RefusalCode
	}{
		{name: "id required", id: "", grade: "g1", label: "T1", ordinal: 1, from: from, to: to},
		{name: "grade required", id: "p1", grade: " ", label: "T1", ordinal: 1, from: from, to: to},
		{name: "name required", id: "p1", grade: "g1", label: "  ", ordinal: 1, from: from, to: to, wantCode: domain.CodePeriodNameRequired},
		{name: "name too long", id: "p1", grade: "g1", label: strings.Repeat("x", domain.MaxPeriodNameLen+1), ordinal: 1, from: from, to: to},
		{name: "ordinal zero", id: "p1", grade: "g1", label: "T1", ordinal: 0, from: from, to: to},
		{name: "ordinal above max", id: "p1", grade: "g1", label: "T1", ordinal: domain.MaxEvaluationPeriods + 1, from: from, to: to},
		{name: "ends before it starts", id: "p1", grade: "g1", label: "T1", ordinal: 1, from: to, to: from, wantCode: domain.CodePeriodEndsBeforeStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewEvaluationPeriod(tt.id, "ten", "c1", tt.grade, tt.label, tt.ordinal, tt.from, tt.to)
			require.Error(t, err)
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(err))
			}
		})
	}
}

func TestNewEvaluationPeriodTrimsAndKeepsOnlyTheDate(t *testing.T) {
	// A browser in Mexico sends 2026-09-01T06:00Z for "1 de septiembre": only
	// the calendar date counts.
	p, err := domain.NewEvaluationPeriod("p1", "ten", "c1", "g1", "  1.er trimestre ", 1,
		time.Date(2026, time.September, 1, 6, 0, 0, 0, time.UTC),
		time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err, "a one-day period is valid")
	assert.Equal(t, "1.er trimestre", p.Name())
	assert.True(t, p.StartsOn().Equal(day(2026, time.September, 1)))
	assert.True(t, p.EndsOn().Equal(day(2026, time.September, 1)))
	assert.Equal(t, "", p.TermID(), "a period starts with no term link")
	assert.Equal(t, "t9", p.WithTermID("t9").TermID())
}

func TestCheckPeriodSetAcceptsAdjacentPeriodsInsideTheCycle(t *testing.T) {
	cycle := testCycle(t)
	set := []domain.EvaluationPeriod{
		period(t, "p2", 2, day(2026, time.December, 1), day(2027, time.March, 31)),
		period(t, "p1", 1, day(2026, time.August, 24), day(2026, time.November, 30)),
		period(t, "p3", 3, day(2027, time.April, 1), day(2027, time.July, 9)),
	}
	assert.NoError(t, domain.CheckPeriodSet(cycle, set))
	assert.NoError(t, domain.CheckPeriodSet(cycle, nil), "an empty set clears the grade")
}

func TestCheckPeriodSetRefusals(t *testing.T) {
	cycle := testCycle(t)
	tests := []struct {
		name     string
		set      []domain.EvaluationPeriod
		wantKind glerrors.Kind
		wantCode domain.RefusalCode
		wantText string
	}{
		{
			name:     "starts before the cycle",
			set:      []domain.EvaluationPeriod{period(t, "p1", 1, day(2026, time.August, 23), day(2026, time.November, 30))},
			wantKind: glerrors.KindInvalid, wantCode: domain.CodePeriodOutsideCycle,
		},
		{
			name:     "ends after the cycle",
			set:      []domain.EvaluationPeriod{period(t, "p1", 1, day(2027, time.April, 1), day(2027, time.July, 10))},
			wantKind: glerrors.KindInvalid, wantCode: domain.CodePeriodOutsideCycle,
		},
		{
			name: "two periods share a day",
			set: []domain.EvaluationPeriod{
				period(t, "p1", 1, day(2026, time.September, 1), day(2026, time.November, 30)),
				period(t, "p2", 2, day(2026, time.November, 30), day(2027, time.March, 31)),
			},
			wantKind: glerrors.KindConflict, wantCode: domain.CodePeriodOverlap, wantText: "1",
		},
		{
			name: "one period inside another, listed out of order",
			set: []domain.EvaluationPeriod{
				period(t, "p3", 3, day(2026, time.October, 1), day(2026, time.October, 15)),
				period(t, "p1", 1, day(2026, time.September, 1), day(2026, time.December, 20)),
				period(t, "p2", 2, day(2027, time.January, 5), day(2027, time.March, 31)),
			},
			wantKind: glerrors.KindConflict, wantCode: domain.CodePeriodOverlap,
		},
		{
			name: "an ordinal twice",
			set: []domain.EvaluationPeriod{
				period(t, "p1", 1, day(2026, time.September, 1), day(2026, time.October, 31)),
				period(t, "p2", 1, day(2026, time.November, 1), day(2026, time.December, 20)),
			},
			wantKind: glerrors.KindInvalid, wantCode: domain.CodePeriodOrdinalTaken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.CheckPeriodSet(cycle, tt.set)
			require.Error(t, err)
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err))
			assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(err))
			assert.Contains(t, glerrors.SafeMessage(err), tt.wantText)
		})
	}
}

func TestCheckPeriodSetRefusesTooManyPeriods(t *testing.T) {
	cycle := testCycle(t)
	set := make([]domain.EvaluationPeriod, 0, domain.MaxEvaluationPeriods+1)
	start := day(2026, time.September, 1)
	for i := 0; i <= domain.MaxEvaluationPeriods; i++ {
		d := start.AddDate(0, 0, i*7)
		p, err := domain.NewEvaluationPeriod("p", "ten", "c1", "g1", "P", 1+i%domain.MaxEvaluationPeriods, d, d)
		require.NoError(t, err)
		set = append(set, p)
	}
	err := domain.CheckPeriodSet(cycle, set)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeTooManyPeriods, domain.RefusalCodeOf(err))
}

func TestPeriodOn(t *testing.T) {
	set := []domain.EvaluationPeriod{
		period(t, "p1", 1, day(2026, time.August, 24), day(2026, time.November, 30)),
		period(t, "p2", 2, day(2026, time.December, 1), day(2027, time.March, 31)),
	}
	other, err := domain.NewEvaluationPeriod("o1", "ten", "c1", "g2", "Otro", 1, day(2026, time.August, 24), day(2027, time.July, 9))
	require.NoError(t, err)
	set = append(set, other)

	tests := []struct {
		name   string
		grade  string
		on     time.Time
		wantID string
	}{
		{name: "first day is inside", grade: "g1", on: day(2026, time.August, 24), wantID: "p1"},
		{name: "last day is inside", grade: "g1", on: day(2026, time.November, 30), wantID: "p1"},
		{name: "the next day is the next period", grade: "g1", on: day(2026, time.December, 1), wantID: "p2"},
		{name: "the time of day does not matter", grade: "g1", on: time.Date(2026, time.November, 30, 23, 59, 0, 0, time.UTC), wantID: "p1"},
		{name: "the calendar date of the given zone counts", grade: "g1",
			on: time.Date(2026, time.November, 30, 20, 0, 0, 0, time.FixedZone("CST", -6*3600)), wantID: "p1"},
		{name: "another grade has its own periods", grade: "g2", on: day(2027, time.May, 1), wantID: "o1"},
		{name: "a gap belongs to no period", grade: "g1", on: day(2027, time.May, 1)},
		{name: "an unknown grade has none", grade: "g3", on: day(2026, time.September, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := domain.PeriodOn(set, tt.grade, tt.on)
			if tt.wantID == "" {
				assert.False(t, ok)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tt.wantID, got.ID())
		})
	}
}
