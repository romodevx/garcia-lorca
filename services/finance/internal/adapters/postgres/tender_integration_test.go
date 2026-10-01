//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// T8 against real SQL: a deposit paid by a credit note and cash.

func tender(t *testing.T, method string, cents int64, reference, cardType string) domain.Tender {
	t.Helper()
	got, err := domain.NewTender(method, cents, reference, cardType)
	if err != nil {
		t.Fatalf("NewTender: %v", err)
	}
	return got
}

func methodPayment(t *testing.T, id, tenant, chargeID string, amount int64, at time.Time, method, key string) domain.Payment {
	t.Helper()
	p, err := domain.NewPayment(id, tenant, chargeID, amount, method, "", at, key)
	if err != nil {
		t.Fatalf("new payment: %v", err)
	}
	return p
}

// The tenders are stored in order and come back on every read; the deposit's
// summary method is 'mixed' (accepted by the widened CHECK), and a credit-note
// payment row is accepted too. The receipt folds the two rows of one charge
// into one line.
func TestATenderedDepositRoundTrips(t *testing.T) {
	ctx := context.Background()
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000008a1"
	paidOn := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	month := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000008a1", "colegiatura t8",
		"61000000-0000-0000-0000-0000000008a1", student, "70000000-0000-0000-0000-0000000008a1", 320000, 0,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))

	d := newDeposit(t, "62000000-0000-0000-0000-0000000008a1", tenant, student, 320000, paidOn, "t8-key-1")
	d, err := d.WithTenders([]domain.Tender{
		tender(t, domain.MethodCreditNote, 120000, "NC-7", ""),
		tender(t, domain.MethodCard, 200000, "V-1", domain.CardCredit),
	})
	if err != nil {
		t.Fatalf("WithTenders: %v", err)
	}
	repo := appRepo(t, tenant)
	if _, _, err := repo.RegisterDeposit(ctx, d, func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
		return ports.DepositWrite{Allocations: []domain.Payment{
			methodPayment(t, "90000000-0000-0000-0000-0000000008a1", tenant, month.ID(), 120000, paidOn, domain.MethodCreditNote, ""),
			methodPayment(t, "90000000-0000-0000-0000-0000000008a2", tenant, month.ID(), 200000, paidOn, domain.MethodCard, ""),
		}}, nil
	}, nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	got, err := repo.GetDeposit(ctx, d.ID())
	if err != nil {
		t.Fatalf("GetDeposit: %v", err)
	}
	tenders := got.Tenders()
	if got.Method() != domain.MethodMixed || len(tenders) != 2 {
		t.Fatalf("deposit = %s with %d tenders, want mixed with 2", got.Method(), len(tenders))
	}
	if tenders[0].Method() != domain.MethodCreditNote || tenders[0].Reference() != "NC-7" ||
		tenders[1].Method() != domain.MethodCard || tenders[1].CardType() != domain.CardCredit {
		t.Errorf("tenders came back as %+v", tenders)
	}

	listed, err := repo.ListDepositsOn(ctx, paidOn)
	if err != nil {
		t.Fatalf("ListDepositsOn: %v", err)
	}
	for _, l := range listed {
		if l.ID() == d.ID() && len(l.Tenders()) != 2 {
			t.Errorf("the day's list lost the tenders: %d", len(l.Tenders()))
		}
	}

	receipt, err := repo.DepositReceipt(ctx, d.ID())
	if err != nil {
		t.Fatalf("DepositReceipt: %v", err)
	}
	if len(receipt.Lines) != 1 || receipt.Lines[0].AmountCents != 320000 {
		t.Errorf("receipt lines = %+v, want one line of 320000", receipt.Lines)
	}

	lines, err := repo.IncomeLines(ctx, paidOn, paidOn)
	if err != nil {
		t.Fatalf("IncomeLines: %v", err)
	}
	var money, note int64
	for _, l := range lines {
		if l.ConceptID != month.ConceptID() {
			continue
		}
		if l.CreditNote {
			note += l.AmountCents
		} else {
			money += l.AmountCents
		}
	}
	if money != 200000 || note != 120000 {
		t.Errorf("income money = %d, credit note = %d; want 200000 and 120000", money, note)
	}

	// RLS: another school sees none of it.
	if _, err := appRepo(t, tenantB).GetDeposit(ctx, d.ID()); !isNotFound(err) {
		t.Errorf("tenant B read tenant A's tendered deposit: %v", err)
	}
	if n := countTenders(t, tenantB, d.ID()); n != 0 {
		t.Errorf("tenant B sees %d of tenant A's tenders", n)
	}
	if n := countTenders(t, tenantA, d.ID()); n != 2 {
		t.Errorf("tenant A sees %d tenders, want 2", n)
	}
}

