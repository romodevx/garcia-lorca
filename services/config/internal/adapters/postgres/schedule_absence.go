package postgres

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// teacherAbsenceModel is the GORM row for `teacher_absences` (0018, T19).
type teacherAbsenceModel struct {
	ID              string     `gorm:"column:id;primaryKey"`
	TenantID        string     `gorm:"column:tenant_id"`
	CycleID         string     `gorm:"column:cycle_id"`
	TeacherID       string     `gorm:"column:teacher_id"`
	Kind            string     `gorm:"column:kind"`
	StartsOn        time.Time  `gorm:"column:starts_on;type:date"`
	EndsOn          time.Time  `gorm:"column:ends_on;type:date"`
	StartMinute     *int       `gorm:"column:start_minute"`
	EndMinute       *int       `gorm:"column:end_minute"`
	Note            string     `gorm:"column:note"`
	CreatedBy       string     `gorm:"column:created_by"`
	CreatedByName   string     `gorm:"column:created_by_name"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
	CancelledAt     *time.Time `gorm:"column:cancelled_at"`
	CancelledBy     *string    `gorm:"column:cancelled_by"`
	CancelledByName *string    `gorm:"column:cancelled_by_name"`
}

func (teacherAbsenceModel) TableName() string { return "teacher_absences" }

func fromDomainAbsence(a domain.TeacherAbsence) teacherAbsenceModel {
	m := teacherAbsenceModel{
		ID: a.ID(), TenantID: a.TenantID(), CycleID: a.CycleID(), TeacherID: a.TeacherID(), Kind: string(a.Kind()),
		StartsOn: dateOnly(a.StartsOn()), EndsOn: dateOnly(a.EndsOn()), Note: a.Note(),
		CreatedBy: a.CreatedBy().UserID, CreatedByName: a.CreatedBy().Name, CreatedAt: a.CreatedAt(), UpdatedAt: a.UpdatedAt(),
	}
	if !a.FullDay() {
		start, end := a.Span().Start(), a.Span().End()
		m.StartMinute, m.EndMinute = &start, &end
	}
	if !a.Active() {
		at, by, name := a.CancelledAt(), a.CancelledBy().UserID, a.CancelledBy().Name
		m.CancelledAt, m.CancelledBy, m.CancelledByName = &at, &by, &name
	}
	return m
}

func toDomainAbsence(m teacherAbsenceModel) (domain.TeacherAbsence, error) {
	var span *domain.ClassTime
	if m.StartMinute != nil && m.EndMinute != nil {
		ct, err := domain.NewClassTime(*m.StartMinute, *m.EndMinute)
		if err != nil {
			return domain.TeacherAbsence{}, err
		}
		span = &ct
	}
	var cancelledAt time.Time
	var cancelledBy domain.Actor
	if m.CancelledAt != nil {
		cancelledAt = m.CancelledAt.UTC()
		cancelledBy = domain.Actor{UserID: deref(m.CancelledBy), Name: deref(m.CancelledByName)}
	}
	return domain.RestoreTeacherAbsence(domain.AbsenceFields{
		ID: m.ID, TenantID: m.TenantID, CycleID: m.CycleID, TeacherID: m.TeacherID, Kind: domain.AbsenceKind(m.Kind),
		StartsOn: m.StartsOn.UTC(), EndsOn: m.EndsOn.UTC(), Span: span, Note: m.Note,
	}, domain.Actor{UserID: m.CreatedBy, Name: m.CreatedByName}, m.CreatedAt.UTC(), m.UpdatedAt.UTC(), cancelledBy, cancelledAt)
}

// GetTeacherAbsence returns an absence by id, or KindNotFound.
func (r *Repository) GetTeacherAbsence(ctx context.Context, id string) (domain.TeacherAbsence, error) {
	var out domain.TeacherAbsence
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = getAbsence(tx, id)
		return err
	})
	return out, err
}

// ListTeacherAbsences returns the absences of cycleID (cancelled ones too)
// touching from..to, by start date.
func (r *Repository) ListTeacherAbsences(ctx context.Context, cycleID string, from, to time.Time) ([]domain.TeacherAbsence, error) {
	if !isUUID(cycleID) {
		return []domain.TeacherAbsence{}, nil
	}
	var out []domain.TeacherAbsence
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = findMapped(absencesTouching(tx, cycleID, from, to), toDomainAbsence)
		return err
	})
	return out, err
}

func absencesTouching(tx *gorm.DB, cycleID string, from, to time.Time) *gorm.DB {
	return tx.Where("cycle_id = ? AND starts_on <= ? AND ends_on >= ?", cycleID, dateOnly(to), dateOnly(from)).
		Order("starts_on, created_at, id")
}

func getAbsence(tx *gorm.DB, id string) (domain.TeacherAbsence, error) {
	if !isUUID(id) {
		return domain.TeacherAbsence{}, glerrors.New(glerrors.KindNotFound, "absence not found")
	}
	var m teacherAbsenceModel
	if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.TeacherAbsence{}, glerrors.New(glerrors.KindNotFound, "absence not found")
		}
		return domain.TeacherAbsence{}, translateFindErr(err)
	}
	return toDomainAbsence(m)
}

// --- inside the timetable lock --------------------------------------------------------

func (s scheduleTx) GetAbsence(id string) (domain.TeacherAbsence, error) { return getAbsence(s.tx, id) }

func (s scheduleTx) ActiveAbsences(cycleID string, from, to time.Time) ([]domain.TeacherAbsence, error) {
	return findMapped(absencesTouching(s.tx.Where("cancelled_at IS NULL"), cycleID, from, to), toDomainAbsence)
}

func (s scheduleTx) InsertAbsence(a domain.TeacherAbsence) error {
	m := fromDomainAbsence(a)
	if err := s.tx.Create(&m).Error; err != nil {
		return translateCreateErr(err, "registering an absence", "absence conflict")
	}
	return nil
}

func (s scheduleTx) UpdateAbsence(a domain.TeacherAbsence) error {
	m := fromDomainAbsence(a)
	res := s.tx.Model(&teacherAbsenceModel{}).Where("id = ?", a.ID()).Updates(map[string]any{
		"kind": m.Kind, "starts_on": m.StartsOn, "ends_on": m.EndsOn, "start_minute": m.StartMinute, "end_minute": m.EndMinute,
		"note": m.Note, "updated_at": m.UpdatedAt, "cancelled_at": m.CancelledAt, "cancelled_by": m.CancelledBy,
		"cancelled_by_name": m.CancelledByName,
	})
	if res.Error != nil {
		return glerrors.Wrap(glerrors.KindInternal, "changing an absence", res.Error)
	}
	if res.RowsAffected == 0 {
		return glerrors.ErrNotFound
	}
	return nil
}
