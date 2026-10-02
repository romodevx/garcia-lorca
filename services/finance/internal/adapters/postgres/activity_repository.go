package postgres

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// T17 (migration 0025): activities, their price options and enrolments. Same
// Repository, same tenant binding, its own port.

var _ ports.ActivityRepository = (*Repository)(nil)

const (
	errActivityNameTaken = "ya existe una actividad con ese nombre"
	errOptionInUse       = "un esquema de cobro que quitaste lo usa un alumno inscrito; cámbialo antes de quitarlo"
	// maxActivityEnrollmentRows bounds the billing read: a school of thousands
	// of pupils in a dozen activities, with room.
	maxActivityEnrollmentRows = 20000
)

// scheduleSlotJSON is one entry of activities.schedule.
type scheduleSlotJSON struct {
	Weekday int    `json:"weekday"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

// CreateActivity stores an activity and its options in one transaction.
func (r *Repository) CreateActivity(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	m, err := r.fromDomainActivity(a)
	if err != nil {
		return domain.Activity{}, err
	}
	err = r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return activityWriteErr(err, "creating an activity")
		}
		return r.writeOptions(tx, a)
	})
	if err != nil {
		return domain.Activity{}, err
	}
	return a, nil
}

// UpdateActivity rewrites the activity and its live options, retiring those
// left out — unless an open enrolment still uses one (KindConflict).
func (r *Repository) UpdateActivity(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	m, err := r.fromDomainActivity(a)
	if err != nil {
		return domain.Activity{}, err
	}
	err = r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&activityModel{}).Where("id = ?", a.ID()).Updates(map[string]any{
			"name": m.Name, "schedule": m.Schedule, "periodicity": m.Periodicity,
			"billing_starts_on": m.BillingStartsOn, "first_due_on": m.FirstDueOn,
			"late_fee_cents": m.LateFeeCents, "active": m.Active, "updated_at": gorm.Expr("now()"),
		})
		if res.Error != nil {
			return activityWriteErr(res.Error, "updating an activity")
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		if err := retireMissingOptions(tx, a); err != nil {
			return err
		}
		return r.writeOptions(tx, a)
	})
	if err != nil {
		return domain.Activity{}, err
	}
	return a, nil
}

// retireMissingOptions retires the live options the activity no longer lists,
// refusing when an open enrolment uses one. It runs BEFORE the live options are
// written, so a retired 2-day option and its new replacement never collide on
// the live-days unique index.
func retireMissingOptions(tx *gorm.DB, a domain.Activity) error {
	keep := make([]string, 0, len(a.Options()))
	for _, o := range a.Options() {
		keep = append(keep, o.ID)
	}
	var inUse int64
	if err := tx.Model(&activityEnrollmentModel{}).
		Where("activity_id = ? AND option_id NOT IN ? AND ends_on IS NULL", a.ID(), keep).
		Count(&inUse).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "checking options in use", err)
	}
	if inUse > 0 {
		return glerrors.New(glerrors.KindConflict, errOptionInUse)
	}
	if err := tx.Model(&activityOptionModel{}).
		Where("activity_id = ? AND id NOT IN ? AND NOT retired", a.ID(), keep).
		Updates(map[string]any{"retired": true, "updated_at": gorm.Expr("now()")}).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "retiring options", err)
	}
	return nil
}

// writeOptions upserts the activity's live options. The upsert lists EVERY
// column it may change (checklist #1: DoUpdates), and un-retires an option
// sent back.
func (r *Repository) writeOptions(tx *gorm.DB, a domain.Activity) error {
	rows := make([]activityOptionModel, 0, len(a.Options()))
	for _, o := range a.Options() {
		rows = append(rows, activityOptionModel{
			ID: o.ID, TenantID: r.tenantID, ActivityID: a.ID(), DaysPerWeek: daysColumn(o), AmountCents: o.AmountCents,
		})
	}
	err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"days_per_week": gorm.Expr("excluded.days_per_week"),
			"amount_cents":  gorm.Expr("excluded.amount_cents"),
			"retired":       false,
			"updated_at":    gorm.Expr("now()"),
		}),
	}).Create(&rows).Error
	if err != nil {
		return activityWriteErr(err, "writing price options")
	}
	return nil
}

func daysColumn(o domain.PriceOption) *int {
	if o.Flat() {
		return nil
	}
	d := o.DaysPerWeek
	return &d
}

// activityWriteErr maps a unique violation (the name, or two live options for
// the same days) to KindConflict.
func activityWriteErr(err error, what string) error {
	if isUniqueViolation(err) {
		return glerrors.Wrap(glerrors.KindConflict, errActivityNameTaken, err)
	}
	return glerrors.Wrap(glerrors.KindInternal, what, err)
}

// GetActivity reads one activity with its live options.
func (r *Repository) GetActivity(ctx context.Context, id string) (domain.Activity, error) {
	if !uuidShaped.MatchString(id) {
		return domain.Activity{}, glerrors.ErrNotFound
	}
	var out domain.Activity
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m activityModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		options, err := liveOptions(tx, []string{id})
		if err != nil {
			return err
		}
		out, err = toDomainActivity(m, options[id])
		return err
	})
	return out, err
}

// ListActivities lists the activities by name, with their live options.
func (r *Repository) ListActivities(ctx context.Context, includeInactive bool) ([]domain.Activity, error) {
	var out []domain.Activity
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []activityModel
		q := tx.Model(&activityModel{}).Order("lower(name) ASC").Limit(maxListLimit)
		if !includeInactive {
			q = q.Where("active")
		}
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing activities", err)
		}
		ids := make([]string, len(rows))
		for i, m := range rows {
			ids[i] = m.ID
		}
		options, err := liveOptions(tx, ids)
		if err != nil {
			return err
		}
		out = make([]domain.Activity, 0, len(rows))
		for _, m := range rows {
			a, err := toDomainActivity(m, options[m.ID])
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return nil
	})
	return out, err
}

// liveOptions reads the live options of the activities, by activity, in one
// query (no N+1).
func liveOptions(tx *gorm.DB, activityIDs []string) (map[string][]domain.PriceOption, error) {
	out := map[string][]domain.PriceOption{}
	if len(activityIDs) == 0 {
		return out, nil
	}
	var rows []activityOptionModel
	if err := tx.Where("activity_id IN ? AND NOT retired", activityIDs).Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing price options", err)
	}
	for _, o := range rows {
		days := 0
		if o.DaysPerWeek != nil {
			days = *o.DaysPerWeek
		}
		out[o.ActivityID] = append(out[o.ActivityID], domain.PriceOption{ID: o.ID, DaysPerWeek: days, AmountCents: o.AmountCents})
	}
	return out, nil
}

// CountActiveEnrollments counts, per activity, the pupils enrolled on `on`.
func (r *Repository) CountActiveEnrollments(ctx context.Context, on time.Time) (map[string]int, error) {
	type row struct {
		ActivityID string
		N          int
	}
	var rows []row
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Model(&activityEnrollmentModel{}).
			Select("activity_id, count(*) AS n").
			Where("ends_on IS NULL OR ends_on > ?::date", on.Format(dateLayout)).
			Group("activity_id").Scan(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "counting enrolments", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.ActivityID] = r.N
	}
	return out, nil
}

// ListActivityEnrollments lists one activity's enrolments, oldest first.
func (r *Repository) ListActivityEnrollments(ctx context.Context, activityID string) ([]domain.ActivityEnrollment, error) {
	if !uuidShaped.MatchString(activityID) {
		return nil, nil
	}
	return r.listEnrollments(ctx, func(q *gorm.DB) *gorm.DB { return q.Where("activity_id = ?", activityID) })
}

// ListBillableEnrollments lists the enrolments of every active activity.
func (r *Repository) ListBillableEnrollments(ctx context.Context) ([]domain.ActivityEnrollment, error) {
	return r.listEnrollments(ctx, func(q *gorm.DB) *gorm.DB {
		return q.Where("activity_id IN (?)", q.Session(&gorm.Session{NewDB: true}).
			Model(&activityModel{}).Select("id").Where("active"))
	})
}

func (r *Repository) listEnrollments(ctx context.Context, filter func(*gorm.DB) *gorm.DB) ([]domain.ActivityEnrollment, error) {
	var out []domain.ActivityEnrollment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []activityEnrollmentModel
		q := filter(tx.Model(&activityEnrollmentModel{})).Order("starts_on ASC, id ASC").Limit(maxActivityEnrollmentRows)
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing enrolments", err)
		}
		out = make([]domain.ActivityEnrollment, 0, len(rows))
		for _, m := range rows {
			e, err := toDomainEnrollment(m)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

// CreateEnrollments inserts open enrolments, skipping a pupil who already has
// one (the partial unique index is the arbiter), and returns those written.
func (r *Repository) CreateEnrollments(ctx context.Context, es []domain.ActivityEnrollment) ([]domain.ActivityEnrollment, error) {
	var out []domain.ActivityEnrollment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, e := range es {
			m := activityEnrollmentModel{
				ID: e.ID(), TenantID: r.tenantID, ActivityID: e.ActivityID(), StudentID: e.StudentID(),
				OptionID: e.OptionID(), StartsOn: e.StartsOn(),
			}
			// TargetWhere as a raw literal, for the reason CreateChargesBulk
			// gives: a bound parameter cannot prove the partial index applies.
			res := tx.Clauses(clause.OnConflict{
				Columns:     []clause.Column{{Name: "tenant_id"}, {Name: "activity_id"}, {Name: "student_id"}},
				TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "ends_on IS NULL"}}},
				DoNothing:   true,
			}).Create(&m)
			if res.Error != nil {
				return glerrors.Wrap(glerrors.KindInvalid, "no se pudo inscribir: revisa el esquema de cobro", res.Error)
			}
			if res.RowsAffected == 1 {
				out = append(out, e)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// EndEnrollments ends the pupils' enrolments still running on `on`.
func (r *Repository) EndEnrollments(ctx context.Context, activityID string, studentIDs []string, on time.Time) (int, error) {
	if !uuidShaped.MatchString(activityID) {
		return 0, nil
	}
	ids := uuidsOnly(studentIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&activityEnrollmentModel{}).
			Where("activity_id = ? AND student_id IN ? AND (ends_on IS NULL OR ends_on > ?::date)", activityID, ids, on.Format(dateLayout)).
			Updates(map[string]any{"ends_on": on, "updated_at": gorm.Expr("now()")})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "ending enrolments", res.Error)
		}
		n = res.RowsAffected
		return nil
	})
	return int(n), err
}

// ChangeEnrollmentOption moves the pupil's enrolment running on `on` to
// another option of the same activity.
func (r *Repository) ChangeEnrollmentOption(ctx context.Context, activityID, studentID, optionID string, on time.Time) (domain.ActivityEnrollment, error) {
	if !uuidShaped.MatchString(activityID) || !uuidShaped.MatchString(studentID) || !uuidShaped.MatchString(optionID) {
		return domain.ActivityEnrollment{}, glerrors.ErrNotFound
	}
	var out domain.ActivityEnrollment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m activityEnrollmentModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("activity_id = ? AND student_id = ? AND (ends_on IS NULL OR ends_on > ?::date)", activityID, studentID, on.Format(dateLayout)).
			Order("starts_on DESC").First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		if err := tx.Model(&activityEnrollmentModel{}).Where("id = ?", m.ID).
			Updates(map[string]any{"option_id": optionID, "updated_at": gorm.Expr("now()")}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "changing the option", err)
		}
		m.OptionID = optionID
		var err error
		out, err = toDomainEnrollment(m)
		return err
	})
	return out, err
}

func uuidsOnly(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if uuidShaped.MatchString(id) {
			out = append(out, id)
		}
	}
	return out
}

// --- mapping ---------------------------------------------------------------

func (r *Repository) fromDomainActivity(a domain.Activity) (activityModel, error) {
	slots := make([]scheduleSlotJSON, 0, len(a.Schedule()))
	for _, s := range a.Schedule() {
		slots = append(slots, scheduleSlotJSON{Weekday: s.Weekday, Start: domain.FormatClock(s.StartMin), End: domain.FormatClock(s.EndMin)})
	}
	schedule, err := json.Marshal(slots)
	if err != nil {
		return activityModel{}, glerrors.Wrap(glerrors.KindInternal, "encoding the schedule", err)
	}
	return activityModel{
		ID: a.ID(), TenantID: r.tenantID, Name: a.Name(), Schedule: schedule, Periodicity: a.Periodicity(),
		BillingStartsOn: a.BillingStartsOn(), FirstDueOn: a.FirstDueOn(), LateFeeCents: a.LateFee().Int64(), Active: a.Active(),
	}, nil
}

func toDomainActivity(m activityModel, options []domain.PriceOption) (domain.Activity, error) {
	var slots []scheduleSlotJSON
	if err := json.Unmarshal(m.Schedule, &slots); err != nil {
		return domain.Activity{}, glerrors.Wrap(glerrors.KindInternal, "decoding the schedule", err)
	}
	schedule := make([]domain.ActivitySlot, 0, len(slots))
	for _, s := range slots {
		start, err := domain.ParseClock(s.Start)
		if err != nil {
			return domain.Activity{}, glerrors.Wrap(glerrors.KindInternal, "a stored schedule is malformed", err)
		}
		end, err := domain.ParseClock(s.End)
		if err != nil {
			return domain.Activity{}, glerrors.Wrap(glerrors.KindInternal, "a stored schedule is malformed", err)
		}
		schedule = append(schedule, domain.ActivitySlot{Weekday: s.Weekday, StartMin: start, EndMin: end})
	}
	return domain.NewActivity(domain.ActivitySpec{
		ID: m.ID, Name: m.Name, Schedule: schedule, Periodicity: m.Periodicity,
		BillingStartsOn: asDate(m.BillingStartsOn), FirstDueOn: asDate(m.FirstDueOn),
		LateFeeCents: m.LateFeeCents, Active: m.Active, Options: options,
	})
}

func toDomainEnrollment(m activityEnrollmentModel) (domain.ActivityEnrollment, error) {
	var ends time.Time
	if m.EndsOn != nil {
		ends = asDate(*m.EndsOn)
	}
	return domain.RestoreActivityEnrollment(m.ID, m.ActivityID, m.StudentID, m.OptionID, asDate(m.StartsOn), ends)
}

// asDate is a `date` column as the service's day: midnight UTC.
func asDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
