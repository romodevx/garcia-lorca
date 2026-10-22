package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter satisfies the import port.
var _ ports.ImportRepository = (*Repository)(nil)

// StudentsByCURP returns the tenant's pupils holding any of the CURPs, with the
// groups they are actively placed in — one query for a whole import.
func (r *Repository) StudentsByCURP(ctx context.Context, curps []string) (map[string]ports.ExistingPupil, error) {
	out := make(map[string]ports.ExistingPupil, len(curps))
	if len(curps) == 0 {
		return out, nil
	}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []struct {
			ID      string
			CURP    string
			GroupID *string
		}
		err := tx.Raw(`SELECT s.id, s.curp, e.group_id
		                 FROM students s
		                 LEFT JOIN enrollments e ON e.student_id = s.id AND e.status = ?
		                WHERE s.curp IN ?`, domain.EnrollmentStatusActive.String(), curps).
			Scan(&rows).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading pupils by CURP", err)
		}
		for _, row := range rows {
			pupil := out[row.CURP]
			pupil.StudentID = row.ID
			if row.GroupID != nil {
				pupil.ActiveGroupIDs = append(pupil.ActiveGroupIDs, *row.GroupID)
			}
			out[row.CURP] = pupil
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BankReferencesTaken returns which of the references a pupil of the tenant
// already holds.
func (r *Repository) BankReferencesTaken(ctx context.Context, refs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(refs))
	if len(refs) == 0 {
		return out, nil
	}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var taken []string
		if err := tx.Model(&studentModel{}).Where("bank_reference IN ?", refs).Pluck("bank_reference", &taken).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading bank references", err)
		}
		for _, ref := range taken {
			out[ref] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DraftedPupils returns every pupil of the tenant an import stored as a draft
// — a curp_missing/curp_invalid issue, open or resolved — whatever their
// status now, with what a later upload recognizes them by. One query for a
// whole import.
func (r *Repository) DraftedPupils(ctx context.Context) ([]ports.ExistingDraft, error) {
	var out []ports.ExistingDraft
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []struct {
			ID               string
			FirstName        string
			LastName         string
			MaternalLastName *string
			BirthDate        *time.Time
			BankReference    *string
			IntendedGroupID  *string
			AdmissionStatus  string
		}
		err := tx.Raw(`SELECT s.id, s.first_name, s.last_name, s.maternal_last_name, s.birth_date, s.bank_reference,
		                      s.admission_status, i.intended_group_id
		                 FROM students s
		                 JOIN student_import_issues i
		                   ON i.tenant_id = s.tenant_id AND i.student_id = s.id
		                  AND i.field = ? AND i.code IN ?`,
			string(domain.FieldCURP), []string{domain.IssueCURPMissing, domain.IssueCURPInvalid}).Scan(&rows).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading the drafted pupils", err)
		}
		out = make([]ports.ExistingDraft, 0, len(rows))
		for _, row := range rows {
			d := ports.ExistingDraft{
				StudentID: row.ID, FirstName: row.FirstName, LastName: row.LastName,
				MaternalLastName: derefOrEmpty(row.MaternalLastName), BankReference: derefOrEmpty(row.BankReference),
				IntendedGroupID: derefOrEmpty(row.IntendedGroupID),
				StillDraft:      row.AdmissionStatus == domain.AdmissionDraft.String(),
			}
			if row.BirthDate != nil {
				d.BirthDate = *row.BirthDate
			}
			out = append(out, d)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ImportStudent stores one imported pupil and their enrollment in ONE
// transaction (ADR 0033).
//
// The bank reference is checked INSIDE the transaction before the insert: a
// unique violation would abort the transaction and cost the pupil, while a
// taken reference must only cost the reference. Under RLS the check sees the
// tenant's pupils, which is exactly the uniqueness scope.
//
// Group capacity is deliberately not counted (unlike the by-grade and transfer
// paths): the import sized its groups in config before committing, and a
// school migrating its real classes must not lose a pupil to a seat count.
// Nothing is published — students has no outbox, and the import holds no
// publisher.
func (r *Repository) ImportStudent(ctx context.Context, cmd ports.ImportCommand) (ports.ImportedPupil, error) {
	model, err := withProfile(fromDomain(cmd.Student), cmd.Student.Profile())
	if err != nil {
		return ports.ImportedPupil{}, err
	}
	model.ImportBatchID = nullableID(cmd.BatchID)
	enrollment := enrollmentFromDomain(cmd.Enrollment)

	var out ports.ImportedPupil
	err = r.withTenant(ctx, func(tx *gorm.DB) error {
		out = ports.ImportedPupil{}
		if model.BankReference != nil {
			taken, err := referenceTaken(tx, *model.BankReference)
			if err != nil {
				return err
			}
			if taken {
				model.BankReference = nil
				out.BankReferenceDropped = true
			}
		}
		number, err := r.nextEnrollmentNumber(tx, cmd.NumberYear)
		if err != nil {
			return err
		}
		model.EnrollmentNumber = number
		if err := tx.Create(&model).Error; err != nil {
			return translateStudentWriteErr(err, "importing student")
		}
		// A draft is stored without an enrollment: it is not placed until its
		// CURP is captured.
		if !cmd.Student.IsDraft() {
			if err := tx.Create(&enrollment).Error; err != nil {
				return translateEnrollmentWriteErr(err)
			}
		}
		if err := insertIssues(tx, issuesToStore(cmd, out.BankReferenceDropped)); err != nil {
			return err
		}
		out.EnrollmentNumber = number
		return nil
	})
	if err != nil {
		return ports.ImportedPupil{}, err
	}
	cmd.Student.AssignEnrollmentNumber(out.EnrollmentNumber)
	if out.BankReferenceDropped {
		cmd.Student.ChangeBankReference("")
	}
	return out, nil
}

// issuesToStore is the command's issues, plus the taken reference's when the
// store dropped it.
func issuesToStore(cmd ports.ImportCommand, referenceDropped bool) []domain.ImportIssue {
	issues := append([]domain.ImportIssue(nil), cmd.Issues...)
	if referenceDropped && cmd.ReferenceTakenIssue != nil {
		issues = append(issues, *cmd.ReferenceTakenIssue)
	}
	return issues
}

// insertIssues stores the import issues of a pupil inside its transaction.
func insertIssues(tx *gorm.DB, issues []domain.ImportIssue) error {
	if len(issues) == 0 {
		return nil
	}
	rows := make([]importIssueModel, 0, len(issues))
	for _, i := range issues {
		rows = append(rows, importIssueFromDomain(i))
	}
	if err := tx.Create(&rows).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "storing the import issues", err)
	}
	return nil
}

