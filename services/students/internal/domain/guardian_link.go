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
}

// NewGuardianLink builds a validated GuardianLink. Every identity reference is
// required and the relationship must be a real one.
func NewGuardianLink(id, tenantID, guardianUserID, studentID string, relationship Relationship) (GuardianLink, error) {
	if strings.TrimSpace(id) == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "guardian link id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(guardianUserID) == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "guardian user id is required")
	}
	if strings.TrimSpace(studentID) == "" {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "student id is required")
	}
	if !relationship.Valid() {
		return GuardianLink{}, glerrors.New(glerrors.KindInvalid, "a valid relationship is required")
	}
	return GuardianLink{
		id:             id,
		tenantID:       tenantID,
		guardianUserID: guardianUserID,
		studentID:      studentID,
		relationship:   relationship,
	}, nil
}

// Accessors.
func (l GuardianLink) ID() string                 { return l.id }
func (l GuardianLink) TenantID() string           { return l.tenantID }
func (l GuardianLink) GuardianUserID() string     { return l.guardianUserID }
func (l GuardianLink) StudentID() string          { return l.studentID }
func (l GuardianLink) Relationship() Relationship { return l.relationship }
