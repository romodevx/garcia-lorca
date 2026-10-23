package postgres

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter satisfies the families port (ADR 0041).
var _ ports.FamilyRepository = (*Repository)(nil)

// leftSchoolStatuses are the admission statuses that can no longer join a
// family nor be suggested into one.
var leftSchoolStatuses = []string{domain.AdmissionRejected.String(), domain.AdmissionWithdrawn.String()}

const (
	// The read ceilings. A school has ~600 pupils, a few contacts each and a
	// few hundred families; the bounds only keep a runaway table from loading
	// unbounded rows into memory. Hitting one is logged (counts only).
	maxSuggestionPupils   = 5000
	maxSuggestionContacts = 20000
	maxCommonValues       = 1000
	maxFamiliesListed     = 1000
	maxMembersListed      = maxFamiliesListed * domain.MaxFamilyPupils

	// Lost races between two writers: answered as a conflict to retry.
	deadlockDetected     = "40P01"
	serializationFailure = "40001"
)

// capRows keeps at most limit rows of a read that asked for limit+1, and says
// whether the ceiling was hit.
func capRows[T any](rows []T, limit int) ([]T, bool) {
	if len(rows) > limit {
		return rows[:limit], true
	}
	return rows, false
}

// warnCeiling logs that a bounded read was cut short. Counts only, no PII.
func warnCeiling(ctx context.Context, read string, ceiling int) {
	slog.WarnContext(ctx, "family read hit its ceiling; result truncated", "read", read, "ceiling", ceiling)
}

// familyRow is one family as read, with its account count computed in the
// same statement.
type familyRow struct {
	ID           string
	TenantID     string
	Name         string
	CreatedAt    time.Time
	AccountCount int
}

// memberRow is one pupil's membership.
type memberRow struct {
	ID       string
	FamilyID string
}

// familiesSelect reads families with account_count: distinct app accounts
// linked to the members (NULL guardian_user_id, an account-less contact, is
// not counted by count(DISTINCT)).
const familiesSelect = `SELECT f.id, f.tenant_id, f.name, f.created_at,
	(SELECT count(DISTINCT g.guardian_user_id)
	   FROM guardian_students g
	   JOIN students s ON s.id = g.student_id AND s.tenant_id = g.tenant_id
	  WHERE s.tenant_id = f.tenant_id AND s.family_id = f.id) AS account_count
	FROM families f`

// familyOrder is the name order of the list; id makes it total.
const familyOrder = ` ORDER BY f.name COLLATE es_primary, f.id`

// ListFamilies returns every family ordered by name (up to
// maxFamiliesListed), in two statements: the families (with their account
// counts) and every membership.
func (r *Repository) ListFamilies(ctx context.Context) ([]domain.Family, error) {
	var out []domain.Family
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []familyRow
		if err := tx.Raw(familiesSelect+familyOrder+` LIMIT ?`, maxFamiliesListed+1).Scan(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing families", err)
		}
		rows, hit := capRows(rows, maxFamiliesListed)
		if hit {
			warnCeiling(ctx, "families", maxFamiliesListed)
		}
		var members []memberRow
		err := tx.Raw(`SELECT id, family_id FROM students WHERE family_id IS NOT NULL ORDER BY `+surnameOrder+` LIMIT ?`,
			maxMembersListed+1).Scan(&members).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing family members", err)
		}
		if members, hit = capRows(members, maxMembersListed); hit {
			warnCeiling(ctx, "family members", maxMembersListed)
		}
		out = assembleFamilies(rows, members)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FamilyByID returns one family, or KindNotFound (also for another tenant's).
func (r *Repository) FamilyByID(ctx context.Context, id string) (domain.Family, error) {
	var out domain.Family
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		f, err := readFamily(tx, id)
		out = f
		return err
	})
	if err != nil {
		return domain.Family{}, err
	}
	return out, nil
}

func readFamily(tx *gorm.DB, id string) (domain.Family, error) {
	var rows []familyRow
	if err := tx.Raw(familiesSelect+` WHERE f.id = ?`, id).Scan(&rows).Error; err != nil {
		return domain.Family{}, glerrors.Wrap(glerrors.KindInternal, "reading family", err)
	}
	if len(rows) == 0 {
		return domain.Family{}, glerrors.ErrNotFound
	}
	var members []memberRow
	err := tx.Raw(`SELECT id, family_id FROM students WHERE family_id = ? ORDER BY `+surnameOrder, id).
		Scan(&members).Error
	if err != nil {
		return domain.Family{}, glerrors.Wrap(glerrors.KindInternal, "reading family members", err)
	}
	return assembleFamilies(rows, members)[0], nil
}

