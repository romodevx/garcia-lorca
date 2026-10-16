package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T5b · D8 · ADR 0035 — finance's "today" is Morelia's calendar day.
//
// Three instants on the night of 15→16 October 2026. Mexico City is UTC-6 all
// year (no DST since 2022), taken from the tz database:
//
//	01:00 UTC on the 16th = 19:00 on the 15th in Morelia
//	05:59 UTC on the 16th = 23:59 on the 15th
//	06:00 UTC on the 16th = 00:00 on the 16th
//
// Before ADR 0035 the first two were already the 16th.
var (
	evening15     = time.Date(2026, time.October, 16, 1, 0, 0, 0, time.UTC)
	lastMinute15  = time.Date(2026, time.October, 16, 5, 59, 0, 0, time.UTC)
	localMidnight = time.Date(2026, time.October, 16, 6, 0, 0, 0, time.UTC)
)

// schoolNight is the boundary table every test below walks.
var schoolNight = []struct {
	name    string
	instant time.Time
	today   string
}{
	{"19:00 in Morelia on the 15th", evening15, "2026-10-15"},
	{"23:59 in Morelia on the 15th", lastMinute15, "2026-10-15"},
	{"00:00 in Morelia on the 16th", localMidnight, "2026-10-16"},
}

func isLocal15(today string) bool { return today == "2026-10-15" }

// The daily surcharge counts Morelia's days: at 19:00 on the 15th the 16th has
// not begun, so it has not cost the family a day. September (due Thursday the
// 10th) is 35 days late on the 15th; October (the 10th is a Saturday, so due
// Monday the 12th) is 3. At local midnight each earns one more.
// The account says which day it was worked out for.
func TestTheDailySurchargeAndAsOfFollowTheSchoolsDay(t *testing.T) {
	for _, tt := range schoolNight {
		t.Run(tt.name, func(t *testing.T) {
			f := depositFixture(t)
			f.clock = tt.instant
			want := int64((35 + 3) * 1000)
			if !isLocal15(tt.today) {
				want = (36 + 4) * 1000
			}

			expected, err := f.svc.ExpectedToday(context.Background(), testStudent, time.Time{})
			require.NoError(t, err)
			assert.Equal(t, want, expected.SurchargeCents, "the counter's surcharge")

			account, err := f.svc.GetAccount(context.Background(), testStudent)
			require.NoError(t, err)
			assert.Equal(t, want, account.SurchargeCents, "the office's account")
			assert.Equal(t, tt.today, account.AsOf)

			family, err := f.svc.GetFamilyAccount(context.Background(), testStudent)
			require.NoError(t, err)
			assert.Equal(t, tt.today, family.AsOf, "the app's \"Al {fecha}\"")
		})
	}
}

// A charge due on the 15th is due TODAY all evening of the 15th in Morelia, not
// overdue; the overdue read is cut at the school's day.
func TestListOverdueIsCutAtTheSchoolsDay(t *testing.T) {
	for _, tt := range schoolNight {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			concept := f.seedConcept(t, "Colegiatura", 250000)
			due15, err := f.svc.CreateCharge(context.Background(), usecase.CreateChargeInput{
				StudentID: testStudent, CycleID: testCycle, ConceptID: concept.ID(), DueDate: "2026-10-15",
			})
			require.NoError(t, err)
			f.clock = tt.instant

			got, err := f.svc.ListOverdue(context.Background(), testCycle)
			require.NoError(t, err)

			assert.Equal(t, tt.today, f.repo.overdueToday.Format("2006-01-02"), "the cutoff handed to the read")
			assert.True(t, f.repo.overdueToday.Equal(ymdUTC(f.repo.overdueToday.Year(), f.repo.overdueToday.Month(), f.repo.overdueToday.Day())),
				"the cutoff is a date at midnight UTC, comparable with due_date")
			if isLocal15(tt.today) {
				assert.Empty(t, got, "due today is not overdue")
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, due15.Charge.ID(), got[0].Charge.ID())
		})
	}
}

// earlyFixture: a window of 11 working days from the 1st, which in October
// 2026 (Thursday the 1st) ends on Thursday the 15th. October's tuition falls
// due on Monday the 12th (the 10th is a Saturday).
func earlyFixture(t *testing.T) *fixture {
	t.Helper()
	f := billingFixture(t)
	savePlan(t, f, usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		Policy: domain.CollectionPolicy{EarlyPaymentCents: 10000, EarlyPaymentDays: 11},
	})
	runOn(t, f, "2026-10-01")
	settleInscription(t, f)
	return f
}

// The early-payment reward lasts the whole of its last day in Morelia: at 23:59
// on the 15th a family still pays October at $3,100, at the counter's quote and
// at the deposit that defaults to "now".
func TestTheEarlyPaymentWindowLastsTheWholeLocalDay(t *testing.T) {
	for _, tt := range schoolNight {
		t.Run(tt.name, func(t *testing.T) {
			f := earlyFixture(t)
			f.clock = tt.instant
			want := int64(10000)
			if !isLocal15(tt.today) {
				want = 0
			}

			expected, err := f.svc.ExpectedToday(context.Background(), testStudent, time.Time{})
			require.NoError(t, err)
			assert.Equal(t, want, expected.DiscountCents, "the quote's reward")

			october := chargeDueOn(t, f, "2026-10-12")
			_, err = f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
				StudentID: testStudent, AmountCents: 320000 - want, Method: domain.MethodCash,
				IdempotencyKey: "early-" + tt.today, ChargeIDs: []string{october.Charge.ID()},
			})
			require.NoError(t, err)
			paid := chargeDueOn(t, f, "2026-10-12")
			assert.Equal(t, domain.StatusPaid, paid.Charge.Status(), "the money settles October")
			assert.Equal(t, want, paid.Charge.Discount().Int64(), "the deposit's reward")
		})
	}
}

