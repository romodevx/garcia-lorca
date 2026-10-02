package report_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/garcia-lorca/services/gateway/internal/report"
)

func TestRangeLabel(t *testing.T) {
	tests := []struct {
		name     string
		from, to string
		want     string
	}{
		{"one day", "2026-09-15", "2026-09-15", "15 de septiembre de 2026"},
		{"a range", "2026-09-01", "2026-09-30", "Del 1 de septiembre de 2026 al 30 de septiembre de 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to := parse(t, tt.from), parse(t, tt.to)
			assert.Equal(t, tt.want, report.RangeLabel(from, to))
		})
	}
}

func TestSchoolHeading(t *testing.T) {
	assert.Equal(t, []string{"Colegio García Lorca", "Educación GL, S.C.", "RFC EGL010101AAA"},
		report.SchoolHeading("Colegio García Lorca", "Educación GL, S.C.", "EGL010101AAA"))
	assert.Equal(t, []string{"Colegio García Lorca"}, report.SchoolHeading("Colegio García Lorca", "", ""))
}

func parse(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
