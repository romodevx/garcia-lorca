package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// T5b part 2 (ADR 0035): a payment's DAY is stored apart from its instant.
// paid_at is when it was recorded (audit); paid_on is the school-local civil
// date every collection rule judges it on. A live payment at 19:00 in Morelia
// on the 15th is 01:00 UTC on the 16th; its day is the 15th.
var evening15UTC = time.Date(2026, time.October, 16, 1, 0, 0, 0, time.UTC)

func TestDepositAndPaymentCarryTheirDayApartFromTheInstant(t *testing.T) {
	cases := []struct {
		name string
		day  time.Time
		want string
	}{
		{"the school's day at midnight UTC", time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC), "2026-10-15"},
		{"a day carrying a stray clock time keeps its date", time.Date(2026, time.October, 15, 13, 30, 0, 0, time.UTC), "2026-10-15"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			d, err := domain.NewDeposit("dep-1", "t1", "st-1", 100000, domain.MethodCash, "", evening15UTC, "", "")
			require.NoError(t, err)
			p, err := domain.NewPayment("pay-1", "t1", "ch-1", 100000, domain.MethodCash, "", evening15UTC, "")
			require.NoError(t, err)

			d = d.WithPaidOn(tt.day)
			p = p.WithPaidOn(tt.day)

			for label, got := range map[string]time.Time{"deposit": d.PaidOn(), "payment": p.PaidOn()} {
				assert.Equal(t, tt.want, got.Format("2006-01-02"), label)
				assert.Equal(t, time.UTC, got.Location(), label+": midnight UTC like every stored date")
				assert.Zero(t, got.Hour()+got.Minute()+got.Second(), label+": a date, not an instant")
			}
			assert.True(t, d.PaidAt().Equal(evening15UTC), "the instant is untouched")
			assert.True(t, p.PaidAt().Equal(evening15UTC), "the instant is untouched")
		})
	}
}

// A deposit nobody dated has no day: the repository refuses to store it rather
// than guess one from the instant (the guess is the bug this column removes).
func TestADepositWithoutADayHasAZeroPaidOn(t *testing.T) {
	d, err := domain.NewDeposit("dep-1", "t1", "st-1", 100000, domain.MethodCash, "", evening15UTC, "", "")
	require.NoError(t, err)
	assert.True(t, d.PaidOn().IsZero())
}

// The correction history shows the day the office saw, not the UTC date of
// the instant.
func TestASnapshotKeepsTheDepositsDay(t *testing.T) {
	d, err := domain.NewDeposit("dep-1", "t1", "st-1", 100000, domain.MethodCash, "", evening15UTC, "", "")
	require.NoError(t, err)
	s := domain.SnapshotOf(d.WithPaidOn(time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)), nil)
	assert.Equal(t, "2026-10-15", s.PaidOn.Format("2006-01-02"))
	assert.True(t, s.PaidAt.Equal(evening15UTC))
}
