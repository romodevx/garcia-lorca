package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// GradeLevel is a grade (grado) within a level, e.g. "1° Primaria". It is
// ordered within its level by `ordinal` (1-based).
type GradeLevel struct {
	id       string
	tenantID string
	levelID  string
	name     string
	ordinal  int
}

// NewGradeLevel constructs a grade level. `id`, `tenantID`, `levelID`, and
// `name` are required, and `ordinal` must be at least 1.
func NewGradeLevel(id, tenantID, levelID, name string, ordinal int) (GradeLevel, error) {
	if strings.TrimSpace(id) == "" {
		return GradeLevel{}, glerrors.New(glerrors.KindInvalid, "grade level id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return GradeLevel{}, glerrors.New(glerrors.KindInvalid, "grade level tenant id is required")
	}
	if strings.TrimSpace(levelID) == "" {
		return GradeLevel{}, glerrors.New(glerrors.KindInvalid, "grade level level id is required")
	}
	if strings.TrimSpace(name) == "" {
		return GradeLevel{}, glerrors.New(glerrors.KindInvalid, "grade level name is required")
	}
	if ordinal < 1 {
		return GradeLevel{}, glerrors.New(glerrors.KindInvalid, "grade level ordinal must be at least 1")
	}
	return GradeLevel{
		id:       id,
		tenantID: tenantID,
		levelID:  levelID,
		name:     name,
		ordinal:  ordinal,
	}, nil
}

// ID returns the stable grade level identifier.
func (g GradeLevel) ID() string { return g.id }

// TenantID returns the tenant the grade level belongs to.
func (g GradeLevel) TenantID() string { return g.tenantID }

// LevelID returns the parent level identifier.
func (g GradeLevel) LevelID() string { return g.levelID }

// Name returns the human-readable grade name (e.g. "1° Primaria").
func (g GradeLevel) Name() string { return g.name }

// Ordinal returns the 1-based ordering of the grade within its level.
func (g GradeLevel) Ordinal() int { return g.ordinal }
