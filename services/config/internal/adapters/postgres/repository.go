package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/authctx"
	"github.com/garcia-lorca/pkg/authctx/gormtx"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// Compile-time proof the adapter satisfies its port.
var _ ports.CatalogRepository = (*Repository)(nil)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
	checkViolation      = "23514"
	// emailNormalizedCheck is the CHECK the email migration adds. It is the one
	// CHECK an API client can trip (a raw email that bypassed the domain, e.g.
	// against the previous revision mid-deploy), so it alone maps to KindInvalid;
	// any other CHECK means the server is inconsistent and stays KindInternal.
	emailNormalizedCheck = "teachers_email_normalized_ck"
)

// Repository is the GORM-backed config CatalogRepository. Every operation runs
// in a transaction bound to a tenant so Row Level Security applies (mirrors iam
// / students, ADR 0001). In M04 the tenant is fixed at construction
// (single-tenant).
type Repository struct {
	db       *gorm.DB
	tenantID string
}

// NewRepository builds the repository scoped to tenantID.
func NewRepository(db *gorm.DB, tenantID string) *Repository {
	return &Repository{db: db, tenantID: tenantID}
}

// withTenant runs fn inside a transaction that has bound app.current_tenant,
// delegating to the shared pkg/authctx RLS primitive so tenant binding lives in
// exactly one place across every service.
func (r *Repository) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return authctx.WithTenantContext(ctx, gormtx.NewRunner(r.db), r.tenantID, func(tx authctx.Tx) error {
		return fn(gormtx.Gorm(tx))
	})
}

// --- cycles ---------------------------------------------------------------

// CreateCycle persists a new cycle. A duplicate name (per tenant) is KindConflict.
func (r *Repository) CreateCycle(ctx context.Context, c domain.Cycle) error {
	model := fromDomainCycle(c)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating cycle", "a cycle with this name already exists")
	})
}

// ActivateCycle promotes one cycle and demotes whichever other was live, in one
// transaction.
//
// Both writes together, because "at most one active cycle" is an invariant
// ACROSS rows: two callers activating two different years would each read one
// active cycle, each demote it and each promote their own, leaving a school with
// two live years and every downstream question ("which cycle is active?")
// answering whichever row came back first.
//
// The demotion is scoped to rows that are actually active and are not this one,
// so re-activating the live year writes nothing anybody has to reason about.
func (r *Repository) ActivateCycle(ctx context.Context, c domain.Cycle) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Model(&cycleModel{}).
			Where("status = ? AND id <> ?", domain.CycleStatusActive.String(), c.ID()).
			Update("status", domain.CycleStatusDraft.String()).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "standing down the previous school year", err)
		}
		res := tx.Model(&cycleModel{}).
			Where("id = ?", c.ID()).
			Updates(map[string]any{"status": c.Status().String(), "updated_at": gorm.Expr("now()")})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "activating the school year", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// GetCycle returns a cycle by id, or KindNotFound.
func (r *Repository) GetCycle(ctx context.Context, id string) (domain.Cycle, error) {
	var out domain.Cycle
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m cycleModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		var derr error
		out, derr = toDomainCycle(m)
		return derr
	})
	if err != nil {
		return domain.Cycle{}, err
	}
	return out, nil
}

// ListCycles returns every cycle in the tenant, ordered by name.
func (r *Repository) ListCycles(ctx context.Context) ([]domain.Cycle, error) {
	var out []domain.Cycle
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []cycleModel
		if err := tx.Order("name").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing cycles", err)
		}
		out = make([]domain.Cycle, 0, len(rows))
		for _, m := range rows {
			c, derr := toDomainCycle(m)
			if derr != nil {
				return derr
			}
			out = append(out, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- levels ---------------------------------------------------------------

// CreateLevel persists a new level. A duplicate name (per tenant) is KindConflict.
func (r *Repository) CreateLevel(ctx context.Context, l domain.Level) error {
	model := fromDomainLevel(l)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating level", "a level with this name already exists")
	})
}

// GetLevel returns a level by id, or KindNotFound.
func (r *Repository) GetLevel(ctx context.Context, id string) (domain.Level, error) {
	var out domain.Level
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m levelModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		var derr error
		out, derr = toDomainLevel(m)
		return derr
	})
	if err != nil {
		return domain.Level{}, err
	}
	return out, nil
}

