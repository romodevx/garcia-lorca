package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// InvitationLifetime is how long a fresh invitation stays usable.
//
// Twenty-four hours because the school asked for twenty-four hours: a teacher
// is handed an account in the morning and opens the mail at home that night.
// It is a constant here, in the domain, because it is a promise the product
// makes to a person — not a timeout an adapter happens to pick.
//
// It is also the reason this token exists at all. The identity provider's own
// reset links expire in one hour and that is not configurable, so honouring the
// school's twenty-four hours means owning the token.
const InvitationLifetime = 24 * time.Hour

// ResetLifetime is how long a password-reset link stays usable.
//
// One hour, deliberately shorter than an invitation. Nobody schedules
// forgetting their password: the link is requested and used in the same few
// minutes, so a longer window would only widen the time a stolen mail is worth
// something.
const ResetLifetime = time.Hour

// InvitationPurpose says which of the two jobs a token was minted for.
//
// They share a table and nothing else. An invitation ACTIVATES an account that
// has never been used; a reset only replaces a credential and must leave the
// account's status exactly as it found it. Folding them together would let a
// suspended user reset their password and walk back in through the front door.
type InvitationPurpose string

const (
	// PurposeInvitation: first access. Consuming it activates the account.
	PurposeInvitation InvitationPurpose = "invitation"
	// PurposeReset: the account already exists and works. Consuming it changes
	// the credential and NOTHING else.
	PurposeReset InvitationPurpose = "reset"
)

// validPurposes is the closed set. A purpose outside it is a programming error
// caught at construction, not a row written to the database.
var validPurposes = map[InvitationPurpose]bool{
	PurposeInvitation: true,
	PurposeReset:      true,
}

// Invitation is a single-use, time-limited permission to set one account's
// password without being logged in.
//
// The token itself is NOT here. Only its SHA-256 hash: the clear value exists
// in exactly one place, the message that carries it to its owner, and it is
// never written down on our side. A database dump therefore leaks nothing
// usable.
//
//	minted ──────────► usable ──── consumed ────► spent
//	                     │
//	                     └──────── expires ─────► spent
//
// Both edges lead to the same place on purpose: from outside, "expired",
// "already used" and "never existed" must be one answer, or the difference
// becomes an oracle for guessing tokens.
type Invitation struct {
	id         string
	userID     string
	tokenHash  string
	purpose    InvitationPurpose
	expiresAt  time.Time
	consumedAt time.Time
}

// NewInvitation constructs one. `consumedAt` is the zero value for a token that
// has not been spent; the repository passes the stored value back on load.
func NewInvitation(id, userID, tokenHash string, purpose InvitationPurpose, expiresAt, consumedAt time.Time) (Invitation, error) {
	if strings.TrimSpace(id) == "" {
		return Invitation{}, glerrors.New(glerrors.KindInvalid, "invitation id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return Invitation{}, glerrors.New(glerrors.KindInvalid, "invitation user id is required")
	}
	if strings.TrimSpace(tokenHash) == "" {
		// A blank hash would match a blank token, so this guard is the reason an
		// empty token can never be accepted by accident.
		return Invitation{}, glerrors.New(glerrors.KindInvalid, "invitation token hash is required")
	}
	if !validPurposes[purpose] {
		return Invitation{}, glerrors.New(glerrors.KindInvalid, "unknown invitation purpose")
	}
	return Invitation{
		id:         id,
		userID:     userID,
		tokenHash:  tokenHash,
		purpose:    purpose,
		expiresAt:  expiresAt,
		consumedAt: consumedAt,
	}, nil
}

// ID returns the invitation's identifier.
func (i Invitation) ID() string { return i.id }

// UserID returns the account this invitation belongs to.
func (i Invitation) UserID() string { return i.userID }

// TokenHash returns the stored SHA-256 of the token. The clear token is not
// recoverable from here, which is the point.
func (i Invitation) TokenHash() string { return i.tokenHash }

// Purpose says whether consuming this activates an account or only rotates a
// credential.
func (i Invitation) Purpose() InvitationPurpose { return i.purpose }

// ExpiresAt is the instant the token stops working.
func (i Invitation) ExpiresAt() time.Time { return i.expiresAt }

// ConsumedAt is when the token was spent; the zero value means "not yet".
func (i Invitation) ConsumedAt() time.Time { return i.consumedAt }

// IsUsable reports whether the token may still be redeemed at `now`.
//
// The expiry boundary belongs to the dead side: a token whose instant has
// arrived is spent. Anything else would make "twenty-four hours" mean
// "twenty-four hours plus however much clock resolution we happen to have".
func (i Invitation) IsUsable(now time.Time) bool {
	if !i.consumedAt.IsZero() {
		return false
	}
	return now.Before(i.expiresAt)
}

// ActivatesAccount reports whether consuming this invitation should also move
// the account from `invited` to `active`.
//
// Only an invitation does. A reset deliberately does not: an account that was
// suspended must still be suspended after its owner changes their password, or
// "reset your password" becomes a way out of a suspension.
func (i Invitation) ActivatesAccount() bool { return i.purpose == PurposeInvitation }
