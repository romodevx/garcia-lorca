package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T5 (ISSUE-006): ONE balance figure. The office's account, its ledger, the
// family's account, the family's ledger and the counter's "del año debe" are
// five readings of the same number: opened charges at the counter's price
// (ladder, scholarship), plus the surcharge the daily rule has earned to today,
// less discounts and every peso received — negative when the school holds the
// family's money.

// balanceReadings are the five surfaces that show a balance.
type balanceReadings struct {
	account, statement, familyAccount, familyStatement, counter int64
}

func readBalances(t *testing.T, f *fixture) balanceReadings {
	t.Helper()
	ctx := context.Background()
	account, err := f.svc.GetAccount(ctx, testStudent)
	require.NoError(t, err)
	family, err := f.svc.GetFamilyAccount(ctx, testStudent)
	require.NoError(t, err)
	counter, err := f.svc.ExpectedToday(ctx, testStudent, time.Time{})
	require.NoError(t, err)
	return balanceReadings{
		account:         account.TotalBalanceCents,
		statement:       statementOf(t, f, testStudent).Totals.BalanceCents,
		familyAccount:   family.TotalBalanceCents,
		familyStatement: familyStatementOf(t, f, testStudent).Totals.BalanceCents,
		counter:         counter.AnnualBalanceCents,
	}
}

func requireOneBalance(t *testing.T, f *fixture, want int64) {
	t.Helper()
	got := readBalances(t, f)
	assert.Equal(t, balanceReadings{want, want, want, want, want}, got,
		"every surface must read the statement's balance")
}

// onlySeptember re-saves the fixture's table billing one month, so a case's
// arithmetic is a line or two.
func onlySeptember(t *testing.T, f *fixture, in usecase.TuitionPlanInput) {
	t.Helper()
	in.CycleID, in.LevelID = testCycle, testLevel
	in.Months, in.FirstBilledMonth, in.DueDay, in.Active = 1, 9, 10, true
	savePlan(t, f, in)
}

// septemberTuition is the pupil's September tuition charge id.
func septemberTuition(t *testing.T, f *fixture) string {
	t.Helper()
	tuition := f.repo.conceptIDOfKind(domain.KindTuition)
	for _, c := range f.repo.charges {
		if c.ConceptID() == tuition && c.DueDate().Month() == time.September {
			return c.ID()
		}
	}
	t.Fatal("no September tuition")
	return ""
}

func withScholarships(t *testing.T, f *fixture) *becaFixture {
	t.Helper()
	srepo := newFakeScholarshipRepo(f.repo)
	activities := newFakeActivityRepo()
	f.svc.WithScholarships(srepo)
	becas := usecase.NewScholarshipService(srepo, f.svc, f.repo, activities, f.students, testTenant,
		seqID("beca"), func() time.Time { return f.clock })
	return &becaFixture{fixture: f, becas: becas, srepo: srepo, activity: activities}
}

func noon(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }

