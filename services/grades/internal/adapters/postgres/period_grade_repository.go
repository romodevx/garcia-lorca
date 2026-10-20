package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter satisfies the period-grade port.
var _ ports.PeriodGradeRepository = (*Repository)(nil)

// UpsertPeriodGrade inserts the cell or, on the (tenant, enrollment, subject,
// period) key, updates value, captured_by(_name) and updated_at — every column
// a correction can change. The stored row is re-read in the same tenant-bound
// transaction, so the caller gets the canonical id and updated_at. A lock
// naming a closed term refuses the write with PERIOD_LOCKED.
func (r *Repository) UpsertPeriodGrade(ctx context.Context, g domain.PeriodGrade, lock ports.PeriodLock) (domain.PeriodGrade, error) {
	model := fromDomainPeriodGrade(g)
	var out domain.PeriodGrade
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := r.guardPeriod(tx, lock, domain.CodePeriodLocked); err != nil {
			return err
		}
		res := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "enrollment_id"}, {Name: "subject_id"}, {Name: "period_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"value":            model.Value,
				"letter":           model.Letter,
				"captured_by":      model.CapturedBy,
				"captured_by_name": model.CapturedByName,
				"updated_at":       gorm.Expr("now()"),
			}),
		}).Create(&model)
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "upserting period grade", res.Error)
		}
		var stored periodGradeModel
		if err := tx.Where("enrollment_id = ? AND subject_id = ? AND period_id = ?",
			model.EnrollmentID, model.SubjectID, model.PeriodID).First(&stored).Error; err != nil {
			return translateFindErr(err)
		}
		d, err := toDomainPeriodGrade(stored)
		out = d
		return err
	})
	if err != nil {
		return domain.PeriodGrade{}, err
	}
	return out, nil
}

// DeletePeriodGrade removes the cell on the key. No row — never captured, or
// hidden by RLS — is not an error. A lock naming a closed term refuses it.
func (r *Repository) DeletePeriodGrade(ctx context.Context, enrollmentID, subjectID, periodID string, lock ports.PeriodLock) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := r.guardPeriod(tx, lock, domain.CodePeriodLocked); err != nil {
			return err
		}
		if err := tx.Where("enrollment_id = ? AND subject_id = ? AND period_id = ?",
			enrollmentID, subjectID, periodID).Delete(&periodGradeModel{}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting period grade", err)
		}
		return nil
	})
}

// PeriodGradesForSheet batch-loads, in ONE query, the grades of many
// enrollments for one subject and period, keyed by enrollment id.
func (r *Repository) PeriodGradesForSheet(ctx context.Context, enrollmentIDs []string, subjectID, periodID string) (map[string]domain.PeriodGrade, error) {
	out := make(map[string]domain.PeriodGrade, len(enrollmentIDs))
	if len(enrollmentIDs) == 0 {
		return out, nil
	}
	var rows []periodGradeModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Where("subject_id = ? AND period_id = ? AND enrollment_id IN ?", subjectID, periodID, enrollmentIDs).
			Find(&rows).Error
	})
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "loading period sheet", err)
	}
	for _, m := range rows {
		d, err := toDomainPeriodGrade(m)
		if err != nil {
			return nil, err
		}
		out[m.EnrollmentID] = d
	}
	return out, nil
}

// PeriodGradesForEnrollment lists every period grade of one enrollment, in a
// stable order (period, then subject).
func (r *Repository) PeriodGradesForEnrollment(ctx context.Context, enrollmentID string) ([]domain.PeriodGrade, error) {
	var rows []periodGradeModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Where("enrollment_id = ?", enrollmentID).Order("period_id, subject_id").Find(&rows).Error
	})
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing period grades", err)
	}
	return toDomainPeriodGrades(rows)
}

// PeriodGradesForEnrollments lists the grades of these enrollments in these
// periods in ONE query.
func (r *Repository) PeriodGradesForEnrollments(ctx context.Context, enrollmentIDs, periodIDs []string) ([]domain.PeriodGrade, error) {
	var out []domain.PeriodGrade
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = periodGradesIn(tx, enrollmentIDs, periodIDs)
		return err
	})
	return out, err
}

