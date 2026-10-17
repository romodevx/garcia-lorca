package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/adapters/protoconv"
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
	outboxFinishTimeout = 2 * time.Second
	// contactEmailNormalizedCheck is the CHECK migrations/students/0006 adds.
	contactEmailNormalizedCheck = "guardian_students_email_normalized_ck"
)

// CreateEnrollment inserts a new enrollment. A duplicate (student, cycle) per
// tenant is KindConflict; a dangling student_id is KindInvalid.
func (r *Repository) CreateEnrollment(ctx context.Context, e domain.Enrollment, seats ports.GroupSeats) error {
	model := enrollmentFromDomain(e)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		student, err := lockStudentForEnrollment(tx, e.StudentID())
		if err != nil {
			return err
		}
		if err := refuseTerminalAdmission(student); err != nil {
			return err
		}
		if err := reserveSeat(tx, seats); err != nil {
			return err
		}
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
		// The withdrawal path takes the student lock before touching an
		// enrollment. Match that order so a concurrent transfer cannot race a
		// terminal admission or deadlock with a withdrawal.
		var student studentModel
		if err := tx.Table("students").Select("students.*").
			Joins("JOIN enrollments ON enrollments.student_id = students.id").
			Where("enrollments.id = ?", cmd.EnrollmentID).
			Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "students"}}).
			First(&student).Error; err != nil {
			return translateFindErr(err)
		}
		if err := refuseTerminalAdmission(student); err != nil {
			return err
		}
		if err := reserveSeat(tx, cmd.To); err != nil {
			return err
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

// ListTransfers returns the moves of one student's enrollments, oldest moved_at
// first (id breaks ties), in one tenant-bound query joined on enrollments.
func (r *Repository) ListTransfers(ctx context.Context, studentID string) ([]domain.EnrollmentTransfer, error) {
	return r.listTransfers(ctx, func(q *gorm.DB) *gorm.DB {
		return q.Joins("JOIN enrollments e ON e.id = enrollment_transfers.enrollment_id").Where("e.student_id = ?", studentID)
	})
}

// ListTransfersIntoGroup returns every move into one group, oldest moved_at
// first (id breaks ties), in one tenant-bound query.
func (r *Repository) ListTransfersIntoGroup(ctx context.Context, groupID string) ([]domain.EnrollmentTransfer, error) {
	return r.listTransfers(ctx, func(q *gorm.DB) *gorm.DB {
		return q.Where("enrollment_transfers.to_group_id = ?", groupID)
	})
}

// ListTransfersTouchingGroup returns every move into OR out of one group,
// oldest moved_at first (id breaks ties), in one tenant-bound query: with the
// enrollments ever in the group, it is who sat in the group on any past day.
// Both sides are indexed (0015 to_group, 0021 from_group).
func (r *Repository) ListTransfersTouchingGroup(ctx context.Context, groupID string) ([]domain.EnrollmentTransfer, error) {
	return r.listTransfers(ctx, func(q *gorm.DB) *gorm.DB {
		return q.Where("enrollment_transfers.to_group_id = ? OR enrollment_transfers.from_group_id = ?", groupID, groupID)
	})
}

// ListEnrollmentsEverInGroup returns, ordered by id, every enrollment that was
// ever in one group: the ones that carry it now, and the ones moved out of it
// since (a transfer rewrites group_id in place, so those carry another group).
func (r *Repository) ListEnrollmentsEverInGroup(ctx context.Context, groupID string) ([]domain.Enrollment, error) {
	var out []domain.Enrollment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []enrollmentModel
		movedOut := tx.Model(&enrollmentTransferModel{}).Select("enrollment_id").Where("from_group_id = ?", groupID)
		if err := tx.Model(&enrollmentModel{}).
			Where("group_id = ? OR id IN (?)", groupID, movedOut).
			Order("id ASC").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing the group's enrollments", err)
		}
		out = make([]domain.Enrollment, 0, len(rows))
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

func (r *Repository) listTransfers(ctx context.Context, filter func(*gorm.DB) *gorm.DB) ([]domain.EnrollmentTransfer, error) {
	var out []domain.EnrollmentTransfer
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []enrollmentTransferModel
		if err := filter(tx.Model(&enrollmentTransferModel{})).
			Order("enrollment_transfers.moved_at ASC, enrollment_transfers.id ASC").
			Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing transfers", err)
		}
		out = make([]domain.EnrollmentTransfer, 0, len(rows))
		for _, m := range rows {
			t, err := transferToDomain(m)
			if err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "invalid transfer in db", err)
			}
			out = append(out, t)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
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

