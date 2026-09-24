package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// PhotoRef returns where the pupil's photo is stored, or KindNotFound.
func (r *Repository) PhotoRef(ctx context.Context, studentID string) (ports.PhotoRef, error) {
	var out ports.PhotoRef
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m photoModel
		err := tx.Where("student_id = ?", studentID).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return glerrors.ErrNotFound
		}
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading photo reference", err)
		}
		out = ports.PhotoRef{Key: m.ObjectKey, ContentType: m.ContentType, Size: m.SizeBytes}
		return nil
	})
	return out, err
}

// SavePhotoRef replaces the pupil's photo reference. A pupil this tenant cannot
// see is KindNotFound (an FK check does not pass through RLS).
func (r *Repository) SavePhotoRef(ctx context.Context, studentID string, ref ports.PhotoRef) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		var visible int64
		if err := tx.Model(&studentModel{}).Where("id = ?", studentID).Count(&visible).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "checking the pupil", err)
		}
		if visible == 0 {
			return glerrors.ErrNotFound
		}
		row := photoModel{TenantID: r.tenantID, StudentID: studentID, ObjectKey: ref.Key, ContentType: ref.ContentType, SizeBytes: ref.Size}
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "student_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"object_key": row.ObjectKey, "content_type": row.ContentType, "size_bytes": row.SizeBytes, "updated_at": gorm.Expr("now()"),
			}),
		}).Create(&row).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "saving photo reference", err)
		}
		return nil
	})
}

// DeletePhotoRef removes the pupil's photo reference. Absent is not an error.
func (r *Repository) DeletePhotoRef(ctx context.Context, studentID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("student_id = ?", studentID).Delete(&photoModel{}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting photo reference", err)
		}
		return nil
	})
}