// ListLevels returns every level in the tenant catalog, ordered by name.
func (r *Repository) ListLevels(ctx context.Context) ([]domain.Level, error) {
	var out []domain.Level
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []levelModel
		if err := tx.Order("name").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing levels", err)
		}
		out = make([]domain.Level, 0, len(rows))
		for _, m := range rows {
			l, derr := toDomainLevel(m)
			if derr != nil {
				return derr
			}
			out = append(out, l)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- grade levels ---------------------------------------------------------

// CreateGradeLevel persists a new grade level. A duplicate (level_id, ordinal)
// is KindConflict; a missing parent level surfaces as KindInvalid.
func (r *Repository) CreateGradeLevel(ctx context.Context, g domain.GradeLevel) error {
	model := fromDomainGradeLevel(g)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating grade level", "a grade level with this ordinal already exists in the level")
	})
}

// GetGradeLevel returns a grade level by id, or KindNotFound.
func (r *Repository) GetGradeLevel(ctx context.Context, id string) (domain.GradeLevel, error) {
	var out domain.GradeLevel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m gradeLevelModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		var derr error
		out, derr = toDomainGradeLevel(m)
		return derr
	})
	if err != nil {
		return domain.GradeLevel{}, err
	}
	return out, nil
}

// ListGradeLevels returns the grade levels of levelID, ordered by ordinal. An
// empty levelID means "no filter": every grade level in the tenant (still
// RLS-scoped by withTenant). Guarding the predicate is required because level_id
// is a uuid column and binding "" to it is a 22P02 error, not an empty result.
func (r *Repository) ListGradeLevels(ctx context.Context, levelID string) ([]domain.GradeLevel, error) {
	var out []domain.GradeLevel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx
		if levelID != "" {
			q = q.Where("level_id = ?", levelID)
		}
		var rows []gradeLevelModel
		if err := q.Order("ordinal").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing grade levels", err)
		}
		out = make([]domain.GradeLevel, 0, len(rows))
		for _, m := range rows {
			g, derr := toDomainGradeLevel(m)
			if derr != nil {
				return derr
			}
			out = append(out, g)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- subjects -------------------------------------------------------------

// CreateSubject persists a new subject. A duplicate (grade_level_id, name) is
// KindConflict; a missing parent grade level surfaces as KindInvalid.
func (r *Repository) CreateSubject(ctx context.Context, s domain.Subject) error {
	model := fromDomainSubject(s)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating subject", "a subject with this name already exists in the grade level")
	})
}

// GetSubject returns a subject by id, or KindNotFound.
func (r *Repository) GetSubject(ctx context.Context, id string) (domain.Subject, error) {
	var out domain.Subject
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m subjectModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		var derr error
		out, derr = toDomainSubject(m)
		return derr
	})
	if err != nil {
		return domain.Subject{}, err
	}
	return out, nil
}

// ListSubjects returns the subjects of gradeLevelID, ordered by name. An empty
// gradeLevelID means "no filter": every subject in the tenant (still RLS-scoped
// by withTenant). Guarding the predicate is required because grade_level_id is a
// uuid column and binding "" to it is a 22P02 error, not an empty result.
func (r *Repository) ListSubjects(ctx context.Context, gradeLevelID string) ([]domain.Subject, error) {
	var out []domain.Subject
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx
		if gradeLevelID != "" {
			q = q.Where("grade_level_id = ?", gradeLevelID)
		}
		var rows []subjectModel
		if err := q.Order("name").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing subjects", err)
		}
		out = make([]domain.Subject, 0, len(rows))
		for _, m := range rows {
			s, derr := toDomainSubject(m)
			if derr != nil {
				return derr
			}
			out = append(out, s)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- terms ----------------------------------------------------------------

// CreateTerm persists a new term. A duplicate (cycle_id, level_id, ordinal) is
// KindConflict; a missing parent cycle or level surfaces as KindInvalid.
func (r *Repository) CreateTerm(ctx context.Context, t domain.Term) error {
	model := fromDomainTerm(t)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating term", "a term with this ordinal already exists in the cycle and level")
	})
}

// GetTerm returns a term by id, or KindNotFound.
func (r *Repository) GetTerm(ctx context.Context, id string) (domain.Term, error) {
	var out domain.Term
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m termModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		var derr error
		out, derr = toDomainTerm(m)
		return derr
	})
	if err != nil {
		return domain.Term{}, err
	}
	return out, nil
}

// ListTerms returns terms filtered by any of cycleID and levelID
// ("" = do not filter on that field), ordered by ordinal. Both are uuid
// columns, so each predicate must be guarded: binding "" would be a 22P02
// error rather than a no-op. Results stay RLS-scoped by withTenant.
func (r *Repository) ListTerms(ctx context.Context, cycleID string, levelID string) ([]domain.Term, error) {
	var out []domain.Term
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&termModel{})
		if cycleID != "" {
			q = q.Where("cycle_id = ?", cycleID)
		}
		if levelID != "" {
			q = q.Where("level_id = ?", levelID)
		}
		var rows []termModel
		if err := q.Order("ordinal").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing terms", err)
		}
		out = make([]domain.Term, 0, len(rows))
		for _, m := range rows {
			t, derr := toDomainTerm(m)
			if derr != nil {
				return derr
			}
			out = append(out, t)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- groups ---------------------------------------------------------------

// CreateGroup persists a new group. A duplicate (cycle_id, grade_level_id, name)
// is KindConflict; a missing parent cycle or grade level surfaces as KindInvalid.
func (r *Repository) CreateGroup(ctx context.Context, g domain.Group) error {
	model := fromDomainGroup(g)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating group", "a group with this name already exists in the cycle and grade level")
	})
}

