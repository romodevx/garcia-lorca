//go:build integration

// Integration tests for family access (ADR 0041, T2): the cap of two accounts
// and the sync of every account to every placeable sibling, driven through the
// real use cases over a REAL Postgres as the non-owner `app` role, so RLS and
// the locking rule are the ones production runs.
//
// Run: go test -tags=integration ./internal/adapters/postgres/ -run Famil
package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/students/internal/adapters/postgres"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// anyGuardian is an iam that knows every id as an active guardian: the iam
// check is the use case's (tested there); these tests are about the database.
type anyGuardian struct{}

func (anyGuardian) ByID(_ context.Context, id string) (ports.UserRef, error) {
	return ports.UserRef{ID: id, Status: "USER_STATUS_ACTIVE", Roles: []string{"guardian"}}, nil
}

// accessServices wires the guardian and family use cases over one app-role
// repository bound to tenant, as cmd/main.go does.
func accessServices(t *testing.T, tenant string) (*usecase.GuardianService, *usecase.FamilyService, *pgadapter.Repository) {
	t.Helper()
	repo := appRepo(t, tenant)
	return usecase.NewGuardianService(repo, repo, repo, anyGuardian{}, tenant, uuid.NewString),
		usecase.NewFamilyService(repo, repo, tenant, uuid.NewString, time.Now),
		repo
}

// seedContact stores an account-less contact on a pupil and returns its id.
func seedContact(t *testing.T, repo *pgadapter.Repository, tenant, student string, person domain.ContactPerson) string {
	t.Helper()
	c, err := domain.NewContact(uuid.NewString(), tenant, student, domain.RelationshipMother, person)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Link(context.Background(), c); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	return c.ID()
}

// seedAccount stores a link with an account directly (unsynced, as before T2).
func seedAccount(t *testing.T, repo *pgadapter.Repository, tenant, user, student string, person domain.ContactPerson) string {
	t.Helper()
	l, err := domain.NewGuardianLink(uuid.NewString(), tenant, user, student, domain.RelationshipMother)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Link(context.Background(), l.WithPerson(person)); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return l.ID()
}

// accountsOf reads the accounts linked to a pupil, sorted (as the superuser:
// what is really stored).
func accountsOf(t *testing.T, student string) []string {
	t.Helper()
	var out []string
	err := superDB.Raw(`SELECT guardian_user_id::text FROM guardian_students
	                     WHERE student_id = ? AND guardian_user_id IS NOT NULL ORDER BY 1`, student).Scan(&out).Error
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// familyAccounts counts the distinct accounts on a family's members.
func familyAccounts(t *testing.T, familyID string) int64 {
	t.Helper()
	var n int64
	err := superDB.Raw(`SELECT count(DISTINCT g.guardian_user_id) FROM guardian_students g
	                     JOIN students s ON s.id = g.student_id WHERE s.family_id = ?`, familyID).Scan(&n).Error
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// guardianRows is every guardian_students row of these pupils, as text, to
// prove a refused write changed nothing.
func guardianRows(t *testing.T, students ...string) []string {
	t.Helper()
	var out []string
	err := superDB.Raw(`SELECT concat_ws('|', id, COALESCE(guardian_user_id::text, '-'), student_id, full_name, email)
	                      FROM guardian_students WHERE student_id IN ? ORDER BY id`, students).Scan(&out).Error
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func withdrawRaw(t *testing.T, student string) {
	t.Helper()
	if err := superDB.Exec(`UPDATE students SET admission_status = 'withdrawn' WHERE id = ?`, student).Error; err != nil {
		t.Fatal(err)
	}
}

// pausingAccess is the real repository's FamilyAccessRepository with a pause
// at the end of the first OnPupil's work, BEFORE its transaction commits: the
// write is done, the locks are held, and the test starts a rival meanwhile.
type pausingAccess struct {
	*pgadapter.Repository
	paused, resume chan struct{}
}

func newPausingAccess(repo *pgadapter.Repository) *pausingAccess {
	return &pausingAccess{Repository: repo, paused: make(chan struct{}), resume: make(chan struct{})}
}

func (p *pausingAccess) OnPupil(ctx context.Context, studentID string, fn ports.FamilyAccessFunc) error {
	return p.Repository.OnPupil(ctx, studentID, func(ctx context.Context, a ports.FamilyAccess) error {
		err := fn(ctx, a)
		close(p.paused)
		<-p.resume
		return err
	})
}

// raceAgainstPaused runs first (whose OnPupil goes through pause) until it is
// paused inside its transaction, then rival; once rival waits on a lock — or
// finished, which is what a missing lock looks like — first commits. Returns
// both errors.
func raceAgainstPaused(t *testing.T, pause *pausingAccess, first, rival func() error) (firstErr, rivalErr error) {
	t.Helper()
	firstDone, rivalDone := make(chan error, 1), make(chan error, 1)
	go func() { firstDone <- first() }()
	select {
	case <-pause.paused:
	case err := <-firstDone:
		t.Fatalf("the first writer ended before pausing: %v", err)
	}
	go func() { rivalDone <- rival() }()
	awaitWaiterOrDone(t, rivalDone)
	close(pause.resume)
	firstErr = <-firstDone
	if rivalErr == nil {
		select {
		case rivalErr = <-rivalDone:
		case <-time.After(10 * time.Second):
			t.Fatal("the rival never finished")
		}
	}
	return firstErr, rivalErr
}

// awaitWaiterOrDone blocks until an app session waits on a lock or done has a
// value, which it puts back.
func awaitWaiterOrDone(t *testing.T, done chan error) {
	t.Helper()
	const poll = 5 * time.Millisecond
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case err := <-done:
			done <- err
			return
		default:
		}
		var waiting int64
		err := superDB.Raw(`SELECT count(*) FROM pg_stat_activity WHERE usename = 'app' AND wait_event_type = 'Lock'`).
			Scan(&waiting).Error
		if err != nil {
			t.Fatalf("pg_stat_activity: %v", err)
		}
		if waiting >= 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the rival neither waited on a lock nor finished")
		}
		time.Sleep(poll)
	}
}

