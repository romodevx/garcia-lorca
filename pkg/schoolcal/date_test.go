package schoolcal_test

import (
	"testing"
	"time"

	"github.com/garcia-lorca/pkg/schoolcal"
)

// The school's day is Morelia's, never the UTC one (ADR 0035): at 19:00 on
// 31 Oct the school is still on 31 Oct though UTC is on 1 Nov. The date is
// held at midnight UTC so it compares exactly with a parsed YYYY-MM-DD.
func TestDateIsTheSchoolsCalendarDay(t *testing.T) {
	morelia, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{"evening in Morelia is still that day", time.Date(2026, 10, 31, 19, 0, 0, 0, morelia), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)},
		{"just after local midnight", time.Date(2026, 11, 1, 0, 0, 1, 0, morelia), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{"a UTC instant is read in the school's zone", time.Date(2027, 1, 1, 3, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		if got := schoolcal.Date(tt.at, morelia); !got.Equal(tt.want) || got.Location() != time.UTC {
			t.Errorf("%s: Date = %v, want %v", tt.name, got, tt.want)
		}
	}
}