func periodGradesIn(tx *gorm.DB, enrollmentIDs, periodIDs []string) ([]domain.PeriodGrade, error) {
	if len(enrollmentIDs) == 0 || len(periodIDs) == 0 {
		return []domain.PeriodGrade{}, nil
	}
	var rows []periodGradeModel
	if err := tx.Where("enrollment_id IN ? AND period_id IN ?", enrollmentIDs, periodIDs).
		Order("enrollment_id, subject_id, period_id").Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading period grades", err)
	}
	return toDomainPeriodGrades(rows)
}

// PeriodsWithGrades answers, in ONE query, which of periodIDs hold a grade.
func (r *Repository) PeriodsWithGrades(ctx context.Context, periodIDs []string) ([]string, error) {
	out := []string{}
	if len(periodIDs) == 0 {
		return out, nil
	}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Model(&periodGradeModel{}).Distinct("period_id").Where("period_id IN ?", periodIDs).
			Order("period_id").Pluck("period_id", &out).Error
	})
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the periods with grades", err)
	}
	return out, nil
}

// CountScaleViolations counts, in ONE tenant-bound query, the period grades of
// the named enrollments that check refuses: out of range (bounds travel as
// decimal text cast to numeric — no float) or a letter, for a numeric check;
// every number, for a switch to letters; everything, for no grades. Period grades are whole numbers,
// so a scale's decimals can never refuse one.
func (r *Repository) CountScaleViolations(ctx context.Context, enrollmentIDs []string, check ports.ScaleCheck) (int, error) {
	if len(enrollmentIDs) == 0 {
		return 0, nil
	}
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&periodGradeModel{}).Where("enrollment_id IN ?", enrollmentIDs)
		switch {
		case check.GradingEnabled:
			s := check.Scale
			q = q.Where("(letter IS NOT NULL OR value < ?::numeric OR value > ?::numeric)", hundredthsText(s.MinHundredths()), hundredthsText(s.MaxHundredths()))
		case check.GradingLetters:
			q = q.Where("letter IS NULL")
		}
		if err := q.Count(&n).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "counting scale violations", err)
		}
		return nil
	})
	return int(n), err
}

// hundredthsText prints hundredths as exact decimal text for a numeric value
// ("9.00", "8.66").
func hundredthsText(h int) string {
	return fmt.Sprintf("%d.%02d", h/hundredthsPerUnit, h%hundredthsPerUnit)
}

// hundredthsPerUnit converts between marks and their hundredths.
const hundredthsPerUnit = 100

func fromDomainPeriodGrade(g domain.PeriodGrade) periodGradeModel {
	m := periodGradeModel{
		ID: g.ID(), TenantID: g.TenantID(), EnrollmentID: g.EnrollmentID(), SubjectID: g.SubjectID(),
		PeriodID: g.PeriodID(), CapturedBy: g.CapturedBy(), CapturedByName: g.CapturedByName(),
	}
	if g.IsLetter() {
		letter := string(g.Letter())
		m.Letter = &letter
	} else {
		value := hundredthsText(g.Value().Hundredths())
		m.Value = &value
	}
	return m
}

func toDomainPeriodGrade(m periodGradeModel) (domain.PeriodGrade, error) {
	fields := domain.PeriodGradeFields{
		ID: m.ID, TenantID: m.TenantID, EnrollmentID: m.EnrollmentID, SubjectID: m.SubjectID, PeriodID: m.PeriodID,
		CapturedBy: m.CapturedBy, CapturedByName: m.CapturedByName, UpdatedAt: m.UpdatedAt,
	}
	switch {
	case m.Letter != nil:
		letter, err := domain.ParseLetter(*m.Letter)
		if err != nil {
			return domain.PeriodGrade{}, glerrors.Wrap(glerrors.KindInternal, "invalid period grade letter", err)
		}
		fields.Letter = letter
	case m.Value != nil:
		h, err := hundredthsOf(*m.Value)
		if err != nil {
			return domain.PeriodGrade{}, err
		}
		fields.Value = domain.GradeValueFromHundredths(h)
	default:
		return domain.PeriodGrade{}, glerrors.New(glerrors.KindInternal, "period grade row has neither value nor letter")
	}
	g, err := domain.NewPeriodGrade(fields)
	if err != nil {
		return domain.PeriodGrade{}, glerrors.Wrap(glerrors.KindInternal, "invalid period grade row", err)
	}
	return g, nil
}

func toDomainPeriodGrades(rows []periodGradeModel) ([]domain.PeriodGrade, error) {
	out := make([]domain.PeriodGrade, 0, len(rows))
	for _, m := range rows {
		g, err := toDomainPeriodGrade(m)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}
