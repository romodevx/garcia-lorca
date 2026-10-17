package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	// deadlockDetected and serializationFailure abort a transaction that raced
	// another writer. Nothing was committed and a retry can succeed, so both are
	// KindConflict, not an internal fault.
	deadlockDetected     = "40P01"
	serializationFailure = "40001"
	// emailNormalizedCheck is the CHECK the email migration adds. It is the one
	// CHECK an API client can trip (a raw email that bypassed the domain, e.g.
	// against the previous revision mid-deploy), so it alone maps to KindInvalid;
	// any other CHECK means the server is inconsistent and stays KindInternal.
	emailNormalizedCheck = "teachers_email_normalized_ck"
	// gradeNamePerLevelKey is the unique index migration 0006 adds: one grade
	// name per level, case-insensitively.
	gradeNamePerLevelKey = "grade_levels_name_per_level_key"
	// termMonthPerLevelKey is migration 0010's UNIQUE: a month belongs to at
	// most one term of a cycle and level.
	termMonthPerLevelKey = "term_months_one_term_per_month"
	// subjectOfGroupGrade is migration 0014's trigger guard: an assignment's
	// subject belongs to its group's grade level.
	subjectOfGroupGrade = "teaching_assignments_subject_of_group_grade"
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
//
// Every write of a level's structure passes through here, so this is where the
// freeze trigger's refusal (migration 0012) becomes LEVEL_BEING_DELETED.
func (r *Repository) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return asLevelBeingDeleted(authctx.WithTenantContext(ctx, gormtx.NewRunner(r.db), r.tenantID, func(tx authctx.Tx) error {
		return fn(gormtx.Gorm(tx))
	}))
}

// --- cycles ---------------------------------------------------------------

// cycleNameTaken is the sentence for the (tenant_id, name) UNIQUE.
const cycleNameTaken = "Ya existe un ciclo con ese nombre"

// cyclesLockKey and nonWorkingLockKey prefix the advisory locks (hashed with
// the tenant, and the tenant + cycle id) that serialize the overlap checks below.
const (
	cyclesLockKey     = "config.cycles:"
	nonWorkingLockKey = "config.non_working_periods:"
)

// advisoryLock takes a transaction-scoped advisory lock on key.
func advisoryLock(tx *gorm.DB, key, what string) error {
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", key).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "locking "+what, err)
	}
	return nil
}

// CreateCycle persists a new cycle. A duplicate name (per tenant) is
// KindConflict; dates shared with another cycle are CYCLE_OVERLAP.
//
// The overlap is checked under a tenant-wide advisory lock rather than an
// EXCLUDE constraint: schools already hold overlapping years (the QA made one),
// and an EXCLUDE cannot be added NOT VALID, so the migration would fail on
// exactly the databases that need the rule. The lock makes two concurrent
// creates take turns, so the second one sees the first.
func (r *Repository) CreateCycle(ctx context.Context, c domain.Cycle) error {
	model := fromDomainCycle(c)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := advisoryLock(tx, cyclesLockKey+r.tenantID, "the school years"); err != nil {
			return err
		}
		others, err := listCycles(tx)
		if err != nil {
			return err
		}
		if err := domain.RefuseCycleOverlap(c, others); err != nil {
			return err
		}
		return translateCreateErr(tx.Create(&model).Error, "creating cycle", cycleNameTaken)
	})
}

