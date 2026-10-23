package usecase_test

import (
	"cmp"
	"context"
	"maps"
	"slices"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// memAccess is an in-memory FamilyAccessRepository over a map of pupils (the
// one the student fake owns, so membership is students.family_id as in the
// database) and a fakeGuardianRepo. A FamilyAccessFunc that fails rolls back
// every link it wrote, as the real transaction does.
type memAccess struct {
	pupils       map[string]*domain.Student
	links        *fakeGuardianRepo
	familyExists func(id string) bool
	// onPupilCalls counts OnPupil transactions (proves the lock path is taken).
	onPupilCalls int
}

func (m *memAccess) OnPupil(ctx context.Context, studentID string, fn ports.FamilyAccessFunc) error {
	m.onPupilCalls++
	s, ok := m.pupils[studentID]
	if !ok {
		return glerrors.ErrNotFound
	}
	return m.run(ctx, &memAccessTx{m: m, familyID: s.FamilyID(), studentID: studentID}, fn)
}

func (m *memAccess) OnFamily(ctx context.Context, familyID string, fn ports.FamilyAccessFunc) error {
	if !m.familyExists(familyID) {
		return glerrors.ErrNotFound
	}
	return m.run(ctx, &memAccessTx{m: m, familyID: familyID}, fn)
}

func (m *memAccess) run(ctx context.Context, tx *memAccessTx, fn ports.FamilyAccessFunc) error {
	saved := maps.Clone(m.links.links)
	if err := fn(ctx, tx); err != nil {
		m.links.links = saved
		return err
	}
	return nil
}

// AccountIDs is the distinct accounts on the pupils, on familyID's members
// and on the members of the pupils' families.
func (m *memAccess) AccountIDs(_ context.Context, familyID string, studentIDs []string) ([]string, error) {
	families := map[string]bool{}
	if familyID != "" {
		families[familyID] = true
	}
	for _, id := range studentIDs {
		if s, ok := m.pupils[id]; ok && s.FamilyID() != "" {
			families[s.FamilyID()] = true
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, l := range m.links.links {
		s, ok := m.pupils[l.StudentID()]
		if !l.HasAccount() || !ok || seen[l.GuardianUserID()] {
			continue
		}
		if slices.Contains(studentIDs, l.StudentID()) || families[s.FamilyID()] {
			seen[l.GuardianUserID()] = true
			out = append(out, l.GuardianUserID())
		}
	}
	slices.Sort(out)
	return out, nil
}

type memAccessTx struct {
	m         *memAccess
	familyID  string
	studentID string
}

func (t *memAccessTx) FamilyID() string { return t.familyID }

func (t *memAccessTx) inScope(studentID string) bool {
	s, ok := t.m.pupils[studentID]
	if !ok {
		return false
	}
	if t.familyID == "" {
		return studentID == t.studentID
	}
	return s.FamilyID() == t.familyID
}

func (t *memAccessTx) Members(context.Context) ([]domain.FamilyMember, error) {
	var out []domain.FamilyMember
	for id, s := range t.m.pupils {
		if !t.inScope(id) {
			continue
		}
		contacts := t.m.links.filter(func(l domain.GuardianLink) bool { return l.StudentID() == id })
		slices.SortFunc(contacts, func(a, b domain.GuardianLink) int { return cmp.Compare(a.ID(), b.ID()) })
		out = append(out, domain.FamilyMember{StudentID: id, Status: s.Status(), Contacts: contacts})
	}
	slices.SortFunc(out, func(a, b domain.FamilyMember) int { return cmp.Compare(a.StudentID, b.StudentID) })
	return out, nil
}

func (t *memAccessTx) AttachAccount(ctx context.Context, l domain.GuardianLink) error {
	if !t.inScope(l.StudentID()) {
		return glerrors.ErrNotFound
	}
	return t.m.links.AttachAccount(ctx, l)
}

func (t *memAccessTx) RemoveLink(ctx context.Context, guardianUserID, studentID string) error {
	if !t.inScope(studentID) {
		return glerrors.ErrNotFound
	}
	return t.m.links.Unlink(ctx, guardianUserID, studentID)
}

func (t *memAccessTx) InsertContact(ctx context.Context, l domain.GuardianLink) error {
	if !t.inScope(l.StudentID()) {
		return glerrors.New(glerrors.KindInvalid, "student does not exist")
	}
	return t.m.links.Link(ctx, l)
}

func (t *memAccessTx) DetachAccount(_ context.Context, guardianUserID string) (int, error) {
	n := 0
	for key, l := range t.m.links.links {
		if l.GuardianUserID() != guardianUserID || !t.inScope(l.StudentID()) {
			continue
		}
		n++
		delete(t.m.links.links, key)
		if l.FullName() == "" {
			continue // a link that would be nobody without its account
		}
		detached, err := domain.RestoreContact(l.ID(), l.TenantID(), l.StudentID(), l.Relationship(), domain.ContactPerson{
			FullName: l.FullName(), Phone: l.Phone(), Email: l.Email(), Emergency: l.IsEmergency(),
		})
		if err != nil {
			return 0, err
		}
		t.m.links.links[detached.ID()] = detached
	}
	return n, nil
}
