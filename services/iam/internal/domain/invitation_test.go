package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// An invited account is NOT authenticable. It exists, it holds roles, and it
// cannot log in until its owner proves they read the mailbox. The whole point
// of the third status is that "created" and "usable" stop being the same thing.
func TestStatusInvitedCannotAuthenticate(t *testing.T) {
	email := mustEmail(t, "maestra@garcialorca.mx")
	for _, tc := range []struct {
		name string
		st   domain.Status
		want bool
	}{
		{"active", domain.StatusActive, true},
		{"invited", domain.StatusInvited, false},
		{"suspended", domain.StatusSuspended, false},
		{"unspecified", domain.StatusUnspecified, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, err := domain.NewUser("u1", "t1", email, "Maestra", tc.st, false, nil)
			if err != nil {
				t.Fatalf("NewUser: %v", err)
			}
			if got := u.CanAuthenticate(); got != tc.want {
				t.Errorf("CanAuthenticate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewInvitationRequiresItsParts(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, id, userID, hash string
		purpose                domain.InvitationPurpose
	}{
		{"blank id", "", "u1", "h", domain.PurposeInvitation},
		{"blank user", "i1", "", "h", domain.PurposeInvitation},
		{"blank hash", "i1", "u1", "", domain.PurposeInvitation},
		{"unknown purpose", "i1", "u1", "h", domain.InvitationPurpose("otra")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewInvitation(tc.id, tc.userID, tc.hash, tc.purpose, now.Add(time.Hour), time.Time{})
			if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

// The three ways a token is dead must be indistinguishable from the outside,
// so they are one predicate here rather than three the caller could leak apart.
func TestInvitationIsUsable(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		expires   time.Time
		consumed  time.Time
		wantUsabl bool
	}{
		{"fresh", now.Add(24 * time.Hour), time.Time{}, true},
		{"expired", now.Add(-time.Second), time.Time{}, false},
		{"already used", now.Add(24 * time.Hour), now.Add(-time.Minute), false},
		{"used and expired", now.Add(-time.Hour), now.Add(-time.Hour), false},
		// The boundary belongs to the dead side: a token that expires exactly
		// now is spent. Anything else makes "24 hours" mean "24 hours plus an
		// instant that depends on clock resolution".
		{"expires exactly now", now, time.Time{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := domain.NewInvitation("i1", "u1", "hash", domain.PurposeInvitation, tc.expires, tc.consumed)
			if err != nil {
				t.Fatalf("NewInvitation: %v", err)
			}
			if got := inv.IsUsable(now); got != tc.wantUsabl {
				t.Errorf("IsUsable() = %v, want %v", got, tc.wantUsabl)
			}
		})
	}
}

// Twenty-four hours is the promise the owner asked for, so it is a named
// constant in the domain rather than a duration typed into a use case.
func TestInvitationLifetimeIsTwentyFourHours(t *testing.T) {
	if domain.InvitationLifetime != 24*time.Hour {
		t.Errorf("InvitationLifetime = %v, want 24h", domain.InvitationLifetime)
	}
	// A reset is a different promise: short, because nobody schedules
	// forgetting their password.
	if domain.ResetLifetime != time.Hour {
		t.Errorf("ResetLifetime = %v, want 1h", domain.ResetLifetime)
	}
}
