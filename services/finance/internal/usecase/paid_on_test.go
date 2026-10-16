package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T5b part 2 · ADR 0035 — a payment's DAY is stored apart from its instant
// (deposits.paid_on, payments.paid_on). A live payment at 19:00 in Morelia on
// the 15th is recorded at 01:00 UTC on the 16th and dated the 15th.

const day15 = "2026-10-15"

// A live deposit (no date given) and a live direct payment are dated on the
// school's today, and so is every payment a deposit splits into.
func TestALivePaymentIsDatedOnTheSchoolsDay(t *testing.T) {
	for _, tt := range schoolNight {
		t.Run(tt.name, func(t *testing.T) {
			f := depositFixture(t)
			f.clock = tt.instant

			got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
				StudentID: testStudent, AmountCents: 100000, Method: domain.MethodCash, IdempotencyKey: "live",
			})
			require.NoError(t, err)
			assert.Equal(t, tt.today, dateOf(got.Deposit.PaidOn()), "the deposit's day")
			assert.True(t, got.Deposit.PaidAt().Equal(tt.instant), "paid_at stays the audit instant")
			require.NotEmpty(t, got.Allocations)
			for _, a := range got.Allocations {
				assert.Equal(t, tt.today, dateOf(a.PaidOn()), "an allocation carries its deposit's day")
			}

			charge := chargeDueOn(t, f, "2026-10-12")
			payment, _, err := f.svc.RegisterPayment(context.Background(), charge.Charge.ID(), 100, domain.MethodCash, "", "")
			require.NoError(t, err)
			assert.Equal(t, tt.today, dateOf(payment.PaidOn()), "a direct payment's day")
		})
	}
}

// A day picked at the counter is that day, whatever the hour it was keyed.
func TestACounterDateIsTheDayPicked(t *testing.T) {
	f := depositFixture(t)
	f.clock = evening15
	picked := ymdUTC(2026, time.October, 14)

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 100000, Method: domain.MethodCash, PaidAt: picked,
	})
	require.NoError(t, err)
	assert.Equal(t, "2026-10-14", dateOf(got.Deposit.PaidOn()))
}

// "No future date" compares DAYS: at 19:00 on the 15th in Morelia, picking the
// 16th is tomorrow and is refused (it used to pass, because midnight UTC of the
// 16th is an instant already in the past); picking the 15th is today.
func TestPickingTomorrowIsRefusedOnTheSchoolsDay(t *testing.T) {
	cases := []struct {
		name    string
		clock   time.Time
		picked  time.Time
		refused bool
	}{
		{"the 16th at 19:00 on the 15th", evening15, ymdUTC(2026, time.October, 16), true},
		{"the 16th at 23:59 on the 15th", lastMinute15, ymdUTC(2026, time.October, 16), true},
		{"the 15th at 19:00 on the 15th", evening15, ymdUTC(2026, time.October, 15), false},
		{"the 16th at local midnight", localMidnight, ymdUTC(2026, time.October, 16), false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := depositFixture(t)
			f.clock = tt.clock
			charge := chargeDueOn(t, f, "2026-10-12")

			_, depErr := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
				StudentID: testStudent, AmountCents: 100000, Method: domain.MethodCash, PaidAt: tt.picked,
			})
			_, _, payErr := f.svc.RegisterPaymentAt(context.Background(), charge.Charge.ID(), 100,
				domain.MethodCash, "", "", tt.picked, "")
			for label, err := range map[string]error{"deposit": depErr, "payment": payErr} {
				if tt.refused {
					assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), label)
					continue
				}
				assert.NoError(t, err, label)
			}
		})
	}
}

// A correction that does not change the date keeps the original's DAY — not
// the UTC date of its instant — and is judged again on it: October paid with
// the early reward at 19:00 on the last day of the window keeps the reward
// when the amount is corrected two days later.
func TestACorrectionWithoutANewDateKeepsTheDay(t *testing.T) {
	for _, tt := range schoolNight {
		t.Run(tt.name, func(t *testing.T) {
			f := earlyFixture(t)
			f.clock = tt.instant
			reward := int64(10000)
			if !isLocal15(tt.today) {
				reward = 0
			}
			october := chargeDueOn(t, f, "2026-10-12")
			paid, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
				StudentID: testStudent, AmountCents: 300000 - reward, Method: domain.MethodCash,
				IdempotencyKey: "short-" + tt.today, ChargeIDs: []string{october.Charge.ID()},
			})
			require.NoError(t, err)

			f.clock = time.Date(2026, time.October, 17, 18, 0, 0, 0, time.UTC)
			got, err := f.svc.CorrectDeposit(context.Background(), usecase.CorrectionInput{
				DepositID: paid.Deposit.ID(), AmountCents: 320000 - reward,
				Tenders:   []usecase.TenderInput{{Method: domain.MethodCash, AmountCents: 320000 - reward}},
				ChargeIDs: []string{october.Charge.ID()}, Reason: "el importe era otro",
			})
			require.NoError(t, err)
			assert.Equal(t, tt.today, dateOf(got.Corrected.Deposit.PaidOn()), "the corrected deposit keeps the day")
			assert.True(t, got.Corrected.Deposit.PaidAt().Equal(tt.instant), "and the instant")
			after := chargeDueOn(t, f, "2026-10-12")
			assert.Equal(t, domain.StatusPaid, after.Charge.Status(), "judged on the original day")
			assert.Equal(t, reward, after.Charge.Discount().Int64(), "the reward of that day")
		})
	}
}

// The statement dates a live deposit on its day.
func TestTheStatementDatesALiveDepositOnItsDay(t *testing.T) {
	f := depositFixture(t)
	f.clock = evening15
	paid, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 100000, Method: domain.MethodCash,
	})
	require.NoError(t, err)

	st, err := f.svc.GetStatement(context.Background(), testStudent)
	require.NoError(t, err)
	found := false
	for _, m := range st.Movements {
		if m.DepositID == paid.Deposit.ID() {
			found = true
			assert.Equal(t, day15, dateOf(m.Date))
		}
	}
	assert.True(t, found, "the deposit is on the statement")
}

// The folio counter only knows years 2000..3000 (migration 0018), so a
// mistyped year picked at the counter (0202-10-01, 1999-12-31) must be refused
// as invalid input — not reach the counter and fail as a 500.
func TestPickingADayBeforeTheEarliestIsRefused(t *testing.T) {
	cases := []struct {
		name    string
		picked  time.Time
		refused bool
	}{
		{"1999-12-31", ymdUTC(1999, time.December, 31), true},
		{"0202-10-01", ymdUTC(202, time.October, 1), true},
		{"2000-01-01", ymdUTC(2000, time.January, 1), false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := depositFixture(t)
			f.clock = evening15
			charge := chargeDueOn(t, f, "2026-10-12")

			_, depErr := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
				StudentID: testStudent, AmountCents: 100000, Method: domain.MethodCash, PaidAt: tt.picked,
			})
			_, _, payErr := f.svc.RegisterPaymentAt(context.Background(), charge.Charge.ID(), 100,
				domain.MethodCash, "", "", tt.picked, "")
			for label, err := range map[string]error{"deposit": depErr, "payment": payErr} {
				if tt.refused {
					assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), label)
					continue
				}
				assert.NoError(t, err, label)
			}
		})
	}
}