// chargeDueOn is the pupil's tuition charge due on that date.
func chargeDueOn(t *testing.T, f *fixture, due string) ports.ChargeWithPaid {
	t.Helper()
	charges, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	require.NoError(t, err)
	tuition := f.repo.conceptIDOfKind(domain.KindTuition)
	for _, c := range charges {
		if c.Charge.ConceptID() == tuition && c.Charge.DueDate().Format("2006-01-02") == due {
			return c
		}
	}
	t.Fatalf("no tuition charge due %s", due)
	return ports.ChargeWithPaid{}
}

// A run without a date bills as of Morelia's day: exactly what the same run
// dated by hand with that day bills. The monthly surcharge of a month due on
// the 16th accrues at local midnight, not at 18:00 the evening before.
func TestRunBillingDefaultsToTheSchoolsDay(t *testing.T) {
	monthlyOn16 := func(t *testing.T) *fixture {
		f := billingFixture(t)
		savePlan(t, f, usecase.TuitionPlanInput{
			CycleID: testCycle, LevelID: testLevel,
			EnrollmentCents: 50000, MonthlyCents: 320000,
			Months: 10, FirstBilledMonth: 9, DueDay: 16, LateFeeCents: 15000, Active: true,
		})
		return f
	}
	dated := map[string]int{}
	for _, day := range []string{"2026-10-15", "2026-10-16"} {
		dated[day] = runOn(t, monthlyOn16(t), day).LateFeesCreated
	}
	require.Equal(t, dated["2026-10-15"]+1, dated["2026-10-16"], "September's fee accrues on the 16th")

	for _, tt := range schoolNight {
		t.Run(tt.name, func(t *testing.T) {
			f := monthlyOn16(t)
			f.clock = tt.instant
			assert.Equal(t, dated[tt.today], runOn(t, f, "").LateFeesCreated)
		})
	}
}

// The family sees a charge from the day it opens in Morelia: an inscription
// window that opens on the 16th is still closed at 23:59 on the 15th.
func TestAChargeOpensOnTheSchoolsDay(t *testing.T) {
	for _, tt := range schoolNight {
		t.Run(tt.name, func(t *testing.T) {
			f := billingFixture(t)
			savePlan(t, f, windowPlan(ymdUTC(2026, time.October, 16), ymdUTC(2026, time.October, 30)))
			runOn(t, f, "2026-10-01")
			f.clock = tt.instant

			account, err := f.svc.GetFamilyAccount(context.Background(), testStudent)
			require.NoError(t, err)
			enrolment := f.repo.conceptIDOfKind(domain.KindEnrollment)
			shown := false
			for _, c := range account.Charges {
				shown = shown || c.Charge.ConceptID() == enrolment
			}
			assert.Equal(t, !isLocal15(tt.today), shown, "the inscription is in play from the 16th")
		})
	}
}

// A scholarship given and an activity joined without a date start on
// Morelia's day.
func TestHoldingsStartOnTheSchoolsDay(t *testing.T) {
	for _, tt := range schoolNight {
		t.Run(tt.name, func(t *testing.T) {
			b := newBecaFixture(t, 0, noPolicy)
			b.clock = tt.instant
			b.give(t, "Excelencia", becaSpec{kind: domain.ScholarshipPercent, value: 5000}, testStudent)
			require.Len(t, b.srepo.holdings, 1)
			assert.Equal(t, tt.today, b.srepo.holdings[0].StartsOn.Format("2006-01-02"), "the scholarship")

			a := newActivityFixture(t)
			a.clock = tt.instant
			swim := a.create(t, swimmingInput())
			_, err := a.svcA.Enroll(context.Background(), swim.ID(), time.Time{},
				[]usecase.EnrollItem{{StudentID: testStudent, OptionID: swim.Options()[0].ID}})
			require.NoError(t, err)
			require.Len(t, a.activities.enrollments, 1)
			assert.Equal(t, tt.today, a.activities.enrollments[0].StartsOn().Format("2006-01-02"), "the activity")
		})
	}
}

// An expense is refused when dated after Morelia's today, and the cash cut
// opens on Morelia's day.
func TestTheCashCutAndExpensesUseTheSchoolsDay(t *testing.T) {
	for _, tt := range schoolNight {
		t.Run(tt.name, func(t *testing.T) {
			f := newExpenseFixture()
			f.clock = tt.instant
			c := f.category(t, "Papelería")
			_, err := f.svc.CreateExpense(context.Background(), domain.ExpenseDraft{
				SpentOn: ymdUTC(2026, time.October, 16), CategoryID: c.ID(), AmountCents: 1000,
				Method: domain.MethodCash, Description: "hojas",
			}, cashier)
			if isLocal15(tt.today) {
				assert.Error(t, err, "the 16th is still in the future")
			} else {
				assert.NoError(t, err)
			}

			_, err = f.svc.CashCut(context.Background(), time.Time{})
			require.NoError(t, err)
			assert.Equal(t, tt.today, f.takings.askedOn.Format("2006-01-02"))
		})
	}
}
