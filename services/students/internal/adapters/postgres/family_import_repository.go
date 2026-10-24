package postgres

import (
	"context"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Families from an import (PLAN-familias-importacion.md, migration 0024).

// maxUndoLinks bounds one undo's read of a batch's links: an import brings a
// school's level, a few hundred pupils. A larger batch is undone in more than
// one call (each call deletes only the links it handled).
const maxUndoLinks = maxSuggestionPupils

// ImportFamilyInputs reads what domain.PlanImportFamilies needs for batchID in
// one transaction and six bounded statements: every pupil still at school,
// their contacts' and their files' emails and phones, every family's counts,
// the dismissed signatures and the common values.
func (r *Repository) ImportFamilyInputs(ctx context.Context, batchID string) (domain.ImportFamilyInput, error) {
	var out domain.ImportFamilyInput
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if out.Pupils, err = importPupils(ctx, tx, batchID); err != nil {
			return err
		}
		if out.Contacts, err = pupilContacts(ctx, tx, atSchool); err != nil {
			return err
		}
		if out.Families, err = importFamilyStates(ctx, tx); err != nil {
			return err
		}
		if out.Dismissed, err = dismissedSignatures(tx); err != nil {
			return err
		}
		out.Common, err = commonContactValues(ctx, tx)
		return err
	})
	if err != nil {
		return domain.ImportFamilyInput{}, err
	}
	return out, nil
}

// importPupils reads every pupil still at school in display order, with its
// family, whether an account is linked to it and whether batchID created it.
func importPupils(ctx context.Context, tx *gorm.DB, batchID string) ([]domain.ImportPupil, error) {
	var pupils []domain.ImportPupil
	err := tx.Raw(`SELECT s.id, s.last_name, COALESCE(s.maternal_last_name, '') AS maternal_last_name,
	                      COALESCE(s.family_id::text, '') AS family_id,
	                      EXISTS (SELECT 1 FROM guardian_students g
	                               WHERE g.tenant_id = s.tenant_id AND g.student_id = s.id
	                                 AND g.guardian_user_id IS NOT NULL) AS has_account,
	                      COALESCE(s.import_batch_id = CAST(? AS uuid), false) AS in_batch
	                 FROM students s
	                WHERE `+atSchool+`
	                ORDER BY `+surnameOrder+` LIMIT ?`,
		batchID, leftSchoolStatuses, maxSuggestionPupils+1).Scan(&pupils).Error
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the import's pupils", err)
	}
	pupils, hit := capRows(pupils, maxSuggestionPupils)
	if hit {
		warnCeiling(ctx, "import pupils", maxSuggestionPupils)
	}
	return pupils, nil
}

// importFamilyStates reads every family's member count (any status) and
// distinct account count. A family past the ceiling is missing from the map,
// and the plan never joins a family it does not know.
func importFamilyStates(ctx context.Context, tx *gorm.DB) (map[string]domain.ImportFamilyState, error) {
	var rows []struct {
		ID       string
		Pupils   int
		Accounts int
	}
	err := tx.Raw(`SELECT f.id,
	                      (SELECT count(*) FROM students s WHERE s.tenant_id = f.tenant_id AND s.family_id = f.id) AS pupils,
	                      (SELECT count(DISTINCT g.guardian_user_id)
	                         FROM guardian_students g
	                         JOIN students s ON s.id = g.student_id AND s.tenant_id = g.tenant_id
	                        WHERE s.tenant_id = f.tenant_id AND s.family_id = f.id) AS accounts
	                 FROM families f
	                ORDER BY f.id LIMIT ?`, maxFamiliesListed+1).Scan(&rows).Error
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the families' counts", err)
	}
	rows, hit := capRows(rows, maxFamiliesListed)
	if hit {
		warnCeiling(ctx, "import families", maxFamiliesListed)
	}
	out := make(map[string]domain.ImportFamilyState, len(rows))
	for _, row := range rows {
		out[row.ID] = domain.ImportFamilyState{Pupils: row.Pupils, Accounts: row.Accounts}
	}
	return out, nil
}

// importLinkRow is one family_import_links row as an undo reads it.
type importLinkRow struct {
	FamilyID      string
	StudentID     string
	CreatedFamily bool
}

