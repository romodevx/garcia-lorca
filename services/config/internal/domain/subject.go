package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Subject is a materia within a grade level, e.g. "Matemáticas".
type Subject struct {
	id           string
	tenantID     string
	gradeLevelID string
	name         string
}

// NewSubject constructs a subject. All of `id`, `tenantID`, `gradeLevelID`, and
// `name` are required; the name is stored trimmed, so " Lengua " can never sit
// beside "Lengua" as a second subject.
func NewSubject(id, tenantID, gradeLevelID, name string) (Subject, error) {
	if strings.TrimSpace(id) == "" {
		return Subject{}, glerrors.New(glerrors.KindInvalid, "subject id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Subject{}, glerrors.New(glerrors.KindInvalid, "subject tenant id is required")
	}
	if strings.TrimSpace(gradeLevelID) == "" {
		return Subject{}, glerrors.New(glerrors.KindInvalid, "subject grade level id is required")
	}
	if strings.TrimSpace(name) == "" {
		return Subject{}, glerrors.New(glerrors.KindInvalid, "subject name is required")
	}
	return Subject{
		id:           id,
		tenantID:     tenantID,
		gradeLevelID: gradeLevelID,
		name:         strings.TrimSpace(name),
	}, nil
}

// ID returns the stable subject identifier.
func (s Subject) ID() string { return s.id }

// TenantID returns the tenant the subject belongs to.
func (s Subject) TenantID() string { return s.tenantID }

// GradeLevelID returns the parent grade level identifier.
func (s Subject) GradeLevelID() string { return s.gradeLevelID }

// Name returns the human-readable subject name (e.g. "Matemáticas").
func (s Subject) Name() string { return s.name }

// Renamed returns the same subject — same id, same grade — under a new name.
// Grades, attendance and teaching assignments point at the id, so a rename
// reaches all of them. A blank name is KindInvalid.
func (s Subject) Renamed(name string) (Subject, error) {
	return NewSubject(s.id, s.tenantID, s.gradeLevelID, name)
}
