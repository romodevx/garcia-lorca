package domain_test

import (
	"testing"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

func TestIsFeeRef(t *testing.T) {
	for ref, want := range map[string]bool{
		"dailyfee:ch-1:dep-1":  true,
		"latefee:ch-1:2026-09": true,
		"tuition:c:s:2026-09":  false,
		"uniforms:f-1":         false,
		"":                     false,
	} {
		if got := domain.IsFeeRef(ref); got != want {
			t.Errorf("IsFeeRef(%q) = %v, want %v", ref, got, want)
		}
	}
}
