package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// useSeededConcept selects the fixture's own concept in the table tests.
const useSeededConcept = ""

// --- concepts ----------------------------------------------------------------

func TestCreateConcept(t *testing.T) {
	f := newFixture(t)

	c, err := f.svc.CreateConcept(context.Background(), "Colegiatura", 250000, domain.RecurrenceMonthly)

	requireNoError(t, err)
	if c.ID() == "" || c.TenantID() != testTenant {
		t.Fatalf("identity = (%q, %q), want a minted id under tenant %q", c.ID(), c.TenantID(), testTenant)
	}
	if c.Name() != "Colegiatura" || c.DefaultAmount().Int64() != 250000 || !c.Active() {
		t.Fatalf("concept = (%q, %d, active=%v), want (Colegiatura, 250000, active=true)",
			c.Name(), c.DefaultAmount().Int64(), c.Active())
	}
}

func TestCreateConceptRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		conceptName string
		amount      int64
		recurrence  string
	}{
		{"blank name", "  ", 250000, domain.RecurrenceMonthly},
		{"negative amount", "Colegiatura", -1, domain.RecurrenceMonthly},
		{"unknown recurrence", "Colegiatura", 250000, "weekly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			_, err := f.svc.CreateConcept(context.Background(), tt.conceptName, tt.amount, tt.recurrence)

			requireKind(t, err, glerrors.KindInvalid)
		})
	}
}

func TestCreateConceptDuplicateNameConflicts(t *testing.T) {
	f := newFixture(t)
	f.seedConcept(t, "Colegiatura", 250000)

	_, err := f.svc.CreateConcept(context.Background(), "Colegiatura", 300000, domain.RecurrenceMonthly)

	requireKind(t, err, glerrors.KindConflict)
}

func TestListConcepts(t *testing.T) {
	f := newFixture(t)
	active := f.seedConcept(t, "Colegiatura", 250000)
	retired := f.seedConcept(t, "Excursión 2024", 50000)
	if _, err := f.svc.UpdateConcept(context.Background(), retired.ID(), retired.Name(), 50000, domain.RecurrenceOnce, false); err != nil {
		t.Fatalf("deactivate concept: %v", err)
	}

	visible, err := f.svc.ListConcepts(context.Background(), false)
	requireNoError(t, err)
	if len(visible) != 1 || visible[0].ID() != active.ID() {
		t.Fatalf("active-only list = %v, want just %q", ids(visible), active.ID())
	}

	all, err := f.svc.ListConcepts(context.Background(), true)
	requireNoError(t, err)
	if len(all) != 2 {
		t.Fatalf("full list = %v, want both concepts", ids(all))
	}
}

func TestUpdateConcept(t *testing.T) {
	f := newFixture(t)
	seeded := f.seedConcept(t, "Colegiatura", 250000)

	updated, err := f.svc.UpdateConcept(context.Background(), seeded.ID(), "Colegiatura 2026", 300000, domain.RecurrenceMonthly, false)

	requireNoError(t, err)
	if updated.ID() != seeded.ID() {
		t.Fatalf("id = %q, want the identity preserved (%q)", updated.ID(), seeded.ID())
	}
	if updated.Name() != "Colegiatura 2026" || updated.DefaultAmount().Int64() != 300000 || updated.Active() {
		t.Fatalf("updated = (%q, %d, active=%v), want (Colegiatura 2026, 300000, active=false)",
			updated.Name(), updated.DefaultAmount().Int64(), updated.Active())
	}
}

func TestUpdateConceptMissingIsInvalid(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.UpdateConcept(context.Background(), "ghost", "Colegiatura", 250000, domain.RecurrenceMonthly, true)

	requireKind(t, err, glerrors.KindInvalid)
	if errors.Is(err, glerrors.ErrNotFound) {
		t.Fatal("a bad client reference must not leak NotFound")
	}
}

// --- charges -----------------------------------------------------------------

