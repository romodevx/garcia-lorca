package schoolcal_test

import (
	"testing"
	"time"

	"github.com/garcia-lorca/pkg/schoolcal"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// A calendar with no holidays still knows about weekends: that is the floor
// every school shares before it loads a single day of its own.
func TestWeekendsAreNotWorkingDays(t *testing.T) {
	cal := schoolcal.New(nil)
	tests := []struct {
		name string
		date time.Time
		want bool
	}{
		{"friday", day(2026, time.September, 4), true},
		{"saturday", day(2026, time.September, 5), false},
		{"sunday", day(2026, time.September, 6), false},
		{"monday", day(2026, time.September, 7), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cal.IsWorkingDay(tt.date); got != tt.want {
				t.Errorf("IsWorkingDay(%s) = %v, want %v", tt.date.Format("2006-01-02"), got, tt.want)
			}
		})
	}
}

func TestHolidaysAreNotWorkingDays(t *testing.T) {
	independence := day(2026, time.September, 16) // a Wednesday
	cal := schoolcal.New([]time.Time{independence})

	if cal.IsWorkingDay(independence) {
		t.Error("a loaded holiday is still reported as a working day")
	}
	if !cal.IsWorkingDay(day(2026, time.September, 17)) {
		t.Error("the day after a holiday must be a working day")
	}
}

// The rule the director gave: a deadline that lands on a non-working day moves
// to the next working one, keeping the benefit or the cost attached to it.
func TestNextWorkingDayRollsForward(t *testing.T) {
	// 16 Sep 2026 is Wednesday (holiday); 19-20 Sep is the weekend.
	cal := schoolcal.New([]time.Time{day(2026, time.September, 16)})
	tests := []struct {
		name string
		from time.Time
		want time.Time
	}{
		{"a working day is already itself", day(2026, time.September, 15), day(2026, time.September, 15)},
		{"a holiday moves to the next day", day(2026, time.September, 16), day(2026, time.September, 17)},
		{"saturday moves to monday", day(2026, time.September, 19), day(2026, time.September, 21)},
		{"sunday moves to monday", day(2026, time.September, 20), day(2026, time.September, 21)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cal.NextWorkingDay(tt.from); !got.Equal(tt.want) {
				t.Errorf("NextWorkingDay(%s) = %s, want %s",
					tt.from.Format("2006-01-02"), got.Format("2006-01-02"), tt.want.Format("2006-01-02"))
			}
		})
	}
}

