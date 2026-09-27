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

// Compile-time proof the adapter satisfies the monthly port.
var _ ports.MonthlyGradeRepository = (*Repository)(nil)

// UpsertMonthlyGrade inserts the cell or, on the (tenant, enrollment, subject,
// month) key, updates value, teacher_id, captured_by and updated_at — every
// column a correction can change. The stored row is re-read in the same
// tenant-bound transaction, so the caller gets the canonical id and updated_at
// even when the upsert hit an existing row. A lock naming a closed term refuses the
// write with MONTH_LOCKED (see guardPeriod).
func (r *Repository) UpsertMonthlyGrade(ctx context.Context, g domain.MonthlyGrade, lock ports.PeriodLock) (domain.MonthlyGrade, error) {
	model := fromDomainMonthly(g)
	var out domain.MonthlyGrade
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := r.guardPeriod(tx, lock); err != nil {
			return err
		}
		res := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "enrollment_id"}, {Name: "subject_id"}, {Name: "month"}},
			DoUpdates: clause.Assignments(map[string]any{
				"value":       model.Value,
				"teacher_id":  model.TeacherID,
				"captured_by": model.CapturedBy,
				"updated_at":  gorm.Expr("now()"),
			}),
		}).Create(&model)
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "upserting monthly grade", res.Error)
		}
		var stored monthlyGradeModel
		if err := tx.Where("enrollment_id = ? AND subject_id = ? AND month = ?",
			model.EnrollmentID, model.SubjectID, model.Month).First(&stored).Error; err != nil {
			return translateFindErr(err)
		}
		d, err := toDomainMonthly(stored)
		out = d
		return err
	})
	if err != nil {
		return domain.MonthlyGrade{}, err
	}
	return out, nil
}

// DeleteMonthlyGrade removes the cell on the key. No row — never captured, or
// hidden by RLS — is not an error: clearing an empty cell succeeds. A lock
// naming a closed term refuses it with MONTH_LOCKED.
func (r *Repository) DeleteMonthlyGrade(ctx context.Context, enrollmentID, subjectID string, month domain.Month, lock ports.PeriodLock) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := r.guardPeriod(tx, lock); err != nil {
			return err
		}
		if err := tx.Where("enrollment_id = ? AND subject_id = ? AND month = ?",
			enrollmentID, subjectID, month.FirstDay()).Delete(&monthlyGradeModel{}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting monthly grade", err)
		}
		return nil
	})
}

// MonthlyGradesForSheet batch-loads, in ONE query, the grades of many
// enrollments for one subject and month, keyed by enrollment id. Enrollments
// without a grade are absent. Empty enrollmentIDs returns without querying.
func (r *Repository) MonthlyGradesForSheet(ctx context.Context, enrollmentIDs []string, subjectID string, month domain.Month) (map[string]domain.MonthlyGrade, error) {
	out := make(map[string]domain.MonthlyGrade, len(enrollmentIDs))
	if len(enrollmentIDs) == 0 {
		return out, nil
	}
	var rows []monthlyGradeModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Where("subject_id = ? AND month = ? AND enrollment_id IN ?", subjectID, month.FirstDay(), enrollmentIDs).
			Find(&rows).Error
	})
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "loading monthly sheet", err)
	}
	for _, m := range rows {
		d, err := toDomainMonthly(m)
		if err != nil {
			return nil, err
		}
		out[m.EnrollmentID] = d
	}
	return out, nil
}

// MonthlyGradesForEnrollment lists every monthly grade of one enrollment,
// ordered by month then subject for a stable read.
func (r *Repository) MonthlyGradesForEnrollment(ctx context.Context, enrollmentID string) ([]domain.MonthlyGrade, error) {
	var rows []monthlyGradeModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Where("enrollment_id = ?", enrollmentID).Order("month ASC, subject_id ASC").Find(&rows).Error
	})
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing monthly grades", err)
	}
	out := make([]domain.MonthlyGrade, 0, len(rows))
	for _, m := range rows {
		d, err := toDomainMonthly(m)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// CountScaleViolations counts, in ONE tenant-bound query, the grades of the
// named enrollments that check refuses. The comparison runs on the exact
// numeric column: bounds travel as decimal text cast to numeric, and "too many
// decimals" is value·10^d not being a whole number — no float anywhere.
func (r *Repository) CountScaleViolations(ctx context.Context, enrollmentIDs []string, check ports.ScaleCheck) (int, error) {
	if len(enrollmentIDs) == 0 {
		return 0, nil
	}
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&monthlyGradeModel{}).Where("enrollment_id IN ?", enrollmentIDs)
		if check.GradingEnabled {
			s := check.Scale
			factor := decimalFactor(s.Decimals())
			q = q.Where("(value < ?::numeric OR value > ?::numeric OR value * ? <> trunc(value * ?))",
				hundredthsText(s.MinHundredths()), hundredthsText(s.MaxHundredths()), factor, factor)
		}
		if err := q.Count(&n).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "counting scale violations", err)
		}
		return nil
	})
	return int(n), err
}

// hundredthsText prints hundredths as exact decimal text for a numeric cast.
func hundredthsText(h int) string {
	return fmt.Sprintf("%d.%02d", h/hundredthsPerUnit, h%hundredthsPerUnit)
}

// decimalFactor is 10^decimals.
func decimalFactor(decimals int) int {
	f := 1
	for i := 0; i < decimals; i++ {
		f *= decimalBase
	}
	return f
}

// decimalBase is the radix decimals count in.
const decimalBase = 10