func TestCreateCharge(t *testing.T) {
	const (
		conceptName   = "Colegiatura"
		conceptAmount = int64(250000)
	)
	tests := []struct {
		name        string
		studentID   string
		cycleID     string
		conceptID   string
		amount      int64
		description string
		dueDate     string
		wantKind    glerrors.Kind
		wantAmount  int64
		wantDesc    string
	}{
		{
			name: "student not found", studentID: "ghost", cycleID: testCycle, conceptID: useSeededConcept,
			dueDate: testDueDate, wantKind: glerrors.KindInvalid,
		},
		{
			name: "cycle not found", studentID: testStudent, cycleID: "ghost", conceptID: useSeededConcept,
			dueDate: testDueDate, wantKind: glerrors.KindInvalid,
		},
		{
			name: "concept not found", studentID: testStudent, cycleID: testCycle, conceptID: "ghost",
			dueDate: testDueDate, wantKind: glerrors.KindInvalid,
		},
		{
			name: "due date is not YYYY-MM-DD", studentID: testStudent, cycleID: testCycle, conceptID: useSeededConcept,
			dueDate: "05/09/2026", wantKind: glerrors.KindInvalid,
		},
		{
			name: "amount zero falls back to the concept default", studentID: testStudent, cycleID: testCycle,
			conceptID: useSeededConcept, amount: 0, dueDate: testDueDate,
			wantAmount: conceptAmount, wantDesc: conceptName,
		},
		{
			name: "empty description falls back to the concept name", studentID: testStudent, cycleID: testCycle,
			conceptID: useSeededConcept, amount: 199900, description: "", dueDate: testDueDate,
			wantAmount: 199900, wantDesc: conceptName,
		},
		{
			name: "explicit overrides win", studentID: testStudent, cycleID: testCycle,
			conceptID: useSeededConcept, amount: 199900, description: "Colegiatura con beca", dueDate: testDueDate,
			wantAmount: 199900, wantDesc: "Colegiatura con beca",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			concept := f.seedConcept(t, conceptName, conceptAmount)
			conceptID := tt.conceptID
			if conceptID == useSeededConcept {
				conceptID = concept.ID()
			}

			got, err := f.svc.CreateCharge(context.Background(), tt.studentID, tt.cycleID, conceptID,
				tt.amount, tt.description, tt.dueDate)

			if tt.wantKind != glerrors.KindUnknown {
				requireKind(t, err, tt.wantKind)
				return
			}
			requireNoError(t, err)
			if got.Charge.Amount().Int64() != tt.wantAmount {
				t.Errorf("amount = %d, want %d", got.Charge.Amount().Int64(), tt.wantAmount)
			}
			if got.Charge.Description() != tt.wantDesc {
				t.Errorf("description = %q, want %q", got.Charge.Description(), tt.wantDesc)
			}
			if got.PaidCents != 0 {
				t.Errorf("paid = %d, want 0 on a brand-new charge", got.PaidCents)
			}
			if got.Charge.Status() != domain.StatusPending {
				t.Errorf("status = %q, want %q", got.Charge.Status(), domain.StatusPending)
			}
			wantDue := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
			if !got.Charge.DueDate().Equal(wantDue) {
				t.Errorf("due date = %v, want %v", got.Charge.DueDate(), wantDue)
			}
		})
	}
}

func TestCreateChargeDuplicateConflicts(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Inscripción", 500000)
	f.seedCharge(t, concept.ID(), 0)

	_, err := f.svc.CreateCharge(context.Background(), testStudent, testCycle, concept.ID(), 0, "", testDueDate)

	requireKind(t, err, glerrors.KindConflict)
}

// --- bulk generation ---------------------------------------------------------

