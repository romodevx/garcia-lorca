package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func aCycle(t *testing.T, status domain.CycleStatus) domain.Cycle {
	t.Helper()
	c, err := domain.NewCycle("cyc-1", "ten-1", "2026-2027", status,
		time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 7, 9, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewCycle: %v", err)
	}
	return c
}

// Cycles were born in draft and nothing could move them, so every school year
// read "Borrador" forever — and the panel, which resolves "the active cycle",
// silently fell back to whichever cycle happened to be first.
func TestActivateMovesADraftCycleIntoService(t *testing.T) {
	activated, err := aCycle(t, domain.CycleStatusDraft).Activate()
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if activated.Status() != domain.CycleStatusActive {
		t.Errorf("status = %v, want active", activated.Status())
	}
}

// Activating the year that is already the live one is what a person does when
// they are not sure. It must be a no-op, not an error.
func TestActivateIsIdempotent(t *testing.T) {
	again, err := aCycle(t, domain.CycleStatusActive).Activate()
	if err != nil {
		t.Fatalf("re-activating the live year must be harmless: %v", err)
	}
	if again.Status() != domain.CycleStatusActive {
		t.Errorf("status = %v, want active", again.Status())
	}
}

// A closed year is history. Re-opening it would let a school issue charges and
// grades against a year it has already reported.
func TestAClosedCycleCannotBeReopened(t *testing.T) {
	_, err := aCycle(t, domain.CycleStatusClosed).Activate()
	if err == nil || glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict, got %v", err)
	}
}

func TestCloseEndsALiveCycle(t *testing.T) {
	closed, err := aCycle(t, domain.CycleStatusActive).Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if closed.Status() != domain.CycleStatusClosed {
		t.Errorf("status = %v, want closed", closed.Status())
	}
}

// Closing a year nobody ever opened is not a lifecycle, it is a mistake.
func TestADraftCycleCannotBeClosed(t *testing.T) {
	if _, err := aCycle(t, domain.CycleStatusDraft).Close(); err == nil {
		t.Error("a draft year has nothing to close")
	}
}
