package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

func TestWithdrawalServiceCancelsOnlyEligibleChargesAndIsIdempotent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	withdrawnOn := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	concept := f.seedConcept(t, "Colegiatura", 10000)
	newCharge := func(id string, due time.Time, status string) domain.Charge {
		t.Helper()
		charge, err := domain.NewCharge(id, testTenant, testStudent, testCycle, concept.ID(), id, 10000, 0, due)
		if err != nil {
			t.Fatal(err)
		}
		if id == "due-future" {
			charge, err = charge.WithExternalRef("tuition:cycle:student:2026-10")
			if err != nil {
				t.Fatal(err)
			}
		}
		if status == domain.StatusCancelled {
			charge, err = charge.Cancel()
			if err != nil {
				t.Fatal(err)
			}
		}
		return charge
	}
	for _, charge := range []domain.Charge{
		newCharge("due-equal", withdrawnOn, domain.StatusPending),
		newCharge("due-future", withdrawnOn.AddDate(0, 0, 1), domain.StatusPending),
		newCharge("due-paid", withdrawnOn.AddDate(0, 0, 2), domain.StatusPending),
		newCharge("due-partial", withdrawnOn.AddDate(0, 0, 3), domain.StatusPending),
		newCharge("due-cancelled", withdrawnOn.AddDate(0, 0, 4), domain.StatusCancelled),
	} {
		f.repo.charges[charge.ID()] = charge
	}
	f.repo.payments["due-paid"] = []domain.Payment{withdrawalPayment(t, "pay-full", "due-paid", 10000, withdrawnOn)}
	f.repo.payments["due-partial"] = []domain.Payment{withdrawalPayment(t, "pay-partial", "due-partial", 2500, withdrawnOn)}

	result, err := f.svc.WithdrawStudent(ctx, testStudent, testCycle, withdrawnOn.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("WithdrawStudent: %v", err)
	}
	if len(result.ToCancel) != 1 || result.ToCancel[0].ID() != "due-future" {
		t.Fatalf("to cancel = %v, want due-future", chargeIDs(result.ToCancel))
	}
	if result.ToCancelCents != 10000 || result.RemainingDebtCents != 17500 {
		t.Fatalf("totals = cancel %d, remaining %d; want 10000 and 17500", result.ToCancelCents, result.RemainingDebtCents)
	}
	if got := f.repo.charges["due-future"].CancellationReason(); got != "baja" {
		t.Errorf("cancellation reason = %q, want baja", got)
	}

	again, err := f.svc.WithdrawStudent(ctx, testStudent, testCycle, withdrawnOn.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if len(again.ToCancel) != 0 {
		t.Errorf("redelivery cancelled %v again", chargeIDs(again.ToCancel))
	}
}

func TestPreviewWithdrawalDoesNotWrite(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Cuota", 7000)
	charge, err := domain.NewCharge("future", testTenant, testStudent, testCycle, concept.ID(), "Cuota", 7000, 0, fixedNow.AddDate(0, 0, 3))
	if err != nil {
		t.Fatal(err)
	}
	charge, err = charge.WithExternalRef("tuition:cycle:student:2026-10")
	if err != nil {
		t.Fatal(err)
	}
	f.repo.charges[charge.ID()] = charge

	preview, err := f.svc.PreviewWithdrawal(context.Background(), testStudent, testCycle, fixedNow.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("PreviewWithdrawal: %v", err)
	}
	if len(preview.ToCancel) != 1 || preview.ToCancel[0].ID() != charge.ID() || preview.ToCancelCents != 7000 {
		t.Errorf("preview = %+v", preview)
	}
	if f.repo.charges[charge.ID()].Status() != domain.StatusPending {
		t.Errorf("preview changed charge status to %q", f.repo.charges[charge.ID()].Status())
	}
}

func TestPreviewWithdrawalPricesAsOfTodayForBackdatedWithdrawal(t *testing.T) {
	b := newBecaFixture(t, 0, noPolicy)
	b.give(t, "Beca vigente hoy", becaSpec{kind: domain.ScholarshipPercent, value: 5000}, testStudent)
	b.clock = time.Date(2026, time.September, 15, 10, 0, 0, 0, time.UTC)

	preview, err := b.svc.PreviewWithdrawal(context.Background(), testStudent, testCycle, "2026-08-31")
	if err != nil {
		t.Fatalf("PreviewWithdrawal: %v", err)
	}
	if preview.ToCancelCents != 1650000 {
		t.Errorf("to cancel = %d, want 1650000 after today's scholarship and surcharge pricing", preview.ToCancelCents)
	}
}

func chargeIDs(charges []domain.Charge) []string {
	ids := make([]string, len(charges))
	for i, charge := range charges {
		ids[i] = charge.ID()
	}
	return ids
}

func withdrawalPayment(t *testing.T, id, chargeID string, cents int64, at time.Time) domain.Payment {
	t.Helper()
	payment, err := domain.NewPayment(id, testTenant, chargeID, cents, domain.MethodCash, "", at, "")
	if err != nil {
		t.Fatal(err)
	}
	return payment
}

func TestWithdrawalEndsStudentActivityEnrollmentsOnEffectiveDate(t *testing.T) {
	f := newFixture(t)
	on := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	repo := newFakeActivityRepo()
	activity, err := domain.NewActivity(domain.ActivitySpec{
		ID: "act-1", Name: "Natación",
		Schedule:        []domain.ActivitySlot{{Weekday: 2, StartMin: 15 * 60, EndMin: 16 * 60}},
		Periodicity:     domain.PeriodicityMonthly,
		BillingStartsOn: on.AddDate(0, -1, 0), FirstDueOn: on.AddDate(0, -1, 9), Active: true,
		Options: []domain.PriceOption{{ID: "opt-1", DaysPerWeek: 1, AmountCents: 10000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo.activities[activity.ID()] = activity
	enrollment, err := domain.NewActivityEnrollment("activity-enrollment", activity.ID(), testStudent, "opt-1", on.AddDate(0, 0, -20))
	if err != nil {
		t.Fatal(err)
	}
	repo.enrollments = append(repo.enrollments, enrollment)
	f.svc.WithActivities(repo)

	if _, err := f.svc.WithdrawStudent(context.Background(), testStudent, testCycle, on.Format("2006-01-02")); err != nil {
		t.Fatalf("WithdrawStudent: %v", err)
	}
	if got := repo.enrollments[0].EndsOn(); !got.Equal(on) {
		t.Errorf("activity ends_on = %s, want %s", got.Format("2006-01-02"), on.Format("2006-01-02"))
	}
	if _, err := f.svc.WithdrawStudent(context.Background(), testStudent, testCycle, on.Format("2006-01-02")); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
}