// seedEnrollments loads the students-service double with one cycle's roster.
func seedEnrollments(f *fixture) {
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{
		{ID: "enr-1", StudentID: testStudent, GroupID: "grp-A", CycleID: testCycle, Status: "active"},
		{ID: "enr-2", StudentID: "stu-2", GroupID: "grp-A", CycleID: testCycle, Status: "active"},
		{ID: "enr-3", StudentID: "stu-3", GroupID: "grp-B", CycleID: testCycle, Status: "active"},
		{ID: "enr-4", StudentID: "stu-4", GroupID: "grp-A", CycleID: testCycle, Status: "withdrawn"},
	}
}

func TestGenerateCharges(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	seedEnrollments(f)

	created, skipped, err := f.svc.GenerateCharges(context.Background(), concept.ID(), testCycle, "", 0, testDueDate)

	requireNoError(t, err)
	if created != 3 || skipped != 0 {
		t.Fatalf("created/skipped = %d/%d, want 3/0 (one per ACTIVE enrollment)", created, skipped)
	}
	if f.enrollments.gotCycleID != testCycle || f.enrollments.gotGroupID != "" {
		t.Fatalf("reader called with (%q, %q), want (%q, \"\")", f.enrollments.gotCycleID, f.enrollments.gotGroupID, testCycle)
	}
	for _, c := range f.repo.charges {
		if c.StudentID() == "stu-4" {
			t.Fatal("a withdrawn enrollment must not be charged")
		}
		if c.Amount().Int64() != 250000 || c.Description() != "Colegiatura" {
			t.Fatalf("charge = (%d, %q), want the concept default and name", c.Amount().Int64(), c.Description())
		}
	}
}

func TestGenerateChargesNarrowsByGroup(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	seedEnrollments(f)

	created, skipped, err := f.svc.GenerateCharges(context.Background(), concept.ID(), testCycle, "grp-A", 199900, testDueDate)

	requireNoError(t, err)
	if created != 2 || skipped != 0 {
		t.Fatalf("created/skipped = %d/%d, want 2/0 (active enrollments of grp-A)", created, skipped)
	}
	if f.enrollments.gotGroupID != "grp-A" {
		t.Fatalf("reader group filter = %q, want grp-A", f.enrollments.gotGroupID)
	}
	for _, c := range f.repo.charges {
		if c.Amount().Int64() != 199900 {
			t.Fatalf("amount = %d, want the 199900 override", c.Amount().Int64())
		}
	}
}

func TestGenerateChargesSkipsExisting(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	seedEnrollments(f)
	f.seedCharge(t, concept.ID(), 0) // stu-1 already owes this concept for the cycle

	created, skipped, err := f.svc.GenerateCharges(context.Background(), concept.ID(), testCycle, "", 0, testDueDate)

	requireNoError(t, err)
	if created != 2 || skipped != 1 {
		t.Fatalf("created/skipped = %d/%d, want 2/1 — re-running must be idempotent", created, skipped)
	}
}

func TestGenerateChargesRejectsBadInput(t *testing.T) {
	tests := []struct {
		name      string
		conceptID string
		cycleID   string
		dueDate   string
	}{
		{"cycle not found", useSeededConcept, "ghost", testDueDate},
		{"concept not found", "ghost", testCycle, testDueDate},
		{"due date is not YYYY-MM-DD", useSeededConcept, testCycle, "next friday"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			concept := f.seedConcept(t, "Colegiatura", 250000)
			seedEnrollments(f)
			conceptID := tt.conceptID
			if conceptID == useSeededConcept {
				conceptID = concept.ID()
			}

			_, _, err := f.svc.GenerateCharges(context.Background(), conceptID, tt.cycleID, "", 0, tt.dueDate)

			requireKind(t, err, glerrors.KindInvalid)
			if len(f.repo.charges) != 0 {
				t.Fatal("no charge may be written when validation fails")
			}
		})
	}
}

// --- discounts and cancellation ----------------------------------------------

