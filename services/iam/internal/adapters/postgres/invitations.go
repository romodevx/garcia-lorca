package postgres

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// invitationModel is the GORM row. Double model + mapper, like every other
// table here: the domain never sees a struct tag.
type invitationModel struct {
	ID         string     `gorm:"column:id;primaryKey"`
	TenantID   string     `gorm:"column:tenant_id"`
	UserID     string     `gorm:"column:user_id"`
	TokenHash  string     `gorm:"column:token_hash"`
	Purpose    string     `gorm:"column:purpose"`
	ExpiresAt  time.Time  `gorm:"column:expires_at"`
	ConsumedAt *time.Time `gorm:"column:consumed_at"`
	SentAt     *time.Time `gorm:"column:sent_at"`
	SentCount  int        `gorm:"column:sent_count"`
	LastError  string     `gorm:"column:last_error"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

func (invitationModel) TableName() string { return "user_invitations" }

// InvitationRepo persists invitations under the tenant's RLS context.
//
// NO METHOD TAKES A TENANT. The redeem endpoints are anonymous, so a tenant
// travelling in the request would be caller-controlled and RLS would quietly
// become "whatever the URL claimed". It is fixed at construction instead, where
// no caller can reach it — and the absence of the parameter is what stops the
// next person from adding one.
type InvitationRepo struct{ base }

// NewInvitationRepo binds the repo to one tenant.
func NewInvitationRepo(db *gorm.DB, tenantID string) *InvitationRepo {
	return &InvitationRepo{base{db: db, tenantID: tenantID}}
}

// Issue stores a fresh invitation, REPLACING any live one the user still holds
// for the same purpose.
//
// Replacing rather than inserting is what makes "resend" mean something: two
// live invitations would leave the previous mail working after the director
// sent a new one, so the old link would keep opening the door for its full
// twenty-four hours.
//
// The delete and the insert share one transaction. Split apart, a failure
// between them would leave the user with no way in at all.
func (r *InvitationRepo) Issue(ctx context.Context, inv domain.Invitation) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND purpose = ? AND consumed_at IS NULL",
			inv.UserID(), string(inv.Purpose())).
			Delete(&invitationModel{}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "replacing the previous invitation", err)
		}
		row := invitationModel{
			ID:        inv.ID(),
			TenantID:  r.tenantID,
			UserID:    inv.UserID(),
			TokenHash: inv.TokenHash(),
			Purpose:   string(inv.Purpose()),
			ExpiresAt: inv.ExpiresAt(),
		}
		if err := tx.Create(&row).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "storing the invitation", err)
		}
		return nil
	})
}

// Consume spends the token and returns what it was.
//
// ONE STATEMENT does all three checks — the hash matches, it is unconsumed, it
// is unexpired — so two simultaneous redemptions cannot both win: the second
// UPDATE matches zero rows because the first already set consumed_at. The hash
// is re-asserted here and not just at lookup, which also closes the
// accept-versus-resend race: a resend landing mid-redemption replaces the row's
// token, and this UPDATE then correctly matches nothing.
//
// Every failure is the SAME KindNotFound. Unknown, expired and already-spent
// must be indistinguishable, or the difference becomes a way to probe which
// tokens exist.
func (r *InvitationRepo) Consume(ctx context.Context, tokenHash string, now time.Time) (domain.Invitation, error) {
	var row invitationModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&invitationModel{}).
			Clauses(clause.Returning{}).
			Where("token_hash = ? AND consumed_at IS NULL AND expires_at > ?", tokenHash, now).
			Updates(map[string]any{"consumed_at": now})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "consuming the invitation", res.Error)
		}
		if res.RowsAffected == 0 {
			return errInvitationDead()
		}
		return tx.Where("token_hash = ?", tokenHash).First(&row).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Invitation{}, errInvitationDead()
		}
		return domain.Invitation{}, err
	}
	return toDomainInvitation(row)
}

// MarkSent records what happened when we tried to deliver the invitation.
//
// sent_count increments on every attempt, not only the successful ones: a row
// that says "tried four times, last error 535" is the difference between the
// director knowing and the director guessing, which is the whole reason the
// original bug went unnoticed for a week.
func (r *InvitationRepo) MarkSent(ctx context.Context, invitationID string, sentAt time.Time, sendErr string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		fields := map[string]any{
			"sent_count": gorm.Expr("sent_count + 1"),
			"last_error":  sendErr,
		}
		if sendErr == "" {
			fields["sent_at"] = sentAt
		}
		if err := tx.Model(&invitationModel{}).
			Where("id = ?", invitationID).Updates(fields).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "recording the invitation delivery", err)
		}
		return nil
	})
}

// Release hands a spent token back, undoing a Consume whose follow-up failed.
//
// Guarded on `consumed_at IS NOT NULL` so it can only ever undo a consumption,
// never resurrect a token somebody already redeemed successfully.
func (r *InvitationRepo) Release(ctx context.Context, invitationID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Model(&invitationModel{}).
			Where("id = ? AND consumed_at IS NOT NULL", invitationID).
			Update("consumed_at", nil).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "releasing the invitation", err)
		}
		return nil
	})
}

// Revoke kills every live invitation this user holds.
//
// Deleting rather than marking consumed: an invitation nobody redeemed is not a
// spent token, it is a token that stopped being relevant, and keeping the row
// would only make the partial unique index reject the next one.
func (r *InvitationRepo) Revoke(ctx context.Context, userID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND consumed_at IS NULL", userID).
			Delete(&invitationModel{}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "revoking live invitations", err)
		}
		return nil
	})
}

// ListLive returns every unconsumed, unexpired invitation, keyed by user id.
//
// ONE query for the whole list. The admin screen draws a state pill per row, and
// the obvious implementation — ask per user — is an N+1 that only shows up once
// the school has a real number of staff.
func (r *InvitationRepo) ListLive(ctx context.Context, now time.Time) (map[string]domain.Invitation, error) {
	var rows []invitationModel
	if err := r.withTenant(ctx, func(tx *gorm.DB) error {
		return tx.Where("consumed_at IS NULL AND expires_at > ?", now).Find(&rows).Error
	}); err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "listing live invitations", err)
	}
	out := make(map[string]domain.Invitation, len(rows))
	for _, row := range rows {
		inv, err := toDomainInvitation(row)
		if err != nil {
			return nil, err
		}
		out[row.UserID] = inv
	}
	return out, nil
}

// errInvitationDead is the ONE answer for every dead token.
func errInvitationDead() error {
	return glerrors.New(glerrors.KindNotFound, "esta invitación ya no es válida")
}

func toDomainInvitation(row invitationModel) (domain.Invitation, error) {
	var consumed time.Time
	if row.ConsumedAt != nil {
		consumed = *row.ConsumedAt
	}
	return domain.NewInvitation(row.ID, row.UserID, row.TokenHash,
		domain.InvitationPurpose(row.Purpose), row.ExpiresAt, consumed)
}

// compile-time proof the adapter satisfies the port it exists for.
var _ ports.InvitationRepository = (*InvitationRepo)(nil)
