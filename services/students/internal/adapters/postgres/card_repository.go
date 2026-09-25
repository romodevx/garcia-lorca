package postgres

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

var _ ports.CardRepository = (*Repository)(nil)

// cardTokenModel is a `student_card_tokens` row (migration 0014).
type cardTokenModel struct {
	ID        string     `gorm:"column:id;primaryKey"`
	TenantID  string     `gorm:"column:tenant_id"`
	StudentID string     `gorm:"column:student_id"`
	Token     string     `gorm:"column:token"`
	IssuedAt  time.Time  `gorm:"column:issued_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
}

func (cardTokenModel) TableName() string { return "student_card_tokens" }

func toDomainCard(m cardTokenModel) domain.CardToken {
	var revoked time.Time
	if m.RevokedAt != nil {
		revoked = *m.RevokedAt
	}
	return domain.RestoreCardToken(m.ID, m.TenantID, m.StudentID, m.Token, m.IssuedAt, revoked)
}

// ActiveCard returns a pupil's active card code, if any.
func (r *Repository) ActiveCard(ctx context.Context, studentID string) (domain.CardToken, bool, error) {
	var (
		out   domain.CardToken
		found bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []cardTokenModel
		if err := tx.Where("student_id = ? AND revoked_at IS NULL", studentID).Limit(1).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading the pupil's card", err)
		}
		if len(rows) == 1 {
			out, found = toDomainCard(rows[0]), true
		}
		return nil
	})
	return out, found, err
}

// SaveCard stores a newly issued card code.
func (r *Repository) SaveCard(ctx context.Context, c domain.CardToken) error {
	row := cardTokenModel{ID: c.ID(), TenantID: c.TenantID(), StudentID: c.StudentID(), Token: c.Token(), IssuedAt: c.IssuedAt()}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			if isUniqueViolation(err) {
				// Two first views of the card at once: the other one issued it.
				return glerrors.Wrap(glerrors.KindConflict, "the pupil already has an active card", err)
			}
			return glerrors.Wrap(glerrors.KindInternal, "storing the card", err)
		}
		return nil
	})
}

// RevokeCards retires every active card code of a pupil.
func (r *Repository) RevokeCards(ctx context.Context, studentID string, at time.Time) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Model(&cardTokenModel{}).Where("student_id = ? AND revoked_at IS NULL", studentID).
			Update("revoked_at", at).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "revoking the card", err)
		}
		return nil
	})
}

// FindCard returns the card carrying a token, active or not.
func (r *Repository) FindCard(ctx context.Context, token string) (domain.CardToken, error) {
	var out domain.CardToken
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m cardTokenModel
		if err := tx.Where("token = ?", token).First(&m).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return glerrors.ErrNotFound
			}
			return glerrors.Wrap(glerrors.KindInternal, "finding the card", err)
		}
		out = toDomainCard(m)
		return nil
	})
	return out, err
}