func TestApplyDiscount(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.ID(), 100000)

	got, err := f.svc.ApplyDiscount(context.Background(), charge.ID(), 100000, "beca completa")

	requireNoError(t, err)
	if got.Charge.Discount().Int64() != 100000 || got.Charge.Effective().Int64() != 0 {
		t.Fatalf("discount/effective = %d/%d, want 100000/0",
			got.Charge.Discount().Int64(), got.Charge.Effective().Int64())
	}
	// Nothing has been paid, so a fully discounted charge stays pending rather
	// than reporting itself as collected (domain rule).
	if got.Charge.Status() != domain.StatusPending {
		t.Fatalf("status = %q, want %q on a fully discounted, unpaid charge", got.Charge.Status(), domain.StatusPending)
	}
	if stored := f.repo.charges[charge.ID()]; stored.Discount().Int64() != 100000 {
		t.Fatalf("stored discount = %d, want the discount persisted", stored.Discount().Int64())
	}
}

func TestApplyDiscountSettlesAPartiallyPaidCharge(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.ID(), 100000)
	if _, _, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 40000, domain.MethodCash, ""); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	got, err := f.svc.ApplyDiscount(context.Background(), charge.ID(), 60000, "beca parcial")

	requireNoError(t, err)
	if got.PaidCents != 40000 {
		t.Fatalf("paid = %d, want the 40000 already collected", got.PaidCents)
	}
	if got.Charge.Status() != domain.StatusPaid {
		t.Fatalf("status = %q, want %q — the discount settles the remaining balance",
			got.Charge.Status(), domain.StatusPaid)
	}
}

func TestApplyDiscountRejected(t *testing.T) {
	t.Run("discount above the amount", func(t *testing.T) {
		f := newFixture(t)
		concept := f.seedConcept(t, "Colegiatura", 250000)
		charge := f.seedCharge(t, concept.ID(), 100000)

		_, err := f.svc.ApplyDiscount(context.Background(), charge.ID(), 100001, "")

		requireKind(t, err, glerrors.KindInvalid)
	})

	t.Run("cancelled charge", func(t *testing.T) {
		f := newFixture(t)
		concept := f.seedConcept(t, "Colegiatura", 250000)
		charge := f.seedCharge(t, concept.ID(), 100000)
		if _, err := f.svc.CancelCharge(context.Background(), charge.ID()); err != nil {
			t.Fatalf("seed cancel: %v", err)
		}

		_, err := f.svc.ApplyDiscount(context.Background(), charge.ID(), 10000, "")

		requireKind(t, err, glerrors.KindInvalid)
	})

	t.Run("unknown charge", func(t *testing.T) {
		f := newFixture(t)

		_, err := f.svc.ApplyDiscount(context.Background(), "ghost", 10000, "")

		requireKind(t, err, glerrors.KindInvalid)
		if errors.Is(err, glerrors.ErrNotFound) {
			t.Fatal("a bad client reference must not leak NotFound")
		}
	})
}

func TestCancelCharge(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.ID(), 100000)

	got, err := f.svc.CancelCharge(context.Background(), charge.ID())

	requireNoError(t, err)
	if got.Charge.Status() != domain.StatusCancelled {
		t.Fatalf("status = %q, want %q", got.Charge.Status(), domain.StatusCancelled)
	}
	if stored := f.repo.charges[charge.ID()]; stored.Status() != domain.StatusCancelled {
		t.Fatalf("stored status = %q, want the cancellation persisted", stored.Status())
	}

	_, err = f.svc.CancelCharge(context.Background(), charge.ID())

	requireKind(t, err, glerrors.KindConflict)
}

func TestCancelChargeUnknownIsInvalid(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.CancelCharge(context.Background(), "ghost")

	requireKind(t, err, glerrors.KindInvalid)
}

// --- payments ----------------------------------------------------------------

