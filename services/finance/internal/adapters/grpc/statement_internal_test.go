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

// ISSUE-T14-106: what a deposit paid reaches the wire, part by part.
func TestADepositsAppliedPartsReachTheWire(t *testing.T) {
	got := toProtoMovement(domain.Movement{Kind: domain.MovementPayment, DepositID: "d", AppliedTo: []domain.AppliedPart{
		{ChargeID: "c1", Description: "Inscripción", AmountCents: 300000},
		{ChargeID: "c2", Description: "Colegiatura octubre", AmountCents: 1000},
	}})
	parts := got.GetAppliedTo()
	if len(parts) != 2 || parts[0].GetChargeId() != "c1" || parts[0].GetDescription() != "Inscripción" ||
		parts[0].GetAmountCents() != 300000 || parts[1].GetChargeId() != "c2" || parts[1].GetAmountCents() != 1000 {
		t.Errorf("applied to = %+v", parts)
	}
}
