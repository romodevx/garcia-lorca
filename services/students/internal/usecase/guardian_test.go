package usecase_test

import (
	"context"
	"errors"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// --- fakes -----------------------------------------------------------------

// fakeGuardianRepo is an in-memory GuardianRepository. It enforces the
// (tenant, guardian, student) uniqueness the real table's UNIQUE constraint owns
// so the use case's conflict path is exercised without a database.
type fakeGuardianRepo struct {
	links   map[string]domain.GuardianLink // tenant|guardian|student
	linkErr error
}

func newGuardianRepo() *fakeGuardianRepo {
	return &fakeGuardianRepo{links: map[string]domain.GuardianLink{}}
}

func guardianKey(tenantID, guardianUserID, studentID string) string {
	return tenantID + "|" + guardianUserID + "|" + studentID
}

func (r *fakeGuardianRepo) Link(_ context.Context, l domain.GuardianLink) error {
	if r.linkErr != nil {
		return r.linkErr
	}
	// Mirrors the DB, where guardian_user_id is NULL for an account-less
	// contact: Postgres does not collide NULLs, so several contacts without an
	// account may sit on the same pupil. A fake that collided them would refuse
	// something production accepts.
	if !l.HasAccount() {
		r.links[l.ID()] = l
		return nil
	}
	key := guardianKey(l.TenantID(), l.GuardianUserID(), l.StudentID())
	if _, ok := r.links[key]; ok {
		return glerrors.New(glerrors.KindConflict, "guardian is already linked to this student")
	}
	r.links[key] = l
	return nil
}

func (r *fakeGuardianRepo) Unlink(_ context.Context, guardianUserID, studentID string) error {
	key := guardianKey(tenant, guardianUserID, studentID)
	if _, ok := r.links[key]; !ok {
		return glerrors.ErrNotFound
	}
	delete(r.links, key)
	return nil
}

func (r *fakeGuardianRepo) ListByStudent(_ context.Context, studentID string) ([]domain.GuardianLink, error) {
	return r.filter(func(l domain.GuardianLink) bool { return l.StudentID() == studentID }), nil
}

func (r *fakeGuardianRepo) ListByGuardian(_ context.Context, guardianUserID string) ([]domain.GuardianLink, error) {
	return r.filter(func(l domain.GuardianLink) bool { return l.GuardianUserID() == guardianUserID }), nil
}

func (r *fakeGuardianRepo) filter(keep func(domain.GuardianLink) bool) []domain.GuardianLink {
	out := make([]domain.GuardianLink, 0, len(r.links))
	for _, l := range r.links {
		if keep(l) {
			out = append(out, l)
		}
	}
	return out
}

// fakeUsers is an in-memory UserReader. A missing user returns
// glerrors.ErrNotFound, matching the real iam Connect client's NotFound mapping.
type fakeUsers struct {
	byID map[string]ports.UserRef
	err  error
}

func newUsers() *fakeUsers { return &fakeUsers{byID: map[string]ports.UserRef{}} }

func (u *fakeUsers) ByID(_ context.Context, userID string) (ports.UserRef, error) {
	if u.err != nil {
		return ports.UserRef{}, u.err
	}
	if ref, ok := u.byID[userID]; ok {
		return ref, nil
	}
	return ports.UserRef{}, glerrors.ErrNotFound
}

// --- helpers ---------------------------------------------------------------

func newGuardianService(t *testing.T) (*usecase.GuardianService, *fakeRepo, *fakeGuardianRepo, *fakeUsers) {
	t.Helper()
	students := newRepo()
	guardians := newGuardianRepo()
	users := newUsers()
	ids := 0
	svc := usecase.NewGuardianService(students, guardians, users, tenant,
		func() string { ids++; return "gl-" + string(rune('0'+ids)) })
	return svc, students, guardians, users
}

// activeUserStatus is the projected iam status of a usable account: the string
// form of iamv1.UserStatus_USER_STATUS_ACTIVE, which is what the iamclient
// adapter puts in UserRef.Status.
const activeUserStatus = "USER_STATUS_ACTIVE"

// seedGuardianUser registers an active iam user holding the roles given.
func seedGuardianUser(u *fakeUsers, id string, roles ...string) {
	seedUserWithStatus(u, id, activeUserStatus, roles...)
}

// seedUserWithStatus registers an iam user with an explicit account status.
func seedUserWithStatus(u *fakeUsers, id, status string, roles ...string) {
	u.byID[id] = ports.UserRef{ID: id, Status: status, Roles: roles}
}

// --- LinkGuardian ----------------------------------------------------------

func TestLinkGuardianHappy(t *testing.T) {
	svc, students, guardians, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "usr-1", "guardian")

	link, err := svc.LinkGuardian(context.Background(), "usr-1", "stu-1", "mother")
	if err != nil {
		t.Fatalf("LinkGuardian: %v", err)
	}
	if link.GuardianUserID() != "usr-1" || link.StudentID() != "stu-1" {
		t.Errorf("link references mismatch: %+v", link)
	}
	if link.TenantID() != tenant {
		t.Errorf("tenant must come from the service scope: %q", link.TenantID())
	}
	if link.Relationship() != domain.RelationshipMother {
		t.Errorf("relationship = %v, want mother", link.Relationship())
	}
	if link.ID() == "" {
		t.Error("the link must be given a minted id")
	}
	if len(guardians.links) != 1 {
		t.Errorf("the link must be persisted: %+v", guardians.links)
	}
}

func TestLinkGuardianMissingStudentIsInvalid(t *testing.T) {
	svc, _, _, users := newGuardianService(t)
	seedGuardianUser(users, "usr-1", "guardian")

	_, err := svc.LinkGuardian(context.Background(), "usr-1", "ghost", "father")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v (kind %v)", err, glerrors.KindOf(err))
	}
}