func TestRegisterPayment(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.ID(), 100000)

	stored, cwp, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 40000, domain.MethodTransfer, "SPEI-778")

	requireNoError(t, err)
	if stored.ID() == "" || stored.ChargeID() != charge.ID() || stored.Amount().Int64() != 40000 {
		t.Fatalf("payment = (%q, %q, %d), want a minted id against %q for 40000",
			stored.ID(), stored.ChargeID(), stored.Amount().Int64(), charge.ID())
	}
	if !stored.PaidAt().Equal(fixedNow) {
		t.Fatalf("paidAt = %v, want the injected clock %v", stored.PaidAt(), fixedNow)
	}
	if cwp.PaidCents != 40000 || cwp.Charge.Status() != domain.StatusPartiallyPaid {
		t.Fatalf("charge = (%d paid, %q), want (40000, %q)",
			cwp.PaidCents, cwp.Charge.Status(), domain.StatusPartiallyPaid)
	}

	// The second payment closes the balance.
	_, cwp, err = f.svc.RegisterPayment(context.Background(), charge.ID(), 60000, domain.MethodCash, "")

	requireNoError(t, err)
	if cwp.PaidCents != 100000 || cwp.Charge.Status() != domain.StatusPaid {
		t.Fatalf("charge = (%d paid, %q), want (100000, %q)",
			cwp.PaidCents, cwp.Charge.Status(), domain.StatusPaid)
	}
}

func TestRegisterPaymentPublishesTheEvent(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.ID(), 100000)

	stored, _, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 40000, domain.MethodCard, "auth-9")

	requireNoError(t, err)
	if len(f.publisher.events) != 1 {
		t.Fatalf("published %d events, want exactly 1", len(f.publisher.events))
	}
	want := ports.PaymentRegisteredEvent{
		PaymentID:   stored.ID(),
		StudentID:   testStudent,
		ChargeID:    charge.ID(),
		TenantID:    testTenant,
		Method:      domain.MethodCard,
		AmountCents: 40000,
		PaidAt:      fixedNow,
	}
	if f.publisher.events[0] != want {
		t.Fatalf("event = %+v, want %+v", f.publisher.events[0], want)
	}
}

func TestRegisterPaymentSurvivesPublisherFailures(t *testing.T) {
	t.Run("publish error does not fail the payment", func(t *testing.T) {
		f := newFixture(t)
		f.publisher.err = errors.New("pubsub unavailable")
		concept := f.seedConcept(t, "Colegiatura", 250000)
		charge := f.seedCharge(t, concept.ID(), 100000)

		_, cwp, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 40000, domain.MethodCash, "")

		requireNoError(t, err)
		if cwp.PaidCents != 40000 {
			t.Fatalf("paid = %d, want the payment to have landed anyway", cwp.PaidCents)
		}
	})

	t.Run("nil publisher does not panic", func(t *testing.T) {
		f := newFixture(t)
		concept := f.seedConcept(t, "Colegiatura", 250000)
		charge := f.seedCharge(t, concept.ID(), 100000)
		svc := usecase.NewFinanceService(f.repo, f.students, f.cycles, f.enrollments, nil,
			testTenant, seqID("np"), func() time.Time { return fixedNow })

		_, cwp, err := svc.RegisterPayment(context.Background(), charge.ID(), 40000, domain.MethodCash, "")

		requireNoError(t, err)
		if cwp.PaidCents != 40000 {
			t.Fatalf("paid = %d, want 40000", cwp.PaidCents)
		}
	})
}

