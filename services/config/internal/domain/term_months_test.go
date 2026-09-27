package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func month(y int, m time.Month) time.Time { return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC) }

func TestParseMonth(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{in: "2026-09", want: month(2026, time.September)},
		{in: " 2027-01 ", want: month(2027, time.January)},
		{in: "2026-13", wantErr: true},
		{in: "2026-9", wantErr: true},
		{in: "2026-09-01", wantErr: true},
		{in: "", wantErr: true},
		{in: "sept", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := domain.ParseMonth(tt.in)
			if tt.wantErr {
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
				return
			}
			require.NoError(t, err)
			assert.True(t, tt.want.Equal(got), "got %v", got)
		})
	}
}

func TestFormatMonth(t *testing.T) {
	assert.Equal(t, "2026-09", domain.FormatMonth(month(2026, time.September)))
	assert.Equal(t, "2027-01", domain.FormatMonth(month(2027, time.January)))
}

func newTestTerm(t *testing.T) domain.Term {
	t.Helper()
	term, err := domain.NewTerm("t1", "ten", "c1", "l1", "Trimestre 1", 1,
		time.Date(2026, time.August, 24, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	return term
}

func TestTermWithMonthsSortsAndDedupes(t *testing.T) {
	term := newTestTerm(t)
	assert.Empty(t, term.Months(), "a new term holds no months")

	got, err := term.WithMonths([]time.Time{
		month(2026, time.November), month(2026, time.September), month(2026, time.October), month(2026, time.September),
	})
	require.NoError(t, err)
	assert.Equal(t, []time.Time{month(2026, time.September), month(2026, time.October), month(2026, time.November)}, got.Months())
	assert.Empty(t, term.Months(), "WithMonths returns a copy; the receiver is unchanged")
}

func TestTermWithMonthsRejectsANonFirstDay(t *testing.T) {
	_, err := newTestTerm(t).WithMonths([]time.Time{time.Date(2026, time.September, 2, 0, 0, 0, 0, time.UTC)})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
}

func TestTermMonthsIsACopy(t *testing.T) {
	got, err := newTestTerm(t).WithMonths([]time.Time{month(2026, time.September)})
	require.NoError(t, err)
	ms := got.Months()
	ms[0] = month(2030, time.January)
	assert.Equal(t, month(2026, time.September), got.Months()[0])
}

func TestTermWithMonthsEmptyClears(t *testing.T) {
	full, err := newTestTerm(t).WithMonths([]time.Time{month(2026, time.September)})
	require.NoError(t, err)
	cleared, err := full.WithMonths(nil)
	require.NoError(t, err)
	assert.Empty(t, cleared.Months())
}

func TestCycleContainsMonth(t *testing.T) {
	cycle, err := domain.NewCycle("c1", "ten", "2026-2027", domain.CycleStatusActive,
		time.Date(2026, time.August, 24, 0, 0, 0, 0, time.UTC),
		time.Date(2027, time.July, 9, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	tests := []struct {
		name string
		m    time.Time
		want bool
	}{
		{"the month the cycle starts in, though it starts late in it", month(2026, time.August), true},
		{"a middle month", month(2026, time.December), true},
		{"the month the cycle ends in", month(2027, time.July), true},
		{"the month before", month(2026, time.July), false},
		{"the month after", month(2027, time.August), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cycle.ContainsMonth(tt.m))
		})
	}
}

func TestMonthRefusalsCarryTheirCodes(t *testing.T) {
	sep := month(2026, time.September)

	inOther := domain.MonthInOtherTerm(sep)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(inOther))
	assert.Equal(t, domain.CodeMonthInOtherTerm, domain.RefusalCodeOf(inOther))
	assert.Equal(t, "Ese mes ya pertenece a otro periodo del nivel: 2026-09", glerrors.SafeMessage(inOther))

	outside := domain.MonthOutsideCycle(sep)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(outside))
	assert.Equal(t, domain.CodeMonthOutsideCycle, domain.RefusalCodeOf(outside))
	assert.Equal(t, "El mes está fuera del ciclo: 2026-09", glerrors.SafeMessage(outside))

	assert.Equal(t, "Ese mes ya pertenece a otro periodo del nivel", glerrors.SafeMessage(domain.MonthInOtherTerm(time.Time{})),
		"a month the adapter could not name is left out, not printed as 0001-01")
	assert.Equal(t, domain.RefusalCode(""), domain.RefusalCodeOf(glerrors.New(glerrors.KindConflict, "x")))
}

func TestFirstMonthInOtherTerm(t *testing.T) {
	base := newTestTerm(t)
	mk := func(id, level string, ms ...time.Time) domain.Term {
		tt, err := domain.NewTerm(id, "ten", "c1", level, id, 2, base.StartsOn(), base.EndsOn())
		require.NoError(t, err)
		tt, err = tt.WithMonths(ms)
		require.NoError(t, err)
		return tt
	}
	self, err := base.WithMonths([]time.Time{month(2026, time.September), month(2026, time.October), month(2026, time.November)})
	require.NoError(t, err)

	tests := []struct {
		name     string
		siblings []domain.Term
		want     time.Time
		found    bool
	}{
		{"no siblings", nil, time.Time{}, false},
		{"the term itself never conflicts", []domain.Term{mk("t1", "l1", month(2026, time.September))}, time.Time{}, false},
		{"another level never conflicts", []domain.Term{mk("t9", "l2", month(2026, time.September))}, time.Time{}, false},
		{"the earliest shared month is named",
			[]domain.Term{mk("t2", "l1", month(2026, time.November)), mk("t3", "l1", month(2026, time.October))},
			month(2026, time.October), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := domain.FirstMonthInOtherTerm(self, tt.siblings)
			assert.Equal(t, tt.found, found)
			assert.True(t, tt.want.Equal(got), "got %v", got)
		})
	}
}

// A term covers at most six months: longer ones are refused (TERM_TOO_LONG),
// counted after de-duplication.
func TestTermWithMonthsCapsAtSixMonths(t *testing.T) {
	term := newTestTerm(t)
	six := []time.Time{
		month(2026, time.September), month(2026, time.October), month(2026, time.November),
		month(2026, time.December), month(2027, time.January), month(2027, time.February),
	}
	got, err := term.WithMonths(append(six, month(2026, time.September)))
	require.NoError(t, err, "a duplicate does not count twice")
	assert.Len(t, got.Months(), 6)

	_, err = term.WithMonths(append(six, month(2027, time.March)))
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeTermTooLong, domain.RefusalCodeOf(err))
	assert.Equal(t, "Un periodo puede tener hasta 6 meses", glerrors.SafeMessage(err))
}
