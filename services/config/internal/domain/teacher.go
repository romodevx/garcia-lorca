package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Teacher is a catalog person record (un maestro), NOT a login account. `userID`
// is reserved for a future IAM link (roles/users phase) and is empty until then.
type Teacher struct {
	id        string
	tenantID  string
	firstName string
	lastName  string
	email     string
	userID    string
}

// NewTeacher constructs a teacher. `id`, `tenantID`, `firstName`, `lastName`, and
// `email` are required; `email` must have a plausible shape. `userID` is
// optional ("" = not yet linked to an IAM user).
func NewTeacher(id, tenantID, firstName, lastName, email, userID string) (Teacher, error) {
	if strings.TrimSpace(id) == "" {
		return Teacher{}, glerrors.New(glerrors.KindInvalid, "teacher id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Teacher{}, glerrors.New(glerrors.KindInvalid, "teacher tenant id is required")
	}
	if strings.TrimSpace(firstName) == "" {
		return Teacher{}, glerrors.New(glerrors.KindInvalid, "teacher first name is required")
	}
	if strings.TrimSpace(lastName) == "" {
		return Teacher{}, glerrors.New(glerrors.KindInvalid, "teacher last name is required")
	}
	if err := validateTeacherEmail(email); err != nil {
		return Teacher{}, err
	}
	return Teacher{
		id:        id,
		tenantID:  tenantID,
		firstName: firstName,
		lastName:  lastName,
		email:     email,
		userID:    userID,
	}, nil
}

// validateTeacherEmail is a minimal boundary shape check (a single '@' with a
// dotted domain), not full RFC 5322 — the authoritative uniqueness constraint
// lives at the database level (UNIQUE(tenant_id, email)).
func validateTeacherEmail(email string) error {
	if strings.TrimSpace(email) == "" {
		return glerrors.New(glerrors.KindInvalid, "teacher email is required")
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return glerrors.New(glerrors.KindInvalid, "teacher email must contain '@'")
	}
	domainPart := email[at+1:]
	dot := strings.IndexByte(domainPart, '.')
	if dot <= 0 || dot == len(domainPart)-1 {
		return glerrors.New(glerrors.KindInvalid, "teacher email domain must contain a dot")
	}
	return nil
}

// ID returns the stable teacher identifier.
func (t Teacher) ID() string { return t.id }

// TenantID returns the tenant the teacher belongs to.
func (t Teacher) TenantID() string { return t.tenantID }

// FirstName returns the teacher's given name.
func (t Teacher) FirstName() string { return t.firstName }

// LastName returns the teacher's family name.
func (t Teacher) LastName() string { return t.lastName }

// Email returns the teacher's email address (unique per tenant).
func (t Teacher) Email() string { return t.email }

// UserID returns the linked IAM user id, or "" when the teacher has no account.
func (t Teacher) UserID() string { return t.userID }

// HasUser reports whether the teacher is linked to an IAM user account.
func (t Teacher) HasUser() bool { return t.userID != "" }