// UndoImportFamilies undoes the batch in ONE transaction, family by family in
// id order, each under its row lock (the lock every membership write takes,
// then the pupils' rows by the UPDATE, as RemoveStudentFromFamily does).
func (r *Repository) UndoImportFamilies(ctx context.Context, batchID string) (ports.ImportUndo, error) {
	var out ports.ImportUndo
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var links []importLinkRow
		err := tx.Raw(`SELECT family_id, student_id, created_family FROM family_import_links
		                WHERE batch_id = ? ORDER BY family_id, student_id LIMIT ?`,
			batchID, maxUndoLinks+1).Scan(&links).Error
		if err != nil {
			return translateFamilyWriteErr(err, "reading the import's family links")
		}
		links, hit := capRows(links, maxUndoLinks)
		if hit {
			warnCeiling(ctx, "import family links", maxUndoLinks)
		}
		for _, family := range linksByFamily(links) {
			if err := undoImportFamily(tx, batchID, family, &out); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ports.ImportUndo{}, err
	}
	return out, nil
}

// importFamilyLinks is one family's links of a batch.
type importFamilyLinks struct {
	familyID   string
	studentIDs []string
	created    bool
}

// linksByFamily groups links already ordered by family id.
func linksByFamily(links []importLinkRow) []importFamilyLinks {
	var out []importFamilyLinks
	for _, l := range links {
		if n := len(out); n == 0 || out[n-1].familyID != l.FamilyID {
			out = append(out, importFamilyLinks{familyID: l.FamilyID})
		}
		last := &out[len(out)-1]
		last.studentIDs = append(last.studentIDs, l.StudentID)
		last.created = last.created || l.CreatedFamily
	}
	return out
}

// undoImportFamily takes the batch's pupils still in the family out of it,
// dissolves it when domain.ImportUndoDissolves says so (the cascade then
// deletes its links) and otherwise deletes the batch's links to it.
func undoImportFamily(tx *gorm.DB, batchID string, f importFamilyLinks, out *ports.ImportUndo) error {
	if err := lockFamily(tx, f.familyID); err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return nil // dissolved since the links were read: nothing left to undo
		}
		return err
	}
	res := tx.Exec(`UPDATE students SET family_id = NULL WHERE family_id = ? AND id IN ?`, f.familyID, f.studentIDs)
	if res.Error != nil {
		return translateFamilyWriteErr(res.Error, "taking the import's pupils out of the family")
	}
	out.Removed += int(res.RowsAffected)
	left, err := countMembers(tx, f.familyID)
	if err != nil {
		return err
	}
	if domain.ImportUndoDissolves(f.created, int(left)) {
		if err := tx.Exec(`UPDATE students SET family_id = NULL WHERE family_id = ?`, f.familyID).Error; err != nil {
			return translateFamilyWriteErr(err, "clearing the family's members")
		}
		out.Dissolved++
		return deleteFamily(tx, f.familyID)
	}
	err = tx.Exec(`DELETE FROM family_import_links WHERE batch_id = ? AND family_id = ?`, batchID, f.familyID).Error
	if err != nil {
		return translateFamilyWriteErr(err, "deleting the import's family links")
	}
	return nil
}

// RecordImportLinks inserts the links on the transaction's family, its
// tenant taken from the family row. A (batch, pupil) already recorded is
// kept (ON CONFLICT DO NOTHING).
func (a *familyAccessTx) RecordImportLinks(_ context.Context, links []ports.FamilyImportLink) error {
	if a.familyID == "" {
		return glerrors.New(glerrors.KindInternal, "an import link outside a family")
	}
	for _, l := range links {
		if err := a.inScope(l.StudentID); err != nil {
			return err
		}
		err := a.tx.Exec(`INSERT INTO family_import_links (id, tenant_id, batch_id, family_id, student_id, created_family)
		                  SELECT ?, f.tenant_id, ?, f.id, ?, ? FROM families f WHERE f.id = ?
		                  ON CONFLICT (tenant_id, batch_id, student_id) DO NOTHING`,
			l.ID, l.BatchID, l.StudentID, l.CreatedFamily, a.familyID).Error
		if err != nil {
			return translateFamilyWriteErr(err, "recording the import's family link")
		}
	}
	return nil
}