var (
	famRosa = domain.ContactPerson{FullName: "Rosa Pérez", Phone: "4431112233", Email: "rosa@example.com"}
	famLuis = domain.ContactPerson{FullName: "Luis Nava", Phone: "4435556677"}
)

func TestFamilyGrantSyncsTheAccountToSiblingsInTheDB(t *testing.T) {
	ctx := context.Background()
	guardians, families, repo := accessServices(t, famTenantA)
	a, b, gone := seedPupil(t, famTenantA, "Sync", "Uno"), seedPupil(t, famTenantA, "Sync", "Uno"), seedPupil(t, famTenantA, "Sync", "Uno")
	f, err := families.CreateFamily(ctx, "Sync", []string{a, b, gone})
	if err != nil {
		t.Fatal(err)
	}
	withdrawRaw(t, gone)
	onA := seedContact(t, repo, famTenantA, a, famRosa)
	onB := seedContact(t, repo, famTenantA, b, domain.ContactPerson{FullName: "Rosa", Email: "Rosa@Example.com"})
	mom := uuid.NewString()

	if _, err := guardians.GrantContactAccess(ctx, onA, a, mom); err != nil {
		t.Fatalf("GrantContactAccess: %v", err)
	}
	if got := accountsOf(t, b); !slices.Equal(got, []string{mom}) {
		t.Errorf("sibling accounts = %v, want the granted one", got)
	}
	if c, err := repo.ByID(ctx, onB); err != nil || c.GuardianUserID() != mom {
		t.Errorf("the sibling's own contact with that email did not take the account: %+v, %v", c, err)
	}
	if n := len(guardianRows(t, b)); n != 1 {
		t.Errorf("sibling rows = %d, want 1 (no second Rosa)", n)
	}
	if got := accountsOf(t, gone); len(got) != 0 {
		t.Errorf("a withdrawn member got %v", got)
	}

	// A sibling without a matching contact gets a copy of Rosa.
	c := seedPupil(t, famTenantA, "Sync", "Uno")
	if _, err := families.AddStudentsToFamily(ctx, f.ID(), []string{c}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	links, err := repo.ListByStudent(ctx, c)
	if err != nil || len(links) != 1 {
		t.Fatalf("new member links = %+v, %v", links, err)
	}
	if l := links[0]; l.GuardianUserID() != mom || l.FullName() != famRosa.FullName || l.Phone() != famRosa.Phone ||
		l.Email() != famRosa.Email || l.IsEmergency() {
		t.Errorf("copied contact = %+v", l)
	}
	if got, _ := repo.FamilyByID(ctx, f.ID()); got.AccountCount() != 1 {
		t.Errorf("account count = %d, want 1", got.AccountCount())
	}
}

func TestFamilyThirdAccountIsRefusedAndWritesNothing(t *testing.T) {
	ctx := context.Background()
	guardians, families, repo := accessServices(t, famTenantA)
	a, b := seedPupil(t, famTenantA, "Tope", "Dos"), seedPupil(t, famTenantA, "Tope", "Dos")
	seedAccount(t, repo, famTenantA, uuid.NewString(), a, famRosa)
	seedAccount(t, repo, famTenantA, uuid.NewString(), b, famLuis)
	if _, err := families.CreateFamily(ctx, "Tope", []string{a, b}); err != nil {
		t.Fatal(err)
	}
	onB := seedContact(t, repo, famTenantA, b, domain.ContactPerson{FullName: "Tía"})
	before := guardianRows(t, a, b)

	_, err := guardians.GrantContactAccess(ctx, onB, b, uuid.NewString())
	if !errors.Is(err, domain.ErrFamilyAccountLimit) {
		t.Fatalf("err = %v, want ErrFamilyAccountLimit", err)
	}
	if _, err := guardians.LinkGuardian(ctx, uuid.NewString(), a, "other"); !errors.Is(err, domain.ErrFamilyAccountLimit) {
		t.Fatalf("LinkGuardian: err = %v, want ErrFamilyAccountLimit", err)
	}
	if after := guardianRows(t, a, b); !slices.Equal(before, after) {
		t.Errorf("a refused write changed the links:\nbefore %v\nafter  %v", before, after)
	}
}

func TestFamilyMergeOverTheCapIsRefused(t *testing.T) {
	ctx := context.Background()
	_, families, repo := accessServices(t, famTenantA)
	a, b := seedPupil(t, famTenantA, "Junta", "Tres"), seedPupil(t, famTenantA, "Junta", "Tres")
	seedAccount(t, repo, famTenantA, uuid.NewString(), a, famRosa)
	seedAccount(t, repo, famTenantA, uuid.NewString(), a, famLuis)
	seedAccount(t, repo, famTenantA, uuid.NewString(), b, domain.ContactPerson{FullName: "Otro"})
	before := guardianRows(t, a, b)

	if _, err := families.CreateFamily(ctx, "Junta", []string{a, b}); !errors.Is(err, domain.ErrFamilyAccountLimit) {
		t.Fatalf("err = %v, want ErrFamilyAccountLimit", err)
	}
	if s, _ := repo.FindByID(ctx, a); s.FamilyID() != "" {
		t.Error("the refused family was stored")
	}
	if after := guardianRows(t, a, b); !slices.Equal(before, after) {
		t.Error("the refused merge synced links")
	}
}

// Two clerks granting two different new accounts on a family that already has
// one: the first is paused inside its transaction after writing (its account
// attached and synced, not committed); the second must queue on the family
// lock and then see the first's account, so exactly one wins. Without the
// family lock the second counts only its own account, and both win.
func TestFamilyConcurrentGrantsKeepTheCap(t *testing.T) {
	ctx := context.Background()
	_, families, repo := accessServices(t, famTenantA)
	a, b := seedPupil(t, famTenantA, "Carrera", "Dos"), seedPupil(t, famTenantA, "Carrera", "Dos")
	seedAccount(t, repo, famTenantA, uuid.NewString(), a, famRosa)
	f, err := families.CreateFamily(ctx, "Carrera", []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	onA := seedContact(t, repo, famTenantA, a, famLuis)
	onB := seedContact(t, repo, famTenantA, b, domain.ContactPerson{FullName: "Tía"})
	pause := newPausingAccess(repo)
	paused := usecase.NewGuardianService(repo, repo, pause, anyGuardian{}, famTenantA, uuid.NewString)
	rival, _, _ := accessServices(t, famTenantA)

	firstErr, rivalErr := raceAgainstPaused(t, pause,
		func() error { _, err := paused.GrantContactAccess(ctx, onA, a, uuid.NewString()); return err },
		func() error { _, err := rival.GrantContactAccess(ctx, onB, b, uuid.NewString()); return err },
	)
	if firstErr != nil {
		t.Errorf("the first grant: %v", firstErr)
	}
	if !errors.Is(rivalErr, domain.ErrFamilyAccountLimit) {
		t.Errorf("the rival grant: %v, want ErrFamilyAccountLimit", rivalErr)
	}
	if n := familyAccounts(t, f.ID()); n != domain.MaxFamilyAccounts {
		t.Errorf("family accounts = %d, want %d", n, domain.MaxFamilyAccounts)
	}
}

// A grant on a pupil with no family, paused inside its transaction, and a
// CreateFamily claiming that pupil meanwhile: the grant's students FOR SHARE
// makes the create's FOR UPDATE wait, so the create counts the granted account
// and is refused. Without that lock the create reads two accounts, commits,
// and the grant commits a third onto the family.
func TestFamilyGrantOnAFamilyLessPupilVersusCreate(t *testing.T) {
	ctx := context.Background()
	_, families, repo := accessServices(t, famTenantA)
	a, b := seedPupil(t, famTenantA, "Alta", "Par"), seedPupil(t, famTenantA, "Alta", "Par")
	seedAccount(t, repo, famTenantA, uuid.NewString(), a, famRosa)
	seedAccount(t, repo, famTenantA, uuid.NewString(), a, famLuis)
	onB := seedContact(t, repo, famTenantA, b, domain.ContactPerson{FullName: "Tía"})
	pause := newPausingAccess(repo)
	paused := usecase.NewGuardianService(repo, repo, pause, anyGuardian{}, famTenantA, uuid.NewString)

	grantErr, createErr := raceAgainstPaused(t, pause,
		func() error { _, err := paused.GrantContactAccess(ctx, onB, b, uuid.NewString()); return err },
		func() error { _, err := families.CreateFamily(ctx, "Alta", []string{a, b}); return err },
	)
	if grantErr != nil {
		t.Errorf("the grant on a family-less pupil: %v", grantErr)
	}
	if !errors.Is(createErr, domain.ErrFamilyAccountLimit) {
		t.Errorf("the create: %v, want ErrFamilyAccountLimit", createErr)
	}
	s, err := repo.FindByID(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if s.FamilyID() != "" {
		t.Errorf("pupil joined family %s with %d accounts", s.FamilyID(), familyAccounts(t, s.FamilyID()))
	}
}

func TestFamilyRevokeKeepsContactsAndDropsNamelessLinks(t *testing.T) {
	ctx := context.Background()
	_, families, repo := accessServices(t, famTenantA)
	a, b, outside := seedPupil(t, famTenantA, "Baja", "Uno"), seedPupil(t, famTenantA, "Baja", "Uno"), seedPupil(t, famTenantA, "Baja", "Otro")
	mom := uuid.NewString()
	onA := seedAccount(t, repo, famTenantA, mom, a, famRosa)
	legacy := seedAccount(t, repo, famTenantA, mom, b, domain.ContactPerson{}) // pre-M16: account only
	seedAccount(t, repo, famTenantA, mom, outside, famRosa)
	f, err := families.CreateFamily(ctx, "Baja", []string{a, b})
	if err != nil {
		t.Fatal(err)
	}

	n, err := families.RevokeFamilyAccess(ctx, f.ID(), mom)
	if err != nil || n != 2 {
		t.Fatalf("revoke = %d, %v; want 2", n, err)
	}
	if c, err := repo.ByID(ctx, onA); err != nil || c.HasAccount() || c.FullName() != famRosa.FullName || c.Phone() != famRosa.Phone {
		t.Errorf("A's contact = %+v, %v; want kept without the account", c, err)
	}
	if _, err := repo.ByID(ctx, legacy); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the nameless legacy link must be deleted: %v", err)
	}
	if got := accountsOf(t, outside); !slices.Equal(got, []string{mom}) {
		t.Errorf("a pupil outside the family lost the account: %v", got)
	}
	if _, err := families.RevokeFamilyAccess(ctx, f.ID(), mom); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("revoking again = %v, want NotFound", err)
	}
}

func TestFamilyContactOnEveryPupil(t *testing.T) {
	ctx := context.Background()
	_, families, repo := accessServices(t, famTenantA)
	a, b, gone := seedPupil(t, famTenantA, "Todos", "Uno"), seedPupil(t, famTenantA, "Todos", "Uno"), seedPupil(t, famTenantA, "Todos", "Uno")
	existing := seedContact(t, repo, famTenantA, a, domain.ContactPerson{FullName: "Rosa", Phone: "4431112233"})
	f, err := families.CreateFamily(ctx, "Todos", []string{a, b, gone})
	if err != nil {
		t.Fatal(err)
	}
	withdrawRaw(t, gone)

	got, err := families.AddFamilyContact(ctx, f.ID(), "mother", famRosa)
	if err != nil {
		t.Fatalf("AddFamilyContact: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("contacts = %+v, want one for each pupil at school", got)
	}
	for _, c := range got {
		if c.StudentID() == a && c.ID() != existing {
			t.Errorf("A already had Rosa by phone; got a new contact %s", c.ID())
		}
	}
	if links, _ := repo.ListByStudent(ctx, b); len(links) != 1 || links[0].FullName() != famRosa.FullName || links[0].HasAccount() {
		t.Errorf("B's contacts = %+v", links)
	}
	if n := len(guardianRows(t, gone)); n != 0 {
		t.Errorf("the withdrawn pupil got %d contacts", n)
	}
}

func TestRLSIsolatesFamilyAccess(t *testing.T) {
	ctx := context.Background()
	_, familiesA, repoA := accessServices(t, famTenantA)
	_, familiesB, _ := accessServices(t, famTenantB)
	a := seedPupil(t, famTenantA, "Ajena", "Uno")
	mom := uuid.NewString()
	seedAccount(t, repoA, famTenantA, mom, a, famRosa)
	f, err := familiesA.CreateFamily(ctx, "Ajena", []string{a})
	if err != nil {
		t.Fatal(err)
	}
	before := guardianRows(t, a)

	if _, err := familiesB.RevokeFamilyAccess(ctx, f.ID(), mom); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("B revoking on A's family: %v, want NotFound", err)
	}
	if _, err := familiesB.AddFamilyContact(ctx, f.ID(), "other", famLuis); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("B adding a contact on A's family: %v, want NotFound", err)
	}
	if after := guardianRows(t, a); !slices.Equal(before, after) {
		t.Error("tenant B changed A's links")
	}
}