func TestRegisterPaymentRejected(t *testing.T) {
	t.Run("overpay stores nothing", func(t *testing.T) {
		f := newFixture(t)
		concept := f.seedConcept(t, "Colegiatura", 250000)
		charge := f.seedCharge(t, concept.ID(), 100000)

		_, _, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 100001, domain.MethodCash, "")

		requireKind(t, err, glerrors.KindInvalid)
		if paid := f.repo.paidFor(charge.ID()); paid != 0 {
			t.Fatalf("paid = %d, want nothing stored after a rejected payment", paid)
		}
		if got := f.repo.charges[charge.ID()].Status(); got != domain.StatusPending {
			t.Fatalf("status = %q, want it untouched (%q)", got, domain.StatusPending)
		}
		if len(f.publisher.events) != 0 {
			t.Fatal("a rejected payment must not publish an event")
		}
	})

	t.Run("overpay after a partial payment", func(t *testing.T) {
		f := newFixture(t)
		concept := f.seedConcept(t, "Colegiatura", 250000)
		charge := f.seedCharge(t, concept.ID(), 100000)
		if _, _, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 40000, domain.MethodCash, ""); err != nil {
			t.Fatalf("seed payment: %v", err)
		}

		_, _, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 60001, domain.MethodCash, "")

		requireKind(t, err, glerrors.KindInvalid)
		if paid := f.repo.paidFor(charge.ID()); paid != 40000 {
			t.Fatalf("paid = %d, want only the first 40000", paid)
		}
	})

	t.Run("cancelled charge", func(t *testing.T) {
		f := newFixture(t)
		concept := f.seedConcept(t, "Colegiatura", 250000)
		charge := f.seedCharge(t, concept.ID(), 100000)
		if _, err := f.svc.CancelCharge(context.Background(), charge.ID()); err != nil {
			t.Fatalf("seed cancel: %v", err)
		}

		_, _, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 10000, domain.MethodCash, "")

		requireKind(t, err, glerrors.KindInvalid)
		if paid := f.repo.paidFor(charge.ID()); paid != 0 {
			t.Fatalf("paid = %d, want nothing stored against a cancelled charge", paid)
		}
	})

	t.Run("unknown charge", func(t *testing.T) {
		f := newFixture(t)

		_, _, err := f.svc.RegisterPayment(context.Background(), "ghost", 10000, domain.MethodCash, "")

		requireKind(t, err, glerrors.KindInvalid)
		if errors.Is(err, glerrors.ErrNotFound) {
			t.Fatal("a bad client reference must not leak NotFound")
		}
	})

	t.Run("non-positive amount", func(t *testing.T) {
		f := newFixture(t)
		concept := f.seedConcept(t, "Colegiatura", 250000)
		charge := f.seedCharge(t, concept.ID(), 100000)

		_, _, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 0, domain.MethodCash, "")

		requireKind(t, err, glerrors.KindInvalid)
	})

	t.Run("unknown method", func(t *testing.T) {
		f := newFixture(t)
		concept := f.seedConcept(t, "Colegiatura", 250000)
		charge := f.seedCharge(t, concept.ID(), 100000)

		_, _, err := f.svc.RegisterPayment(context.Background(), charge.ID(), 10000, "crypto", "")

		requireKind(t, err, glerrors.KindInvalid)
	})
}

// --- account statement and overdue -------------------------------------------

func TestGetAccount(t *testing.T) {
	f := newFixture(t)
	f.students.byID["stu-2"] = ports.StudentRef{ID: "stu-2", Status: "active"}
	tuition := f.seedConcept(t, "Colegiatura", 250000)
	inscription := f.seedConcept(t, "Inscripción", 500000)
	trip := f.seedConcept(t, "Excursión", 30000)
	ctx := context.Background()

	partly := f.seedCharge(t, tuition.ID(), 100000)
	if _, _, err := f.svc.RegisterPayment(ctx, partly.ID(), 40000, domain.MethodCash, ""); err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	settled := f.seedCharge(t, inscription.ID(), 50000)
	if _, _, err := f.svc.RegisterPayment(ctx, settled.ID(), 50000, domain.MethodCash, ""); err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	voided := f.seedCharge(t, trip.ID(), 30000)
	if _, err := f.svc.CancelCharge(ctx, voided.ID()); err != nil {
		t.Fatalf("seed cancel: %v", err)
	}
	// Another student's debt must never reach this statement.
	if _, err := f.svc.CreateCharge(ctx, "stu-2", testCycle, tuition.ID(), 777000, "", testDueDate); err != nil {
		t.Fatalf("seed other student: %v", err)
	}

	got, err := f.svc.GetAccount(ctx, testStudent)

	requireNoError(t, err)
	if len(got.Charges) != 3 {
		t.Fatalf("charges = %d, want the student's 3 (cancelled included, other students excluded)", len(got.Charges))
	}
	if len(got.Payments) != 2 {
		t.Fatalf("payments = %d, want 2", len(got.Payments))
	}
	// 60000 outstanding on the partly-paid charge; the settled one owes nothing
	// and the cancelled one is excluded outright.
	if got.TotalBalanceCents != 60000 {
		t.Fatalf("balance = %d, want 60000", got.TotalBalanceCents)
	}
}