func TestEverySurfaceReadsTheStatementsBalance(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T) *fixture
		want  int64
	}{
		{
			// $3,200 due 10 Sep, $1,000 paid on the 20th, read on 10 Oct: thirty
			// days at $10 accrued and not written down.
			name: "daily surcharge accrued on a partly paid overdue month",
			setup: func(t *testing.T) *fixture {
				f := dailyFixture(t)
				onlySeptember(t, f, usecase.TuitionPlanInput{
					EnrollmentCents: 50000, MonthlyCents: 320000,
					Policy: domain.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true},
				})
				runOn(t, f, "2026-10-10")
				settleInscription(t, f)
				f.clock = noon(2026, time.October, 10)
				_, _, err := f.svc.RegisterPaymentAt(context.Background(), septemberTuition(t, f),
					100000, domain.MethodCash, "", "", ymdUTC(2026, time.September, 20), "")
				require.NoError(t, err)
				return f
			},
			want: 320000 - 100000 + 30*1000,
		},
		{
			// A 50% scholarship on the $3,000 month, read before its due date:
			// the month is owed at the scholarship's $1,500.
			name: "scholarship in time",
			setup: func(t *testing.T) *fixture {
				f := billingFixture(t)
				onlySeptember(t, f, usecase.TuitionPlanInput{EnrollmentCents: 50000, MonthlyCents: monthlyFee})
				runOn(t, f, "2026-09-01")
				settleInscription(t, f)
				f.clock = noon(2026, time.September, 3)
				b := withScholarships(t, f)
				b.give(t, "Media", becaSpec{kind: domain.ScholarshipPercent, value: 5000})
				return f
			},
			want: monthlyFee / 2,
		},
		{
			// September and October billed, the inscription settled on time,
			// and by 15 Dec the monthly run has written $150 per completed
			// overdue month: September three (Oct, Nov, Dec), October two.
			// October is then cancelled (the pupil left) while its surcharge
			// rows stayed. A cancelled month's surcharge is owed by nobody.
			name: "cancelled month's surcharge does not count",
			setup: func(t *testing.T) *fixture {
				f := billingFixture(t)
				in := usecase.TuitionPlanInput{EnrollmentCents: 50000, MonthlyCents: 320000, LateFeeCents: 15000}
				in.CycleID, in.LevelID = testCycle, testLevel
				in.Months, in.FirstBilledMonth, in.DueDay, in.Active = 2, 9, 10, true
				savePlan(t, f, in)
				runOn(t, f, "2026-09-01")
				settleInscription(t, f)
				runOn(t, f, "2026-12-15")
				f.clock = noon(2026, time.December, 15)
				cancelMonth(t, f, time.October)
				return f
			},
			want: 320000 + 3*15000,
		},
		{
			// $6,000 handed over against $3,200 of September plus its thirty
			// days: the rest is the family's credit.
			name: "unallocated credit is a negative balance",
			setup: func(t *testing.T) *fixture {
				f := dailyFixture(t)
				onlySeptember(t, f, usecase.TuitionPlanInput{
					EnrollmentCents: 50000, MonthlyCents: 320000,
					Policy: domain.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true},
				})
				runOn(t, f, "2026-10-10")
				settleInscription(t, f)
				f.clock = noon(2026, time.October, 10)
				deposit(t, f, 600000, "2026-10-10")
				return f
			},
			want: -(600000 - 320000 - 30*1000),
		},
		{
			// Inscription issued at $2,200; its second rung ($3,300) started on
			// 1 Nov. Read on 5 Nov unpaid, it is owed at today's rung.
			name: "unpaid inscription at the ladder's price today",
			setup: func(t *testing.T) *fixture {
				f := billingFixture(t)
				onlySeptember(t, f, usecase.TuitionPlanInput{
					MonthlyCents: 320000,
					EnrollmentConcepts: []domain.EnrollmentConcept{{Name: "Inscripción", Tiers: []domain.PriceTier{
						{EffectiveFrom: ymdUTC(2026, time.August, 1), AmountCents: 220000},
						{EffectiveFrom: ymdUTC(2026, time.November, 1), AmountCents: 330000},
					}}},
				})
				runOn(t, f, "2026-09-15")
				f.clock = noon(2026, time.November, 5)
				return f
			},
			want: 330000 + 320000,
		},
		{
			// The inscription opens on 1 June; read on 20 May, the office lists
			// it but nobody owes it yet. The year's ten months are billed up
			// front and carry no window: they are the balance.
			name: "a charge not yet open is outside every balance",
			setup: func(t *testing.T) *fixture {
				f := windowFixture(t)
				runOn(t, f, "2026-05-15")
				f.clock = ymdUTC(2026, time.May, 20)
				return f
			},
			want: 10 * 320000,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.setup(t)
			requireOneBalance(t, f, tc.want)
		})
	}
}

// The QA case (ISSUE-006): tuition $3,500 with a 50% scholarship, $1,000 paid in
// time, the rest late — the scholarship is lost. Due on the 10th, read on the
// 20th (all days count): ten days at $20. Every surface says $2,700, the $200
// surcharge included.
func TestTheLostScholarshipCaseReadsOneBalanceWithItsSurcharge(t *testing.T) {
	f := billingFixture(t)
	onlySeptember(t, f, usecase.TuitionPlanInput{
		EnrollmentCents: 50000, MonthlyCents: 350000,
		Policy: domain.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 2000, SurchargeCountsAllDays: true},
	})
	runOn(t, f, "2026-09-01")
	settleInscription(t, f)
	f.clock = noon(2026, time.September, 3)
	b := withScholarships(t, f)
	b.give(t, "Media", becaSpec{kind: domain.ScholarshipPercent, value: 5000})
	deposit(t, f, 100000, "2026-09-03")
	f.clock = noon(2026, time.September, 20)

	requireOneBalance(t, f, 350000-100000+10*2000)

	st := statementOf(t, f, testStudent)
	var accrued int64
	for _, m := range movementsOf(st, domain.MovementSurcharge) {
		if m.Accrued {
			accrued += m.AmountCents
		}
	}
	assert.Equal(t, int64(20000), accrued, "the $200 surcharge is a line of the ledger")
}