// quietEvents drops every event: the promotion's event is not under test.
type quietEvents struct{}

func (quietEvents) PublishStudentEnrolled(context.Context, domain.StudentEnrolledEvent) error {
	return nil
}
func (quietEvents) PublishAdmissionChanged(context.Context, domain.AdmissionChangedEvent) error {
	return nil
}
func (quietEvents) PublishOutboxEvent(context.Context, ports.OutboxEvent) error { return nil }

// A draft in a family gets no account; the save that gives it a valid CURP
// promotes it and links the family's accounts to it in the database.
func TestFamilyDraftPromotedGetsTheFamilysAccounts(t *testing.T) {
	ctx := context.Background()
	_, families, repo := accessServices(t, famTenantA)
	sibling := seedPupil(t, famTenantA, "Borrador", "Uno")
	draft := uuid.NewString()
	s, err := domain.NewImportedDraft(domain.NewApplicantParams{
		ID: draft, TenantID: famTenantA, FirstName: "Lucía", LastName: "Borrador", MaternalLastName: "Uno",
		Level: domain.RestoreLevel(primariaLevelID, "Primaria"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ImportStudent(ctx, ports.ImportCommand{Student: s, BatchID: importBatchID, NumberYear: testNumberYear}); err != nil {
		t.Fatalf("ImportStudent: %v", err)
	}
	mom := uuid.NewString()
	seedAccount(t, repo, famTenantA, mom, sibling, famRosa)
	if _, err := families.CreateFamily(ctx, "Borrador", []string{sibling, draft}); err != nil {
		t.Fatal(err)
	}
	if got := accountsOf(t, draft); len(got) != 0 {
		t.Fatalf("the draft got %v before its CURP", got)
	}

	students := usecase.NewStudentService(repo, quietEvents{}, nil, famTenantA, uuid.NewString).WithFamilyAccess(repo)
	got, err := students.UpdateDetails(ctx, usecase.UpdateDetailsInput{
		ID: draft, FirstName: "Lucía", LastName: "Borrador", MaternalLastName: "Uno", CURP: famCURP(),
	})
	if err != nil || !got.PromotedFromDraft {
		t.Fatalf("UpdateDetails = promoted %v, %v", got.PromotedFromDraft, err)
	}
	if accounts := accountsOf(t, draft); !slices.Equal(accounts, []string{mom}) {
		t.Errorf("promoted pupil accounts = %v, want the family's", accounts)
	}
}

// Unlinking a family pupil is refused in the database too: the link stays.
func TestFamilyPupilUnlinkIsRefused(t *testing.T) {
	ctx := context.Background()
	guardians, families, repo := accessServices(t, famTenantA)
	a := seedPupil(t, famTenantA, "Unlink", "Uno")
	mom := uuid.NewString()
	seedAccount(t, repo, famTenantA, mom, a, famRosa)
	if _, err := families.CreateFamily(ctx, "Unlink", []string{a}); err != nil {
		t.Fatal(err)
	}
	if err := guardians.UnlinkGuardian(ctx, mom, a); !errors.Is(err, domain.ErrFamilyManagedAccess) {
		t.Fatalf("err = %v, want ErrFamilyManagedAccess", err)
	}
	if got := accountsOf(t, a); !slices.Equal(got, []string{mom}) {
		t.Errorf("accounts = %v, want the link kept", got)
	}
	// Without a family the unlink works as before.
	b := seedPupil(t, famTenantA, "Unlink", "Dos")
	seedAccount(t, repo, famTenantA, mom, b, famRosa)
	if err := guardians.UnlinkGuardian(ctx, mom, b); err != nil {
		t.Fatalf("family-less unlink: %v", err)
	}
	if got := accountsOf(t, b); len(got) != 0 {
		t.Errorf("accounts = %v after unlink", got)
	}
}
