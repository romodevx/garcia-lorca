package domain_test

import (
	"testing"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// T7 · A4: the ledger is ONE computation. These cases pin what every reader —
// the office's drawer, the parents' app and the PDF — is shown.
func TestBuildStatementOrdersAndRunsTheBalance(t *testing.T) {
	in := []domain.Movement{
		{Kind: domain.MovementPayment, Date: day(2026, 9, 12), DepositID: "d1", AmountCents: 2_000_00},
		{Kind: domain.MovementCharge, Date: day(2026, 9, 10), ChargeID: "sep", AmountCents: 3_500_00},
		{Kind: domain.MovementDiscount, Date: day(2026, 9, 10), ChargeID: "sep", AmountCents: 200_00},
		{Kind: domain.MovementSurcharge, Date: day(2026, 9, 20), ChargeID: "sep", AmountCents: 150_00, Accrued: true},
	}

	got, totals := domain.BuildStatement(in)

	wantKinds := []domain.MovementKind{domain.MovementCharge, domain.MovementDiscount, domain.MovementPayment, domain.MovementSurcharge}
	wantBalances := []int64{3_500_00, 3_300_00, 1_300_00, 1_450_00}
	if len(got) != len(wantKinds) {
		t.Fatalf("want %d movements, got %d", len(wantKinds), len(got))
	}
	for i := range got {
		if got[i].Kind != wantKinds[i] || got[i].BalanceCents != wantBalances[i] {
			t.Errorf("movement %d: want %s/%d, got %s/%d", i, wantKinds[i], wantBalances[i], got[i].Kind, got[i].BalanceCents)
		}
	}
	want := domain.StatementTotals{ChargedCents: 3_650_00, DiscountCents: 200_00, PaidCents: 2_000_00, BalanceCents: 1_450_00}
	if totals != want {
		t.Fatalf("totals: want %+v, got %+v", want, totals)
	}
}

func TestBuildStatementKeepsNoEffectRowsOutOfTheBalance(t *testing.T) {
	in := []domain.Movement{
		{Kind: domain.MovementCharge, Date: day(2026, 9, 10), ChargeID: "sep", AmountCents: 1_000_00},
		// A voided deposit: shown, struck through, moves nothing.
		{Kind: domain.MovementPayment, Date: day(2026, 9, 11), DepositID: "void", AmountCents: 1_000_00, Voided: true},
		// A deposit replaced by its correction (T9): shown, moves nothing.
		{Kind: domain.MovementPayment, Date: day(2026, 9, 12), DepositID: "old", AmountCents: 500_00, Superseded: true, SupersededByFolio: "P-2026-000002"},
		// A charge that has not opened (T5): shown to the office, owed by nobody yet.
		{Kind: domain.MovementCharge, Date: day(2027, 1, 10), ChargeID: "jan", AmountCents: 1_000_00, NotYetDue: true},
		{Kind: domain.MovementDiscount, Date: day(2027, 1, 10), ChargeID: "jan", AmountCents: 100_00, NotYetDue: true},
	}

	got, totals := domain.BuildStatement(in)

	for i, m := range got {
		if m.BalanceCents != 1_000_00 {
			t.Errorf("movement %d (%s): balance want 1000.00, got %d", i, m.Kind, m.BalanceCents)
		}
	}
	want := domain.StatementTotals{ChargedCents: 1_000_00, BalanceCents: 1_000_00, NotYetDueCents: 900_00}
	if totals != want {
		t.Fatalf("totals: want %+v, got %+v", want, totals)
	}
}

func TestBuildStatementCreditsLowerTheBalance(t *testing.T) {
	// Scholarships (T18) and credit notes (T8) have their kinds already: a
	// reader written today renders them the day they start to exist.
	in := []domain.Movement{
		{Kind: domain.MovementCharge, Date: day(2026, 9, 10), ChargeID: "sep", AmountCents: 1_000_00},
		{Kind: domain.MovementScholarship, Date: day(2026, 9, 10), ChargeID: "sep", AmountCents: 500_00},
		{Kind: domain.MovementCreditNote, Date: day(2026, 9, 11), DepositID: "d", AmountCents: 300_00},
		{Kind: domain.MovementPayment, Date: day(2026, 9, 11), DepositID: "e", AmountCents: 400_00},
	}

	_, totals := domain.BuildStatement(in)

	// Overpaid by 200: the family's credit shows as a negative balance.
	want := domain.StatementTotals{ChargedCents: 1_000_00, DiscountCents: 500_00, PaidCents: 700_00, BalanceCents: -200_00}
	if totals != want {
		t.Fatalf("totals: want %+v, got %+v", want, totals)
	}
}

func TestBuildStatementIsDeterministicOnTies(t *testing.T) {
	a := []domain.Movement{
		{Kind: domain.MovementPayment, Date: day(2026, 9, 10), DepositID: "z", AmountCents: 1},
		{Kind: domain.MovementCharge, Date: day(2026, 9, 10), ChargeID: "b", AmountCents: 1},
		{Kind: domain.MovementDiscount, Date: day(2026, 9, 10), ChargeID: "a", AmountCents: 1},
		{Kind: domain.MovementCharge, Date: day(2026, 9, 10), ChargeID: "a", AmountCents: 2},
	}
	b := []domain.Movement{a[3], a[0], a[2], a[1]}

	got1, _ := domain.BuildStatement(a)
	got2, _ := domain.BuildStatement(b)

	// Same day: each charge with its own discount right after it, money last.
	want := []string{"charge/a", "discount/a", "charge/b", "payment/z"}
	for i := range want {
		k1 := string(got1[i].Kind) + "/" + got1[i].ChargeID + got1[i].DepositID
		k2 := string(got2[i].Kind) + "/" + got2[i].ChargeID + got2[i].DepositID
		if k1 != want[i] || k2 != want[i] {
			t.Fatalf("position %d: want %s, got %s and %s", i, want[i], k1, k2)
		}
	}
}

func TestMovementKindDirection(t *testing.T) {
	debits := map[domain.MovementKind]bool{
		domain.MovementCharge: true, domain.MovementSurcharge: true,
		domain.MovementDiscount: false, domain.MovementScholarship: false,
		domain.MovementPayment: false, domain.MovementCreditNote: false,
	}
	for k, want := range debits {
		if k.Debit() != want {
			t.Errorf("%s: Debit() want %v", k, want)
		}
	}
}
