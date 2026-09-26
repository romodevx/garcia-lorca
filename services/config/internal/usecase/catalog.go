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

// CreateCycle validates and persists a new cycle. An unspecified status defaults
// to draft (matching the proto contract, the DDL DEFAULT 'draft', and the
// fewest-clicks UX) — a new cycle always begins as a draft. A duplicate name
// (per tenant) surfaces as KindConflict from the repo.
func (s *CatalogService) CreateCycle(ctx context.Context, name string, status domain.CycleStatus, startsOn, endsOn time.Time) (domain.Cycle, error) {
	if status == domain.CycleStatusUnspecified {
		status = domain.CycleStatusDraft
	}
	c, err := domain.NewCycle(s.newID(), s.tenant, name, status, startsOn, endsOn)
	if err != nil {
		return domain.Cycle{}, err
	}
	if err := s.repo.CreateCycle(ctx, c); err != nil {
		return domain.Cycle{}, err
	}
	return c, nil
}

// ActivateCycle makes a cycle the school's live year.
//
// Everything downstream asks "which cycle is active" — the roster's header, the
// group dropdowns, the tuition table, the billing run — and until now nothing
// could answer, because cycles were created in draft and had no way out. The
// panel papered over it by falling back to the first cycle it found, which is
// right exactly while a school has one year and wrong from the second onwards.
func (s *CatalogService) ActivateCycle(ctx context.Context, id string) (domain.Cycle, error) {
	current, err := s.repo.GetCycle(ctx, id)
	if err != nil {
		return domain.Cycle{}, err
	}
	activated, err := current.Activate()
	if err != nil {
		return domain.Cycle{}, err
	}
	if err := s.repo.ActivateCycle(ctx, activated); err != nil {
		return domain.Cycle{}, err
	}
	return activated, nil
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

// SetLevelAttendanceMode changes how a level takes attendance and returns the
// updated level. An unknown mode is KindInvalid (checked before any write); an
// unknown level is KindNotFound.
func (s *CatalogService) SetLevelAttendanceMode(ctx context.Context, id string, mode domain.AttendanceMode) (domain.Level, error) {
	if !mode.Valid() {
		return domain.Level{}, glerrors.New(glerrors.KindInvalid, "level attendance mode is invalid")
	}
	if err := s.repo.UpdateLevelAttendanceMode(ctx, id, mode); err != nil {
		return domain.Level{}, err
	}
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

// --- teachers --------------------------------------------------------------

// CreateTeacher validates and persists a new teacher (a catalog person record,
// not a login account). `userID` is optional ("" = no IAM link yet). A duplicate
// email (per tenant) surfaces as KindConflict from the repo.
// SchoolEventInput is something the school does: a festival, a parents' meeting,
// an exam week.
type SchoolEventInput struct {
	CycleID     string
	Title       string
	Description string
	From        time.Time
	To          time.Time
	// Audience is one level, one group, or — left empty — the whole school.
	Audience domain.EventAudience
}

// CreateSchoolEvent records something the school DOES.
//
// It is stored apart from the closed days on purpose. finance reads
// non_working_periods to move a due date and stop a surcharge; an activity must
// never do either. A festival on the 10th does not mean nobody could pay on the
// 10th, and one table with a kind column would put the school's agenda one typo
// away from moving four hundred families' deadlines.
func (s *CatalogService) CreateSchoolEvent(ctx context.Context, in SchoolEventInput) (domain.SchoolEvent, error) {
	if _, err := s.repo.GetCycle(ctx, in.CycleID); err != nil {
		return domain.SchoolEvent{}, glerrors.New(glerrors.KindInvalid, "cycle does not exist")
	}
	event, err := domain.NewSchoolEvent(s.newID(), s.tenant, in.CycleID,
		in.Title, in.Description, in.From, in.To, in.Audience)
	if err != nil {
		return domain.SchoolEvent{}, err
	}
	if err := s.repo.CreateSchoolEvent(ctx, event); err != nil {
		return domain.SchoolEvent{}, err
	}
	return event, nil
}

// GetSchoolProfile returns what a receipt prints about the school (task 12);
// found is false until somebody fills it in.
func (s *CatalogService) GetSchoolProfile(ctx context.Context) (domain.SchoolProfile, bool, error) {
	return s.repo.GetSchoolProfile(ctx)
}

// SetSchoolProfile validates and saves the school's details, replacing any.
func (s *CatalogService) SetSchoolProfile(ctx context.Context, in domain.SchoolProfileInput) (domain.SchoolProfile, error) {
	profile, err := domain.NewSchoolProfile(s.tenant, in)
	if err != nil {
		return domain.SchoolProfile{}, err
	}
	if err := s.repo.UpsertSchoolProfile(ctx, profile); err != nil {
		return domain.SchoolProfile{}, err
	}
	return profile, nil
}

// ListSchoolEvents returns a cycle's agenda, earliest first.
func (s *CatalogService) ListSchoolEvents(ctx context.Context, cycleID string) ([]domain.SchoolEvent, error) {
	return s.repo.ListSchoolEvents(ctx, cycleID)
}

// DeleteSchoolEvent removes one event (KindNotFound if absent).
func (s *CatalogService) DeleteSchoolEvent(ctx context.Context, id string) error {
	return s.repo.DeleteSchoolEvent(ctx, id)
}

// --- school calendar (M16) -------------------------------------------------

// CreateNonWorkingPeriod records a day, or a run of days, on which the school
// does not operate.
//
// The cycle is checked first. A calendar hanging from a school year that does
// not exist is a typo, and the cost of finding out later is a due date that
// moves in March for a reason nobody can explain.
func (s *CatalogService) CreateNonWorkingPeriod(ctx context.Context, cycleID, label, kind string, from, to time.Time) (domain.NonWorkingPeriod, error) {
	if _, err := s.repo.GetCycle(ctx, cycleID); err != nil {
		return domain.NonWorkingPeriod{}, glerrors.New(glerrors.KindInvalid, "cycle does not exist")
	}
	p, err := domain.NewNonWorkingPeriod(s.newID(), s.tenant, cycleID, label, kind, from, to)
	if err != nil {
		return domain.NonWorkingPeriod{}, err
	}
	if err := s.repo.CreateNonWorkingPeriod(ctx, p); err != nil {
		return domain.NonWorkingPeriod{}, err
	}
	return p, nil
}

// ListNonWorkingPeriods returns the closed periods of a cycle, ordered by start
// date.
func (s *CatalogService) ListNonWorkingPeriods(ctx context.Context, cycleID string) ([]domain.NonWorkingPeriod, error) {
	return s.repo.ListNonWorkingPeriods(ctx, cycleID)
}

// DeleteNonWorkingPeriod removes one period (KindNotFound if absent).
func (s *CatalogService) DeleteNonWorkingPeriod(ctx context.Context, id string) error {
	return s.repo.DeleteNonWorkingPeriod(ctx, id)
}

func (s *CatalogService) CreateTeacher(ctx context.Context, firstName, lastName, email, userID string) (domain.Teacher, error) {
	te, err := domain.NewTeacher(s.newID(), s.tenant, firstName, lastName, email, userID)
	if err != nil {
		return domain.Teacher{}, err
	}
	if err := s.repo.CreateTeacher(ctx, te); err != nil {
		return domain.Teacher{}, err
	}
	return te, nil
}

// GetTeacher returns a teacher by id (KindNotFound if absent).
func (s *CatalogService) GetTeacher(ctx context.Context, id string) (domain.Teacher, error) {
	return s.repo.GetTeacher(ctx, id)
}

// ListTeachers returns every teacher in the tenant.
func (s *CatalogService) ListTeachers(ctx context.Context) ([]domain.Teacher, error) {
	return s.repo.ListTeachers(ctx)
}

// SetTeacherUser links a teacher catalog record to an IAM user, or unlinks it
// when userID is "" (stored as SQL NULL). The teacher is loaded first so a
// missing teacher surfaces as KindNotFound before any write. The IAM user is NOT
// validated here — the gateway checks it exists (via iam GetUser) before calling
// this; config just stores the link.
func (s *CatalogService) SetTeacherUser(ctx context.Context, teacherID, userID string) (domain.Teacher, error) {
	te, err := s.repo.GetTeacher(ctx, teacherID)
	if err != nil {
		return domain.Teacher{}, err // KindNotFound propagates
	}
	var stored *string
	if userID != "" {
		stored = &userID
	}
	if err := s.repo.SetTeacherUserID(ctx, teacherID, stored); err != nil {
		return domain.Teacher{}, err
	}
	return te.WithUserID(userID), nil
}

// --- teaching assignments (the knot) ---------------------------------------

// CreateTeachingAssignment ties a teacher to a group and a subject within a
// cycle. It enforces the relational invariants the school cares about, in order,
// before writing — so a violation is a precise KindInvalid at the boundary
// rather than an opaque FK error:
//
//  1. the group exists (missing → KindInvalid);
//  2. the subject exists (missing → KindInvalid);
//  3. the subject is teachable for the group — its grade level matches the
//     group's grade level (mismatch → KindInvalid);
//  4. the teacher exists (missing → KindInvalid).
//
// The assignment's cycle is DERIVED from the group (never taken from the caller),
// so it cannot drift from the group it belongs to. A duplicate
// (cycle, group, subject) surfaces as KindConflict from the repo.
func (s *CatalogService) CreateTeachingAssignment(ctx context.Context, groupID, subjectID, teacherID string) (domain.TeachingAssignment, error) {
	grp, err := s.repo.GetGroup(ctx, groupID)
	if err != nil {
		return domain.TeachingAssignment{}, requireParent(err, "group not found")
	}
	subj, err := s.repo.GetSubject(ctx, subjectID)
	if err != nil {
		return domain.TeachingAssignment{}, requireParent(err, "subject not found")
	}
	if subj.GradeLevelID() != grp.GradeLevelID() {
		return domain.TeachingAssignment{}, glerrors.New(glerrors.KindInvalid, "subject does not belong to the group's grade level")
	}
	if _, err := s.repo.GetTeacher(ctx, teacherID); err != nil {
		return domain.TeachingAssignment{}, requireParent(err, "teacher not found")
	}
	a, err := domain.NewTeachingAssignment(s.newID(), s.tenant, grp.CycleID(), groupID, subjectID, teacherID)
	if err != nil {
		return domain.TeachingAssignment{}, err
	}
	if err := s.repo.CreateTeachingAssignment(ctx, a); err != nil {
		return domain.TeachingAssignment{}, err
	}
	return a, nil
}

// GetTeachingAssignment returns an assignment by id (KindNotFound if absent).
func (s *CatalogService) GetTeachingAssignment(ctx context.Context, id string) (domain.TeachingAssignment, error) {
	return s.repo.GetTeachingAssignment(ctx, id)
}

// ListTeachingAssignments returns assignments filtered by any of cycleID,
// groupID, and teacherID ("" = do not filter on that field).
func (s *CatalogService) ListTeachingAssignments(ctx context.Context, cycleID, groupID, teacherID string) ([]domain.TeachingAssignment, error) {
	return s.repo.ListTeachingAssignments(ctx, cycleID, groupID, teacherID)
}