func TestLinkGuardianMissingUserIsInvalid(t *testing.T) {
	svc, students, _, _ := newGuardianService(t)
	seedStudent(t, students, "stu-1")

	_, err := svc.LinkGuardian(context.Background(), "ghost", "stu-1", "father")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v (kind %v)", err, glerrors.KindOf(err))
	}
}

// A real iam user without the guardian role must not become a guardian: this is
// the authorization invariant the family read path leans on.
func TestLinkGuardianRequiresGuardianRole(t *testing.T) {
	svc, students, guardians, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "usr-1", "teacher", "owner")

	_, err := svc.LinkGuardian(context.Background(), "usr-1", "stu-1", "mother")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v (kind %v)", err, glerrors.KindOf(err))
	}
	if len(guardians.links) != 0 {
		t.Errorf("nothing may be persisted for a non-guardian user: %+v", guardians.links)
	}
}

// A suspended (or otherwise non-active) iam account must not acquire a guardian
// link: the link is what the family read path authorizes on, so a disabled
// account would keep reading its children's records through it.
func TestLinkGuardianRequiresActiveAccount(t *testing.T) {
	for _, status := range []string{"USER_STATUS_SUSPENDED", "USER_STATUS_UNSPECIFIED", ""} {
		t.Run(status, func(t *testing.T) {
			svc, students, guardians, users := newGuardianService(t)
			seedStudent(t, students, "stu-1")
			seedUserWithStatus(users, "usr-1", status, "guardian")

			_, err := svc.LinkGuardian(context.Background(), "usr-1", "stu-1", "mother")
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("want KindInvalid, got %v (kind %v)", err, glerrors.KindOf(err))
			}
			if len(guardians.links) != 0 {
				t.Errorf("nothing may be persisted for a non-active user: %+v", guardians.links)
			}
		})
	}
}

func TestLinkGuardianUnknownRelationshipIsInvalid(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "usr-1", "guardian")

	if _, err := svc.LinkGuardian(context.Background(), "usr-1", "stu-1", "uncle"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestLinkGuardianDuplicateIsConflict(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "usr-1", "guardian")

	if _, err := svc.LinkGuardian(context.Background(), "usr-1", "stu-1", "mother"); err != nil {
		t.Fatalf("first link: %v", err)
	}
	_, err := svc.LinkGuardian(context.Background(), "usr-1", "stu-1", "father")
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate link: want KindConflict, got %v (kind %v)", err, glerrors.KindOf(err))
	}
}

// An iam transport failure is NOT a client error: it must propagate as-is so the
// edge answers Internal rather than blaming the caller's arguments.
func TestLinkGuardianPropagatesUserReaderFailure(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	users.err = glerrors.Wrap(glerrors.KindInternal, "iam user lookup failed", errors.New("boom"))

	_, err := svc.LinkGuardian(context.Background(), "usr-1", "stu-1", "mother")
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Errorf("want KindInternal, got %v (kind %v)", err, glerrors.KindOf(err))
	}
}

// --- UnlinkGuardian -------------------------------------------------------

func TestUnlinkGuardian(t *testing.T) {
	svc, students, guardians, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "usr-1", "guardian")
	if _, err := svc.LinkGuardian(context.Background(), "usr-1", "stu-1", "mother"); err != nil {
		t.Fatalf("seed link: %v", err)
	}

	if err := svc.UnlinkGuardian(context.Background(), "usr-1", "stu-1"); err != nil {
		t.Fatalf("UnlinkGuardian: %v", err)
	}
	if len(guardians.links) != 0 {
		t.Errorf("the link must be gone: %+v", guardians.links)
	}
	// Unlinking again is NotFound (the proto contract).
	if err := svc.UnlinkGuardian(context.Background(), "usr-1", "stu-1"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown link: want KindNotFound, got %v", err)
	}
}

// --- lists ----------------------------------------------------------------

func TestListGuardiansForStudentAndStudentsForGuardian(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	ctx := context.Background()
	seedStudent(t, students, "stu-1")
	seedStudent(t, students, "stu-2")
	seedGuardianUser(users, "usr-1", "guardian")
	seedGuardianUser(users, "usr-2", "guardian")

	// usr-1 guards both siblings; usr-2 guards only stu-1.
	for _, l := range []struct{ user, student, rel string }{
		{"usr-1", "stu-1", "mother"},
		{"usr-1", "stu-2", "mother"},
		{"usr-2", "stu-1", "father"},
	} {
		if _, err := svc.LinkGuardian(ctx, l.user, l.student, l.rel); err != nil {
			t.Fatalf("seed link %+v: %v", l, err)
		}
	}

	guardiansOfOne, err := svc.ListGuardiansForStudent(ctx, "stu-1")
	if err != nil {
		t.Fatalf("ListGuardiansForStudent: %v", err)
	}
	if len(guardiansOfOne) != 2 {
		t.Errorf("stu-1 should have 2 guardians, got %d", len(guardiansOfOne))
	}

	studentsOfOne, err := svc.ListStudentsForGuardian(ctx, "usr-1")
	if err != nil {
		t.Fatalf("ListStudentsForGuardian: %v", err)
	}
	if len(studentsOfOne) != 2 {
		t.Errorf("usr-1 should have 2 students, got %d", len(studentsOfOne))
	}

	studentsOfTwo, err := svc.ListStudentsForGuardian(ctx, "usr-2")
	if err != nil {
		t.Fatalf("ListStudentsForGuardian: %v", err)
	}
	if len(studentsOfTwo) != 1 || studentsOfTwo[0].StudentID() != "stu-1" {
		t.Errorf("usr-2 should guard only stu-1, got %+v", studentsOfTwo)
	}
}