// assembleFamilies joins the memberships (already in display order) to their
// families, keeping the families' order.
func assembleFamilies(rows []familyRow, members []memberRow) []domain.Family {
	byFamily := make(map[string][]string, len(rows))
	for _, m := range members {
		byFamily[m.FamilyID] = append(byFamily[m.FamilyID], m.ID)
	}
	out := make([]domain.Family, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RestoreFamily(domain.FamilyParams{
			ID: row.ID, TenantID: row.TenantID, Name: row.Name, StudentIDs: byFamily[row.ID],
			AccountCount: row.AccountCount, CreatedAt: row.CreatedAt,
		}))
	}
	return out
}

// CreateFamily inserts the family and claims its pupils in ONE transaction.
// The claim is conditional (claimPupils): a pupil that joined another family
// or left the school since the use case read it makes the whole create a
// KindConflict, rolled back.
func (r *Repository) CreateFamily(ctx context.Context, f domain.Family) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		err := tx.Exec(`INSERT INTO families (id, tenant_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, now())`,
			f.ID(), f.TenantID(), f.Name(), f.CreatedAt()).Error
		if err != nil {
			return translateFamilyWriteErr(err, "creating family")
		}
		return claimPupils(tx, f.ID(), f.StudentIDs())
	})
}

// RenameFamily sets a family's name. Unknown (or invisible) is KindNotFound.
func (r *Repository) RenameFamily(ctx context.Context, id, name string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE families SET name = ?, updated_at = now() WHERE id = ?`, name, id)
		if res.Error != nil {
			return translateFamilyWriteErr(res.Error, "renaming family")
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// AddStudentsToFamily claims pupils for an existing family under its row
// lock, and refuses (rolled back) a family that would outgrow
// domain.MaxFamilyPupils — two adds racing are serialised by the lock.
func (r *Repository) AddStudentsToFamily(ctx context.Context, familyID string, studentIDs []string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := lockFamily(tx, familyID); err != nil {
			return err
		}
		if err := claimPupils(tx, familyID, studentIDs); err != nil {
			return err
		}
		n, err := countMembers(tx, familyID)
		if err != nil {
			return err
		}
		if n > domain.MaxFamilyPupils {
			return glerrors.New(glerrors.KindInvalid, "a family has 1 to 20 pupils")
		}
		return nil
	})
}

// RemoveStudentFromFamily takes one pupil out under the family's row lock;
// the last one out deletes the family in the same transaction.
func (r *Repository) RemoveStudentFromFamily(ctx context.Context, familyID, studentID string) (bool, error) {
	var dissolved bool
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := lockFamily(tx, familyID); err != nil {
			return err
		}
		res := tx.Exec(`UPDATE students SET family_id = NULL WHERE id = ? AND family_id = ?`, studentID, familyID)
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "removing a pupil from the family", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		n, err := countMembers(tx, familyID)
		if err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		dissolved = true
		return deleteFamily(tx, familyID)
	})
	if err != nil {
		return false, err
	}
	return dissolved, nil
}

// DissolveFamily clears the members, then deletes the row (the composite FK
// is NO ACTION), under the family's lock, in one transaction.
func (r *Repository) DissolveFamily(ctx context.Context, id string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := lockFamily(tx, id); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE students SET family_id = NULL WHERE family_id = ?`, id).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "clearing the family's members", err)
		}
		return deleteFamily(tx, id)
	})
}

// SuggestionInputs reads, in one transaction and four bounded statements,
// the pupils without a family that may join one (display order), their
// contacts, the dismissed signatures and the school-wide common values.
func (r *Repository) SuggestionInputs(ctx context.Context) (ports.SuggestionInputs, error) {
	var out ports.SuggestionInputs
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if out.Pupils, err = suggestionPupils(ctx, tx); err != nil {
			return err
		}
		if out.Contacts, err = suggestionContacts(ctx, tx); err != nil {
			return err
		}
		if out.Dismissed, err = dismissedSignatures(tx); err != nil {
			return err
		}
		out.Common, err = commonContactValues(ctx, tx)
		return err
	})
	if err != nil {
		return ports.SuggestionInputs{}, err
	}
	return out, nil
}

