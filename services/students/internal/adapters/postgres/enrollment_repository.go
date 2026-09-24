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

// Compile-time proof the adapter satisfies the enrollment port. The same
// tenant-scoped Repository backs both students and their enrollments so RLS
// binding lives in exactly one place (withTenant / pkg/authctx).
var _ ports.EnrollmentRepository = (*Repository)(nil)

const (
	foreignKeyViolation = "23503"
	checkViolation      = "23514"
	// contactEmailNormalizedCheck is the CHECK migrations/students/0006 adds.
	contactEmailNormalizedCheck = "guardian_students_email_normalized_ck"
)

// CreateEnrollment inserts a new enrollment. A duplicate (student, cycle) per
// tenant is KindConflict; a dangling student_id is KindInvalid.
func (r *Repository) CreateEnrollment(ctx context.Context, e domain.Enrollment) error {
	model := enrollmentFromDomain(e)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			return translateEnrollmentWriteErr(err)
		}
		return nil
	})
}

// GetEnrollment returns an enrollment or KindNotFound (also when RLS hides it).
func (r *Repository) GetEnrollment(ctx context.Context, id string) (domain.Enrollment, error) {
	var out domain.Enrollment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m enrollmentModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		e, err := enrollmentToDomain(m)
		if err != nil {
			return err
		}
		out = e
		return nil
	})
	if err != nil {
		return domain.Enrollment{}, err
	}
	return out, nil
}

// ListEnrollments returns enrollments matching the given filters; an empty
// string skips that filter. Results are ordered by id for a stable read.
func (r *Repository) ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]domain.Enrollment, error) {
	var out []domain.Enrollment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&enrollmentModel{}).Order("id ASC")
		if studentID != "" {
			q = q.Where("student_id = ?", studentID)
		}
		if cycleID != "" {
			q = q.Where("cycle_id = ?", cycleID)
		}
		if groupID != "" {
			q = q.Where("group_id = ?", groupID)
		}
		var rows []enrollmentModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing enrollments", err)
		}
		for _, m := range rows {
			e, err := enrollmentToDomain(m)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TransferEnrollment writes the transfer history row AND moves the enrollment to
// the destination group in ONE transaction, so a move is atomic: either both the
// history and the group change persist, or neither does. RLS binds the tenant for
// the whole transaction, so a caller cannot transfer another tenant's enrollment
// (the UPDATE matches zero rows → KindNotFound → rollback).
func (r *Repository) TransferEnrollment(ctx context.Context, cmd ports.TransferCommand) error {
	model := transferFromDomain(cmd.Transfer)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		// Seats first, under the same per-group lock the by-grade path takes, so
		// two moves into the last seat cannot both win.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, cmd.To.ID).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reserving the group", err)
		}
		var taken int64
		if err := tx.Model(&enrollmentModel{}).
			Where("group_id = ? AND status = ?", cmd.To.ID, domain.EnrollmentStatusActive.String()).
			Count(&taken).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "counting the group", err)
		}
		if cmd.To.Capacity <= 0 || taken >= int64(cmd.To.Capacity) {
			return glerrors.New(glerrors.KindConflict, "ese grupo ya no tiene lugares")
		}

		if err := tx.Create(&model).Error; err != nil {
			return translateTransferWriteErr(err)
		}
		res := tx.Model(&enrollmentModel{}).Where("id = ?", cmd.EnrollmentID).Updates(map[string]any{
			"group_id":   cmd.To.ID,
			"updated_at": gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating enrollment group", res.Error)
		}
		if res.RowsAffected == 0 {
			// No visible enrollment with this id in the bound tenant — treat as
			// not found and roll the transfer insert back.
			return glerrors.ErrNotFound
		}
		// The pupil's file names the group they are now in, in this same
		// transaction: a move can never leave it naming the one they left.
		file := tx.Exec(`UPDATE students SET grade_group = ?, updated_at = now()
		                  WHERE id = (SELECT student_id FROM enrollments WHERE id = ?)`,
			cmd.To.Name, cmd.EnrollmentID)
		if file.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "recording the new group on the file", file.Error)
		}
		return nil
	})
}

// SetEnrollmentStatus updates an enrollment's status (e.g. to withdrawn).
func (r *Repository) SetEnrollmentStatus(ctx context.Context, id string, status domain.EnrollmentStatus) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&enrollmentModel{}).Where("id = ?", id).Updates(map[string]any{
			"status":     status.String(),
			"updated_at": gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating enrollment status", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// translateEnrollmentWriteErr maps Postgres constraint failures on an enrollment
// insert to domain kinds: the (tenant, student, cycle) UNIQUE → KindConflict, a
// missing student FK → KindInvalid.
func translateEnrollmentWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case uniqueViolation:
			return glerrors.Wrap(glerrors.KindConflict, "student is already enrolled this cycle", err)
		case foreignKeyViolation:
			return glerrors.Wrap(glerrors.KindInvalid, "student does not exist", err)
		}
	}
	return glerrors.Wrap(glerrors.KindInternal, "creating enrollment", err)
}

// translateTransferWriteErr maps constraint failures on a transfer insert: a
// dangling enrollment FK → KindInvalid; anything else → KindInternal.
func translateTransferWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return glerrors.Wrap(glerrors.KindInvalid, "enrollment does not exist", err)
	}
	return glerrors.Wrap(glerrors.KindInternal, "recording transfer", err)
}

// CreateEnrollmentInFirstFreeGroup places a pupil in the first group with a seat
// left, deciding and writing inside ONE transaction.
//
// The lock is the point. Counting the occupancy and then inserting would be a
// textbook race with a real cost: two secretaries enrolling at the same moment
// would each read 24 of 25 seats taken in 1°A and each take the last one, and the
// school would find out in September when twenty-six children turned up.
//
// It is a transaction-scoped ADVISORY lock keyed on the group, not a row lock,
// because the row that would be locked lives in the config service's `groups`
// table and this transaction cannot reach across a bounded context to take it.
// Advisory locks need no row to exist and are released with the transaction, so
// a caller that dies mid-placement releases it rather than wedging a group
// forever. Two callers aiming at the same group serialise; the second sees the
// first's pupil and moves on to 1°B.
func (r *Repository) CreateEnrollmentInFirstFreeGroup(
	ctx context.Context,
	e domain.Enrollment,
	groups []ports.GroupSeats,
) (ports.GroupSeats, error) {
	var chosen ports.GroupSeats
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, group := range groups {
			if group.Capacity <= 0 {
				// A group with no seats declared is not "infinitely large"; it is
				// a group nobody has sized yet, and guessing would be worse.
				continue
			}
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, group.ID).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "reserving the group", err)
			}

			var taken int64
			if err := tx.Model(&enrollmentModel{}).
				Where("group_id = ? AND status = ?", group.ID, domain.EnrollmentStatusActive.String()).
				Count(&taken).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "counting the group", err)
			}
			if taken >= int64(group.Capacity) {
				continue
			}

			model := enrollmentFromDomain(e)
			model.GroupID = group.ID
			model.CycleID = group.CycleID
			if err := tx.Create(&model).Error; err != nil {
				return translateEnrollmentWriteErr(err)
			}
			chosen = group
			return nil
		}
		return glerrors.New(glerrors.KindConflict,
			"no quedan lugares en ese grado: abre otro grupo en Configuración")
	})
	if err != nil {
		return ports.GroupSeats{}, err
	}
	return chosen, nil
}
