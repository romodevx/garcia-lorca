package domain_test

import (
	"testing"
	"time"

	"github.com/garcia-lorca/services/iam/internal/domain"
)

// A day is the cap for EVERY surface. The kinds still exist as a seam — the
// panel and the family app can diverge again without touching a caller — but
// neither may hand out a session that outlives the school's own limit, and the
// panel must never get the longer of the two.
func TestNoSessionOutlivesADay(t *testing.T) {
	for _, kind := range []domain.SessionKind{domain.SessionStaff, domain.SessionFamily} {
		if kind.RefreshTTL() > 24*time.Hour {
			t.Errorf("%v sessions last %v, which is longer than a day", kind, kind.RefreshTTL())
		}
	}
	if domain.SessionStaff.RefreshTTL() > domain.SessionFamily.RefreshTTL() {
		t.Errorf("the panel (%v) must never outlive a family session (%v)",
			domain.SessionStaff.RefreshTTL(), domain.SessionFamily.RefreshTTL())
	}
}

// The panel's session lasts a day of NOT being used, which is what makes
// closing the browser and coming back the next morning land on the dashboard
// instead of the login screen. It replaced a fifteen-minute idle window that
// was the school's rule for a shared front desk; the school asked for the day
// instead, knowing what it costs on that desk.
func TestTheStaffSessionLastsADayOfDisuse(t *testing.T) {
	if got := domain.SessionStaff.RefreshTTL(); got != 24*time.Hour {
		t.Errorf("staff refresh TTL = %v, want 24h", got)
	}
}

// An unknown or absent kind is a staff session. A caller that says nothing gets
// the SHORTER life, because guessing wrong in that direction costs a login and
// guessing wrong the other way leaves an office session open all day.
func TestAnUnknownKindIsTreatedAsStaff(t *testing.T) {
	if domain.ParseSessionKind("") != domain.SessionStaff {
		t.Error("an empty kind should be a staff session")
	}
	if domain.ParseSessionKind("something else") != domain.SessionStaff {
		t.Error("an unknown kind should be a staff session")
	}
	if domain.ParseSessionKind("family") != domain.SessionFamily {
		t.Error("family should parse")
	}
}

// The kind round-trips through its wire name, because it travels inside the
// refresh token: a rotation has to know which lifetime to give the new one.
func TestSessionKindRoundTrips(t *testing.T) {
	for _, kind := range []domain.SessionKind{domain.SessionStaff, domain.SessionFamily} {
		if got := domain.ParseSessionKind(kind.String()); got != kind {
			t.Errorf("%v round-tripped to %v", kind, got)
		}
	}
}