func suggestionPupils(ctx context.Context, tx *gorm.DB) ([]domain.SuggestionPupil, error) {
	var pupils []domain.SuggestionPupil
	err := tx.Raw(`SELECT id, last_name, COALESCE(maternal_last_name, '') AS maternal_last_name
	                 FROM students
	                WHERE family_id IS NULL AND admission_status NOT IN ?
	                ORDER BY `+surnameOrder+` LIMIT ?`,
		leftSchoolStatuses, maxSuggestionPupils+1).Scan(&pupils).Error
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading suggestion pupils", err)
	}
	pupils, hit := capRows(pupils, maxSuggestionPupils)
	if hit {
		warnCeiling(ctx, "suggestion pupils", maxSuggestionPupils)
	}
	return pupils, nil
}

func suggestionContacts(ctx context.Context, tx *gorm.DB) ([]domain.SuggestionContact, error) {
	var contacts []domain.SuggestionContact
	err := tx.Raw(`SELECT g.student_id, g.email, g.phone, COALESCE(g.guardian_user_id::text, '') AS guardian_user_id
	                 FROM guardian_students g
	                 JOIN students s ON s.id = g.student_id AND s.tenant_id = g.tenant_id
	                WHERE s.family_id IS NULL AND s.admission_status NOT IN ?
	                ORDER BY g.id LIMIT ?`,
		leftSchoolStatuses, maxSuggestionContacts+1).Scan(&contacts).Error
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading suggestion contacts", err)
	}
	contacts, hit := capRows(contacts, maxSuggestionContacts)
	if hit {
		warnCeiling(ctx, "suggestion contacts", maxSuggestionContacts)
	}
	return contacts, nil
}

func dismissedSignatures(tx *gorm.DB) (map[string]bool, error) {
	var signatures []string
	if err := tx.Raw(`SELECT signature FROM family_suggestion_dismissals`).Scan(&signatures).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading dismissed suggestions", err)
	}
	out := make(map[string]bool, len(signatures))
	for _, sig := range signatures {
		out[sig] = true
	}
	return out, nil
}

// commonContactValuesSQL counts each contact value's distinct pupils over the
// WHOLE school still attending (any family or none) and keeps the ones past
// the cap. Its keys MUST match the domain's: lower(btrim(email)) =
// domain.ContactEmailKey, the phone's digits with at least
// domain.MinPhoneDigits = domain.ContactPhoneKey, the account id as text.
const commonContactValuesSQL = `
WITH attending AS (
	SELECT g.student_id, g.email, g.phone, g.guardian_user_id
	  FROM guardian_students g
	  JOIN students s ON s.id = g.student_id AND s.tenant_id = g.tenant_id
	 WHERE s.admission_status NOT IN ?
), keyed AS (
	SELECT student_id, 'email' AS kind, lower(btrim(email)) AS value FROM attending
	UNION ALL
	SELECT student_id, 'phone', regexp_replace(phone, '[^0-9]', '', 'g') FROM attending
	UNION ALL
	SELECT student_id, 'account', guardian_user_id::text FROM attending WHERE guardian_user_id IS NOT NULL
)
SELECT kind, value FROM keyed
 WHERE value <> '' AND (kind <> 'phone' OR length(value) >= ?)
 GROUP BY kind, value
HAVING count(DISTINCT student_id) > ?
 ORDER BY kind, value
 LIMIT ?`

func commonContactValues(ctx context.Context, tx *gorm.DB) (domain.CommonContactValues, error) {
	var rows []struct {
		Kind  string
		Value string
	}
	err := tx.Raw(commonContactValuesSQL, leftSchoolStatuses, domain.MinPhoneDigits,
		domain.MaxPupilsPerSharedContact, maxCommonValues+1).Scan(&rows).Error
	if err != nil {
		return domain.CommonContactValues{}, glerrors.Wrap(glerrors.KindInternal, "reading common contact values", err)
	}
	rows, hit := capRows(rows, maxCommonValues)
	if hit {
		warnCeiling(ctx, "common contact values", maxCommonValues)
	}
	out := domain.CommonContactValues{Emails: map[string]bool{}, Phones: map[string]bool{}, Accounts: map[string]bool{}}
	for _, row := range rows {
		switch row.Kind {
		case "email":
			out.Emails[row.Value] = true
		case "phone":
			out.Phones[row.Value] = true
		case "account":
			out.Accounts[row.Value] = true
		}
	}
	return out, nil
}

