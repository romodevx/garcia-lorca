package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// GetMedical returns the pupil's medical record, or an empty one when nothing
// was captured. It does not check that the pupil exists: the use case does.
func (r *Repository) GetMedical(ctx context.Context, studentID string) (domain.MedicalRecord, error) {
	var out domain.MedicalRecord
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m medicalModel
		err := tx.Where("student_id = ?", studentID).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading medical record", err)
		}
		out = domain.RestoreMedicalRecord(domain.BloodType(m.BloodType), m.Allergies, m.Conditions)
		return nil
	})
	return out, err
}

// SaveMedical writes (upserts) the pupil's medical record. A pupil this tenant
// cannot see is KindNotFound: an FK check does not pass through RLS, so the
// visibility check is what stops a row being written against another tenant's
// pupil.
func (r *Repository) SaveMedical(ctx context.Context, studentID string, rec domain.MedicalRecord) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		var visible int64
		if err := tx.Model(&studentModel{}).Where("id = ?", studentID).Count(&visible).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "checking the pupil", err)
		}
		if visible == 0 {
			return glerrors.ErrNotFound
		}
		row := medicalModel{
			TenantID: r.tenantID, StudentID: studentID,
			BloodType: string(rec.BloodType()), Allergies: rec.Allergies(), Conditions: rec.Conditions(),
		}
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "student_id"}},
			DoUpdates: clause.Assignments(map[string]any{"blood_type": row.BloodType, "allergies": row.Allergies, "conditions": row.Conditions, "updated_at": gorm.Expr("now()")}),
		}).Create(&row).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "saving medical record", err)
		}
		return nil
	})
}
