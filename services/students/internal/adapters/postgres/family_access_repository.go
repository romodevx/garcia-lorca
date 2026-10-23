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

// Compile-time proof the adapter satisfies the family access port (ADR 0041).
var _ ports.FamilyAccessRepository = (*Repository)(nil)

// maxFamilyLockAttempts bounds OnPupil's retries when the pupil's family
// changes between the unlocked read and the locked re-read. Each retry means
// another clerk moved this very pupil meanwhile; three in a row is a conflict
// to retry by hand, not a loop.
const maxFamilyLockAttempts = 3

// errFamilyMoved: the pupil's family changed while its locks were taken.
var errFamilyMoved = errors.New("the pupil's family changed while locking")

// OnPupil runs fn under the family locking rule (ports.FamilyRepository):
// family_id read unlocked → families FOR UPDATE → students FOR SHARE →
// retried in a fresh transaction when family_id changed in between. Lock order
// family → pupil is the same AddStudentsToFamily, RemoveStudentFromFamily and
// DissolveFamily take, so they queue instead of deadlocking.
func (r *Repository) OnPupil(ctx context.Context, studentID string, fn ports.FamilyAccessFunc) error {
	for range maxFamilyLockAttempts {
		err := r.withTenant(ctx, func(tx *gorm.DB) error {
			familyID, err := pupilFamily(tx, studentID, "")
			if err != nil {
				return err
			}
			if familyID != "" {
				if err := lockFamily(tx, familyID); err != nil {
					if glerrors.KindOf(err) == glerrors.KindNotFound {
						return errFamilyMoved // dissolved meanwhile
					}
					return err
				}
			}
			locked, err := pupilFamily(tx, studentID, " FOR SHARE")
			if err != nil {
				return err
			}
			if locked != familyID {
				return errFamilyMoved
			}
			return fn(ctx, &familyAccessTx{tx: tx, familyID: familyID, studentID: studentID})
		})
		if !errors.Is(err, errFamilyMoved) {
			return err
		}
	}
	return glerrors.New(glerrors.KindConflict, "the pupil's family kept changing; try again")
}

// OnFamily runs fn with the family's row locked FOR UPDATE. Unknown (or
// another tenant's) family: KindNotFound.
func (r *Repository) OnFamily(ctx context.Context, familyID string, fn ports.FamilyAccessFunc) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := lockFamily(tx, familyID); err != nil {
			return err
		}
		return fn(ctx, &familyAccessTx{tx: tx, familyID: familyID})
	})
}

// runSync runs a membership write's sync over the family inside its tx.
func runSync(ctx context.Context, tx *gorm.DB, familyID string, sync ports.FamilyAccessFunc) error {
	if sync == nil {
		return nil
	}
	return sync(ctx, &familyAccessTx{tx: tx, familyID: familyID})
}

// pupilFamily reads a pupil's family_id ("" for none) with an optional lock
// clause. An invisible pupil is KindNotFound.
func pupilFamily(tx *gorm.DB, studentID, lock string) (string, error) {
	var rows []string
	err := tx.Raw(`SELECT COALESCE(family_id::text, '') FROM students WHERE id = ?`+lock, studentID).Scan(&rows).Error
	if err != nil {
		return "", translateFamilyWriteErr(err, "reading the pupil's family")
	}
	if len(rows) == 0 {
		return "", glerrors.ErrNotFound
	}
	return rows[0], nil
}

// familyAccessTx is ports.FamilyAccess over one transaction. Its scope is the
// family (familyID) or, when that is empty, the single pupil studentID.
type familyAccessTx struct {
	tx        *gorm.DB
	familyID  string
	studentID string
}

func (a *familyAccessTx) FamilyID() string { return a.familyID }

// scope is the students predicate of the transaction's pupils and its value.
func (a *familyAccessTx) scope() (string, string) {
	if a.familyID != "" {
		return "family_id = ?", a.familyID
	}
	return "id = ?", a.studentID
}

