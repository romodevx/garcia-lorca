package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Relationship is how a guardian relates to a student. It is a domain type (no
// proto import); the wire carries its canonical name and the adapters map it with
// ParseRelationship / String. The canonical names are the vocabulary the
// guardian_students CHECK constraint owns.
type Relationship uint8

const (
	// RelationshipUnspecified is the zero value; not a valid relationship.
	RelationshipUnspecified Relationship = iota
	// RelationshipMother is the student's mother.
	RelationshipMother
	// RelationshipFather is the student's father.
	RelationshipFather
	// RelationshipGuardian is a legal guardian who is not a parent.
	RelationshipGuardian
	// RelationshipOther is any other authorized adult.
	RelationshipOther
)

var relationshipNames = map[Relationship]string{
	RelationshipMother:   "mother",
	RelationshipFather:   "father",
	RelationshipGuardian: "guardian",
	RelationshipOther:    "other",
}

// Valid reports whether the relationship is a real one (not unspecified).
func (r Relationship) Valid() bool {
	return r >= RelationshipMother && r <= RelationshipOther
}

// String returns the canonical relationship name ("" for unspecified).
func (r Relationship) String() string { return relationshipNames[r] }

// ParseRelationship resolves a relationship name (case- and space-insensitive)
// to a Relationship. An unknown name is KindInvalid.
func ParseRelationship(raw string) (Relationship, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	for rel, name := range relationshipNames {
		if name == key {
			return rel, nil
		}
	}
	return RelationshipUnspecified, glerrors.New(glerrors.KindInvalid, "unknown relationship: "+raw)
}

// GuardianLink ties a guardian to a student. guardianUserID is a cross-service
// reference to an iam user (a plain UUID, RPC-validated by the use case — never a
// cross-service FK), while studentID belongs to this bounded context. The pair is
// unique per tenant: a guardian is linked to a student at most once.
type GuardianLink struct {
	id             string
	tenantID       string
	guardianUserID string
	studentID      string
	relationship   Relationship
	person         ContactPerson
}

// ContactPerson is who to call: the details of a contact that have nothing to do
// with whether they can log in.
//
// They exist because most of a pupil's contacts have no account. A grandmother
// who may collect the child, or the father whose phone actually answers, is
// somebody the school needs recorded — and requiring a login first is why the
// office kept the second number on paper.
type ContactPerson struct {
	FullName string
	Phone    string
	Email    string
}

// NewContact builds a contact who may have no account at all: a name and,
// usually, a phone. A contact with neither an account nor a name is nobody, and
// a row for nobody is a number the office cannot use.
func NewContact(id, tenantID, studentID string, relationship Relationship, person ContactPerson) (GuardianLink, error) {
	if strings.TrimSpace(person.FullName) == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "a contact without an account needs a name")
	}
	link, err := newLink(id, tenantID, studentID, relationship)
	if err != nil {
		return GuardianLink{}, err
	}
	return link.WithPerson(person), nil
}

// WithPerson attaches (or replaces) the contact details. A guardian with an
// account may carry a phone too: the account says who can log in, the phone says
// who answers, and they are not the same question.
func (l GuardianLink) WithPerson(person ContactPerson) GuardianLink {
	l.person = ContactPerson{
		FullName: strings.TrimSpace(person.FullName),
		Phone:    strings.TrimSpace(person.Phone),
		Email:    strings.TrimSpace(person.Email),
	}
	return l
}

// WithAccount attaches an iam account to a contact the school already recorded,
// turning a phone number the office calls into a family that can open the app.
//
// It attaches to the row that is already there rather than creating a second
// one: a pupil has ONE list of contacts, and a person who appears twice — once
// as a name and once as an account — is a list the office stops trusting.
//
// Refuses a contact that already has an account. That is not a duplicate to
// swallow: quietly replacing the user id would hand one family's child to
// another, and the family read path authorizes on exactly this field.
func (l GuardianLink) WithAccount(guardianUserID string) (GuardianLink, error) {
	id := strings.TrimSpace(guardianUserID)
	if id == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "guardian user id is required")
	}
	if l.HasAccount() {
		return GuardianLink{}, glerrors.New(glerrors.KindConflict, "this contact already has an account")
	}
	l.guardianUserID = id
	return l, nil
}

// FullName is the contact's name as the office would read it aloud.
func (l GuardianLink) FullName() string { return l.person.FullName }

// Phone is the number to call. Empty for a contact recorded only as an account.
func (l GuardianLink) Phone() string { return l.person.Phone }

// Email is the contact's address, if the school has one.
func (l GuardianLink) Email() string { return l.person.Email }

// HasAccount reports whether this contact can open the family app.
func (l GuardianLink) HasAccount() bool { return l.guardianUserID != "" }

// newLink is the part both constructors share: identity and relationship.
func newLink(id, tenantID, studentID string, relationship Relationship) (GuardianLink, error) {
	if strings.TrimSpace(id) == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "guardian link id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(studentID) == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "student id is required")
	}
	if !relationship.Valid() {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "a valid relationship is required")
	}
	return GuardianLink{
		id: id, tenantID: tenantID, studentID: studentID, relationship: relationship,
	}, nil
}

// NewGuardianLink builds a validated GuardianLink. Every identity reference is
// required and the relationship must be a real one.
func NewGuardianLink(id, tenantID, guardianUserID, studentID string, relationship Relationship) (GuardianLink, error) {
	if strings.TrimSpace(guardianUserID) == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "guardian user id is required")
	}
	link, err := newLink(id, tenantID, studentID, relationship)
	if err != nil {
		return GuardianLink{}, err
	}
	link.guardianUserID = guardianUserID
	return link, nil
}

// Accessors.
func (l GuardianLink) ID() string                 { return l.id }
func (l GuardianLink) TenantID() string           { return l.tenantID }
func (l GuardianLink) GuardianUserID() string     { return l.guardianUserID }
func (l GuardianLink) StudentID() string          { return l.studentID }
func (l GuardianLink) Relationship() Relationship { return l.relationship }
