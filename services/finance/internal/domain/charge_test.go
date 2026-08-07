package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

var dueDate = time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)

func validCharge(t *testing.T, amount, discount int64) domain.Charge {
	t.Helper()
	c, err := domain.NewCharge("ch-1", "t1", "stu-1", "cyc-1", "cc-1", "Colegiatura Septiembre",
		amount, discount, dueDate)
	if err != nil {
		t.Fatalf("NewCharge(%d, %d): %v", amount, discount, err)
	}
	return c
}

func TestNewChargeAccessors(t *testing.T) {
	c := validCharge(t, 250000, 50000)
	if c.ID() != "ch-1" || c.TenantID() != "t1" || c.StudentID() != "stu-1" ||
		c.CycleID() != "cyc-1" || c.ConceptID() != "cc-1" || c.Description() != "Colegiatura Septiembre" ||
		c.Amount().Int64() != 250000 || c.Discount().Int64() != 50000 ||
		!c.DueDate().Equal(dueDate) || c.Status() != domain.StatusPending {
		t.Errorf("accessors mismatch: %+v", c)
	}
}

func TestNewChargeValidation(t *testing.T) {
	cases := []struct {
		name                                                     string
		id, tenantID, studentID, cycleID, conceptID, description string
		amount, discount                                         int64
		due                                                      time.Time
	}{
		{"empty id", "", "t1", "stu-1", "cyc-1", "cc-1", "d", 100, 0, dueDate},
		{"empty tenant", "ch-1", "", "stu-1", "cyc-1", "cc-1", "d", 100, 0, dueDate},
		{"empty student", "ch-1", "t1", "", "cyc-1", "cc-1", "d", 100, 0, dueDate},
		{"empty cycle", "ch-1", "t1", "stu-1", "", "cc-1", "d", 100, 0, dueDate},
		{"empty concept", "ch-1", "t1", "stu-1", "cyc-1", "", "d", 100, 0, dueDate},
		{"empty description", "ch-1", "t1", "stu-1", "cyc-1", "cc-1", "", 100, 0, dueDate},
		{"blank description", "ch-1", "t1", "stu-1", "cyc-1", "cc-1", "   ", 100, 0, dueDate},
		{"negative amount", "ch-1", "t1", "stu-1", "cyc-1", "cc-1", "d", -1, 0, dueDate},
		{"negative discount", "ch-1", "t1", "stu-1", "cyc-1", "cc-1", "d", 100, -1, dueDate},
		{"discount exceeds amount", "ch-1", "t1", "stu-1", "cyc-1", "cc-1", "d", 100, 101, dueDate},
		{"zero due date", "ch-1", "t1", "stu-1", "cyc-1", "cc-1", "d", 100, 0, time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewCharge(tc.id, tc.tenantID, tc.studentID, tc.cycleID, tc.conceptID,
				tc.description, tc.amount, tc.discount, tc.due)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestNewChargeDiscountEqualToAmount(t *testing.T) {
	// A full-scholarship charge: discount == amount is legal, effective is zero.
	c := validCharge(t, 250000, 250000)
	if c.Effective().Int64() != 0 {
		t.Errorf("Effective() = %d, want 0", c.Effective().Int64())
	}
}

func TestChargeEffective(t *testing.T) {
	c := validCharge(t, 250000, 50000)
	if got := c.Effective().Int64(); got != 200000 {
		t.Errorf("Effective() = %d, want 200000", got)
	}
}

func TestChargeDeriveStatus(t *testing.T) {
	c := validCharge(t, 250000, 50000) // effective 200000
	cases := []struct {
		name string
		paid int64
		want string
	}{
		{"nothing paid", 0, domain.StatusPending},
		{"negative paid is treated as nothing", -10, domain.StatusPending},
		{"partial", 100000, domain.StatusPartiallyPaid},
		{"exact", 200000, domain.StatusPaid},
		{"overpaid", 250000, domain.StatusPaid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.DeriveStatus(tc.paid); got != tc.want {
				t.Errorf("DeriveStatus(%d) = %q, want %q", tc.paid, got, tc.want)
			}
		})
	}
	// A cancelled charge stays cancelled whatever the ledger says.
	cancelled, err := c.Cancel()
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	for _, paid := range []int64{0, 100000, 200000} {
		if got := cancelled.DeriveStatus(paid); got != domain.StatusCancelled {
			t.Errorf("cancelled DeriveStatus(%d) = %q, want %q", paid, got, domain.StatusCancelled)
		}
	}
	// DeriveStatus is pure: the receiver keeps its own status.
	if c.Status() != domain.StatusPending {
		t.Errorf("DeriveStatus mutated the receiver: %q", c.Status())
	}
}

func TestChargeDeriveStatusZeroEffective(t *testing.T) {
	// A fully discounted charge owes nothing, yet the "no payment ⇒ pending"
	// rule is evaluated before the "covered ⇒ paid" rule, so it stays pending
	// until something is actually paid against it. Locked down here because it
	// is the one case where the derivation is not intuitive.
	c := validCharge(t, 250000, 250000)
	if got := c.DeriveStatus(0); got != domain.StatusPending {
		t.Errorf("DeriveStatus(0) with zero effective = %q, want %q", got, domain.StatusPending)
	}
	if got := c.DeriveStatus(1); got != domain.StatusPaid {
		t.Errorf("DeriveStatus(1) with zero effective = %q, want %q", got, domain.StatusPaid)
	}
}

func TestChargeWithDiscount(t *testing.T) {
	c := validCharge(t, 250000, 0)
	updated, err := c.WithDiscount(50000, 200000)
	if err != nil {
		t.Fatalf("WithDiscount: %v", err)
	}
	if updated.Discount().Int64() != 50000 {
		t.Errorf("Discount() = %d, want 50000", updated.Discount().Int64())
	}
	// The new discount drops the effective amount to exactly what was paid.
	if updated.Status() != domain.StatusPaid {
		t.Errorf("Status() = %q, want %q", updated.Status(), domain.StatusPaid)
	}
	if c.Discount().Int64() != 0 || c.Status() != domain.StatusPending {
		t.Errorf("WithDiscount mutated the source: %+v", c)
	}
}

func TestChargeWithDiscountValidation(t *testing.T) {
	c := validCharge(t, 250000, 0)
	for _, discount := range []int64{-1, 250001} {
		if _, err := c.WithDiscount(discount, 0); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("WithDiscount(%d): want KindInvalid, got %v", discount, err)
		}
	}
}

func TestChargeCancel(t *testing.T) {
	c := validCharge(t, 250000, 0)
	cancelled, err := c.Cancel()
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status() != domain.StatusCancelled {
		t.Errorf("Status() = %q, want %q", cancelled.Status(), domain.StatusCancelled)
	}
	if c.Status() != domain.StatusPending {
		t.Errorf("Cancel mutated the source: %q", c.Status())
	}
	if _, err := cancelled.Cancel(); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("second Cancel: want KindConflict, got %v", err)
	}
}

