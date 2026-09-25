package domain

import (
	"encoding/base64"
	"regexp"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// CardCodePrefix says what a scanned code is. The QR carries this and the token,
// and nothing else: no name, no CURP, no matrícula, no URL. Scanned by somebody
// without a session, it is a string that means nothing (task 13).
const CardCodePrefix = "GLCARD:"

// cardTokenShape is 16 random bytes in unpadded base64url: 22 characters.
var cardTokenShape = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

// CardToken is what a pupil's virtual card shows as a QR. One is active per
// pupil. Revoking it (a lost phone, a card shown by somebody else) makes the old
// code verify as no longer valid and suspends the card until the office
// reissues it.
type CardToken struct {
	id        string
	tenantID  string
	studentID string
	token     string
	issuedAt  time.Time
	revokedAt time.Time
}

// NewCardToken builds an active card token from 128 random bits.
func NewCardToken(id, tenantID, studentID string, random [16]byte, issuedAt time.Time) (CardToken, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(studentID) == "" {
		return CardToken{}, glerrors.New(glerrors.KindInvalid, "a card token needs its id, tenant and pupil")
	}
	return CardToken{
		id: id, tenantID: tenantID, studentID: studentID,
		token: base64.RawURLEncoding.EncodeToString(random[:]), issuedAt: issuedAt,
	}, nil
}

// RestoreCardToken rebuilds a stored card token.
func RestoreCardToken(id, tenantID, studentID, token string, issuedAt, revokedAt time.Time) CardToken {
	return CardToken{id: id, tenantID: tenantID, studentID: studentID, token: token, issuedAt: issuedAt, revokedAt: revokedAt}
}

// ParseCardCode reads a scanned code back into its token. Surrounding blanks
// (a scanner's Enter) are forgiven; anything that is not GLCARD: and 22
// url-safe characters is KindInvalid.
func ParseCardCode(code string) (string, error) {
	trimmed := strings.TrimSpace(code)
	token, found := strings.CutPrefix(trimmed, CardCodePrefix)
	if !found || !cardTokenShape.MatchString(token) {
		return "", glerrors.New(glerrors.KindInvalid, "not a student card code")
	}
	return token, nil
}

// Revoke returns the token revoked at the given instant.
func (c CardToken) Revoke(at time.Time) CardToken {
	c.revokedAt = at
	return c
}

// Code is what the QR encodes.
func (c CardToken) Code() string { return CardCodePrefix + c.token }

// Active reports whether the token still identifies its pupil.
func (c CardToken) Active() bool { return c.revokedAt.IsZero() }

// Accessors.
func (c CardToken) ID() string           { return c.id }
func (c CardToken) TenantID() string     { return c.tenantID }
func (c CardToken) StudentID() string    { return c.studentID }
func (c CardToken) Token() string        { return c.token }
func (c CardToken) IssuedAt() time.Time  { return c.issuedAt }
func (c CardToken) RevokedAt() time.Time { return c.revokedAt }
