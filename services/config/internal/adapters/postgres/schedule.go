package postgres

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// Compile-time proof the adapter satisfies the timetable port.
var _ ports.ScheduleRepository = (*Repository)(nil)

// scheduleAssignmentFK is 0017's composite FK: a class names its assignment
// as a whole, so an assignment freed or changed meanwhile trips it.
const scheduleAssignmentFK = "schedule_blocks_assignment_fk"

// scheduleLockKey namespaces the cycle's timetable lock among the database's
// advisory locks (hashed with the cycle id).
const scheduleLockKey = "config.schedule:"

// --- models (0017) ------------------------------------------------------------

type scheduleSettingsModel struct {
	TenantID  string    `gorm:"column:tenant_id;primaryKey"`
	LevelID   string    `gorm:"column:level_id;primaryKey"`
	GridStart int       `gorm:"column:grid_start"`
	GridEnd   int       `gorm:"column:grid_end"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (scheduleSettingsModel) TableName() string { return "schedule_settings" }

type scheduleBlockModel struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TenantID     string    `gorm:"column:tenant_id"`
	CycleID      string    `gorm:"column:cycle_id"`
	AssignmentID string    `gorm:"column:assignment_id"`
	GroupID      string    `gorm:"column:group_id"`
	SubjectID    string    `gorm:"column:subject_id"`
	TeacherID    string    `gorm:"column:teacher_id"`
	Weekday      int       `gorm:"column:weekday"`
	StartsAt     int       `gorm:"column:starts_at"`
	EndsAt       int       `gorm:"column:ends_at"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (scheduleBlockModel) TableName() string { return "schedule_blocks" }

type scheduleOverrideModel struct {
	ID            string    `gorm:"column:id;primaryKey"`
	TenantID      string    `gorm:"column:tenant_id"`
	CycleID       string    `gorm:"column:cycle_id"`
	BlockID       string    `gorm:"column:block_id"`
	OnDate        time.Time `gorm:"column:on_date;type:date"`
	Kind          string    `gorm:"column:kind"`
	TeacherID     *string   `gorm:"column:teacher_id"`
	SwapBlockID   *string   `gorm:"column:swap_block_id"`
	Reason        string    `gorm:"column:reason"`
	CreatedBy     string    `gorm:"column:created_by"`
	CreatedByName string    `gorm:"column:created_by_name"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (scheduleOverrideModel) TableName() string { return "schedule_overrides" }

type scheduleEventModel struct {
	ID         string     `gorm:"column:id;primaryKey"`
	TenantID   string     `gorm:"column:tenant_id"`
	CycleID    string     `gorm:"column:cycle_id"`
	At         time.Time  `gorm:"column:at"`
	ActorID    string     `gorm:"column:actor_id"`
	ActorName  string     `gorm:"column:actor_name"`
	Action     string     `gorm:"column:action"`
	BlockID    *string    `gorm:"column:block_id"`
	OverrideID *string    `gorm:"column:override_id"`
	AbsenceID  *string    `gorm:"column:absence_id"`
	OnDate     *time.Time `gorm:"column:on_date;type:date"`
	Reason     string     `gorm:"column:reason"`
	Summary    string     `gorm:"column:summary"`
}

func (scheduleEventModel) TableName() string { return "schedule_events" }

func fromDomainBlock(b domain.ScheduleBlock) scheduleBlockModel {
	return scheduleBlockModel{
		ID: b.ID(), TenantID: b.TenantID(), CycleID: b.CycleID(), AssignmentID: b.AssignmentID(),
		GroupID: b.GroupID(), SubjectID: b.SubjectID(), TeacherID: b.TeacherID(), Weekday: b.Weekday(),
		StartsAt: b.Time().Start(), EndsAt: b.Time().End(),
	}
}

func toDomainBlock(m scheduleBlockModel) (domain.ScheduleBlock, error) {
	t, err := domain.NewClassTime(m.StartsAt, m.EndsAt)
	if err != nil {
		return domain.ScheduleBlock{}, err
	}
	return domain.NewScheduleBlock(m.ID, m.TenantID, m.CycleID, m.AssignmentID, m.GroupID, m.SubjectID, m.TeacherID, m.Weekday, t)
}

func fromDomainOverride(o domain.ScheduleOverride) scheduleOverrideModel {
	return scheduleOverrideModel{
		ID: o.ID(), TenantID: o.TenantID(), CycleID: o.CycleID(), BlockID: o.BlockID(), OnDate: o.Date(),
		Kind: string(o.Kind()), TeacherID: nullable(o.TeacherID()), SwapBlockID: nullable(o.SwapBlockID()),
		Reason: o.Reason(), CreatedBy: o.By().UserID, CreatedByName: o.By().Name, CreatedAt: o.CreatedAt(),
	}
}

func toDomainOverride(m scheduleOverrideModel) (domain.ScheduleOverride, error) {
	return domain.NewScheduleOverride(m.ID, m.TenantID, m.CycleID, m.BlockID, m.OnDate.UTC(), domain.OverrideKind(m.Kind),
		deref(m.TeacherID), deref(m.SwapBlockID), m.Reason, domain.Actor{UserID: m.CreatedBy, Name: m.CreatedByName}, m.CreatedAt.UTC())
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// --- reads ----------------------------------------------------------------------

// GetScheduleBlock returns a class by id, or KindNotFound.
func (r *Repository) GetScheduleBlock(ctx context.Context, id string) (domain.ScheduleBlock, error) {
	var out domain.ScheduleBlock
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = getBlock(tx, id)
		return err
	})
	return out, err
}

// GetScheduleOverride returns a date-specific change by id, or KindNotFound.
func (r *Repository) GetScheduleOverride(ctx context.Context, id string) (domain.ScheduleOverride, error) {
	var out domain.ScheduleOverride
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = getOverride(tx, id)
		return err
	})
	return out, err
}

// LoadScheduleCatalog reads, in one transaction, everything the timetable of
// cycleID is drawn and named with (see ports.ScheduleCatalog). Every list is
// bounded like every list read.
func (r *Repository) LoadScheduleCatalog(ctx context.Context, cycleID string) (ports.ScheduleCatalog, error) {
	if !isUUID(cycleID) {
		return ports.ScheduleCatalog{}, glerrors.ErrNotFound
	}
	var cat ports.ScheduleCatalog
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var cm cycleModel
		if err := tx.Where("id = ?", cycleID).First(&cm).Error; err != nil {
			return translateFindErr(err)
		}
		c, err := toDomainCycle(cm)
		if err != nil {
			return err
		}
		cat.Cycle = c
		return loadCatalogLists(tx, cycleID, &cat)
	})
	if err != nil {
		return ports.ScheduleCatalog{}, err
	}
	return cat, nil
}

func loadCatalogLists(tx *gorm.DB, cycleID string, cat *ports.ScheduleCatalog) error {
	var err error
	if cat.Levels, err = findMapped(tx.Order("name, id"), toDomainLevel); err != nil {
		return err
	}
	if cat.Grades, err = findMapped(tx.Order("level_id, ordinal, id"), toDomainGradeLevel); err != nil {
		return err
	}
	if cat.Groups, err = findMapped(tx.Where("cycle_id = ?", cycleID).Order("grade_level_id, name, id"), toDomainGroup); err != nil {
		return err
	}
	if cat.Subjects, err = findMapped(tx.Where("grade_level_id IN (?)",
		tx.Model(&groupModel{}).Select("grade_level_id").Where("cycle_id = ?", cycleID)).Order("grade_level_id, name, id"), toDomainSubject); err != nil {
		return err
	}
	if cat.Teachers, err = findMapped(tx.Order("last_name, first_name, id"), toDomainTeacher); err != nil {
		return err
	}
	if cat.Assignments, err = findMapped(tx.Where("cycle_id = ?", cycleID).Order("group_id, subject_id"), toDomainTeachingAssignment); err != nil {
		return err
	}
	cat.Grids, err = findMapped(tx.Order("level_id"), func(m scheduleSettingsModel) (domain.ScheduleGrid, error) {
		return domain.NewScheduleGrid(m.LevelID, m.GridStart, m.GridEnd)
	})
	return err
}

// findMapped runs q over M's table (bounded by maxScheduleRows) and maps each
// row to the domain.
func findMapped[M any, D any](q *gorm.DB, toDomain func(M) (D, error)) ([]D, error) {
	var rows []M
	if err := q.Limit(maxScheduleRows).Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the timetable catalog", err)
	}
	out := make([]D, 0, len(rows))
	for _, m := range rows {
		d, err := toDomain(m)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// maxScheduleRows bounds every timetable read: a school of a few dozen groups
// has a few hundred classes a week.
const maxScheduleRows = 5000

// ListScheduleBlocks returns the classes of cycleID by weekday and start.
func (r *Repository) ListScheduleBlocks(ctx context.Context, cycleID string) ([]domain.ScheduleBlock, error) {
	if !isUUID(cycleID) {
		return []domain.ScheduleBlock{}, nil
	}
	var out []domain.ScheduleBlock
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = blocksOf(tx, cycleID)
		return err
	})
	return out, err
}

// ListScheduleOverrides returns the changes of cycleID dated from..to.
func (r *Repository) ListScheduleOverrides(ctx context.Context, cycleID string, from, to time.Time) ([]domain.ScheduleOverride, error) {
	if !isUUID(cycleID) {
		return []domain.ScheduleOverride{}, nil
	}
	var out []domain.ScheduleOverride
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = findMapped(tx.Where("cycle_id = ? AND on_date BETWEEN ? AND ?", cycleID, dateOnly(from), dateOnly(to)).
			Order("on_date, created_at, id"), toDomainOverride)
		return err
	})
	return out, err
}

// ListScheduleEvents returns the history of cycleID, newest first.
func (r *Repository) ListScheduleEvents(ctx context.Context, cycleID string, limit int) ([]domain.ScheduleEvent, error) {
	if !isUUID(cycleID) {
		return []domain.ScheduleEvent{}, nil
	}
	var out []domain.ScheduleEvent
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []scheduleEventModel
		if err := tx.Where("cycle_id = ?", cycleID).Order("at DESC, id").Limit(limit).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing the timetable history", err)
		}
		out = make([]domain.ScheduleEvent, 0, len(rows))
		for _, m := range rows {
			e := domain.ScheduleEvent{
				ID: m.ID, TenantID: m.TenantID, CycleID: m.CycleID, At: m.At.UTC(),
				By: domain.Actor{UserID: m.ActorID, Name: m.ActorName}, Action: domain.ScheduleAction(m.Action),
				BlockID: deref(m.BlockID), OverrideID: deref(m.OverrideID), AbsenceID: deref(m.AbsenceID), Reason: m.Reason, Summary: m.Summary,
			}
			if m.OnDate != nil {
				e.Date = m.OnDate.UTC()
			}
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

// SaveScheduleGrid upserts a level's grid.
func (r *Repository) SaveScheduleGrid(ctx context.Context, g domain.ScheduleGrid) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		m := scheduleSettingsModel{TenantID: r.tenantID, LevelID: g.LevelID(), GridStart: g.Start(), GridEnd: g.End(), UpdatedAt: time.Now().UTC()}
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "level_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"grid_start", "grid_end", "updated_at"}),
		}).Create(&m).Error
		return translateCreateErr(err, "saving the timetable grid", "grid conflict")
	})
}

// --- the locked unit of work ---------------------------------------------------

// RunScheduleTx runs fn in one transaction holding the cycle's timetable lock.
func (r *Repository) RunScheduleTx(ctx context.Context, cycleID string, fn func(ports.ScheduleTx) error) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", scheduleLockKey+cycleID).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "locking the timetable", err)
		}
		return fn(scheduleTx{tx: tx})
	})
}

// scheduleTx is ports.ScheduleTx over one GORM transaction.
type scheduleTx struct{ tx *gorm.DB }

func (s scheduleTx) Blocks(cycleID string) ([]domain.ScheduleBlock, error) {
	return blocksOf(s.tx, cycleID)
}

func (s scheduleTx) OverridesFrom(cycleID string, from time.Time) ([]domain.ScheduleOverride, error) {
	return findMapped(s.tx.Where("cycle_id = ? AND on_date >= ?", cycleID, dateOnly(from)).Order("on_date, created_at, id"), toDomainOverride)
}

func (s scheduleTx) OverridesOn(cycleID string, date time.Time) ([]domain.ScheduleOverride, error) {
	return findMapped(s.tx.Where("cycle_id = ? AND on_date = ?", cycleID, dateOnly(date)).Order("created_at, id"), toDomainOverride)
}

func (s scheduleTx) GetBlock(id string) (domain.ScheduleBlock, error) { return getBlock(s.tx, id) }

func (s scheduleTx) GetOverride(id string) (domain.ScheduleOverride, error) {
	return getOverride(s.tx, id)
}

func (s scheduleTx) InsertBlock(b domain.ScheduleBlock) error {
	m := fromDomainBlock(b)
	now := time.Now().UTC()
	m.CreatedAt, m.UpdatedAt = now, now
	return translateScheduleErr(s.tx.Create(&m).Error, "adding a class")
}

func (s scheduleTx) UpdateBlock(b domain.ScheduleBlock) error {
	m := fromDomainBlock(b)
	res := s.tx.Model(&scheduleBlockModel{}).Where("id = ?", b.ID()).Updates(map[string]any{
		"assignment_id": m.AssignmentID, "group_id": m.GroupID, "subject_id": m.SubjectID, "teacher_id": m.TeacherID,
		"weekday": m.Weekday, "starts_at": m.StartsAt, "ends_at": m.EndsAt, "updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return translateScheduleErr(res.Error, "changing a class")
	}
	if res.RowsAffected == 0 {
		return glerrors.ErrNotFound
	}
	return nil
}

func (s scheduleTx) DeleteBlock(id string) error {
	res := s.tx.Where("id = ?", id).Delete(&scheduleBlockModel{})
	if res.Error != nil {
		return glerrors.Wrap(glerrors.KindInternal, "removing a class", res.Error)
	}
	if res.RowsAffected == 0 {
		return glerrors.ErrNotFound
	}
	return nil
}

func (s scheduleTx) InsertOverride(o domain.ScheduleOverride) error {
	m := fromDomainOverride(o)
	return translateScheduleErr(s.tx.Create(&m).Error, "recording a change")
}

func (s scheduleTx) DeleteOverride(id string) error {
	res := s.tx.Where("id = ?", id).Delete(&scheduleOverrideModel{})
	if res.Error != nil {
		return glerrors.Wrap(glerrors.KindInternal, "undoing a change", res.Error)
	}
	if res.RowsAffected == 0 {
		return glerrors.ErrNotFound
	}
	return nil
}

func (s scheduleTx) AppendEvent(e domain.ScheduleEvent) error {
	m := scheduleEventModel{
		ID: e.ID, TenantID: e.TenantID, CycleID: e.CycleID, At: e.At, ActorID: e.By.UserID, ActorName: e.By.Name,
		Action: string(e.Action), BlockID: nullable(e.BlockID), OverrideID: nullable(e.OverrideID), AbsenceID: nullable(e.AbsenceID),
		Reason: e.Reason, Summary: e.Summary,
	}
	if !e.Date.IsZero() {
		d := dateOnly(e.Date)
		m.OnDate = &d
	}
	if err := s.tx.Create(&m).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "recording the timetable history", err)
	}
	return nil
}

// --- helpers ----------------------------------------------------------------------

func blocksOf(tx *gorm.DB, cycleID string) ([]domain.ScheduleBlock, error) {
	return findMapped(tx.Where("cycle_id = ?", cycleID).Order("weekday, starts_at, group_id, id"), toDomainBlock)
}

func getBlock(tx *gorm.DB, id string) (domain.ScheduleBlock, error) {
	if !isUUID(id) {
		return domain.ScheduleBlock{}, glerrors.New(glerrors.KindNotFound, "class not found")
	}
	var m scheduleBlockModel
	if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ScheduleBlock{}, glerrors.New(glerrors.KindNotFound, "class not found")
		}
		return domain.ScheduleBlock{}, translateFindErr(err)
	}
	return toDomainBlock(m)
}

func getOverride(tx *gorm.DB, id string) (domain.ScheduleOverride, error) {
	if !isUUID(id) {
		return domain.ScheduleOverride{}, glerrors.New(glerrors.KindNotFound, "change not found")
	}
	var m scheduleOverrideModel
	if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ScheduleOverride{}, glerrors.New(glerrors.KindNotFound, "change not found")
		}
		return domain.ScheduleOverride{}, translateFindErr(err)
	}
	return toDomainOverride(m)
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// translateScheduleErr maps what the database refuses on a timetable write.
// The use case checks every rule under the cycle lock first, so these only
// fire when something outside that lock changed meanwhile (an assignment
// freed or reassigned): an EXCLUDE or a unique key is SCHEDULE_CHANGED, the
// assignment FK is SCHEDULE_ASSIGNMENT_INVALID.
func translateScheduleErr(err error, internalMsg string) error {
	if err == nil {
		return nil
	}
	switch {
	case pgCode(err) == exclusionViolation, pgCode(err) == uniqueViolation:
		return domain.ScheduleChangedMeanwhile()
	case isConstraint(err, scheduleAssignmentFK):
		return domain.ScheduleRefusal(glerrors.KindInvalid, domain.CodeScheduleAssignmentInvalid,
			"Esa asignación ya no existe o cambió; recarga el horario")
	}
	return translateCreateErr(err, internalMsg, "conflict")
}