// GetGroup returns a group by id, or KindNotFound.
func (r *Repository) GetGroup(ctx context.Context, id string) (domain.Group, error) {
	var out domain.Group
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m groupModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		var derr error
		out, derr = toDomainGroup(m)
		return derr
	})
	if err != nil {
		return domain.Group{}, err
	}
	return out, nil
}

// ListGroups returns groups filtered by any of cycleID and gradeLevelID
// ("" = do not filter on that field), ordered by name. Both are uuid columns,
// so each predicate must be guarded: binding "" would be a 22P02 error rather
// than a no-op. Results stay RLS-scoped by withTenant.
func (r *Repository) ListGroups(ctx context.Context, cycleID string, gradeLevelID string) ([]domain.Group, error) {
	var out []domain.Group
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&groupModel{})
		if cycleID != "" {
			q = q.Where("cycle_id = ?", cycleID)
		}
		if gradeLevelID != "" {
			q = q.Where("grade_level_id = ?", gradeLevelID)
		}
		var rows []groupModel
		if err := q.Order("name").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing groups", err)
		}
		out = make([]domain.Group, 0, len(rows))
		for _, m := range rows {
			g, derr := toDomainGroup(m)
			if derr != nil {
				return derr
			}
			out = append(out, g)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- teachers -------------------------------------------------------------

// CreateTeacher persists a new teacher. A duplicate email (per tenant) is
// KindConflict.
func (r *Repository) CreateTeacher(ctx context.Context, t domain.Teacher) error {
	model := fromDomainTeacher(t)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating teacher", "a teacher with this email already exists")
	})
}

// GetTeacher returns a teacher by id, or KindNotFound.
func (r *Repository) GetTeacher(ctx context.Context, id string) (domain.Teacher, error) {
	var out domain.Teacher
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m teacherModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		var derr error
		out, derr = toDomainTeacher(m)
		return derr
	})
	if err != nil {
		return domain.Teacher{}, err
	}
	return out, nil
}

// ListTeachers returns every teacher in the tenant, ordered by (last, first) name.
func (r *Repository) ListTeachers(ctx context.Context) ([]domain.Teacher, error) {
	var out []domain.Teacher
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []teacherModel
		if err := tx.Order("last_name, first_name, id").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing teachers", err)
		}
		out = make([]domain.Teacher, 0, len(rows))
		for _, m := range rows {
			te, derr := toDomainTeacher(m)
			if derr != nil {
				return derr
			}
			out = append(out, te)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetTeacherUserID links a teacher to an IAM user, or unlinks it when userID is
// nil (persisting SQL NULL). An id that matches no row is KindNotFound.
func (r *Repository) SetTeacherUserID(ctx context.Context, teacherID string, userID *string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&teacherModel{}).
			Where("id = ?", teacherID).
			Update("user_id", userID)
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "linking teacher to user", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// --- teaching assignments (the knot) --------------------------------------

// CreateTeachingAssignment persists a new assignment. A duplicate
// (cycle_id, group_id, subject_id) is KindConflict; a missing referenced row
// surfaces as an FK violation mapped to KindInvalid (the use case forestalls
// this by checking the group, subject, and teacher up front).
func (r *Repository) CreateTeachingAssignment(ctx context.Context, a domain.TeachingAssignment) error {
	model := fromDomainTeachingAssignment(a)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating teaching assignment", "a teacher is already assigned to this group and subject in the cycle")
	})
}

// GetTeachingAssignment returns an assignment by id, or KindNotFound.
func (r *Repository) GetTeachingAssignment(ctx context.Context, id string) (domain.TeachingAssignment, error) {
	var out domain.TeachingAssignment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m teachingAssignmentModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		var derr error
		out, derr = toDomainTeachingAssignment(m)
		return derr
	})
	if err != nil {
		return domain.TeachingAssignment{}, err
	}
	return out, nil
}