// RenameCycle writes c's name. A malformed or unknown id is KindNotFound; a
// name another cycle has is KindConflict.
func (r *Repository) RenameCycle(ctx context.Context, c domain.Cycle) error {
	if !isUUID(c.ID()) {
		return glerrors.ErrNotFound
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&cycleModel{}).
			Where("id = ?", c.ID()).
			Updates(map[string]any{"name": c.Name(), "updated_at": gorm.Expr("now()")})
		if res.Error != nil {
			return translateCreateErr(res.Error, "renaming cycle", cycleNameTaken)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
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
//
// A year whose dates share a day with another is refused (CYCLE_OVERLAP) under
// the same tenant lock CreateCycle takes, before anything is stood down: drafts
// stored before the overlap rule existed must not become the live year.
func (r *Repository) ActivateCycle(ctx context.Context, c domain.Cycle) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := advisoryLock(tx, cyclesLockKey+r.tenantID, "the school years"); err != nil {
			return err
		}
		others, err := listCycles(tx)
		if err != nil {
			return err
		}
		if err := domain.RefuseCycleOverlap(c, others); err != nil {
			return err
		}
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
		var lerr error
		out, lerr = listCycles(tx)
		return lerr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// listCycles reads every cycle the transaction's tenant can see, by name.
func listCycles(tx *gorm.DB) ([]domain.Cycle, error) {
	var rows []cycleModel
	if err := tx.Order("name").Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing cycles", err)
	}
	out := make([]domain.Cycle, 0, len(rows))
	for _, m := range rows {
		c, err := toDomainCycle(m)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
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
	if !isUUID(id) {
		return domain.Level{}, glerrors.ErrNotFound
	}
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

// ListLevels returns every level in the tenant catalog in school order
// (sort_order, then name).
func (r *Repository) ListLevels(ctx context.Context) ([]domain.Level, error) {
	var out []domain.Level
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var lerr error
		out, lerr = listLevels(tx)
		return lerr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// listLevels reads the bound tenant's levels in school order.
func listLevels(tx *gorm.DB) ([]domain.Level, error) {
	var rows []levelModel
	if err := tx.Order("sort_order, name").Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing levels", err)
	}
	out := make([]domain.Level, 0, len(rows))
	for _, m := range rows {
		l, err := toDomainLevel(m)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

// UpdateLevelAttendanceMode changes one level's attendance mode. It writes only
// attendance_mode and updated_at; a level that does not exist — or that RLS
// hides, i.e. another tenant's — is KindNotFound.
func (r *Repository) UpdateLevelAttendanceMode(ctx context.Context, id string, mode domain.AttendanceMode) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&levelModel{}).Where("id = ?", id).Updates(map[string]any{
			"attendance_mode": mode.String(),
			"updated_at":      gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating level attendance mode", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// UpdateLevel writes the present fields of patch in ONE UPDATE, so the mode and
// the scale land together or not at all. A level that does not exist — or that
// RLS hides — is KindNotFound.
func (r *Repository) UpdateLevel(ctx context.Context, id string, patch ports.LevelPatch) error {
	if !isUUID(id) {
		return glerrors.ErrNotFound
	}
	cols := map[string]any{"updated_at": gorm.Expr("now()")}
	if patch.AttendanceMode != nil {
		cols["attendance_mode"] = patch.AttendanceMode.String()
	}
	if g := patch.Grading; g != nil {
		cols["grading_enabled"] = g.Enabled()
		cols["grade_min"] = fromHundredths(g.MinHundredths())
		cols["grade_max"] = fromHundredths(g.MaxHundredths())
		cols["grade_decimals"] = g.Decimals()
		cols["grade_passing"] = fromHundredths(g.PassingHundredths())
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&levelModel{}).Where("id = ?", id).Updates(cols)
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating level", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// UpdateLevelGrading replaces one level's grading scale. It writes only the five
// grading columns and updated_at; a level that does not exist — or that RLS
// hides, i.e. another tenant's — is KindNotFound.
func (r *Repository) UpdateLevelGrading(ctx context.Context, id string, scale domain.GradingScale) error {
	if !isUUID(id) {
		return glerrors.ErrNotFound
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&levelModel{}).Where("id = ?", id).Updates(map[string]any{
			"grading_enabled": scale.Enabled(),
			"grade_min":       fromHundredths(scale.MinHundredths()),
			"grade_max":       fromHundredths(scale.MaxHundredths()),
			"grade_decimals":  scale.Decimals(),
			"grade_passing":   fromHundredths(scale.PassingHundredths()),
			"updated_at":      gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating level grading scale", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// --- grade levels ---------------------------------------------------------

// CreateGradeLevel persists a new grade level. A duplicate (level_id, ordinal)
// is KindConflict; a missing parent level surfaces as KindInvalid.
func (r *Repository) CreateGradeLevel(ctx context.Context, g domain.GradeLevel) error {
	model := fromDomainGradeLevel(g)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		err := tx.Create(&model).Error
		if isConstraint(err, gradeNamePerLevelKey) {
			return glerrors.Wrap(glerrors.KindConflict, "a grade level with this name already exists in the level", err)
		}
		return translateCreateErr(err, "creating grade level", "a grade level with this ordinal already exists in the level")
	})
}

// GetGradeLevel returns a grade level by id, or KindNotFound.
func (r *Repository) GetGradeLevel(ctx context.Context, id string) (domain.GradeLevel, error) {
	if !isUUID(id) {
		return domain.GradeLevel{}, glerrors.ErrNotFound
	}
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

// RenameSubject changes a subject's name in place and returns the stored row.
// A malformed or unknown id is KindNotFound; a name the grade already has is
// KindConflict (the (tenant, grade_level_id, name) UNIQUE).
func (r *Repository) RenameSubject(ctx context.Context, id, name string) (domain.Subject, error) {
	if !isUUID(id) {
		return domain.Subject{}, glerrors.ErrNotFound
	}
	var out domain.Subject
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []subjectModel
		res := tx.Model(&rows).Clauses(clause.Returning{}).
			Where("id = ?", id).
			Updates(map[string]any{"name": name, "updated_at": gorm.Expr("now()")})
		if res.Error != nil {
			return translateCreateErr(res.Error, "renaming subject", "a subject with this name already exists in the grade level")
		}
		if len(rows) == 0 {
			return glerrors.ErrNotFound
		}
		var derr error
		out, derr = toDomainSubject(rows[0])
		return derr
	})
	if err != nil {
		return domain.Subject{}, err
	}
	return out, nil
}

// DeleteSubject removes a subject that never entered a teacher's load. A
// malformed or unknown id is KindNotFound; a subject a teaching assignment
// points at now (ON DELETE RESTRICT) or ever did (first_assigned_at, 0015) is
// KindConflict — grades and attendance in other services keep its id.
func (r *Repository) DeleteSubject(ctx context.Context, id string) error {
	if !isUUID(id) {
		return glerrors.ErrNotFound
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND first_assigned_at IS NULL", id).Delete(&subjectModel{})
		if pgCode(res.Error) == foreignKeyViolation {
			return glerrors.Wrap(glerrors.KindConflict, subjectInUse, res.Error)
		}
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting subject", res.Error)
		}
		if res.RowsAffected == 1 {
			return nil
		}
		var n int64
		if err := tx.Model(&subjectModel{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting subject", err)
		}
		if n == 0 {
			return glerrors.ErrNotFound
		}
		return glerrors.New(glerrors.KindConflict, subjectInUse)
	})
}

// subjectInUse is the refusal for deleting a subject that was ever assigned.
const subjectInUse = "the subject has been in a teaching load; it can be renamed but not deleted"

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
		months, err := loadTermMonths(tx, []string{m.ID})
		if err != nil {
			return err
		}
		var derr error
		out, derr = toDomainTerm(m, months[m.ID])
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
		ids := make([]string, 0, len(rows))
		for _, m := range rows {
			ids = append(ids, m.ID)
		}
		months, err := loadTermMonths(tx, ids)
		if err != nil {
			return err
		}
		out = make([]domain.Term, 0, len(rows))
		for _, m := range rows {
			t, derr := toDomainTerm(m, months[m.ID])
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

// loadTermMonths reads the months of every term in termIDs in ONE query (never
// one per term), keyed by term id, each list ascending.
func loadTermMonths(tx *gorm.DB, termIDs []string) (map[string][]time.Time, error) {
	out := make(map[string][]time.Time, len(termIDs))
	if len(termIDs) == 0 {
		return out, nil
	}
	var rows []termMonthModel
	if err := tx.Where("term_id IN ?", termIDs).Order("term_id, month").Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing term months", err)
	}
	for _, r := range rows {
		out[r.TermID] = append(out[r.TermID], r.Month.UTC())
	}
	return out, nil
}

// ApplyTermMonths writes creates (with their months) and replaces the months
// of updates, in ONE transaction:
//
//  1. every update's current months are released first, so two siblings can
//     swap months in one call without tripping the per-level UNIQUE;
//  2. the new terms are inserted;
//  3. for each (cycle, level) touched, a month still held by another term is
//     refused as MonthInOtherTerm naming the earliest one — the use case checks
//     this too, so here it only catches a concurrent writer;
//  4. the months are inserted. A UNIQUE violation at this point is a writer
//     that committed between 3 and 4: still MonthInOtherTerm, unnamed.
func (r *Repository) ApplyTermMonths(ctx context.Context, creates []domain.Term, updates []domain.Term) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := releaseTermMonths(tx, updates); err != nil {
			return err
		}
		for _, t := range creates {
			model := fromDomainTerm(t)
			if err := translateCreateErr(tx.Create(&model).Error, "creating term", "a term with this ordinal already exists in the cycle and level"); err != nil {
				return err
			}
		}
		written := append(append([]domain.Term(nil), creates...), updates...)
		if err := refuseHeldMonths(tx, written); err != nil {
			return err
		}
		rows := make([]termMonthModel, 0, len(written))
		for _, t := range written {
			rows = append(rows, fromDomainTermMonths(t)...)
		}
		if len(rows) == 0 {
			return nil
		}
		if err := tx.Create(&rows).Error; err != nil {
			if isConstraint(err, termMonthPerLevelKey) {
				return domain.MonthInOtherTerm(time.Time{})
			}
			return translateCreateErr(err, "writing term months", "a term month already exists")
		}
		return nil
	})
}

// releaseTermMonths deletes the current months of every term in terms.
func releaseTermMonths(tx *gorm.DB, terms []domain.Term) error {
	if len(terms) == 0 {
		return nil
	}
	ids := make([]string, 0, len(terms))
	for _, t := range terms {
		ids = append(ids, t.ID())
	}
	if err := tx.Where("term_id IN ?", ids).Delete(&termMonthModel{}).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "releasing term months", err)
	}
	return nil
}

// refuseHeldMonths answers MonthInOtherTerm for the earliest month of terms
// that a term outside the batch still holds in the same cycle and level. One
// query per (cycle, level) touched — a copy touches a handful of levels.
func refuseHeldMonths(tx *gorm.DB, terms []domain.Term) error {
	type scope struct{ cycleID, levelID string }
	wanted := make(map[scope][]time.Time)
	for _, t := range terms {
		k := scope{t.CycleID(), t.LevelID()}
		wanted[k] = append(wanted[k], t.Months()...)
	}
	for k, months := range wanted {
		if len(months) == 0 {
			continue
		}
		var held []time.Time
		if err := tx.Model(&termMonthModel{}).
			Where("cycle_id = ? AND level_id = ? AND month IN ?", k.cycleID, k.levelID, months).
			Order("month").Limit(1).Pluck("month", &held).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "checking term months", err)
		}
		if len(held) > 0 {
			return domain.MonthInOtherTerm(held[0].UTC())
		}
	}
	return nil
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
	if !isUUID(id) {
		return domain.Group{}, glerrors.ErrNotFound
	}
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
	case deadlockDetected, serializationFailure:
		return glerrors.Wrap(glerrors.KindConflict, "the request collided with a concurrent change; retry", err)
	case checkViolation:
		if isEmailCheck(err) {
			return glerrors.Wrap(glerrors.KindInvalid, "email is not in its normalized form", err)
		}
		if isConstraint(err, subjectOfGroupGrade) {
			return glerrors.Wrap(glerrors.KindInvalid, "subject does not belong to the group's grade level", err)
		}
		return glerrors.Wrap(glerrors.KindInternal, internalMsg, err)
	default:
		return glerrors.Wrap(glerrors.KindInternal, internalMsg, err)
	}
}

