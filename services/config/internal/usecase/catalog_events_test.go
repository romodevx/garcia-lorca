package usecase_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

func sept(d int) time.Time { return time.Date(2026, time.September, d, 0, 0, 0, 0, time.UTC) }

// The school's agenda: what it does, in date order.
func TestCreateAndListSchoolEvents(t *testing.T) {
	svc, _ := newService(t)
	cycleID := seedCycle(t, svc).ID()

	if _, err := svc.CreateSchoolEvent(ctx(), usecase.SchoolEventInput{
		CycleID: cycleID, Title: "Festival de primavera", Description: "En el patio, 9:00",
		From: sept(20), To: sept(20),
	}); err != nil {
		t.Fatalf("CreateSchoolEvent: %v", err)
	}
	if _, err := svc.CreateSchoolEvent(ctx(), usecase.SchoolEventInput{
		CycleID: cycleID, Title: "Junta de padres", From: sept(10), To: sept(10),
	}); err != nil {
		t.Fatalf("CreateSchoolEvent: %v", err)
	}

	events, err := svc.ListSchoolEvents(ctx(), cycleID)
	if err != nil {
		t.Fatalf("ListSchoolEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("listed %d events, want 2", len(events))
	}
	// Earliest first: a person reads this as a calendar.
	if events[0].Title() != "Junta de padres" {
		t.Errorf("order wrong: %q came first", events[0].Title())
	}
}

// THE POINT OF KEEPING THEM APART. An activity is not a closed day. A festival
// on the 10th does not mean nobody could pay on the 10th, and if it did, every
// due date that month would move because somebody added a party.
func TestAnEventIsNotAClosedDay(t *testing.T) {
	svc, _ := newService(t)
	cycleID := seedCycle(t, svc).ID()

	if _, err := svc.CreateSchoolEvent(ctx(), usecase.SchoolEventInput{
		CycleID: cycleID, Title: "Festival de primavera", From: sept(10), To: sept(10),
	}); err != nil {
		t.Fatalf("CreateSchoolEvent: %v", err)
	}

	closed, err := svc.ListNonWorkingPeriods(ctx(), cycleID)
	if err != nil {
		t.Fatalf("ListNonWorkingPeriods: %v", err)
	}
	if len(closed) != 0 {
		t.Errorf("the school is shut on %d days because somebody added a party", len(closed))
	}
}

// An event on a school year that does not exist is a typo nobody would find.
func TestAnEventNeedsAKnownCycle(t *testing.T) {
	svc, repo := newService(t)

	_, err := svc.CreateSchoolEvent(ctx(), usecase.SchoolEventInput{
		CycleID: "no-such-cycle", Title: "Junta", From: sept(10), To: sept(10),
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unknown cycle: want KindInvalid, got %v", err)
	}
	if len(repo.events) != 0 {
		t.Errorf("nothing should be persisted, got %d", len(repo.events))
	}
}

// An event for one group, so a junta of 1° A does not reach the whole school.
func TestAnEventCanBeScopedToAGroup(t *testing.T) {
	svc, _ := newService(t)
	cycleID := seedCycle(t, svc).ID()

	event, err := svc.CreateSchoolEvent(ctx(), usecase.SchoolEventInput{
		CycleID: cycleID, Title: "Junta de 1° A", From: sept(10), To: sept(10),
		Audience: domain.EventAudience{GroupID: "gr-1"},
	})
	if err != nil {
		t.Fatalf("CreateSchoolEvent: %v", err)
	}
	if event.ForWholeSchool() {
		t.Error("the event reached the whole school")
	}
}

// A date entered by mistake has to be removable, same as a closed day.
func TestDeleteSchoolEvent(t *testing.T) {
	svc, _ := newService(t)
	cycleID := seedCycle(t, svc).ID()
	event, err := svc.CreateSchoolEvent(ctx(), usecase.SchoolEventInput{
		CycleID: cycleID, Title: "Junta", From: sept(10), To: sept(10),
	})
	if err != nil {
		t.Fatalf("CreateSchoolEvent: %v", err)
	}

	if err := svc.DeleteSchoolEvent(ctx(), event.ID()); err != nil {
		t.Fatalf("DeleteSchoolEvent: %v", err)
	}
	events, err := svc.ListSchoolEvents(ctx(), cycleID)
	if err != nil {
		t.Fatalf("ListSchoolEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("listed %d events after deleting the only one", len(events))
	}
}
