package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func validConcept(t *testing.T) domain.ChargeConcept {
	t.Helper()
	c, err := domain.NewChargeConcept("cc-1", "t1", "Colegiatura", 250000, domain.RecurrenceMonthly, true)
	if err != nil {
		t.Fatalf("NewChargeConcept: %v", err)
	}
	return c
}

func TestNewChargeConceptAccessors(t *testing.T) {
	c := validConcept(t)
	if c.ID() != "cc-1" || c.TenantID() != "t1" || c.Name() != "Colegiatura" ||
		c.DefaultAmount().Int64() != 250000 || c.Recurrence() != domain.RecurrenceMonthly || !c.Active() {
		t.Errorf("accessors mismatch: %+v", c)
	}
}

func TestNewChargeConceptRecurrences(t *testing.T) {
	for _, r := range []string{domain.RecurrenceOnce, domain.RecurrenceMonthly} {
		if _, err := domain.NewChargeConcept("cc-1", "t1", "Inscripción", 500000, r, true); err != nil {
			t.Errorf("recurrence %q: want accepted, got %v", r, err)
		}
	}
}

func TestNewChargeConceptValidation(t *testing.T) {
	cases := []struct {
		name             string
		id, tenantID, cn string
		amount           int64
		recurrence       string
	}{
		{"empty id", "", "t1", "Colegiatura", 250000, domain.RecurrenceMonthly},
		{"blank id", "   ", "t1", "Colegiatura", 250000, domain.RecurrenceMonthly},
		{"empty tenant", "cc-1", "", "Colegiatura", 250000, domain.RecurrenceMonthly},
		{"empty name", "cc-1", "t1", "", 250000, domain.RecurrenceMonthly},
		{"blank name", "cc-1", "t1", "  ", 250000, domain.RecurrenceMonthly},
		{"negative amount", "cc-1", "t1", "Colegiatura", -1, domain.RecurrenceMonthly},
		{"unknown recurrence", "cc-1", "t1", "Colegiatura", 250000, "yearly"},
		{"empty recurrence", "cc-1", "t1", "Colegiatura", 250000, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewChargeConcept(tc.id, tc.tenantID, tc.cn, tc.amount, tc.recurrence, true)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestChargeConceptWithUpdate(t *testing.T) {
	c := validConcept(t)
	updated, err := c.WithUpdate("Colegiatura Agosto", 300000, domain.RecurrenceOnce, false)
	if err != nil {
		t.Fatalf("WithUpdate: %v", err)
	}
	if updated.Name() != "Colegiatura Agosto" || updated.DefaultAmount().Int64() != 300000 ||
		updated.Recurrence() != domain.RecurrenceOnce || updated.Active() {
		t.Errorf("WithUpdate did not apply: %+v", updated)
	}
	// Identity is preserved and the source stays immutable.
	if updated.ID() != c.ID() || updated.TenantID() != c.TenantID() ||
		c.Name() != "Colegiatura" || c.DefaultAmount().Int64() != 250000 {
		t.Errorf("WithUpdate mutated identity or source: %+v (src %+v)", updated, c)
	}
}

func TestChargeConceptWithUpdateRevalidates(t *testing.T) {
	c := validConcept(t)
	cases := []struct {
		name       string
		cn         string
		amount     int64
		recurrence string
	}{
		{"empty name", "", 300000, domain.RecurrenceMonthly},
		{"negative amount", "Colegiatura", -50, domain.RecurrenceMonthly},
		{"unknown recurrence", "Colegiatura", 300000, "weekly"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.WithUpdate(tc.cn, tc.amount, tc.recurrence, true); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}
