package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// FamilyService groups siblings into families (ADR 0041): confirm, rename,
// add and remove pupils, dissolve, and the suggestions the office confirms or
// dismisses, and the family's access (T2): one contact on every pupil, and
// revoking an account from all of them. Who may call each operation
// (students:read / students:update / guardians:write) and the module_families
// flag are the gateway's call. Confirming or growing a family syncs its
// accounts to every placeable pupil under the cap (syncFamilyAccess).
type FamilyService struct {
	students ports.StudentRepository
	families ports.FamilyRepository
	tenant   string
	newID    func() string
	now      func() time.Time
}

// NewFamilyService wires the use case. newID and now are injectable for
// deterministic tests.
func NewFamilyService(
	students ports.StudentRepository,
	families ports.FamilyRepository,
	tenant string,
	newID func() string,
	now func() time.Time,
) *FamilyService {
	return &FamilyService{students: students, families: families, tenant: tenant, newID: newID, now: now}
}

// minSuggestionPupils: a single pupil is never suggested, so never dismissed.
const minSuggestionPupils = 2

// ListFamilies returns every family of the school, ordered by name.
func (s *FamilyService) ListFamilies(ctx context.Context) ([]domain.Family, error) {
	return s.families.ListFamilies(ctx)
}

// GetFamily returns one family; unknown is KindNotFound, malformed KindInvalid.
func (s *FamilyService) GetFamily(ctx context.Context, familyID string) (domain.Family, error) {
	id, err := canonicalID(familyID, "family id")
	if err != nil {
		return domain.Family{}, err
	}
	return s.families.FamilyByID(ctx, id)
}

// CreateFamily groups pupils that have no family yet. An empty name is the
// first pupil's surnames. Unknown, withdrawn or rejected pupils are
// KindInvalid; a pupil already in a family is KindConflict. A draft may join.
// Pupils whose accounts add up to more than domain.MaxFamilyAccounts are
// domain.ErrFamilyAccountLimit and nothing is written.
func (s *FamilyService) CreateFamily(ctx context.Context, name string, studentIDs []string) (domain.Family, error) {
	ids, err := canonicalIDs(studentIDs)
	if err != nil {
		return domain.Family{}, err
	}
	pupils, err := s.joinablePupils(ctx, ids)
	if err != nil {
		return domain.Family{}, err
	}
	if strings.TrimSpace(name) == "" {
		first := pupils[ids[0]]
		name = domain.ProposedFamilyName(first.LastName(), first.MaternalLastName())
	}
	family, err := domain.NewFamily(s.newID(), s.tenant, name, ids, s.now())
	if err != nil {
		return domain.Family{}, err
	}
	if err := s.families.CreateFamily(ctx, family, s.syncAccess); err != nil {
		return domain.Family{}, err
	}
	return s.families.FamilyByID(ctx, family.ID())
}

// RenameFamily renames a family; unknown is KindNotFound.
func (s *FamilyService) RenameFamily(ctx context.Context, familyID, name string) (domain.Family, error) {
	family, err := s.GetFamily(ctx, familyID)
	if err != nil {
		return domain.Family{}, err
	}
	renamed, err := family.Rename(name)
	if err != nil {
		return domain.Family{}, err
	}
	if err := s.families.RenameFamily(ctx, renamed.ID(), renamed.Name()); err != nil {
		return domain.Family{}, err
	}
	return s.families.FamilyByID(ctx, renamed.ID())
}

// AddStudentsToFamily adds pupils without a family to an existing one, under
// the same rules as CreateFamily.
func (s *FamilyService) AddStudentsToFamily(ctx context.Context, familyID string, studentIDs []string) (domain.Family, error) {
	family, err := s.GetFamily(ctx, familyID)
	if err != nil {
		return domain.Family{}, err
	}
	ids, err := canonicalIDs(studentIDs)
	if err != nil {
		return domain.Family{}, err
	}
	if _, err := s.joinablePupils(ctx, ids); err != nil {
		return domain.Family{}, err
	}
	if _, err := family.AddStudents(ids); err != nil {
		return domain.Family{}, err
	}
	if err := s.families.AddStudentsToFamily(ctx, family.ID(), ids, s.syncAccess); err != nil {
		return domain.Family{}, err
	}
	return s.families.FamilyByID(ctx, family.ID())
}