func referenceTaken(tx *gorm.DB, ref string) (bool, error) {
	var taken bool
	if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM students WHERE bank_reference = ?)`, ref).Scan(&taken).Error; err != nil {
		return false, glerrors.Wrap(glerrors.KindInternal, "checking the bank reference", err)
	}
	return taken, nil
}

// movableImports is the predicate of an earlier import to move back to the
// cycle's first day: the pupil's import enrollment in that cycle, dated after
// it. Whatever its status — only enrolled_at changes, never withdrawn_on.
const movableImports = `tenant_id = current_setting('app.current_tenant', true)::uuid
	AND cycle_id = ? AND origin = ? AND enrolled_at > ? AND student_id IN ?`

// MoveImportedToCycleStart moves (or, dry, lists) the earlier imports of the
// pupils back to the first day of the cycle in ONE statement, and returns the
// pupils it moved. The predicate is the idempotency: a moved enrollment no
// longer matches, so a second run moves nobody.
func (r *Repository) MoveImportedToCycleStart(ctx context.Context, move ports.CycleStartMove) ([]string, error) {
	if len(move.StudentIDs) == 0 {
		return nil, nil
	}
	args := []any{move.CycleID, string(domain.EnrollmentOriginImport), move.StartsAt, move.StudentIDs}
	var moved []string
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		query := `SELECT student_id FROM enrollments WHERE ` + movableImports
		if !move.DryRun {
			// updated_at only moves on an ACTIVE row: a non-active one's
			// updated_at is the instant it ended (enrollmentToDomain).
			query = `UPDATE enrollments SET enrolled_at = ?,
			            updated_at = CASE WHEN status = 'active' THEN now() ELSE updated_at END
			          WHERE ` + movableImports + ` RETURNING student_id`
			args = append([]any{move.StartsAt}, args...)
		}
		if err := tx.Raw(query, args...).Scan(&moved).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "moving imports to the cycle start", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return moved, nil
}
