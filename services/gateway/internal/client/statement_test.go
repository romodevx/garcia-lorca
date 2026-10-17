package client

import (
	"reflect"
	"testing"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// T7: every field of the ledger reaches the JSON the drawer, the app and the
// PDF read. A field the mapper forgets is a zero nobody notices.
func TestAStatementCarriesEveryField(t *testing.T) {
	got := toStatement(&financev1.GetStatementResponse{
		Movements: []*financev1.StatementMovement{{
			Kind: "payment", Date: "2026-09-06", Description: "Pago", AmountCents: 30000, BalanceCents: -100,
			ChargeId: "c", DepositId: "d", Method: "card", CardType: "debit", Reference: "v-9", Folio: "P-2026-000007",
			Accrued: true, NotYetDue: true, Voided: true, VoidReason: "dos veces",
			AppliedTo: []*financev1.StatementAppliedPart{{ChargeId: "c", Description: "Inscripción", AmountCents: 30000}},
		}},
		Totals: &financev1.StatementTotals{
			ChargedCents: 1, DiscountCents: 2, PaidCents: 3, BalanceCents: 4, NotYetDueCents: 5,
		},
		DueTodayCents: 6, SurchargeCents: 7, EarlyDiscountCents: 8, UnallocatedCents: 9, AsOf: "2026-10-01",
	})

	wantMovement := StatementMovement{
		Kind: "payment", Date: "2026-09-06", Description: "Pago", AmountCents: 30000, BalanceCents: -100,
		ChargeID: "c", DepositID: "d", Method: "card", CardType: "debit", Reference: "v-9", Folio: "P-2026-000007",
		Accrued: true, NotYetDue: true, Voided: true, VoidReason: "dos veces",
		AppliedTo: []StatementAppliedPart{{ChargeID: "c", Description: "Inscripción", AmountCents: 30000}},
	}
	if len(got.Movements) != 1 || !reflect.DeepEqual(got.Movements[0], wantMovement) {
		t.Errorf("movements = %+v, want %+v", got.Movements, wantMovement)
	}
	wantTotals := StatementTotals{ChargedCents: 1, DiscountCents: 2, PaidCents: 3, BalanceCents: 4, NotYetDueCents: 5}
	if got.Totals != wantTotals {
		t.Errorf("totals = %+v", got.Totals)
	}
	if got.DueTodayCents != 6 || got.SurchargeCents != 7 || got.EarlyDiscountCents != 8 || got.UnallocatedCents != 9 || got.AsOf != "2026-10-01" {
		t.Errorf("figures = %+v", got)
	}
}

// An empty ledger is an empty list on the wire, never null: the app iterates it.
func TestAnEmptyStatementHasAnEmptyList(t *testing.T) {
	if got := toStatement(&financev1.GetStatementResponse{}); got.Movements == nil {
		t.Error("movements is nil")
	}
}
