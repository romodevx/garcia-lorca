package domain

import (
	"strings"
	"time"
)

// SessionKind is which surface a session belongs to, and therefore how long it
// may be resumed without being used.
//
// Two surfaces, two lifetimes, and the difference is not a preference. The
// school's panel sits on a shared desk in the office: somebody walks away and
// the session has to be over, which is the school's own rule — fifteen minutes.
// The family app lives on a parent's own phone, where logging them out every
// quarter of an hour would make it unusable and teach them to stop opening it.
//
// The kind travels inside the refresh token because rotating one has to know
// which lifetime the new one gets. A session cannot promote itself by asking.
type SessionKind uint8

const (
	// SessionStaff is the school's own panel. Short by design.
	SessionStaff SessionKind = iota
	// SessionFamily is a parent's phone.
	SessionFamily
)

const (
	// staffRefreshTTL is the idle window the school asked for: fifteen minutes
	// without using the panel and the session is closed. Every use of the panel
	// rotates the token, so it is a window of INACTIVITY, not of total time.
	staffRefreshTTL = 15 * time.Minute
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
// session: guessing wrong that way costs somebody a login, and guessing wrong
// the other way leaves an office session open all afternoon.
func ParseSessionKind(raw string) SessionKind {
	if strings.EqualFold(strings.TrimSpace(raw), "family") {
		return SessionFamily
	}
	return SessionStaff
}
