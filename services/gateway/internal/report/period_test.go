package report_test

import (
	"testing"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/report"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestNewPeriod(t *testing.T) {
	anchor := day(2026, 10, 1) // a Thursday
	tests := []struct {
		kind     string
		from, to time.Time
		label    string
	}{
		{"daily", day(2026, 10, 1), day(2026, 10, 1), "1 de octubre de 2026"},
		{"weekly", day(2026, 9, 28), day(2026, 10, 4), "Semana del 28 de septiembre al 4 de octubre de 2026"},
		{"monthly", day(2026, 10, 1), day(2026, 10, 31), "Octubre de 2026"},
		{"yearly", day(2026, 1, 1), day(2026, 12, 31), "Año 2026"},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			p, err := report.NewPeriod(tc.kind, anchor.Add(17*time.Hour))
			if err != nil {
				t.Fatalf("NewPeriod: %v", err)
			}
			if !p.From.Equal(tc.from) || !p.To.Equal(tc.to) {
				t.Fatalf("got %s..%s, want %s..%s", p.From, p.To, tc.from, tc.to)
			}
			if p.Label() != tc.label {
				t.Fatalf("label = %q, want %q", p.Label(), tc.label)
			}
		})
	}
}

func TestNewPeriodEdges(t *testing.T) {
	// A Sunday belongs to the week that started the Monday before.
	p, _ := report.NewPeriod("weekly", day(2026, 10, 4))
	if !p.From.Equal(day(2026, 9, 28)) {
		t.Fatalf("sunday's week starts %s", p.From)
	}
	// February of a leap year.
	p, _ = report.NewPeriod("monthly", day(2028, 2, 10))
	if !p.To.Equal(day(2028, 2, 29)) {
		t.Fatalf("leap february ends %s", p.To)
	}
	// A week across two years names both.
	p, _ = report.NewPeriod("weekly", day(2026, 12, 31))
	if p.Label() != "Semana del 28 de diciembre de 2026 al 3 de enero de 2027" {
		t.Fatalf("label = %q", p.Label())
	}
	if _, err := report.NewPeriod("hourly", day(2026, 1, 1)); err == nil {
		t.Fatalf("an unknown period must be refused")
	}
}