// Members reads the scope's pupils (id order) and all their contacts (id
// order) in two statements.
func (a *familyAccessTx) Members(context.Context) ([]domain.FamilyMember, error) {
	where, value := a.scope()
	var pupils []struct {
		ID              string
		AdmissionStatus string
	}
	err := a.tx.Raw(`SELECT id, admission_status FROM students WHERE `+where+` ORDER BY id`, value).Scan(&pupils).Error
	if err != nil {
		return nil, translateFamilyWriteErr(err, "reading the family's pupils")
	}
	if len(pupils) == 0 {
		return nil, nil
	}
	ids := make([]string, len(pupils))
	for i, p := range pupils {
		ids[i] = p.ID
	}
	var rows []guardianStudentModel
	if err := a.tx.Where("student_id IN ?", ids).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, translateFamilyWriteErr(err, "reading the family's contacts")
	}
	contacts := make(map[string][]domain.GuardianLink, len(pupils))
	for _, m := range rows {
		l, err := guardianLinkToDomain(m)
		if err != nil {
			return nil, err
		}
		contacts[m.StudentID] = append(contacts[m.StudentID], l)
	}
	out := make([]domain.FamilyMember, len(pupils))
	for i, p := range pupils {
		status, err := domain.ParseAdmissionStatus(p.AdmissionStatus)
		if err != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "unknown admission status in db", err)
		}
		out[i] = domain.FamilyMember{StudentID: p.ID, Status: status, Contacts: contacts[p.ID]}
	}
	return out, nil
}

// inScope refuses a write on a pupil outside the transaction's scope: the
// locks taken cover only those pupils.
func (a *familyAccessTx) inScope(studentID string) error {
	where, value := a.scope()
	var n int64
	err := a.tx.Raw(`SELECT count(*) FROM students WHERE id = ? AND `+where, studentID, value).Scan(&n).Error
	if err != nil {
		return translateFamilyWriteErr(err, "checking the pupil's family")
	}
	if n == 0 {
		return glerrors.New(glerrors.KindInternal, "an account write outside the locked family")
	}
	return nil
}

func (a *familyAccessTx) AttachAccount(_ context.Context, l domain.GuardianLink) error {
	if err := a.inScope(l.StudentID()); err != nil {
		return err
	}
	return attachAccount(a.tx, l)
}

func (a *familyAccessTx) RemoveLink(_ context.Context, guardianUserID, studentID string) error {
	if err := a.inScope(studentID); err != nil {
		return err
	}
	return unlink(a.tx, guardianUserID, studentID)
}

func (a *familyAccessTx) InsertContact(_ context.Context, l domain.GuardianLink) error {
	if err := a.inScope(l.StudentID()); err != nil {
		return err
	}
	model := guardianLinkFromDomain(l)
	if err := a.tx.Create(&model).Error; err != nil {
		return translateAccessErr(err)
	}
	return nil
}

// DetachAccount deletes the scope's links of guardianUserID that would be
// nobody without it (no name: guardian_students_is_somebody_ck, and a blank
// name the domain cannot read back either), then nulls the account on the
// rest. Two statements, one transaction.
func (a *familyAccessTx) DetachAccount(_ context.Context, guardianUserID string) (int, error) {
	where, value := a.scope()
	members := `student_id IN (SELECT id FROM students WHERE ` + where + `)`
	deleted := a.tx.Exec(`DELETE FROM guardian_students WHERE guardian_user_id = ? AND btrim(full_name) = '' AND `+members,
		guardianUserID, value)
	if deleted.Error != nil {
		return 0, translateAccessErr(deleted.Error)
	}
	detached := a.tx.Exec(`UPDATE guardian_students SET guardian_user_id = NULL WHERE guardian_user_id = ? AND `+members,
		guardianUserID, value)
	if detached.Error != nil {
		return 0, translateAccessErr(detached.Error)
	}
	return int(deleted.RowsAffected + detached.RowsAffected), nil
}

// translateAccessErr maps an account write's failure: a lost race (deadlock,
// serialization) is a conflict to retry, the rest as a guardian write.
func translateAccessErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == deadlockDetected || pgErr.Code == serializationFailure) {
		return translateFamilyWriteErr(err, "")
	}
	return translateGuardianWriteErr(err)
}
