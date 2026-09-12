package domain

import (
	"strings"
	"time"
)

// SessionKind is which surface a session belongs to, and therefore how long it
// may be resumed without being used.
//
// Both surfaces now last a day, and the type stays anyway. It is the seam that
// lets one of them change without the other: the panel already had a different
// number once — fifteen minutes, for the shared desk in the office — and the
// only reason that was expressible is that the kind rides inside the token.
// Collapsing them into one constant now would mean re-deriving all of this the
// next time the school wants the two surfaces to differ.
//
// The kind travels inside the refresh token because rotating one has to know
// which lifetime the new one gets. A session cannot promote itself by asking.
type SessionKind uint8

const (
	// SessionStaff is the school's own panel, and the default: a token that
	// names no kind is one of these.
	SessionStaff SessionKind = iota
	// SessionFamily is a parent's phone.
	SessionFamily
)

const (
	// staffRefreshTTL is the idle window the school asked for: a day without
	// opening the panel and the session is closed. Every use of the panel
	// rotates the token, so it is a window of INACTIVITY, not of total time —
	// somebody who works in it daily is never asked for a password again.
	//
	// It was fifteen minutes, to protect the shared computer at the front desk
	// from whoever sat down next. The school weighed that against being thrown
	// out mid-shift and chose the day; the trade is real and it is theirs.
	staffRefreshTTL = 24 * time.Hour
	// familyRefreshTTL keeps a parent signed in on their own device for a day.
	familyRefreshTTL = 24 * time.Hour
)

// RefreshTTL is how long a session of this kind may sit unused before it can no
// longer be resumed.
func (k SessionKind) RefreshTTL() time.Duration {
	if k == SessionFamily {
		return familyRefreshTTL
	}
	return staffRefreshTTL
}

// String is the wire name, which is what goes in the token.
func (k SessionKind) String() string {
	if k == SessionFamily {
		return "family"
	}
	return "staff"
}

// ParseSessionKind reads a wire name. Anything it does not recognise — an empty
// string, an older token minted before kinds existed, a typo — is a STAFF
// session, which is the floor: the kinds are equal today, and if they diverge
// again the panel is the one that must not be able to claim the longer life by
// arriving with a name nobody wrote.
func ParseSessionKind(raw string) SessionKind {
	if strings.EqualFold(strings.TrimSpace(raw), "family") {
		return SessionFamily
	}
	return SessionStaff
}
