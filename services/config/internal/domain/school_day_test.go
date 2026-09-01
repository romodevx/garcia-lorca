package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestNewNonWorkingPeriod(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		tenantID string
		cycleID  string
		label    string
		kind     string
		from     time.Time
		to       time.Time
		wantErr  bool
	}{
		{
			name: "a single holiday", id: "np-1", tenantID: "t-1", cycleID: "c-1",
			label: "Independencia", kind: "holiday",
			from: d(2026, time.September, 16), to: d(2026, time.September, 16),
		},
		{
			name: "a vacation range", id: "np-2", tenantID: "t-1", cycleID: "c-1",
			label: "Vacaciones de invierno", kind: "vacation",
			from: d(2026, time.December, 21), to: d(2027, time.January, 5),
		},
		{
			name: "id is required", id: "  ", tenantID: "t-1", cycleID: "c-1",
			label: "x", kind: "holiday", from: d(2026, time.September, 16), to: d(2026, time.September, 16),
			wantErr: true,
		},
		{
			name: "label is required", id: "np-3", tenantID: "t-1", cycleID: "c-1",
			label: "   ", kind: "holiday", from: d(2026, time.September, 16), to: d(2026, time.September, 16),
			wantErr: true,
		},
		{
			name: "an unknown kind is refused", id: "np-4", tenantID: "t-1", cycleID: "c-1",
			label: "x", kind: "puente-largo", from: d(2026, time.September, 16), to: d(2026, time.September, 16),
			wantErr: true,
		},
		{
			// An inverted range is a data-entry slip that would otherwise expand
			// to zero days and silently do nothing.
			name: "the end may not precede the start", id: "np-5", tenantID: "t-1", cycleID: "c-1",
			label: "x", kind: "vacation", from: d(2026, time.December, 21), to: d(2026, time.December, 20),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewNonWorkingPeriod(tt.id, tt.tenantID, tt.cycleID, tt.label, tt.kind, tt.from, tt.to)
			if tt.wantErr {
				if glerrors.KindOf(err) != glerrors.KindInvalid {
					t.Fatalf("kind = %v, want Invalid (err=%v)", glerrors.KindOf(err), err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Label() != tt.label || got.Kind() != tt.kind {
				t.Errorf("label/kind = %q/%q, want %q/%q", got.Label(), got.Kind(), tt.label, tt.kind)
			}
		})
	}
}

// Days() is what the calendar consumes: a range has to become the individual
// days it covers, both ends included.
func TestDaysExpandsTheRangeInclusively(t *testing.T) {
	p, err := domain.NewNonWorkingPeriod("np-1", "t-1", "c-1", "Puente", "holiday",
		d(2026, time.November, 16), d(2026, time.November, 18))
	if err != nil {
		t.Fatalf("NewNonWorkingPeriod: %v", err)
	}
	got := p.Days()
	want := []time.Time{d(2026, time.November, 16), d(2026, time.November, 17), d(2026, time.November, 18)}
	if len(got) != len(want) {
		t.Fatalf("Days() returned %d days, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("Days()[%d] = %s, want %s", i, got[i].Format("2006-01-02"), want[i].Format("2006-01-02"))
		}
	}
}

func TestSingleDayPeriodYieldsOneDay(t *testing.T) {
	p, _ := domain.NewNonWorkingPeriod("np-1", "t-1", "c-1", "Independencia", "holiday",
		d(2026, time.September, 16), d(2026, time.September, 16))
	if got := p.Days(); len(got) != 1 {
		t.Fatalf("Days() = %d days for a single-day period, want 1", len(got))
	}
}

// A school year of closure is not a calendar entry; it is a typo that would
// expand into an unbounded slice inside a request.
func TestAnAbsurdlyLongPeriodIsRefused(t *testing.T) {
	_, err := domain.NewNonWorkingPeriod("np-1", "t-1", "c-1", "x", "vacation",
		d(2026, time.January, 1), d(2028, time.January, 1))
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("kind = %v, want Invalid for a two-year period", glerrors.KindOf(err))
	}
}