// cancelMonth cancels the tuition month in place, leaving the surcharges
// written on it untouched — the state a withdrawal leaves when the month's
// surcharge row was not eligible to cancel.
func cancelMonth(t *testing.T, f *fixture, m time.Month) {
	t.Helper()
	tuition := f.repo.conceptIDOfKind(domain.KindTuition)
	for id, c := range f.repo.charges {
		if c.ConceptID() == tuition && c.DueDate().Month() == m {
			cancelled, err := c.Cancel()
			require.NoError(t, err)
			f.repo.charges[id] = cancelled
			return
		}
	}
	t.Fatalf("no tuition in %s", m)
}

// windowWithReward is the window school with a $200 early-payment reward per
// month, read on 20 May: the ten months are billed up front and are the
// balance; the inscription opens on 1 June and is not.
func windowWithReward(t *testing.T) *fixture {
	t.Helper()
	f := windowFixture(t)
	in := windowPlan(ymdUTC(2026, time.June, 1), ymdUTC(2026, time.June, 30))
	in.Policy = domain.CollectionPolicy{EarlyPaymentCents: 20000, EarlyPaymentDays: 10}
	savePlan(t, f, in)
	runOn(t, f, "2026-05-15")
	f.clock = ymdUTC(2026, time.May, 20)
	return f
}

// D1 (T5 review): the counter lists a charge not yet open — it may be paid
// ahead — but it is not part of the debt. The server says which lines the
// balance holds, and works out "si paga a tiempo" from those lines only, so
// the drawer never has to add up a figure of its own.
func TestTheCounterSaysWhichLinesTheBalanceHolds(t *testing.T) {
	f := windowWithReward(t)
	inscription := inscriptionOf(t, f, testStudent).Charge.ID()

	got, err := f.svc.ExpectedToday(context.Background(), testStudent, time.Time{})
	require.NoError(t, err)

	var held, rewards int64
	for _, l := range got.Lines {
		if l.ChargeID == inscription {
			assert.False(t, l.InBalance, "the inscription opens on 1 June: listed, not owed")
			continue
		}
		assert.True(t, l.InBalance, "%s is billed and open", l.Description)
		held += l.TotalCents + l.DiscountCents
		rewards += l.DiscountCents
	}
	assert.Equal(t, int64(10*20000), rewards, "every month paid ahead earns its reward")
	assert.Equal(t, int64(10*320000), got.AnnualBalanceCents)
	assert.Equal(t, got.AnnualBalanceCents, held, "the lines in the balance add up to it")
	assert.Equal(t, int64(10*320000-10*20000), got.OnTimeBalanceCents,
		"the balance less the rewards of the lines it holds")
}

// A charge not yet open that already has money on it is in play: the ledger
// counts it, so the counter marks it in the balance.
func TestAPartPaidChargeNotYetOpenIsInTheBalance(t *testing.T) {
	f := windowWithReward(t)
	inscription := inscriptionOf(t, f, testStudent).Charge.ID()
	pay(t, f, inscription, 10000)

	got, err := f.svc.ExpectedToday(context.Background(), testStudent, time.Time{})
	require.NoError(t, err)
	for _, l := range got.Lines {
		if l.ChargeID == inscription {
			assert.True(t, l.InBalance, "money on it puts it in play")
		}
	}
	requireOneBalance(t, f, 10*320000+inscriptionCents-10000)
}

// The counter's search shows only what is due today for each name on screen:
// it asks without the balance, which costs two reads per row.
func TestExpectedWithoutBalanceLeavesTheBalanceOut(t *testing.T) {
	f := windowWithReward(t)
	full, err := f.svc.ExpectedToday(context.Background(), testStudent, time.Time{})
	require.NoError(t, err)

	got, err := f.svc.ExpectedWithoutBalance(context.Background(), testStudent, time.Time{})
	require.NoError(t, err)

	assert.Equal(t, full.AmountCents, got.AmountCents)
	assert.Equal(t, full.Lines, got.Lines, "the lines, in_balance included, need no extra read")
	assert.Zero(t, got.AnnualBalanceCents)
	assert.Zero(t, got.OnTimeBalanceCents)
}
