package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/garcia-lorca/services/grades/internal/domain"
)

func values(hundredths ...int) []domain.GradeValue {
	out := make([]domain.GradeValue, 0, len(hundredths))
	for _, h := range hundredths {
		out = append(out, domain.GradeValueFromHundredths(h))
	}
	return out
}

// T7: a subject's trimester grade is the arithmetic mean of its captured
// monthly grades, one decimal, rounded half-up with exact arithmetic (a
// float64 would read 8.45 as 8.4499… and round it down).
func TestSubjectAverage(t *testing.T) {
	tests := []struct {
		name   string
		grades []domain.GradeValue
		want   string
	}{
		{"8.45 rounds up to 8.5", values(840, 850), "8.5"},
		{"8.25 rounds up to 8.3", values(800, 850), "8.3"},
		{"8.24 rounds down to 8.2", values(824), "8.2"},
		{"a single grade keeps one decimal", values(900), "9.0"},
		{"three months", values(1000, 900, 850), "9.2"},               // 9.1666…
		{"exact half on two-decimal grades", values(995, 900), "9.5"}, // 9.475 → 9.5
		{"just under a half", values(994, 900), "9.5"},                // 9.47 → 9.5
		{"ten", values(1000, 1000), "10.0"},
		{"lowest", values(500), "5.0"},
		{"9.95 exactly halfway rounds up", values(995), "10.0"},
		{"9.94 rounds down", values(994), "9.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.SubjectAverage(tt.grades)
			assert.True(t, got.Present())
			assert.Equal(t, tt.want, got.String())
		})
	}
}

// A subject with no grade in the term has no trimester grade at all — never a
// zero that would drag the general average down.
func TestSubjectAverageWithoutGrades(t *testing.T) {
	got := domain.SubjectAverage(nil)
	assert.False(t, got.Present())
	assert.Equal(t, "", got.String())
}

// The general average is the mean of the subjects' trimester grades (only
// subjects with at least one grade), one decimal half-up.
func TestGeneralAverage(t *testing.T) {
	tests := []struct {
		name     string
		subjects []domain.Average
		want     string
	}{
		{"mean of two", []domain.Average{domain.AverageFromTenths(85), domain.AverageFromTenths(90)}, "8.8"}, // 8.75
		{"absent subjects are ignored", []domain.Average{domain.AverageFromTenths(80), {}, domain.AverageFromTenths(90)}, "8.5"},
		{"thirds", []domain.Average{domain.AverageFromTenths(80), domain.AverageFromTenths(80), domain.AverageFromTenths(81)}, "8.0"},     // 8.0333
		{"two thirds", []domain.Average{domain.AverageFromTenths(80), domain.AverageFromTenths(81), domain.AverageFromTenths(81)}, "8.1"}, // 8.0666
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, domain.GeneralAverage(tt.subjects).String())
		})
	}
	assert.False(t, domain.GeneralAverage([]domain.Average{{}, {}}).Present(), "no subject graded ⇒ no general average")
}

func TestAverageTenths(t *testing.T) {
	a := domain.AverageFromTenths(85)
	assert.Equal(t, 85, a.Tenths())
	assert.Equal(t, "8.5", a.String())
}
