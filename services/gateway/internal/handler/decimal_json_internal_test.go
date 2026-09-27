package handler

import (
	"encoding/json"
	"testing"
)

// parseHundredths reads only plain decimals of at most 16 characters: exponent,
// hex and binary forms never reach big.Rat ("1e1000000000" would make it build
// a billion-digit number).
func TestParseHundredthsRefusesNonPlainNumbers(t *testing.T) {
	tests := []struct {
		raw  string
		want int
		ok   bool
	}{
		{`5`, 500, true},
		{`69.5`, 6950, true},
		{`-1`, -100, true},
		{`9.50000000000000`, 950, true}, // 16 chars
		{`9.500000000000000`, 0, false}, // 17 chars
		{`1e1`, 0, false},
		{`1E1000000000`, 0, false},
		{`0x10`, 0, false},
		{`0b1`, 0, false},
		{`"5"`, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, ok := parseHundredths(json.RawMessage(tt.raw))
			if ok != tt.ok || got != tt.want {
				t.Errorf("parseHundredths(%s) = %d, %v; want %d, %v", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}