// A payment straight against a charge, combined: every row lands in one
// transaction, and a retry under the same key returns the whole set.
func TestACombinedPaymentIsAtomicAndReplays(t *testing.T) {
	ctx := context.Background()
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000008b1"
	paidOn := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000008b1", "uniformes t8",
		"61000000-0000-0000-0000-0000000008b1", student, "70000000-0000-0000-0000-0000000008b1", 100000, 0, paidOn)
	repo := appRepo(t, tenant)
	set := []domain.Payment{
		methodPayment(t, "91000000-0000-0000-0000-0000000008b1", tenant, charge.ID(), 40000, paidOn, domain.MethodCreditNote, "sale-1"),
		methodPayment(t, "91000000-0000-0000-0000-0000000008b2", tenant, charge.ID(), 60000, paidOn, domain.MethodCash, "sale-1#2"),
	}
	stored, after, replayed, err := repo.RegisterPayment(ctx, set, derive, nil)
	if err != nil || replayed {
		t.Fatalf("RegisterPayment: replayed=%v err=%v", replayed, err)
	}
	if len(stored) != 2 || after.PaidCents != 100000 || after.Charge.Status() != domain.StatusPaid {
		t.Fatalf("stored %d rows, paid %d (%s); want 2 rows paying it", len(stored), after.PaidCents, after.Charge.Status())
	}

	retry := []domain.Payment{
		methodPayment(t, "91000000-0000-0000-0000-0000000008b3", tenant, charge.ID(), 40000, paidOn, domain.MethodCreditNote, "sale-1"),
		methodPayment(t, "91000000-0000-0000-0000-0000000008b4", tenant, charge.ID(), 60000, paidOn, domain.MethodCash, "sale-1#2"),
	}
	again, _, replayed, err := repo.RegisterPayment(ctx, retry, derive, nil)
	if err != nil || !replayed {
		t.Fatalf("retry: replayed=%v err=%v", replayed, err)
	}
	if len(again) != 2 || again[0].ID() != stored[0].ID() || again[1].ID() != stored[1].ID() {
		t.Errorf("retry returned %+v, want the stored set in order", again)
	}
	if n := countPayments(t, tenant, charge.ID()); n != 2 {
		t.Errorf("payment rows = %d after a retry, want 2", n)
	}

	// An overpaying set is vetoed whole: neither row lands.
	other := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000008b2", "uniformes t8 b",
		"61000000-0000-0000-0000-0000000008b2", student, "70000000-0000-0000-0000-0000000008b2", 50000, 0, paidOn)
	over := []domain.Payment{
		methodPayment(t, "91000000-0000-0000-0000-0000000008b5", tenant, other.ID(), 30000, paidOn, domain.MethodCreditNote, ""),
		methodPayment(t, "91000000-0000-0000-0000-0000000008b6", tenant, other.ID(), 30000, paidOn, domain.MethodCash, ""),
	}
	if _, _, _, err := repo.RegisterPayment(ctx, over, derive, nil); err == nil {
		t.Fatal("an overpaying combined payment was accepted")
	}
	if n := countPayments(t, tenant, other.ID()); n != 0 {
		t.Errorf("a vetoed combined payment left %d rows", n)
	}
}

// countTenders reads deposit_tenders as the app role with the tenant bound,
// so it exercises RLS.
func countTenders(t *testing.T, tenant, depositID string) int64 {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, _ := db.DB()
	defer func() { _ = sqlDB.Close() }()
	var n int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT count(*) FROM deposit_tenders WHERE deposit_id = ?", depositID).Scan(&n).Error
	})
	if err != nil {
		t.Fatalf("count tenders: %v", err)
	}
	return n
}