// isEmailCheck reports whether err is the email-normalization CHECK violation.
func isEmailCheck(err error) bool { return isConstraint(err, emailNormalizedCheck) }

// isConstraint reports whether err is a Postgres violation of the named
// constraint (or unique index).
func isConstraint(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == name
}

// pgCode extracts the SQLSTATE code from a driver error, or "" if it is not one.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// isUUID reports whether id can name a row of a uuid-keyed table. A point read
// by a malformed id finds nothing — NotFound — instead of letting Postgres fail
// the uuid cast (22P02) and surface a caller's typo as an Internal error.
func isUUID(id string) bool {
	return uuid.Validate(id) == nil
}

func translateFindErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "query failed", err)
}

// --- school calendar (M16) -------------------------------------------------

// CreateNonWorkingPeriod persists a closed period. The same label twice in one
// cycle is a duplicate entry, not two closures, so it comes back KindConflict;
// so is a day another period of the cycle already closes (NON_WORKING_OVERLAP),
// checked under a per-cycle advisory lock for the reason CreateCycle gives.
func (r *Repository) CreateNonWorkingPeriod(ctx context.Context, p domain.NonWorkingPeriod) error {
	model := fromDomainNonWorkingPeriod(p)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := advisoryLock(tx, nonWorkingLockKey+r.tenantID+":"+p.CycleID(), "the school calendar"); err != nil {
			return err
		}
		others, err := listNonWorkingPeriods(tx, p.CycleID())
		if err != nil {
			return err
		}
		if err := domain.RefuseNonWorkingOverlap(p, others); err != nil {
			return err
		}
		return translateCreateErr(tx.Create(&model).Error, "creating non-working period",
			"Ya hay un día sin clases con ese nombre en el ciclo")
	})
}

