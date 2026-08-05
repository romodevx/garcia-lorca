package ports

import (
	"context"

	"github.com/garcia-lorca/services/config/internal/domain"
)

// This file declares the seam the config use cases depend on. The interface
// lives here (consumer side); the postgres adapter implements it. The domain
// imports none of this — dependencies point inward.

// CatalogRepository loads and persists the six academic-catalog entities. All
// six live in ONE service/DB, so a single repository owns them and intra-catalog
// references are real FKs (no cross-service RPC within config). Every method is
// tenant-scoped for RLS: the adapter binds app.current_tenant per transaction
// (as in iam/students), so callers never pass a tenant id.
//
// Error conventions (adapters map to these Kinds; use cases and the gRPC edge
// rely on them):
//   - Create* returns glerrors.KindConflict on a unique-constraint violation
//     (e.g. a duplicate name within its parent scope). When a referenced parent
//     row is missing the adapter surfaces the FK violation; the use case guards
//     against that up front by checking the parent exists (see CatalogService),
//     mapping a missing parent to glerrors.KindInvalid.
//   - Get* returns glerrors.KindNotFound when the id does not exist (or is hidden
//     by RLS).
//   - List* returns an empty slice (not an error) when the parent scope has no
//     children; it returns glerrors.KindNotFound only for a malformed request the
//     adapter cannot satisfy.
type CatalogRepository interface {
	// --- cycles (per-cycle umbrella, tenant-scoped) ---

	// CreateCycle persists a new cycle. A duplicate name (per tenant) is KindConflict.
	CreateCycle(ctx context.Context, c domain.Cycle) error
	// GetCycle returns a cycle by id, or KindNotFound.
	GetCycle(ctx context.Context, id string) (domain.Cycle, error)
	// ListCycles returns every cycle in the tenant.
	ListCycles(ctx context.Context) ([]domain.Cycle, error)

	// --- levels (cross-cycle catalog) ---

	// CreateLevel persists a new level. A duplicate name (per tenant) is KindConflict.
	CreateLevel(ctx context.Context, l domain.Level) error
	// GetLevel returns a level by id, or KindNotFound.
	GetLevel(ctx context.Context, id string) (domain.Level, error)
	// ListLevels returns every level in the tenant catalog.
	ListLevels(ctx context.Context) ([]domain.Level, error)

	// --- grade levels (grados, catalog; child of level) ---

	// CreateGradeLevel persists a new grade level. A duplicate (level_id, ordinal)
	// is KindConflict; a missing parent level surfaces as an FK violation.
	CreateGradeLevel(ctx context.Context, g domain.GradeLevel) error
	// GetGradeLevel returns a grade level by id, or KindNotFound.
	GetGradeLevel(ctx context.Context, id string) (domain.GradeLevel, error)
	// ListGradeLevels returns the grade levels of levelID.
	ListGradeLevels(ctx context.Context, levelID string) ([]domain.GradeLevel, error)

	// --- subjects (materias, catalog; child of grade level) ---

	// CreateSubject persists a new subject. A duplicate (grade_level_id, name) is
	// KindConflict; a missing parent grade level surfaces as an FK violation.
	CreateSubject(ctx context.Context, s domain.Subject) error
	// GetSubject returns a subject by id, or KindNotFound.
	GetSubject(ctx context.Context, id string) (domain.Subject, error)
	// ListSubjects returns the subjects of gradeLevelID.
	ListSubjects(ctx context.Context, gradeLevelID string) ([]domain.Subject, error)

	// --- terms (periodos, per (cycle, level)) ---

	// CreateTerm persists a new term. A duplicate (cycle_id, level_id, ordinal) is
	// KindConflict; a missing parent cycle or level surfaces as an FK violation.
	CreateTerm(ctx context.Context, t domain.Term) error
	// GetTerm returns a term by id, or KindNotFound.
	GetTerm(ctx context.Context, id string) (domain.Term, error)
	// ListTerms returns the terms of cycleID, optionally narrowed to levelID
	// ("" = all levels of the cycle).
	ListTerms(ctx context.Context, cycleID string, levelID string) ([]domain.Term, error)

	// --- groups (grupos, per (cycle, grade level)) ---

	// CreateGroup persists a new group. A duplicate (cycle_id, grade_level_id, name)
	// is KindConflict; a missing parent cycle or grade level surfaces as an FK violation.
	CreateGroup(ctx context.Context, g domain.Group) error
	// GetGroup returns a group by id, or KindNotFound.
	GetGroup(ctx context.Context, id string) (domain.Group, error)
	// ListGroups returns the groups of cycleID, optionally narrowed to
	// gradeLevelID ("" = all grade levels of the cycle).
	ListGroups(ctx context.Context, cycleID string, gradeLevelID string) ([]domain.Group, error)
}
