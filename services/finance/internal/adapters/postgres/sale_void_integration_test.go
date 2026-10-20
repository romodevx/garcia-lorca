//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// seedSale is a uniforms counter sale as finance sees it: a sale charge tagged
// uniforms:<fulfillment> and one loose payment on it, dated paidOn.
func seedSale(t *testing.T, chargeID, paymentID, ref string, paidOn time.Time) {
	t.Helper()
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const conceptID = "c0000000-0000-0000-0000-00000000c215"
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "uniformes-c215", 25000)); err != nil &&
		glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("seed concept: %v", err)
	}
	sale, err := domain.NewSaleCharge(chargeID, tenantA, "50000000-0000-0000-0000-00000000c215", conceptID,
		"Uniformes · U-2026-000215", 25000, paidOn, ref)
	if err != nil {
		t.Fatalf("new sale charge: %v", err)
	}
	if _, err := repo.CreateCharge(ctx, sale); err != nil {
		t.Fatalf("seed charge: %v", err)
	}
	if _, _, _, err := repo.RegisterPayment(ctx, one(newPayment(t, paymentID, tenantA, chargeID, 25000, paidOn)), derive, nil); err != nil {
		t.Fatalf("seed payment: %v", err)
	}
}

// TestVoidSalePaymentsOnTheDayOfTheSale: C2-015 — returned the day it was sold,
// the sale's money leaves the day's takings, stays on record as cancelled with
// why and who, and the charge is cancelled. A retry changes nothing.
func TestVoidSalePaymentsOnTheDayOfTheSale(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const (
		chargeID  = "d0000000-0000-0000-0000-00000000c215"
		paymentID = "e0000000-0000-0000-0000-00000000c215"
		actorID   = "a0000000-0000-0000-0000-00000000c215"
	)
	day := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	seedSale(t, chargeID, paymentID, "uniforms:f0000000-0000-0000-0000-00000000c215", day)
	before := time.Now().Add(-time.Second)

	voided, err := repo.VoidSalePayments(ctx, chargeID, "Talla equivocada", actorID, day)
	if err != nil {
		t.Fatalf("VoidSalePayments: %v", err)
	}
	if len(voided) != 1 || voided[0].AmountCents != 25000 || voided[0].Reason != "Talla equivocada" {
		t.Fatalf("voided = %+v, want the one $250 payment with its reason", voided)
	}

	loose, err := repo.ListLoosePaymentsOn(ctx, day)
	if err != nil {
		t.Fatalf("ListLoosePaymentsOn: %v", err)
	}
	for _, p := range loose {
		if p.Payment.ID() == paymentID {
			t.Fatal("the voided payment still counts in the day's takings")
		}
	}
	listed, err := repo.ListVoidedSalePaymentsOn(ctx, day)
	if err != nil {
		t.Fatalf("ListVoidedSalePaymentsOn: %v", err)
	}
	if !containsVoided(listed, paymentID) {
		t.Errorf("the cash-up cannot list the cancelled sale: %+v", listed)
	}
	changed, err := repo.DayChangedSince(ctx, day, before)
	if err != nil || !changed {
		t.Errorf("DayChangedSince = %v, %v; a closed cash-up must see the void", changed, err)
	}

	var charge struct {
		Status      string
		CancelledBy *string
	}
	if err := superDB.Raw("SELECT status, cancelled_by::text AS cancelled_by FROM charges WHERE id = ?", chargeID).Scan(&charge).Error; err != nil {
		t.Fatalf("read charge: %v", err)
	}
	if charge.Status != "cancelled" || charge.CancelledBy == nil || *charge.CancelledBy != actorID {
		t.Errorf("charge = %+v, want cancelled by %s", charge, actorID)
	}

	again, err := repo.VoidSalePayments(ctx, chargeID, "Talla equivocada", actorID, day)
	if err != nil || len(again) != 1 {
		t.Fatalf("retry = %+v, %v; want the same void, no error", again, err)
	}
}

// TestVoidSalePaymentsRefusesAnotherDay: a later day's return is an expense of
// that day; the money already counted on a past day is not rewritten.
func TestVoidSalePaymentsRefusesAnotherDay(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const chargeID = "d0000000-0000-0000-0000-00000000c216"
	sold := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	seedSale(t, chargeID, "e0000000-0000-0000-0000-00000000c216", "uniforms:f0000000-0000-0000-0000-00000000c216", sold)

	_, err := repo.VoidSalePayments(ctx, chargeID, "Talla equivocada", "", sold.AddDate(0, 0, 1))

	requireKindIs(t, err, glerrors.KindConflict)
}

// TestVoidSalePaymentsOnlyForASale: a tuition charge is never voided this way.
func TestVoidSalePaymentsOnlyForASale(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const (
		conceptID = "c0000000-0000-0000-0000-00000000c217"
		chargeID  = "d0000000-0000-0000-0000-00000000c217"
	)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "colegiatura-c217", 25000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	if _, err := repo.CreateCharge(ctx, newCharge(t, chargeID, tenantA, "50000000-0000-0000-0000-00000000c217", cycleOne, conceptID, 25000, 0, tomorrow())); err != nil {
		t.Fatalf("seed charge: %v", err)
	}

	_, err := repo.VoidSalePayments(ctx, chargeID, "no", "", today)

	requireKindIs(t, err, glerrors.KindInvalid)
}

func containsVoided(list []ports.VoidedSalePayment, paymentID string) bool {
	for _, v := range list {
		if v.PaymentID == paymentID {
			return true
		}
	}
	return false
}

func requireKindIs(t *testing.T, err error, want glerrors.Kind) {
	t.Helper()
	if glerrors.KindOf(err) != want {
		t.Fatalf("err = %v, want kind %v", err, want)
	}
}
