package repo

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/comms/internal/service"
)

// Compile-time proof the adapter satisfies the template seam.
var _ service.TemplateStore = (*Repo)(nil)

// BirthdayTemplate reads the tenant's saved greeting. found is false while the
// school has not saved one — the service then sends its default.
func (r *Repo) BirthdayTemplate(ctx context.Context) (service.BirthdayTemplate, bool, error) {
	var (
		out   service.BirthdayTemplate
		found bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m birthdayTemplateModel
		err := tx.Where("tenant_id = ?", r.tenantID).Take(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading the birthday greeting", err)
		}
		out, found = toBirthdayTemplate(m), true
		return nil
	})
	if err != nil {
		return service.BirthdayTemplate{}, false, err
	}
	return out, found, nil
}

// SaveBirthdayTemplate replaces the tenant's greeting: an upsert on the tenant,
// so there is never more than one.
func (r *Repo) SaveBirthdayTemplate(ctx context.Context, t service.BirthdayTemplate) (service.BirthdayTemplate, error) {
	m := birthdayTemplateModel{TenantID: r.tenantID, Title: t.Title, Body: t.Body, UpdatedAt: t.UpdatedAt, UpdatedBy: t.UpdatedBy}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "body", "updated_at", "updated_by"}),
		}).Create(&m).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "saving the birthday greeting", err)
		}
		return nil
	})
	if err != nil {
		return service.BirthdayTemplate{}, err
	}
	return toBirthdayTemplate(m), nil
}

func toBirthdayTemplate(m birthdayTemplateModel) service.BirthdayTemplate {
	return service.BirthdayTemplate{Title: m.Title, Body: m.Body, UpdatedAt: m.UpdatedAt, UpdatedBy: m.UpdatedBy}
}
