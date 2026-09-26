package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// A new level takes attendance the way its SEP level usually does: the lower
// levels have one homeroom teacher per group (daily), the upper ones a teacher
// per subject (per class). The name is matched without case, accents or padding.
func TestNewLevelDefaultsAttendanceModeByName(t *testing.T) {
	tests := []struct {
		name string
		want domain.AttendanceMode
	}{
		{"Preescolar", domain.AttendanceDaily},
		{"Primaria", domain.AttendanceDaily},
		{"Secundaria", domain.AttendancePerClass},
		{"  SECUNDARIA ", domain.AttendancePerClass},
		{"Preparatoria", domain.AttendancePerClass},
		{"Bachillerato", domain.AttendancePerClass},
		{"Maternal", domain.AttendanceDaily},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := domain.NewLevel("l1", "t1", tt.name, "", domain.GranularityCycle)
			if err != nil {
				t.Fatalf("NewLevel: %v", err)
			}
			if l.AttendanceMode() != tt.want {
				t.Errorf("mode = %v, want %v", l.AttendanceMode(), tt.want)
			}
		})
	}
}

func TestLevelWithAttendanceMode(t *testing.T) {
	l, err := domain.NewLevel("l1", "t1", "Primaria", "", domain.GranularityCycle)
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	changed, err := l.WithAttendanceMode(domain.AttendancePerClass)
	if err != nil {
		t.Fatalf("WithAttendanceMode: %v", err)
	}
	if changed.AttendanceMode() != domain.AttendancePerClass {
		t.Errorf("mode = %v, want per_class", changed.AttendanceMode())
	}
	if l.AttendanceMode() != domain.AttendanceDaily {
		t.Errorf("original level mutated: %v", l.AttendanceMode())
	}
	if _, err := l.WithAttendanceMode(domain.AttendanceModeUnspecified); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unspecified mode: want KindInvalid, got %v", err)
	}
}

func TestParseAttendanceMode(t *testing.T) {
	tests := []struct {
		raw     string
		want    domain.AttendanceMode
		wantErr bool
	}{
		{"daily", domain.AttendanceDaily, false},
		{"per_class", domain.AttendancePerClass, false},
		{" PER_CLASS ", domain.AttendancePerClass, false},
		{"weekly", domain.AttendanceModeUnspecified, true},
		{"", domain.AttendanceModeUnspecified, true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := domain.ParseAttendanceMode(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if err == nil && got.String() != "daily" && got.String() != "per_class" {
				t.Errorf("String() = %q", got.String())
			}
		})
	}
}
