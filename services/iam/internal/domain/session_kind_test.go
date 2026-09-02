package domain_test

import (
	"testing"

	"github.com/garcia-lorca/services/iam/internal/domain"
)

// Two surfaces, two lifetimes. The school's panel sits on a shared desk and has
// to close when somebody walks away; the family app lives on a parent's own
// phone and logging them out every quarter of an hour would make it unusable.
func TestSessionKindsHaveDifferentLifetimes(t *testing.T) {
	if domain.SessionStaff.RefreshTTL() >= domain.SessionFamily.RefreshTTL() {
		t.Errorf("staff sessions (%v) should be shorter than family ones (%v)",
			domain.SessionStaff.RefreshTTL(), domain.SessionFamily.RefreshTTL())
	}
}

// The staff window is the school's own rule: fifteen minutes without using it
// and the session is over.
func TestTheStaffSessionClosesAfterFifteenIdleMinutes(t *testing.T) {
	if got := domain.SessionStaff.RefreshTTL().Minutes(); got != 15 {
		t.Errorf("staff refresh TTL = %v minutes, want 15", got)
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
