package grpcadapter

import (
	"testing"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// The accrued flag is set only under the daily rule, which the handler fixture
// does not run; pinned here on the mapper so it cannot be dropped.
func TestAnAccruedSurchargeKeepsItsFlagOnTheWire(t *testing.T) {
	got := toProtoMovement(domain.Movement{Kind: domain.MovementSurcharge, Accrued: true, AmountCents: 1})
	if !got.GetAccrued() || got.GetKind() != "surcharge" {
		t.Errorf("movement = %+v", got)
	}
}
