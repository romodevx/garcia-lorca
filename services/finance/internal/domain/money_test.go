package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func TestNewCents(t *testing.T) {
	cases := []struct {
		name    string
		value   int64
		wantErr bool
	}{
		{"zero is a valid amount", 0, false},
		{"positive", 12550, false},
		{"negative is rejected", -1, true},
		{"large negative is rejected", -12550, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := domain.NewCents(tc.value)
			if tc.wantErr {
				if glerrors.KindOf(err) != glerrors.KindInvalid {
					t.Fatalf("NewCents(%d): want KindInvalid, got %v", tc.value, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewCents(%d): %v", tc.value, err)
			}
			if c.Int64() != tc.value {
				t.Errorf("Int64() = %d, want %d", c.Int64(), tc.value)
			}
		})
	}
}
