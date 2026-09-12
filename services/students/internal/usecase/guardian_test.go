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

func (r *fakeGuardianRepo) ByID(_ context.Context, id string) (domain.GuardianLink, error) {
	for _, l := range r.links {
		if l.ID() == id {
			return l, nil
		}
	}
	return domain.GuardianLink{}, glerrors.ErrNotFound
}

// AttachAccount mirrors the real adapter's conditional UPDATE: it only writes
// while the stored contact still has no account, so a second grant loses rather
// than overwrites. Re-keying to the (tenant, guardian, student) key is how the
// fake keeps enforcing the table's UNIQUE afterwards.
func (r *fakeGuardianRepo) AttachAccount(_ context.Context, l domain.GuardianLink) error {
	for key, stored := range r.links {
		if stored.ID() != l.ID() {
			continue
		}
		if stored.HasAccount() {
			return glerrors.New(glerrors.KindConflict, "this contact already has an account")
		}
		newKey := guardianKey(l.TenantID(), l.GuardianUserID(), l.StudentID())
		if _, taken := r.links[newKey]; taken {
			return glerrors.New(glerrors.KindConflict, "guardian is already linked to this student")
		}
		delete(r.links, key)
		r.links[newKey] = l
		return nil
	}
	return glerrors.ErrNotFound
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

// --- GrantContactAccess ----------------------------------------------------

// seedContact records an account-less contact on a pupil and returns it.
func seedContact(t *testing.T, svc *usecase.GuardianService, studentID, name, email string) domain.GuardianLink {
	t.Helper()
	contact, err := svc.AddContact(context.Background(), usecase.ContactInput{
		StudentID:    studentID,
		Relationship: "father",
		FullName:     name,
		Email:        email,
	})
	if err != nil {
		t.Fatalf("AddContact: %v", err)
	}
	return contact
}

// The school gives a contact it already recorded access to the app. The account
// lands on the SAME row: one person, one entry in the pupil's contact list.
func TestGrantContactAccessHappy(t *testing.T) {
	svc, students, guardians, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "user-1", "guardian")
	contact := seedContact(t, svc, "stu-1", "Mateo Contreras", "mateo@example.com")

	got, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-1", "user-1")
	if err != nil {
		t.Fatalf("GrantContactAccess: %v", err)
	}
	if got.ID() != contact.ID() {
		t.Errorf("id = %q, want the contact that was already there (%q)", got.ID(), contact.ID())
	}
	if got.GuardianUserID() != "user-1" || !got.HasAccount() {
		t.Errorf("account did not attach: %+v", got)
	}
	if got.FullName() != "Mateo Contreras" {
		t.Errorf("the person was lost: %+v", got)
	}

	links, err := guardians.ListByStudent(context.Background(), "stu-1")
	if err != nil {
		t.Fatalf("ListByStudent: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("the pupil has %d contacts, want 1 — granting access must not create a second person", len(links))
	}
}

// The same rule as linking: the family read path authorizes on this link, so a
// user that does not hold the guardian role must never acquire one.
func TestGrantContactAccessRequiresGuardianRole(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "user-1", "admin")
	contact := seedContact(t, svc, "stu-1", "Mateo Contreras", "mateo@example.com")

	_, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-1", "user-1")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("role-less user: want KindInvalid, got %v", err)
	}
}

// A suspended account must not gain a link, for the same reason a role-less one
// must not: the link IS the authorization.
func TestGrantContactAccessRequiresActiveAccount(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedUserWithStatus(users, "user-1", "USER_STATUS_SUSPENDED", "guardian")
	contact := seedContact(t, svc, "stu-1", "Mateo Contreras", "mateo@example.com")

	_, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-1", "user-1")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("suspended account: want KindInvalid, got %v", err)
	}
}

// THE SCOPING TEST. The pupil is a required argument and it is checked HERE, not
// only at the edge: a caller that names somebody else's contact must not be able
// to hand that family's child to an account of their choosing. Answering
// NotFound (never a conflict or a 403) also refuses to confirm the contact
// exists.
func TestGrantContactAccessRefusesAContactOfAnotherPupil(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedStudent(t, students, "stu-2")
	seedGuardianUser(users, "user-1", "guardian")
	contact := seedContact(t, svc, "stu-1", "Mateo Contreras", "mateo@example.com")

	_, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-2", "user-1")
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("contact of another pupil: want KindNotFound, got %v", err)
	}
}

// An unknown contact is NotFound, not a crash and not a silent create.
func TestGrantContactAccessUnknownContactIsNotFound(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "user-1", "guardian")

	_, err := svc.GrantContactAccess(context.Background(), "gl-nope", "stu-1", "user-1")
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown contact: want KindNotFound, got %v", err)
	}
}

// Granting access twice must not move the pupil to whoever asked last.
func TestGrantContactAccessTwiceIsConflict(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "user-1", "guardian")
	seedGuardianUser(users, "user-2", "guardian")
	contact := seedContact(t, svc, "stu-1", "Mateo Contreras", "mateo@example.com")

	if _, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-1", "user-1"); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	_, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-1", "user-2")
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("second grant: want KindConflict, got %v", err)
	}
}

// An iam outage is not the caller's fault: it must not surface as a bad request.
func TestGrantContactAccessPropagatesUserReaderFailure(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	contact := seedContact(t, svc, "stu-1", "Mateo Contreras", "mateo@example.com")
	users.err = errors.New("iam unreachable")

	_, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-1", "user-1")
	if glerrors.KindOf(err) == glerrors.KindInvalid {
		t.Errorf("transport failure must not blame the caller, got %v", err)
	}
}

// An INVITED account may be linked, and must be: the office decides today who
// the mother is, and she opens her link tomorrow. Refusing the link until she
// does would mean the school cannot record a family until that family gets round
// to reading its email — and the account can read nothing meanwhile anyway,
// because an invited account has no password and cannot sign in at all.
//
// The rule this corrects was written before invitations existed. It said
// "active", but what it MEANT was "not suspended" — the test below still holds
// that line.
func TestGrantContactAccessAcceptsAnInvitedAccount(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedUserWithStatus(users, "user-1", "USER_STATUS_INVITED", "guardian")
	contact := seedContact(t, svc, "stu-1", "Lucía Herrera", "lucia@example.com")

	got, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-1", "user-1")
	if err != nil {
		t.Fatalf("an invited guardian must be linkable: %v", err)
	}
	if got.GuardianUserID() != "user-1" {
		t.Errorf("account did not attach: %+v", got)
	}
}

// Linking an invited account is fine; linking a SUSPENDED one is not. That
// account could sign in once and was cut off — handing it a child undoes the
// cutting off.
func TestLinkGuardianStillRefusesASuspendedAccount(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedUserWithStatus(users, "user-1", "USER_STATUS_SUSPENDED", "guardian")

	_, err := svc.LinkGuardian(context.Background(), "user-1", "stu-1", "mother")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("suspended account: want KindInvalid, got %v", err)
	}
}

// An unspecified status is not a state anybody is in; it is iam failing to say.
// Guessing "probably fine" about the field that authorizes a family read is the
// wrong way to be wrong.
func TestGrantContactAccessRefusesAnUnknownStatus(t *testing.T) {
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedUserWithStatus(users, "user-1", "USER_STATUS_UNSPECIFIED", "guardian")
	contact := seedContact(t, svc, "stu-1", "Lucía Herrera", "lucia@example.com")

	_, err := svc.GrantContactAccess(context.Background(), contact.ID(), "stu-1", "user-1")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unspecified status: want KindInvalid, got %v", err)
	}
}