// RemoveStudentFromFamily takes one pupil out of a family; its guardian links
// stay. The last pupil out dissolves the family: dissolved is true and the
// family is the zero value. A pupil not in that family is KindNotFound.
func (s *FamilyService) RemoveStudentFromFamily(ctx context.Context, familyID, studentID string) (family domain.Family, dissolved bool, err error) {
	fid, err := canonicalID(familyID, "family id")
	if err != nil {
		return domain.Family{}, false, err
	}
	sid, err := canonicalID(studentID, "student id")
	if err != nil {
		return domain.Family{}, false, err
	}
	dissolved, err = s.families.RemoveStudentFromFamily(ctx, fid, sid)
	if err != nil || dissolved {
		return domain.Family{}, dissolved, err
	}
	family, err = s.families.FamilyByID(ctx, fid)
	return family, false, err
}

// DissolveFamily leaves every member without a family; unknown is KindNotFound.
func (s *FamilyService) DissolveFamily(ctx context.Context, familyID string) error {
	id, err := canonicalID(familyID, "family id")
	if err != nil {
		return err
	}
	return s.families.DissolveFamily(ctx, id)
}

// SuggestFamilies proposes groups of pupils without a family that look like
// siblings (domain.SuggestFamilies), minus the dismissed ones.
func (s *FamilyService) SuggestFamilies(ctx context.Context) ([]domain.FamilySuggestion, error) {
	in, err := s.families.SuggestionInputs(ctx)
	if err != nil {
		return nil, err
	}
	return domain.SuggestFamilies(in.Pupils, in.Contacts, in.Dismissed, in.Common), nil
}

// DismissFamilySuggestion records "not a family" for exactly these pupils
// (2..MaxFamilyPupils distinct ids), by userID ("" when unknown). Idempotent.
func (s *FamilyService) DismissFamilySuggestion(ctx context.Context, studentIDs []string, userID string) error {
	ids, err := canonicalIDs(studentIDs)
	if err != nil {
		return err
	}
	if len(ids) < minSuggestionPupils {
		return glerrors.New(glerrors.KindInvalid, "a suggestion has at least two pupils")
	}
	by := ""
	if strings.TrimSpace(userID) != "" {
		if by, err = canonicalID(userID, "user id"); err != nil {
			return err
		}
	}
	return s.families.DismissSuggestion(ctx, ports.FamilyDismissal{
		ID: s.newID(), Signature: domain.SuggestionSignature(ids), DismissedBy: by,
	})
}

// joinablePupils loads the pupils (one read) and checks each may join a
// family: it exists, has not left the school, and has no family yet.
func (s *FamilyService) joinablePupils(ctx context.Context, ids []string) (map[string]*domain.Student, error) {
	found, _, err := s.students.List(ctx, ports.ListFilter{IDs: ids, PageSize: domain.MaxFamilyPupils})
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*domain.Student, len(found))
	for _, p := range found {
		byID[p.ID()] = p
	}
	for _, id := range ids {
		p, ok := byID[id]
		switch {
		case !ok:
			return nil, glerrors.New(glerrors.KindInvalid, "student not found")
		case p.Status() == domain.AdmissionWithdrawn || p.Status() == domain.AdmissionRejected:
			return nil, glerrors.New(glerrors.KindInvalid, "a withdrawn or rejected pupil cannot join a family")
		case p.FamilyID() != "":
			return nil, glerrors.New(glerrors.KindConflict, "the pupil is already in a family")
		}
	}
	return byID, nil
}

// canonicalIDs canonicalizes the pupil ids of a family operation: 1..20,
// each a uuid in its canonical lower-case form, so two spellings of one id
// never pass as two pupils nor change a signature. A repeated pupil is
// KindInvalid.
func canonicalIDs(raw []string) ([]string, error) {
	if len(raw) == 0 || len(raw) > domain.MaxFamilyPupils {
		return nil, glerrors.New(glerrors.KindInvalid, "a family has 1 to 20 pupils")
	}
	out := make([]string, len(raw))
	for i, r := range raw {
		id, err := canonicalID(r, "student id")
		if err != nil {
			return nil, err
		}
		out[i] = id
	}
	if hasRepeats(out) {
		return nil, glerrors.New(glerrors.KindInvalid, "a pupil is listed twice")
	}
	return out, nil
}

func hasRepeats(ids []string) bool {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

// canonicalID parses a uuid and returns its canonical form; malformed is
// KindInvalid (never a database cast error answered as 500).
func canonicalID(raw, what string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", glerrors.New(glerrors.KindInvalid, "malformed "+what)
	}
	return parsed.String(), nil
}