// ListTeachingAssignments returns assignments filtered by any of cycleID,
// groupID, and teacherID ("" = do not filter on that field), ordered
// deterministically by (cycle, group, subject).
func (r *Repository) ListTeachingAssignments(ctx context.Context, cycleID string, groupID string, teacherID string) ([]domain.TeachingAssignment, error) {
	var out []domain.TeachingAssignment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&teachingAssignmentModel{})
		if cycleID != "" {
			q = q.Where("cycle_id = ?", cycleID)
		}
		if groupID != "" {
			q = q.Where("group_id = ?", groupID)
		}
		if teacherID != "" {
			q = q.Where("teacher_id = ?", teacherID)
		}
		var rows []teachingAssignmentModel
		if err := q.Order("cycle_id, group_id, subject_id").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing teaching assignments", err)
		}
		out = make([]domain.TeachingAssignment, 0, len(rows))
		for _, m := range rows {
			a, derr := toDomainTeachingAssignment(m)
			if derr != nil {
				return derr
			}
			out = append(out, a)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- shared helpers -------------------------------------------------------

// translateCreateErr maps a GORM insert error to the port's error contract: a
// unique violation is KindConflict (conflictMsg), a foreign-key violation is
// KindInvalid (the referenced parent is missing), the email-normalization CHECK
// is KindInvalid, anything else is KindInternal.
func translateCreateErr(err error, internalMsg, conflictMsg string) error {
	if err == nil {
		return nil
	}
	switch pgCode(err) {
	case uniqueViolation:
		return glerrors.Wrap(glerrors.KindConflict, conflictMsg, err)
	case foreignKeyViolation:
		return glerrors.Wrap(glerrors.KindInvalid, "referenced parent does not exist", err)
	case checkViolation:
		if isEmailCheck(err) {
			return glerrors.Wrap(glerrors.KindInvalid, "email is not in its normalized form", err)
		}
		return glerrors.Wrap(glerrors.KindInternal, internalMsg, err)
	default:
		return glerrors.Wrap(glerrors.KindInternal, internalMsg, err)
	}
}

// isEmailCheck reports whether err is the email-normalization CHECK violation.
func isEmailCheck(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == emailNormalizedCheck
}

// pgCode extracts the SQLSTATE code from a driver error, or "" if it is not one.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func translateFindErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "query failed", err)
}

// --- school calendar (M16) -------------------------------------------------

// CreateNonWorkingPeriod persists a closed period. The same label twice in one
// cycle is a duplicate entry, not two closures, so it comes back KindConflict.
func (r *Repository) CreateNonWorkingPeriod(ctx context.Context, p domain.NonWorkingPeriod) error {
	model := fromDomainNonWorkingPeriod(p)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating non-working period",
			"this cycle already has a period with that name")
	})
}

// ListNonWorkingPeriods returns every closed period of a cycle, earliest first.
// The order is part of the contract: a person reads this as a calendar.
func (r *Repository) ListNonWorkingPeriods(ctx context.Context, cycleID string) ([]domain.NonWorkingPeriod, error) {
	var out []domain.NonWorkingPeriod
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []nonWorkingPeriodModel
		if err := tx.Where("cycle_id = ?", cycleID).Order("starts_on, id").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing non-working periods", err)
		}
		out = make([]domain.NonWorkingPeriod, 0, len(rows))
		for _, m := range rows {
			p, derr := toDomainNonWorkingPeriod(m)
			if derr != nil {
				return derr
			}
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteNonWorkingPeriod removes one period. An id that matches no row is
// KindNotFound rather than a silent success: the caller asked for a day to stop
// being closed, and needs to know whether it did.
func (r *Repository) DeleteNonWorkingPeriod(ctx context.Context, id string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Where("id = ?", id).Delete(&nonWorkingPeriodModel{})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting non-working period", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// --- school events (M16) ---------------------------------------------------

// CreateSchoolEvent persists something the school does.
func (r *Repository) CreateSchoolEvent(ctx context.Context, e domain.SchoolEvent) error {
	model := fromDomainSchoolEvent(e)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating school event",
			"this event already exists")
	})
}

// ListSchoolEvents returns a cycle's agenda, earliest first. The order is part
// of the contract: a person reads this as a calendar.
func (r *Repository) ListSchoolEvents(ctx context.Context, cycleID string) ([]domain.SchoolEvent, error) {
	var out []domain.SchoolEvent
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []schoolEventModel
		if err := tx.Where("cycle_id = ?", cycleID).Order("starts_on, id").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing school events", err)
		}
		out = make([]domain.SchoolEvent, 0, len(rows))
		for _, m := range rows {
			e, derr := toDomainSchoolEvent(m)
			if derr != nil {
				return derr
			}
			out = append(out, e)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteSchoolEvent removes one event. An id that matches no row is
// KindNotFound rather than a silent success: somebody asked for a date to come
// off the calendar and needs to know whether it did.
func (r *Repository) DeleteSchoolEvent(ctx context.Context, id string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Where("id = ?", id).Delete(&schoolEventModel{})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting school event", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}
