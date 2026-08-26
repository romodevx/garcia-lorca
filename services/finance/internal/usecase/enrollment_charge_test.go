package usecase_test

import (
	"context"
	"testing"
)

// The rule the school chose: the inscription appears because the pupil took a
// place in a group, not because somebody billed the whole roll.
func TestEnsureEnrollmentChargeIssuesTheInscription(t *testing.T) {
	f := billingFixture(t)

	created, err := f.svc.EnsureEnrollmentCharge(context.Background(), testCycle, testStudent)
	if err != nil {
		t.Fatalf("EnsureEnrollmentCharge: %v", err)
	}
	if !created {
		t.Fatal("an enrolled pupil at a priced level must be charged their inscription")
	}
}

// Pub/Sub delivers at least once. There is no idempotency ledger here — the
// charge's own external ref is the ledger — so this is the test that proves it.
func TestEnsureEnrollmentChargeSurvivesRedelivery(t *testing.T) {
	f := billingFixture(t)
	ctx := context.Background()

	if _, err := f.svc.EnsureEnrollmentCharge(ctx, testCycle, testStudent); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	created, err := f.svc.EnsureEnrollmentCharge(ctx, testCycle, testStudent)
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if created {
		t.Error("a redelivered event charged the family a second inscription")
	}
}

// And the monthly run must not double up on what the event already issued.
func TestTheMonthlyRunDoesNotRechargeAnInscriptionTheEventAlreadyIssued(t *testing.T) {
	f := billingFixture(t)
	ctx := context.Background()

	if _, err := f.svc.EnsureEnrollmentCharge(ctx, testCycle, testStudent); err != nil {
		t.Fatalf("event: %v", err)
	}
	got, err := f.svc.RunBilling(ctx, testCycle, "2026-11-15")
	if err != nil {
		t.Fatalf("RunBilling: %v", err)
	}

	if got.EnrollmentCreated != 0 {
		t.Errorf("the run issued %d inscriptions on top of the event's", got.EnrollmentCreated)
	}
}

// A pupil at a level nobody priced is not charged zero. Silence is better than a
// wrong number: the monthly run reports the unpriced level to a person.
func TestEnsureEnrollmentChargeSkipsAnUnpricedLevel(t *testing.T) {
	f := billingFixture(t)
	f.repo.plans = clearPlans()

	created, err := f.svc.EnsureEnrollmentCharge(context.Background(), testCycle, testStudent)
	if err != nil {
		t.Fatalf("EnsureEnrollmentCharge: %v", err)
	}
	if created {
		t.Error("a level with no tuition table must not produce a zero-peso inscription")
	}
}