// A holiday on a Friday followed by the weekend must roll all the way to Monday,
// not one day at a time into another non-working day.
func TestNextWorkingDaySkipsARunOfNonWorkingDays(t *testing.T) {
	friday := day(2026, time.September, 18)
	cal := schoolcal.New([]time.Time{friday})
	got := cal.NextWorkingDay(friday)
	want := day(2026, time.September, 21) // Monday
	if !got.Equal(want) {
		t.Fatalf("NextWorkingDay(Friday holiday) = %s, want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

// The early-payment window is "the first N working days", which is what makes a
// discount survivable when the 5th falls on a Sunday.
func TestAddWorkingDaysCountsOnlyWorkingDays(t *testing.T) {
	// September 2026: 1 Tue … 5 Sat, 6 Sun, 7 Mon. 16th is a holiday.
	cal := schoolcal.New([]time.Time{day(2026, time.September, 16)})
	tests := []struct {
		name string
		from time.Time
		n    int
		want time.Time
	}{
		{"zero days is the same day when it works", day(2026, time.September, 1), 0, day(2026, time.September, 1)},
		{"four working days from Tuesday", day(2026, time.September, 1), 4, day(2026, time.September, 7)},
		{"crossing a holiday adds a day", day(2026, time.September, 15), 1, day(2026, time.September, 17)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cal.AddWorkingDays(tt.from, tt.n); !got.Equal(tt.want) {
				t.Errorf("AddWorkingDays(%s, %d) = %s, want %s",
					tt.from.Format("2006-01-02"), tt.n, got.Format("2006-01-02"), tt.want.Format("2006-01-02"))
			}
		})
	}
}

// The surcharge counts days late. Whether weekends and holidays are billable is
// a POLICY the school sets, so the calendar answers both questions and refuses
// to decide for them.
func TestWorkingDaysBetweenExcludesNonWorkingDays(t *testing.T) {
	cal := schoolcal.New([]time.Time{day(2026, time.September, 16)})

	// 15 Sep (Tue) → 18 Sep (Fri): 16th is a holiday, so 15→16,17,18 = 2 working
	// days elapsed (17th and 18th).
	if got := cal.WorkingDaysBetween(day(2026, time.September, 15), day(2026, time.September, 18)); got != 2 {
		t.Errorf("WorkingDaysBetween = %d, want 2", got)
	}
	// Calendar days over the same span is 3, which is what a school charging
	// every day would use.
	if got := cal.CalendarDaysBetween(day(2026, time.September, 15), day(2026, time.September, 18)); got != 3 {
		t.Errorf("CalendarDaysBetween = %d, want 3", got)
	}
}

// A date before the start never yields a negative count: "not yet late" is zero
// days late, and a negative surcharge would be a refund nobody authorised.
func TestDaysBetweenNeverGoesNegative(t *testing.T) {
	cal := schoolcal.New(nil)
	if got := cal.CalendarDaysBetween(day(2026, time.September, 18), day(2026, time.September, 15)); got != 0 {
		t.Errorf("CalendarDaysBetween(later, earlier) = %d, want 0", got)
	}
	if got := cal.WorkingDaysBetween(day(2026, time.September, 18), day(2026, time.September, 15)); got != 0 {
		t.Errorf("WorkingDaysBetween(later, earlier) = %d, want 0", got)
	}
}

// Times of day must never change an answer: the counter records a day, and a
// payment at 23:00 is the same day as one at 08:00.
func TestTimeOfDayIsIgnored(t *testing.T) {
	cal := schoolcal.New([]time.Time{time.Date(2026, time.September, 16, 17, 30, 0, 0, time.UTC)})
	if cal.IsWorkingDay(time.Date(2026, time.September, 16, 8, 0, 0, 0, time.UTC)) {
		t.Error("a holiday loaded with a time did not match the same day at another hour")
	}
	late := time.Date(2026, time.September, 15, 23, 59, 0, 0, time.UTC)
	if got := cal.NextWorkingDay(late); !got.Equal(day(2026, time.September, 15)) {
		t.Errorf("NextWorkingDay of a working day at 23:59 = %s, want the same day at midnight", got)
	}
}

// Two invariants that must hold for ANY calendar and any date, not just the
// examples above. Cheap to state, and they catch the off-by-one that a
// hand-picked case can miss.
func TestInvariants(t *testing.T) {
	// A November with a holiday, a weekend, and a holiday adjacent to a weekend.
	cal := schoolcal.New([]time.Time{
		day(2026, time.November, 2),  // Monday
		day(2026, time.November, 16), // Monday
		day(2026, time.November, 20), // Friday
	})

	start := day(2026, time.November, 1)
	for i := 0; i < 60; i++ {
		d := start.AddDate(0, 0, i)

		// NextWorkingDay always lands on a working day, and never goes backwards.
		next := cal.NextWorkingDay(d)
		if !cal.IsWorkingDay(next) {
			t.Fatalf("NextWorkingDay(%s) = %s, which is not a working day", d.Format("2006-01-02"), next.Format("2006-01-02"))
		}
		if next.Before(d) {
			t.Fatalf("NextWorkingDay(%s) = %s went backwards", d.Format("2006-01-02"), next.Format("2006-01-02"))
		}

		// Working days elapsed can never exceed calendar days elapsed.
		for _, span := range []int{1, 7, 30} {
			to := d.AddDate(0, 0, span)
			w, c := cal.WorkingDaysBetween(d, to), cal.CalendarDaysBetween(d, to)
			if w > c {
				t.Fatalf("%s..%s: working days %d > calendar days %d", d.Format("2006-01-02"), to.Format("2006-01-02"), w, c)
			}
		}
	}
}
