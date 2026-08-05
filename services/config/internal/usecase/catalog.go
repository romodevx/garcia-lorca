package usecase

import (
	"context"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// CatalogService orchestrates the academic catalog: it constructs the six
// domain entities (the constructors own invariant validation) and persists them
// through the repository. For nested entities it verifies the referenced parent
// exists before writing, so a dangling reference is reported as a precise
// KindInvalid at the boundary rather than as an opaque FK error from the DB.
//
// It depends only on ports + domain. Ids come from an injected mint (uuid in
// production, a counter in tests), so use cases are deterministic under test.
// created_at/updated_at are owned by the DB, so no clock is injected here.
type CatalogService struct {
	repo   ports.CatalogRepository
	tenant string
	newID  func() string
}

// NewCatalogService wires the use case. `tenant` is the single-tenant scope for
// the M-phase (fixed at construction, as in iam/students); `newID` mints entity
// ids — injected for deterministic tests.
func NewCatalogService(repo ports.CatalogRepository, tenant string, newID func() string) *CatalogService {
	return &CatalogService{repo: repo, tenant: tenant, newID: newID}
}

// requireParent maps a parent-existence probe into a create precondition. A
// missing parent (KindNotFound from the Get) becomes KindInvalid with a precise
// message; any other error (e.g. KindInternal) propagates unchanged.
func requireParent(err error, msg string) error {
	if err == nil {
		return nil
	}
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return glerrors.New(glerrors.KindInvalid, msg)
	}
	return err
}

// --- cycles ----------------------------------------------------------------

// CreateCycle validates and persists a new cycle. A duplicate name (per tenant)
// surfaces as KindConflict from the repo.
func (s *CatalogService) CreateCycle(ctx context.Context, name string, status domain.CycleStatus, startsOn, endsOn time.Time) (domain.Cycle, error) {
	c, err := domain.NewCycle(s.newID(), s.tenant, name, status, startsOn, endsOn)
	if err != nil {
		return domain.Cycle{}, err
	}
	if err := s.repo.CreateCycle(ctx, c); err != nil {
		return domain.Cycle{}, err
	}
	return c, nil
}

// GetCycle returns a cycle by id (KindNotFound if absent).
func (s *CatalogService) GetCycle(ctx context.Context, id string) (domain.Cycle, error) {
	return s.repo.GetCycle(ctx, id)
}

// ListCycles returns every cycle in the tenant.
func (s *CatalogService) ListCycles(ctx context.Context) ([]domain.Cycle, error) {
	return s.repo.ListCycles(ctx)
}

// --- levels ----------------------------------------------------------------

// CreateLevel validates and persists a new level. A duplicate name (per tenant)
// surfaces as KindConflict from the repo.
func (s *CatalogService) CreateLevel(ctx context.Context, name, sepCode string, gran domain.EnrollmentGranularity) (domain.Level, error) {
	l, err := domain.NewLevel(s.newID(), s.tenant, name, sepCode, gran)
	if err != nil {
		return domain.Level{}, err
	}
	if err := s.repo.CreateLevel(ctx, l); err != nil {
		return domain.Level{}, err
	}
	return l, nil
}

// GetLevel returns a level by id (KindNotFound if absent).
func (s *CatalogService) GetLevel(ctx context.Context, id string) (domain.Level, error) {
	return s.repo.GetLevel(ctx, id)
}

// ListLevels returns every level in the tenant catalog.
func (s *CatalogService) ListLevels(ctx context.Context) ([]domain.Level, error) {
	return s.repo.ListLevels(ctx)
}

// --- grade levels ----------------------------------------------------------

// CreateGradeLevel validates the input, verifies the parent level exists (a
// missing parent → KindInvalid), then persists. A duplicate (level, ordinal) is
// KindConflict.
func (s *CatalogService) CreateGradeLevel(ctx context.Context, levelID, name string, ordinal int) (domain.GradeLevel, error) {
	g, err := domain.NewGradeLevel(s.newID(), s.tenant, levelID, name, ordinal)
	if err != nil {
		return domain.GradeLevel{}, err
	}
	if _, err := s.repo.GetLevel(ctx, levelID); err != nil {
		return domain.GradeLevel{}, requireParent(err, "level not found")
	}
	if err := s.repo.CreateGradeLevel(ctx, g); err != nil {
		return domain.GradeLevel{}, err
	}
	return g, nil
}

// GetGradeLevel returns a grade level by id (KindNotFound if absent).
func (s *CatalogService) GetGradeLevel(ctx context.Context, id string) (domain.GradeLevel, error) {
	return s.repo.GetGradeLevel(ctx, id)
}