// ListNonWorkingPeriods returns every closed period of a cycle, earliest first.
// The order is part of the contract: a person reads this as a calendar.
func (r *Repository) ListNonWorkingPeriods(ctx context.Context, cycleID string) ([]domain.NonWorkingPeriod, error) {
	var out []domain.NonWorkingPeriod
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var lerr error
		out, lerr = listNonWorkingPeriods(tx, cycleID)
		return lerr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// listNonWorkingPeriods reads a cycle's closed periods, earliest first.
func listNonWorkingPeriods(tx *gorm.DB, cycleID string) ([]domain.NonWorkingPeriod, error) {
	var rows []nonWorkingPeriodModel
	if err := tx.Where("cycle_id = ?", cycleID).Order("starts_on, id").Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing non-working periods", err)
	}
	out := make([]domain.NonWorkingPeriod, 0, len(rows))
	for _, m := range rows {
		p, err := toDomainNonWorkingPeriod(m)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
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

// --- the school's details (task 12) ----------------------------------------

// GetSchoolProfile returns the school's details; found is false when there is
// no row yet.
func (r *Repository) GetSchoolProfile(ctx context.Context) (domain.SchoolProfile, bool, error) {
	var (
		out   domain.SchoolProfile
		found bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []schoolProfileModel
		if err := tx.Limit(1).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading the school's details", err)
		}
		if len(rows) == 0 {
			return nil
		}
		p, derr := toDomainSchoolProfile(rows[0])
		if derr != nil {
			return derr
		}
		out, found = p, true
		return nil
	})
	if err != nil {
		return domain.SchoolProfile{}, false, err
	}
	return out, found, nil
}

// UpsertSchoolProfile writes the school's one set of details, replacing every
// field: a field left empty on save is empty afterwards.
func (r *Repository) UpsertSchoolProfile(ctx context.Context, p domain.SchoolProfile) error {
	model := fromDomainSchoolProfile(p)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "legal_name", "rfc", "cct", "address", "phone", "legend", "updated_at"}),
		}).Create(&model).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "saving the school's details", err)
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