// DismissSuggestion records a dismissal once per (tenant, signature); a
// repeat keeps the first record.
func (r *Repository) DismissSuggestion(ctx context.Context, d ports.FamilyDismissal) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		err := tx.Exec(`INSERT INTO family_suggestion_dismissals (id, tenant_id, signature, dismissed_by)
		                VALUES (?, ?, ?, ?)
		                ON CONFLICT (tenant_id, signature) DO NOTHING`,
			d.ID, r.tenantID, d.Signature, nullableID(d.DismissedBy)).Error
		if err != nil {
			return translateFamilyWriteErr(err, "dismissing a family suggestion")
		}
		return nil
	})
}

// claimPupils points pupils at a family ONLY if each still has none and has
// not left the school — the condition rides in the WHERE, so two clerks
// claiming the same pupil cannot both win: the second waits on the row lock,
// re-reads family_id and matches nothing. Fewer rows than ids is
// KindConflict; the caller's transaction rolls back.
//
// Raw SQL, not GORM's Update: studentModel.FamilyID is read-only (`->`), and
// GORM skips non-updatable fields silently.
func claimPupils(tx *gorm.DB, familyID string, studentIDs []string) error {
	if err := lockPupils(tx, studentIDs); err != nil {
		return err
	}
	res := tx.Exec(`UPDATE students SET family_id = ?
	                 WHERE id IN ? AND family_id IS NULL AND admission_status NOT IN ?`,
		familyID, studentIDs, leftSchoolStatuses)
	if res.Error != nil {
		return translateFamilyWriteErr(res.Error, "adding pupils to the family")
	}
	if res.RowsAffected != int64(len(studentIDs)) {
		return glerrors.New(glerrors.KindConflict, "a pupil is already in a family or can no longer join one")
	}
	return nil
}

// lockPupils locks the pupils' rows FOR UPDATE in id order before they are
// claimed. A fixed order means a Create and an Add over overlapping pupils
// queue instead of deadlocking (the planner's order of a bare UPDATE ... IN
// is not fixed).
func lockPupils(tx *gorm.DB, studentIDs []string) error {
	var locked []string
	err := tx.Raw(`SELECT id FROM students WHERE id IN ? ORDER BY id FOR UPDATE`, studentIDs).Scan(&locked).Error
	if err != nil {
		return translateFamilyWriteErr(err, "locking the pupils")
	}
	return nil
}

// lockFamily holds the family row FOR UPDATE until the transaction ends: the
// one lock every membership write takes, which T2's account cap counts under.
// Unknown (or another tenant's) family: KindNotFound.
func lockFamily(tx *gorm.DB, id string) error {
	var ids []string
	if err := tx.Raw(`SELECT id FROM families WHERE id = ? FOR UPDATE`, id).Scan(&ids).Error; err != nil {
		return translateFamilyWriteErr(err, "locking the family")
	}
	if len(ids) == 0 {
		return glerrors.ErrNotFound
	}
	return nil
}

func countMembers(tx *gorm.DB, familyID string) (int64, error) {
	var n int64
	if err := tx.Raw(`SELECT count(*) FROM students WHERE family_id = ?`, familyID).Scan(&n).Error; err != nil {
		return 0, glerrors.Wrap(glerrors.KindInternal, "counting the family's members", err)
	}
	return n, nil
}

func deleteFamily(tx *gorm.DB, id string) error {
	if err := tx.Exec(`DELETE FROM families WHERE id = ?`, id).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "deleting the family", err)
	}
	return nil
}

// translateFamilyWriteErr maps failures of a family write: a CHECK (the
// name's length, the signature's shape) or a dangling FK is the caller's bad
// input; a deadlock or serialization failure is a lost race (KindConflict);
// anything else is internal.
func translateFamilyWriteErr(err error, doing string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case checkViolation:
			return glerrors.Wrap(glerrors.KindInvalid, "a value is not allowed", err)
		case foreignKeyViolation:
			return glerrors.Wrap(glerrors.KindInvalid, "the family does not exist", err)
		case uniqueViolation:
			return glerrors.Wrap(glerrors.KindConflict, "the family already exists", err)
		case deadlockDetected, serializationFailure:
			return glerrors.Wrap(glerrors.KindConflict, "another change to these pupils won; try again", err)
		}
	}
	return glerrors.Wrap(glerrors.KindInternal, doing, err)
}