// WithdrawEnrollment persists the withdrawal and its StudentWithdrawn event in
// one tenant-bound transaction.
func (r *Repository) WithdrawEnrollment(ctx context.Context, e domain.Enrollment) error {
	data, err := proto.Marshal(&studentsv1.StudentWithdrawn{
		TenantId: e.TenantID(), StudentId: e.StudentID(), CycleId: e.CycleID(),
		EnrollmentId: e.ID(), WithdrawnOn: e.WithdrawnOn().Format("2006-01-02"),
	})
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "encoding student withdrawal", err)
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		var student studentModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", e.StudentID()).First(&student).Error; err != nil {
			return translateFindErr(err)
		}
		res := tx.Model(&enrollmentModel{}).Where("id = ? AND status = ?", e.ID(), domain.EnrollmentStatusActive.String()).Updates(map[string]any{
			"status": domain.EnrollmentStatusWithdrawn.String(), "withdrawn_on": e.WithdrawnOn(),
			"withdrawal_reason": e.WithdrawalReason(), "updated_at": gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "withdrawing enrollment", res.Error)
		}
		if res.RowsAffected == 0 {
			var current enrollmentModel
			if err := tx.Where("id = ?", e.ID()).First(&current).Error; err != nil {
				return translateFindErr(err)
			}
			return glerrors.New(glerrors.KindConflict, "enrollment is not active")
		}
		var newerActive int64
		// A cycle ending does not close its enrollment. Only a later placement
		// means the pupil has already returned after this enrollment.
		if err := tx.Model(&enrollmentModel{}).
			Where("student_id = ? AND status = ? AND enrolled_at > ?", e.StudentID(), domain.EnrollmentStatusActive.String(), e.EnrolledAt()).
			Count(&newerActive).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "checking newer active enrollments", err)
		}
		if newerActive == 0 {
			previous, err := domain.ParseAdmissionStatus(student.AdmissionStatus)
			if err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "reading student admission status", err)
			}
			if previous != domain.AdmissionWithdrawn {
				if !previous.CanTransitionTo(domain.AdmissionWithdrawn) {
					return glerrors.New(glerrors.KindConflict, "student cannot transition to withdrawn")
				}
				statusResult := tx.Model(&studentModel{}).Where("id = ? AND admission_status = ?", e.StudentID(), previous.String()).Updates(map[string]any{
					"admission_status": domain.AdmissionWithdrawn.String(), "updated_at": gorm.Expr("now()"),
				})
				if statusResult.Error != nil {
					return glerrors.Wrap(glerrors.KindInternal, "withdrawing student admission", statusResult.Error)
				}
				if statusResult.RowsAffected == 0 {
					return glerrors.New(glerrors.KindConflict, "student admission status changed concurrently")
				}
				admissionData, err := proto.Marshal(&studentsv1.AdmissionStatusChanged{
					StudentId: e.StudentID(), TenantId: e.TenantID(),
					PreviousStatus: protoconv.ToProtoStatus(previous),
					NewStatus:      protoconv.ToProtoStatus(domain.AdmissionWithdrawn),
					Reason:         e.WithdrawalReason(),
				})
				if err != nil {
					return glerrors.Wrap(glerrors.KindInternal, "encoding admission status change", err)
				}
				admissionRow := outboxModel{TenantID: e.TenantID(), EventType: "AdmissionStatusChanged", Payload: admissionData}
				if err := tx.Create(&admissionRow).Error; err != nil {
					return glerrors.Wrap(glerrors.KindInternal, "queueing admission status change", err)
				}
			}
		}
		row := outboxModel{TenantID: e.TenantID(), EventType: "StudentWithdrawn", Payload: data}
		if err := tx.Create(&row).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "queueing student withdrawal", err)
		}
		return nil
	})
}

const (
	outboxBaseDelay = 30 * time.Second
	outboxMaxDelay  = 30 * time.Minute
)