func TestGetAccountOverpaidChargeNeverGoesNegative(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	charge := f.seedCharge(t, concept.ID(), 100000)
	ctx := context.Background()
	if _, _, err := f.svc.RegisterPayment(ctx, charge.ID(), 100000, domain.MethodCash, ""); err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	// A later discount leaves the charge collecting more than it now owes.
	if _, err := f.svc.ApplyDiscount(ctx, charge.ID(), 20000, "ajuste"); err != nil {
		t.Fatalf("seed discount: %v", err)
	}

	got, err := f.svc.GetAccount(ctx, testStudent)

	requireNoError(t, err)
	if got.TotalBalanceCents != 0 {
		t.Fatalf("balance = %d, want 0 — a credit never subtracts from the debt", got.TotalBalanceCents)
	}
}

func TestListOverduePassesTheInjectedClock(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	overdue := f.seedCharge(t, concept.ID(), 100000) // due 2026-09-05, before fixedNow

	got, err := f.svc.ListOverdue(context.Background(), testCycle)

	requireNoError(t, err)
	if !f.repo.overdueToday.Equal(fixedNow) {
		t.Fatalf("repo received today = %v, want the injected clock %v", f.repo.overdueToday, fixedNow)
	}
	if f.repo.overdueCycleID != testCycle {
		t.Fatalf("repo received cycle = %q, want %q", f.repo.overdueCycleID, testCycle)
	}
	if len(got) != 1 || got[0].Charge.ID() != overdue.ID() {
		t.Fatalf("overdue = %v, want just %q", got, overdue.ID())
	}
}

// TestListChargesFiltersAndCarriesTheCollectedTotal proves the read reaches the
// ledger with both filters applied and returns each charge paired with what has
// already been collected against it — the pair the wire's derived paid/remaining
// fields are built from.
func TestListChargesFiltersAndCarriesTheCollectedTotal(t *testing.T) {
	const otherStudent = "stu-2"
	f := newFixture(t)
	f.students.byID[otherStudent] = ports.StudentRef{ID: otherStudent, Status: "active"}
	concept := f.seedConcept(t, "Colegiatura", 250000)
	ctx := context.Background()

	mine := f.seedCharge(t, concept.ID(), 100000)
	if _, err := f.svc.CreateCharge(ctx, otherStudent, testCycle, concept.ID(), 100000, "", testDueDate); err != nil {
		t.Fatalf("seed another student's charge: %v", err)
	}
	if _, _, err := f.svc.RegisterPayment(ctx, mine.ID(), 40000, domain.MethodCash, ""); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	got, err := f.svc.ListCharges(ctx, testStudent, testCycle)

	requireNoError(t, err)
	if len(got) != 1 || got[0].Charge.ID() != mine.ID() {
		t.Fatalf("charges = %v, want just %q — the student filter must reach the ledger", got, mine.ID())
	}
	if got[0].PaidCents != 40000 {
		t.Fatalf("paid = %d, want 40000", got[0].PaidCents)
	}
}

// ids extracts concept ids for readable failure messages.
func ids(cs []domain.ChargeConcept) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID())
	}
	return out
}
