package usecase_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// A school closes on Independence Day and the calendar has to be able to say so.
func TestCreateAndListNonWorkingPeriods(t *testing.T) {
	svc, _ := newService(t)
	cycleID := seedCycle(t, svc).ID()

	sep16 := time.Date(2026, time.September, 16, 0, 0, 0, 0, time.UTC)
	p, err := svc.CreateNonWorkingPeriod(ctx(), cycleID, "Independencia", domain.KindHoliday, sep16, sep16)
	if err != nil {
		t.Fatalf("CreateNonWorkingPeriod: %v", err)
	}
	if p.ID() == "" || p.CycleID() != cycleID || p.Label() != "Independencia" {
		t.Errorf("period wrong: %+v", p)
	}

	got, err := svc.ListNonWorkingPeriods(ctx(), cycleID)
	if err != nil {
		t.Fatalf("ListNonWorkingPeriods: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("listed %d periods, want 1", len(got))
	}
}

// The calendar hangs from a cycle. A period on a year that does not exist is a
// typo, and the person who made it should hear about it now rather than have a
// due date quietly move next March.
func TestNonWorkingPeriodOnAnUnknownCycleIsRefused(t *testing.T) {
	svc, repo := newService(t)

	_, err := svc.CreateNonWorkingPeriod(ctx(), "no-such-cycle", "Fantasma", domain.KindHoliday, someDate, someDate)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unknown cycle: want KindInvalid, got %v", err)
	}
	if len(repo.periods) != 0 {
		t.Errorf("nothing should be persisted, got %d", len(repo.periods))
	}
}

// A day entered by mistake has to be removable, or the calendar stops being
// trusted and the school goes back to a printed sheet on the wall.
func TestDeleteNonWorkingPeriod(t *testing.T) {
	svc, _ := newService(t)
	cycleID := seedCycle(t, svc).ID()
	p, err := svc.CreateNonWorkingPeriod(ctx(), cycleID, "Puente", domain.KindVacation, someDate, someDate)
	if err != nil {
		t.Fatalf("CreateNonWorkingPeriod: %v", err)
	}

	if err := svc.DeleteNonWorkingPeriod(ctx(), p.ID()); err != nil {
		t.Fatalf("DeleteNonWorkingPeriod: %v", err)
	}
	got, err := svc.ListNonWorkingPeriods(ctx(), cycleID)
	if err != nil {
		t.Fatalf("ListNonWorkingPeriods: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("listed %d periods after deleting the only one", len(got))
	}
}
