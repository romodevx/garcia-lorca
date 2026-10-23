package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter satisfies the guardian port. The same
// tenant-scoped Repository backs students, their enrollments, and their guardian
// links so RLS binding lives in exactly one place (withTenant / pkg/authctx).
var _ ports.GuardianRepository = (*Repository)(nil)

// Link inserts a guardian ↔ student link. A duplicate (guardian, student) per
// tenant is KindConflict; a dangling student_id is KindInvalid (guardian_user_id
// is a cross-service reference with no FK — the use case validates it against iam).
func (r *Repository) Link(ctx context.Context, l domain.GuardianLink) error {
	model := guardianLinkFromDomain(l)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			return translateGuardianWriteErr(err)
		}
		return nil
	})
}

// Unlink removes a link by its (guardian, student) pair. A link the bound tenant
// cannot see is KindNotFound, so RLS makes a cross-tenant delete indistinguishable
// from a missing link.
func (r *Repository) Unlink(ctx context.Context, guardianUserID, studentID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Where("guardian_user_id = ? AND student_id = ?", guardianUserID, studentID).
			Delete(&guardianStudentModel{})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "unlinking guardian", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// ListByStudent returns the links of one student (its guardians), ordered by id
// for a stable read.
func (r *Repository) ListByStudent(ctx context.Context, studentID string) ([]domain.GuardianLink, error) {
	return r.listGuardianLinks(ctx, "student_id = ?", studentID)
}

// ListByGuardian returns the links of one guardian (their students), ordered by id
// for a stable read.
func (r *Repository) ListByGuardian(ctx context.Context, guardianUserID string) ([]domain.GuardianLink, error) {
	return r.listGuardianLinks(ctx, "guardian_user_id = ?", guardianUserID)
}

// ByID returns one contact by its own id. A row the bound tenant cannot see is
// KindNotFound, so RLS makes a cross-tenant read indistinguishable from a
// missing contact.
func (r *Repository) ByID(ctx context.Context, id string) (domain.GuardianLink, error) {
	var out domain.GuardianLink
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var row guardianStudentModel
		if err := tx.Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return glerrors.ErrNotFound
			}
			return glerrors.Wrap(glerrors.KindInternal, "reading contact", err)
		}
		l, err := guardianLinkToDomain(row)
		if err != nil {
			return err
		}
		out = l
		return nil
	})
	if err != nil {
		return domain.GuardianLink{}, err
	}
	return out, nil
}

// UpdateContact writes an edited contact: relationship, the person and the
// emergency mark. The account column is never written here. A row the bound
// tenant cannot see is KindNotFound.
func (r *Repository) UpdateContact(ctx context.Context, l domain.GuardianLink) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&guardianStudentModel{}).Where("id = ?", l.ID()).Updates(map[string]any{
			"relationship": l.Relationship().String(),
			"full_name":    l.FullName(),
			"phone":        l.Phone(),
			"email":        l.Email(),
			"is_emergency": l.IsEmergency(),
		})
		if res.Error != nil {
			return translateGuardianWriteErr(res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// RemoveContact deletes a contact that has no account. A row the bound tenant
// cannot see is KindNotFound.
//
// `guardian_user_id IS NULL` rides in the WHERE for the same reason as in
// AttachAccount: the service checked "no account" in an earlier transaction,
// and access granted since then must not disappear with the phone number.
func (r *Repository) RemoveContact(ctx context.Context, id string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND guardian_user_id IS NULL", id).Delete(&guardianStudentModel{})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "removing contact", res.Error)
		}
		if res.RowsAffected == 1 {
			return nil
		}
		var exists int64
		if err := tx.Model(&guardianStudentModel{}).Where("id = ?", id).Count(&exists).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading contact", err)
		}
		if exists == 0 {
			return glerrors.ErrNotFound
		}
		return glerrors.New(glerrors.KindConflict, "this contact can open the app: revoke its access first")
	})
}

// AttachAccount sets the iam account on a contact that has none.
//
// The UPDATE is CONDITIONAL — `guardian_user_id IS NULL` rides in the WHERE, not
// in an `if` above it. Two clerks granting access at the same time both read a
// contact without an account; if the write were unconditional the second would
// silently move the pupil to a different family. Guarded, the loser writes
// nothing and is told so.
//
// Zero rows therefore means one of two things, and the difference does not
// matter to the caller: the contact is gone, or it already has an account.
// Both are answered as a conflict EXCEPT when the row is genuinely absent,
// which we distinguish with a follow-up read so an unknown contact is not
// reported as a duplicate.
//
// It takes NO family lock: the use case attaches accounts only through
// OnPupil (FamilyAccess.AttachAccount, the same conditional write), which
// keeps the family cap. This entry point stays for the adapter's own tests.
func (r *Repository) AttachAccount(ctx context.Context, l domain.GuardianLink) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return attachAccount(tx, l)
	})
}

// attachAccount is AttachAccount's conditional write inside tx.
func attachAccount(tx *gorm.DB, l domain.GuardianLink) error {
	res := tx.Model(&guardianStudentModel{}).
		Where("id = ? AND student_id = ? AND guardian_user_id IS NULL", l.ID(), l.StudentID()).
		Update("guardian_user_id", l.GuardianUserID())
	if res.Error != nil {
		return translateAccessErr(res.Error)
	}
	if res.RowsAffected == 1 {
		return nil
	}
	var exists int64
	if err := tx.Model(&guardianStudentModel{}).Where("id = ? AND student_id = ?", l.ID(), l.StudentID()).Count(&exists).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "reading contact", err)
	}
	if exists == 0 {
		return glerrors.ErrNotFound
	}
	return glerrors.New(glerrors.KindConflict, "this contact already has an account")
}

// listGuardianLinks is the shared read for both projections: same tenant-bound
// transaction and mapping, only the filtered column differs.
func (r *Repository) listGuardianLinks(ctx context.Context, where, value string) ([]domain.GuardianLink, error) {
	var out []domain.GuardianLink
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []guardianStudentModel
		if err := tx.Model(&guardianStudentModel{}).Where(where, value).Order("id ASC").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing guardian links", err)
		}
		for _, m := range rows {
			l, err := guardianLinkToDomain(m)
			if err != nil {
				return err
			}
			out = append(out, l)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// translateGuardianWriteErr maps Postgres constraint failures on a link insert to
// domain kinds: the (tenant, guardian, student) UNIQUE → KindConflict, a missing
// student FK → KindInvalid, the email-normalization CHECK → KindInvalid.
func translateGuardianWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case uniqueViolation:
			return glerrors.Wrap(glerrors.KindConflict, "guardian is already linked to this student", err)
		case foreignKeyViolation:
			return glerrors.Wrap(glerrors.KindInvalid, "student does not exist", err)
		case checkViolation:
			// Only the email CHECK can be tripped by a client (a raw email that
			// bypassed the domain); any other CHECK is a server inconsistency.
			if pgErr.ConstraintName == contactEmailNormalizedCheck {
				return glerrors.Wrap(glerrors.KindInvalid, "email is not in its normalized form", err)
			}
		}
	}
	return glerrors.Wrap(glerrors.KindInternal, "linking guardian", err)
}
