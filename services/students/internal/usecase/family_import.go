package usecase

import (
	"context"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Families from an import (PLAN-familias-importacion.md, ADR 0041 amendment):
// confirming an import builds the very likely families on its own, and the
// summary offers to undo it. Who may call it (the import is the owner's) and
// module_families are the gateway's call.

// ImportFamiliesResult is what BuildFamiliesFromImport did: the families it
// created, the existing ones it added pupils to (as they now are), and how
// many groups it left to the office (they stay suggestions).
type ImportFamiliesResult struct {
	Created []domain.Family
	Joined  []domain.Family
	Skipped int
}

// errImportMeetsAccount refuses, inside the write, a family the import would
// build with an app account on it: the plan saw none, but one arrived before
// the write. Building it would hand that account the siblings (security H1).
var errImportMeetsAccount = glerrors.New(glerrors.KindConflict,
	"an app account reached the family; it is left to the office")

// BuildFamiliesFromImport builds the families domain.PlanImportFamilies finds
// for the import batchID (malformed: KindInvalid). Each family is written
// through the same guarded path as CreateFamily or AddStudentsToFamily (the
// locks, the account cap, the sync), and in that same transaction the import
// records which pupils it put there (for the undo) and re-checks that no app
// account is on the family.
//
// The plan is read without locks, so the office may have moved a pupil
// meanwhile: a family whose write is refused (a conflict, FAMILY_ACCOUNT_LIMIT,
// FAMILY_BUSY, a pupil that left, a family gone or full) is skipped and
// counted, and the rest still runs. Any other failure stops the build and is
// returned; the families written before it stay, recorded, so the undo still
// takes them back. Idempotent: a second call finds everyone placed and builds
// nothing new.
func (s *FamilyService) BuildFamiliesFromImport(ctx context.Context, batchID string) (ImportFamiliesResult, error) {
	batch, err := canonicalID(batchID, "batch id")
	if err != nil {
		return ImportFamiliesResult{}, err
	}
	in, err := s.families.ImportFamilyInputs(ctx, batch)
	if err != nil {
		return ImportFamiliesResult{}, err
	}
	plan := domain.PlanImportFamilies(in)
	out := ImportFamiliesResult{Skipped: plan.Skipped}
	for _, ids := range plan.Create {
		f, err := s.createFamily(ctx, "", ids, s.importHook(batch, ids, true))
		switch {
		case lostToTheOffice(err):
			out.Skipped++
		case err != nil:
			return ImportFamiliesResult{}, err
		default:
			out.Created = append(out.Created, f)
		}
	}
	for _, j := range plan.Join {
		f, err := s.addStudentsToFamily(ctx, j.FamilyID, j.StudentIDs, s.importHook(batch, j.StudentIDs, false))
		switch {
		case lostToTheOffice(err):
			out.Skipped++
		case err != nil:
			return ImportFamiliesResult{}, err
		default:
			out.Joined = append(out.Joined, f)
		}
	}
	return out, nil
}

// UndoFamiliesFromImport takes back what BuildFamiliesFromImport did for
// batchID (malformed: KindInvalid): each pupil the import put in a family and
// still in it leaves it, and a family the import created left with fewer
// than two pupils is dissolved (domain.ImportUndoDissolves). A pupil the
// office moved meanwhile is not touched; guardian links never are.
// Idempotent: a second call does nothing.
func (s *FamilyService) UndoFamiliesFromImport(ctx context.Context, batchID string) (removed, dissolved int, err error) {
	batch, err := canonicalID(batchID, "batch id")
	if err != nil {
		return 0, 0, err
	}
	undo, err := s.families.UndoImportFamilies(ctx, batch)
	if err != nil {
		return 0, 0, err
	}
	return undo.Removed, undo.Dissolved, nil
}

// importHook runs in a family write's transaction, after its pupils are
// claimed: it refuses the write when any member of the family now has an app
// account (errImportMeetsAccount), then records the import's links of ids.
func (s *FamilyService) importHook(batch string, ids []string, created bool) ports.FamilyAccessFunc {
	return func(ctx context.Context, a ports.FamilyAccess) error {
		members, err := a.Members(ctx)
		if err != nil {
			return err
		}
		if anyAccountOn(members) {
			return errImportMeetsAccount
		}
		links := make([]ports.FamilyImportLink, len(ids))
		for i, id := range ids {
			links[i] = ports.FamilyImportLink{ID: s.newID(), BatchID: batch, StudentID: id, CreatedFamily: created}
		}
		return a.RecordImportLinks(ctx, links)
	}
}

func anyAccountOn(members []domain.FamilyMember) bool {
	for _, m := range members {
		for _, c := range m.Contacts {
			if c.HasAccount() {
				return true
			}
		}
	}
	return false
}

// lostToTheOffice reports a family write refused because the school changed
// since the plan was read: a conflict (a pupil placed meanwhile, the account
// cap, FAMILY_BUSY, a lost lock race), a pupil that can no longer join or a
// family that would outgrow its cap (invalid), or a family gone (not found).
func lostToTheOffice(err error) bool {
	switch glerrors.KindOf(err) {
	case glerrors.KindConflict, glerrors.KindInvalid, glerrors.KindNotFound:
		return true
	default:
		return false
	}
}

// chainAccess runs the non-nil fns in order in one transaction; the first
// error stops it (and rolls the write back).
func chainAccess(fns ...ports.FamilyAccessFunc) ports.FamilyAccessFunc {
	return func(ctx context.Context, a ports.FamilyAccess) error {
		for _, fn := range fns {
			if fn == nil {
				continue
			}
			if err := fn(ctx, a); err != nil {
				return err
			}
		}
		return nil
	}
}
