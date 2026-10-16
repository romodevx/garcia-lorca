package service

import "testing"

func TestPesos(t *testing.T) {
	tests := []struct {
		cents int64
		want  string
	}{
		{0, "$0.00"},
		{5, "$0.05"},
		{99_900, "$999.00"},
		{100_000, "$1,000.00"},
		{700_000, "$7,000.00"},
		{123_456_789, "$1,234,567.89"},
		{-123_456, "-$1,234.56"},
		{-5, "-$0.05"},
	}
	for _, tt := range tests {
		if got := pesos(tt.cents); got != tt.want {
			t.Errorf("pesos(%d) = %q, want %q", tt.cents, got, tt.want)
		}
	}
}