// ListGradeLevels returns the grade levels of levelID.
func (s *CatalogService) ListGradeLevels(ctx context.Context, levelID string) ([]domain.GradeLevel, error) {
	return s.repo.ListGradeLevels(ctx, levelID)
}

// --- subjects --------------------------------------------------------------

// CreateSubject validates the input, verifies the parent grade level exists (a
// missing parent → KindInvalid), then persists. A duplicate (grade level, name)
// is KindConflict.
func (s *CatalogService) CreateSubject(ctx context.Context, gradeLevelID, name string) (domain.Subject, error) {
	sub, err := domain.NewSubject(s.newID(), s.tenant, gradeLevelID, name)
	if err != nil {
		return domain.Subject{}, err
	}
	if _, err := s.repo.GetGradeLevel(ctx, gradeLevelID); err != nil {
		return domain.Subject{}, requireParent(err, "grade level not found")
	}
	if err := s.repo.CreateSubject(ctx, sub); err != nil {
		return domain.Subject{}, err
	}
	return sub, nil
}

// GetSubject returns a subject by id (KindNotFound if absent).
func (s *CatalogService) GetSubject(ctx context.Context, id string) (domain.Subject, error) {
	return s.repo.GetSubject(ctx, id)
}

// ListSubjects returns the subjects of gradeLevelID.
func (s *CatalogService) ListSubjects(ctx context.Context, gradeLevelID string) ([]domain.Subject, error) {
	return s.repo.ListSubjects(ctx, gradeLevelID)
}

// --- terms -----------------------------------------------------------------

// CreateTerm validates the input, verifies both parents (cycle and level) exist
// (a missing parent → KindInvalid), then persists. A duplicate
// (cycle, level, ordinal) is KindConflict.
func (s *CatalogService) CreateTerm(ctx context.Context, cycleID, levelID, name string, ordinal int, startsOn, endsOn time.Time) (domain.Term, error) {
	t, err := domain.NewTerm(s.newID(), s.tenant, cycleID, levelID, name, ordinal, startsOn, endsOn)
	if err != nil {
		return domain.Term{}, err
	}
	if _, err := s.repo.GetCycle(ctx, cycleID); err != nil {
		return domain.Term{}, requireParent(err, "cycle not found")
	}
	if _, err := s.repo.GetLevel(ctx, levelID); err != nil {
		return domain.Term{}, requireParent(err, "level not found")
	}
	if err := s.repo.CreateTerm(ctx, t); err != nil {
		return domain.Term{}, err
	}
	return t, nil
}

// GetTerm returns a term by id (KindNotFound if absent).
func (s *CatalogService) GetTerm(ctx context.Context, id string) (domain.Term, error) {
	return s.repo.GetTerm(ctx, id)
}

// ListTerms returns the terms of cycleID, optionally narrowed to levelID
// ("" = all levels of the cycle).
func (s *CatalogService) ListTerms(ctx context.Context, cycleID, levelID string) ([]domain.Term, error) {
	return s.repo.ListTerms(ctx, cycleID, levelID)
}

// --- groups ----------------------------------------------------------------

// CreateGroup validates the input, verifies both parents (cycle and grade
// level) exist (a missing parent → KindInvalid), then persists. A duplicate
// (cycle, grade level, name) is KindConflict.
func (s *CatalogService) CreateGroup(ctx context.Context, cycleID, gradeLevelID, name string, capacity int) (domain.Group, error) {
	g, err := domain.NewGroup(s.newID(), s.tenant, cycleID, gradeLevelID, name, capacity)
	if err != nil {
		return domain.Group{}, err
	}
	if _, err := s.repo.GetCycle(ctx, cycleID); err != nil {
		return domain.Group{}, requireParent(err, "cycle not found")
	}
	if _, err := s.repo.GetGradeLevel(ctx, gradeLevelID); err != nil {
		return domain.Group{}, requireParent(err, "grade level not found")
	}
	if err := s.repo.CreateGroup(ctx, g); err != nil {
		return domain.Group{}, err
	}
	return g, nil
}

// GetGroup returns a group by id (KindNotFound if absent).
func (s *CatalogService) GetGroup(ctx context.Context, id string) (domain.Group, error) {
	return s.repo.GetGroup(ctx, id)
}

// ListGroups returns the groups of cycleID, optionally narrowed to
// gradeLevelID ("" = all grade levels of the cycle).
func (s *CatalogService) ListGroups(ctx context.Context, cycleID, gradeLevelID string) ([]domain.Group, error) {
	return s.repo.ListGroups(ctx, cycleID, gradeLevelID)
}
