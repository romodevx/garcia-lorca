package domain_test

import (
	"testing"
	"time"

	"github.com/garcia-lorca/pkg/schoolcal"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func on(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// policy is the school's rule as the owner stated it: $10 a day from the day
// after the due date, every calendar day, and a $150 discount per month settled
// within the first 5 working days.
func policy() domain.CollectionPolicy {
	return domain.CollectionPolicy{
		LateFeeDailyCents:    1000,
		EarlyPaymentCents:    15000,
		EarlyPaymentDays:     5,
		SurchargeCountsAllDays: true,
	}
}

// THE OWNER'S OWN EXAMPLE. Tuition falls due on the 10th of September; the
// family pays on the 10th of October. They owe September plus thirty days of
// surcharge.
func TestTheOwnersExample(t *testing.T) {
	cal := schoolcal.New(nil)
	due := on(2026, time.September, 10)
	paid := on(2026, time.October, 10)

	got := domain.Surcharge(policy(), cal, due, paid)

	if want := int64(30 * 1000); got != want {
		t.Fatalf("surcharge = %d cents, want %d (30 days at $10)", got, want)
	}
}

func TestSurchargeRunsOnWeekendsAndHolidays(t *testing.T) {
	// Independence day plus a weekend inside the span. The owner was explicit:
	// the surcharge runs on all of them.
	cal := schoolcal.New([]time.Time{on(2026, time.September, 16)})
	due := on(2026, time.September, 10)
	paid := on(2026, time.September, 20) // 10 calendar days later

	if got, want := domain.Surcharge(policy(), cal, due, paid), int64(10*1000); got != want {
		t.Fatalf("surcharge = %d, want %d — weekends and holidays are billable days here", got, want)
	}
}

func TestNoSurchargeOnOrBeforeTheDueDate(t *testing.T) {
	cal := schoolcal.New(nil)
	due := on(2026, time.September, 10)

	for _, paid := range []time.Time{on(2026, time.September, 1), due} {
		if got := domain.Surcharge(policy(), cal, due, paid); got != 0 {
			t.Errorf("paying on %s produced a surcharge of %d, want 0", paid.Format("2006-01-02"), got)
		}
	}
}

// A school that has not set a daily rate charges nothing, rather than charging
// zero-times-something and looking like it worked.
func TestNoDailyRateMeansNoSurcharge(t *testing.T) {
	p := policy()
	p.LateFeeDailyCents = 0
	if got := domain.Surcharge(p, schoolcal.New(nil), on(2026, time.September, 10), on(2026, time.October, 10)); got != 0 {
		t.Fatalf("surcharge = %d with no daily rate configured, want 0", got)
	}
}

// The early-payment window is counted in WORKING days, so a discount survives a
// 5th that falls on a Sunday. That is the one place the school calendar still
// matters once the surcharge runs on every day.
func TestEarlyPaymentDiscountUsesWorkingDays(t *testing.T) {
	// September 2026: 1 Tue. The 5th and 6th are Saturday and Sunday.
	// Five working days from the 1st reaches Monday the 7th.
	cal := schoolcal.New(nil)
	cycleMonthStart := on(2026, time.September, 1)

	tests := []struct {
		name string
		paid time.Time
		want int64
	}{
		{"first day of the month", on(2026, time.September, 1), 15000},
		{"the fifth working day", on(2026, time.September, 7), 15000},
		{"one day past the window", on(2026, time.September, 8), 0},
		{"the weekend inside the window does not consume it", on(2026, time.September, 5), 15000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.EarlyPaymentDiscount(policy(), cal, cycleMonthStart, tt.paid)
			if got != tt.want {
				t.Errorf("discount = %d, want %d", got, tt.want)
			}
		})
	}
}

// A holiday inside the window pushes the deadline out, keeping the benefit —
// the rule the director gave for deadlines landing on closed days.
func TestAHolidayExtendsTheEarlyPaymentWindow(t *testing.T) {
	// Make the 2nd (Wednesday) a holiday: the fifth working day moves from the
	// 7th to the 8th.
	cal := schoolcal.New([]time.Time{on(2026, time.September, 2)})
	start := on(2026, time.September, 1)

	if got := domain.EarlyPaymentDiscount(policy(), cal, start, on(2026, time.September, 8)); got != 15000 {
		t.Fatalf("discount = %d on the 8th, want 15000 — a holiday inside the window must extend it", got)
	}
}

// A school with no discount configured gives none, and a family paying late gets
// none either. Both are zero, but for different reasons, and both must hold.
func TestNoDiscountWhenNotConfiguredOrPaidLate(t *testing.T) {
	cal := schoolcal.New(nil)
	start := on(2026, time.September, 1)

	p := policy()
	p.EarlyPaymentCents = 0
	if got := domain.EarlyPaymentDiscount(p, cal, start, start); got != 0 {
		t.Errorf("discount = %d with none configured, want 0", got)
	}
	if got := domain.EarlyPaymentDiscount(policy(), cal, start, on(2026, time.September, 30)); got != 0 {
		t.Errorf("discount = %d for a late payment, want 0", got)
	}
}

// The full scenario the owner described, month by month. Worth pinning as one
// test because the interesting part is not any single number — it is that the
// September surcharge keeps growing while October's has not started.
func TestTheOwnersScenarioAcrossTwoMonths(t *testing.T) {
	cal := schoolcal.New(nil)
	p := policy()
	sepDue := on(2026, time.September, 10)
	octDue := on(2026, time.October, 10)

	t.Run("pays on 10 October: September plus thirty days, October clean", func(t *testing.T) {
		paid := octDue
		if got, want := domain.Surcharge(p, cal, sepDue, paid), int64(30*1000); got != want {
			t.Errorf("September surcharge = %d, want %d", got, want)
		}
		if got := domain.Surcharge(p, cal, octDue, paid); got != 0 {
			t.Errorf("October surcharge = %d on its own due date, want 0", got)
		}
	})

	t.Run("does not pay: both months accrue, September stays ahead", func(t *testing.T) {
		// A month later still unpaid: September has 61 days, October has 31.
		asOf := on(2026, time.November, 10)
		sep := domain.Surcharge(p, cal, sepDue, asOf)
		oct := domain.Surcharge(p, cal, octDue, asOf)
		if sep != int64(61*1000) {
			t.Errorf("September surcharge = %d, want %d", sep, 61*1000)
		}
		if oct != int64(31*1000) {
			t.Errorf("October surcharge = %d, want %d", oct, 31*1000)
		}
		if sep <= oct {
			t.Error("the older debt must always carry the larger surcharge")
		}
	})
}

// Settling a month stops ITS surcharge and no other. This is the property that
// makes the ledger explainable to a parent: each month answers for itself.
func TestSettlingOneMonthDoesNotStopAnother(t *testing.T) {
	cal := schoolcal.New(nil)
	p := policy()
	sepDue := on(2026, time.September, 10)
	octDue := on(2026, time.October, 10)

	// October is settled the day it falls due; September is still open.
	octSettled := octDue
	if got := domain.Surcharge(p, cal, octDue, octSettled); got != 0 {
		t.Fatalf("October surcharge = %d when settled on time, want 0", got)
	}
	// September, asked about later, has kept running regardless.
	if got := domain.Surcharge(p, cal, sepDue, on(2026, time.October, 20)); got != int64(40*1000) {
		t.Fatalf("September surcharge = %d, want %d — paying October must not pause it", got, 40*1000)
	}
}