func outboxRetryDelay(attempts int) time.Duration {
	d := outboxBaseDelay
	for i := 1; i < attempts && d < outboxMaxDelay; i++ {
		d *= 2
	}
	if d > outboxMaxDelay {
		return outboxMaxDelay
	}
	return d
}

// DrainOutbox claims due events in a short transaction, publishes them without
// holding row locks, then records success or the retry delay in another short
// transaction. SKIP LOCKED lets multiple instances relay independently.
func (r *Repository) DrainOutbox(ctx context.Context, limit int, publish func(context.Context, ports.OutboxEvent) error) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	var claimed []outboxModel
	if err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("published_at IS NULL AND next_attempt_at <= now()").Order("created_at, id").Limit(limit).Find(&claimed).Error; err != nil {
			return err
		}
		for i := range claimed {
			claimed[i].Attempts++
			if err := tx.Model(&outboxModel{}).Where("id = ?", claimed[i].ID).Updates(map[string]any{
				"attempts":        claimed[i].Attempts,
				"next_attempt_at": gorm.Expr("now() + make_interval(secs => ?)", outboxRetryDelay(claimed[i].Attempts).Seconds()),
			}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return 0, glerrors.Wrap(glerrors.KindInternal, "claiming student outbox events", err)
	}

	published := 0
	for _, row := range claimed {
		event := ports.OutboxEvent{TenantID: row.TenantID, Type: row.EventType, Payload: row.Payload}
		publishErr := publish(ctx, event)
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), outboxFinishTimeout)
		err := r.withTenant(finishCtx, func(tx *gorm.DB) error {
			updates := map[string]any{}
			if publishErr != nil {
				updates["next_attempt_at"] = gorm.Expr("now() + make_interval(secs => ?)", outboxRetryDelay(row.Attempts).Seconds())
			} else {
				updates["published_at"] = gorm.Expr("now()")
			}
			return tx.Model(&outboxModel{}).Where("id = ? AND published_at IS NULL", row.ID).Updates(updates).Error
		})
		cancel()
		if err != nil {
			return published, glerrors.Wrap(glerrors.KindInternal, "finishing student outbox event", err)
		}
		if publishErr == nil {
			published++
		}
	}
	return published, nil
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
		student, err := lockStudentForEnrollment(tx, e.StudentID())
		if err != nil {
			return err
		}
		if err := refuseTerminalAdmission(student); err != nil {
			return err
		}
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

// lockStudentForEnrollment serializes placement creation with withdrawal. Both
// paths lock the student first; the admission state is therefore authoritative
// when it is checked before the enrollment insert.
func lockStudentForEnrollment(tx *gorm.DB, studentID string) (studentModel, error) {
	var student studentModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", studentID).First(&student).Error; err != nil {
		return studentModel{}, translateFindErr(err)
	}
	return student, nil
}

func refuseTerminalAdmission(student studentModel) error {
	status, err := domain.ParseAdmissionStatus(student.AdmissionStatus)
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "reading student admission status", err)
	}
	if status.IsTerminal() {
		return glerrors.New(glerrors.KindConflict, "student admission is terminal")
	}
	return nil
}

// reserveSeat takes the group's seat lock and refuses a group with no free
// seat — the rule a transfer and a placement in a chosen group share, under
// the same per-group lock the by-grade path takes, so two placements into the
// last seat cannot both win. A group nobody sized is refused as the by-grade
// path refuses it, but saying so, so the director fixes the size instead of
// hunting for seats that were never counted.
func reserveSeat(tx *gorm.DB, g ports.GroupSeats) error {
	if g.Capacity <= 0 {
		return glerrors.New(glerrors.KindConflict, "ese grupo no tiene cupo definido: fíjalo en Configuración")
	}
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, g.ID).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "reserving the group", err)
	}
	var taken int64
	if err := tx.Model(&enrollmentModel{}).
		Where("group_id = ? AND status = ?", g.ID, domain.EnrollmentStatusActive.String()).
		Count(&taken).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "counting the group", err)
	}
	if taken >= int64(g.Capacity) {
		return glerrors.New(glerrors.KindConflict, "ese grupo ya no tiene lugares")
	}
	return nil
}