func TestChargeWithStatus(t *testing.T) {
	c := validCharge(t, 250000, 50000) // effective 200000
	cases := []struct {
		name string
		paid int64
		want string
	}{
		{"pending", 0, domain.StatusPending},
		{"partially paid", 120000, domain.StatusPartiallyPaid},
		{"paid", 200000, domain.StatusPaid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.WithStatus(tc.paid)
			if got.Status() != tc.want {
				t.Errorf("WithStatus(%d).Status() = %q, want %q", tc.paid, got.Status(), tc.want)
			}
			// Identity and money are untouched.
			if got.ID() != c.ID() || got.Amount().Int64() != c.Amount().Int64() ||
				got.Discount().Int64() != c.Discount().Int64() {
				t.Errorf("WithStatus altered identity or money: %+v", got)
			}
		})
	}
	if c.Status() != domain.StatusPending {
		t.Errorf("WithStatus mutated the source: %q", c.Status())
	}
}

func TestChargeWithStatusKeepsCancelled(t *testing.T) {
	c := validCharge(t, 250000, 0)
	cancelled, err := c.Cancel()
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := cancelled.WithStatus(250000); got.Status() != domain.StatusCancelled {
		t.Errorf("WithStatus on a cancelled charge = %q, want %q", got.Status(), domain.StatusCancelled)
	}
}
