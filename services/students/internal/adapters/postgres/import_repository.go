package postgres

import (
	"context"

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
		number, err := r.nextEnrollmentNumber(tx)
		if err != nil {
			return err
		}
		model.EnrollmentNumber = number
		if err := tx.Create(&model).Error; err != nil {
			return translateStudentWriteErr(err, "importing student")
		}
		if err := tx.Create(&enrollment).Error; err != nil {
			return translateEnrollmentWriteErr(err)
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

func referenceTaken(tx *gorm.DB, ref string) (bool, error) {
	var taken bool
	if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM students WHERE bank_reference = ?)`, ref).Scan(&taken).Error; err != nil {
		return false, glerrors.Wrap(glerrors.KindInternal, "checking the bank reference", err)
	}
	return taken, nil
}
