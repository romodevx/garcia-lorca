package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func eventDay(d int) time.Time {
	return time.Date(2026, time.September, d, 0, 0, 0, 0, time.UTC)
}

// The things the school does, as opposed to the days it is shut. A festival on
// the 10th does not mean nobody could pay on the 10th.
func TestNewSchoolEvent(t *testing.T) {
	e, err := domain.NewSchoolEvent("ev-1", "t1", "c1", "Festival de primavera",
		"En el patio, 9:00", eventDay(10), eventDay(10), domain.EventAudience{})
	if err != nil {
		t.Fatalf("NewSchoolEvent: %v", err)
	}
	if e.Title() != "Festival de primavera" || !e.From().Equal(eventDay(10)) {
		t.Errorf("event wrong: %+v", e)
	}
	if !e.ForWholeSchool() {
		t.Error("an event with no audience is for everybody")
	}
}

// For one level, or one group, or everybody — and never two at once, which
// would be an event whose audience nobody could state.
func TestAnEventHasAtMostOneAudience(t *testing.T) {
	_, err := domain.NewSchoolEvent("ev-1", "t1", "c1", "Junta", "", eventDay(10), eventDay(10),
		domain.EventAudience{LevelID: "lv-1", GroupID: "gr-1"})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("two audiences: want KindInvalid, got %v", err)
	}
}

func TestAnEventForOneGroup(t *testing.T) {
	e, err := domain.NewSchoolEvent("ev-1", "t1", "c1", "Junta de 1° A", "",
		eventDay(10), eventDay(10), domain.EventAudience{GroupID: "gr-1"})
	if err != nil {
		t.Fatalf("NewSchoolEvent: %v", err)
	}
	if e.ForWholeSchool() || e.Audience().GroupID != "gr-1" {
		t.Errorf("audience wrong: %+v", e.Audience())
	}
}

// An untitled event is a coloured block on a calendar that nobody can read.
func TestAnEventNeedsATitle(t *testing.T) {
	_, err := domain.NewSchoolEvent("ev-1", "t1", "c1", "   ", "", eventDay(10), eventDay(10), domain.EventAudience{})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("blank title: want KindInvalid, got %v", err)
	}
}

func TestAnEventCannotEndBeforeItStarts(t *testing.T) {
	_, err := domain.NewSchoolEvent("ev-1", "t1", "c1", "Semana cultural", "",
		eventDay(12), eventDay(10), domain.EventAudience{})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("backwards range: want KindInvalid, got %v", err)
	}
}
