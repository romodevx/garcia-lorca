//go:build integration

// Integration tests for families (ADR 0041, migration 0023) against a REAL
// Postgres, connected as the non-owner `app` role so RLS applies. They run in
// tenants of their own so the suggestion reads see only these fixtures.
//
// Run: go test -tags=integration ./internal/adapters/postgres/ -run Famil
package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/students/internal/adapters/postgres"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

const (
	famTenantA = "fa1a0000-0000-0000-0000-00000000000a"
	famTenantB = "fa1b0000-0000-0000-0000-00000000000b"
)

var (
	famCURPSeq atomic.Int64
	famAt      = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
)

// famCURP mints a CURP no other family fixture holds.
func famCURP() string {
	n := famCURPSeq.Add(1)
	return fmt.Sprintf("FAMA050312MDFRNN%c%d", 'A'+rune(n/10), n%10)
}

// seedPupil stores an applicant with these surnames and returns its id.
func seedPupil(t *testing.T, tenant, last, maternal string) string {
	t.Helper()
	curp, err := domain.NewCURP(famCURP())
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	s, err := domain.NewApplicant(domain.NewApplicantParams{
		ID: id, TenantID: tenant, CURP: curp, FirstName: "Ana", LastName: last, MaternalLastName: maternal,
		Level: domain.RestoreLevel(primariaLevelID, "Primaria"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := appRepo(t, tenant).Create(context.Background(), s, testNumberYear); err != nil {
		t.Fatalf("seed pupil: %v", err)
	}
	return id
}

func newFamily(t *testing.T, tenant, name string, ids ...string) domain.Family {
	t.Helper()
	f, err := domain.NewFamily(uuid.NewString(), tenant, name, ids, famAt)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func createFamily(t *testing.T, repo *pgadapter.Repository, tenant, name string, ids ...string) domain.Family {
	t.Helper()
	f := newFamily(t, tenant, name, ids...)
	if err := repo.CreateFamily(context.Background(), f, nil); err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	return f
}

// withAppTx runs fn as the app role with the tenant bound, as pkg/authctx does.
func withAppTx(t *testing.T, tenant string, fn func(tx *gorm.DB) error) error {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return fn(tx)
	})
}

// sqlState is the SQLSTATE of a Postgres error ("" for anything else).
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// holdRowLock locks one row FOR UPDATE as the superuser until release (or the
// end of the test), so the writers under test queue on it for real.
func holdRowLock(t *testing.T, table, id string) (release func()) {
	t.Helper()
	tx := superDB.Begin()
	if err := tx.Exec("SELECT 1 FROM "+table+" WHERE id = ? FOR UPDATE", id).Error; err != nil {
		t.Fatalf("hold lock on %s: %v", table, err)
	}
	var once sync.Once
	release = func() { once.Do(func() { tx.Rollback() }) }
	t.Cleanup(release)
	return release
}

// awaitLockWaiters blocks until n app-role sessions wait on a lock: proof the
// racing writers are both inside their transactions, queued on the held row.
func awaitLockWaiters(t *testing.T, n int64) {
	t.Helper()
	const poll = 5 * time.Millisecond
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int64
		err := superDB.Raw(`SELECT count(*) FROM pg_stat_activity WHERE usename = 'app' AND wait_event_type = 'Lock'`).
			Scan(&waiting).Error
		if err != nil {
			t.Fatalf("pg_stat_activity: %v", err)
		}
		if waiting >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d writers reached the lock", waiting, n)
		}
		time.Sleep(poll)
	}
}

// race runs the writers at once behind a lock held on (table, id), releases it
// once all of them wait, and returns their errors in order.
func race(t *testing.T, table, id string, writers ...func() error) []error {
	t.Helper()
	release := holdRowLock(t, table, id)
	errs := make([]error, len(writers))
	var done sync.WaitGroup
	for i, w := range writers {
		done.Add(1)
		go func() {
			defer done.Done()
			errs[i] = w()
		}()
	}
	awaitLockWaiters(t, int64(len(writers)))
	release()
	done.Wait()
	return errs
}

func TestFamilyCRUD(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, famTenantA)
	// Display order: surname, then first name — Zapata is listed last.
	zapata := seedPupil(t, famTenantA, "Zapata", "Ríos")
	alba := seedPupil(t, famTenantA, "Alba", "Ríos")

	f := createFamily(t, repo, famTenantA, "Ríos", zapata, alba)

	got, err := repo.FamilyByID(ctx, f.ID())
	if err != nil {
		t.Fatalf("FamilyByID: %v", err)
	}
	if got.Name() != "Ríos" || got.TenantID() != famTenantA || !slices.Equal(got.StudentIDs(), []string{alba, zapata}) ||
		got.AccountCount() != 0 || got.CreatedAt().IsZero() {
		t.Errorf("family = %+v", got)
	}
	// The pupils carry the family, and the list filters by it.
	if s, err := repo.FindByID(ctx, alba); err != nil || s.FamilyID() != f.ID() {
		t.Errorf("FindByID family = %q, %v", s.FamilyID(), err)
	}
	members, _, err := repo.List(ctx, ports.ListFilter{FamilyID: f.ID()})
	if err != nil || len(members) != 2 || members[0].ID() != alba {
		t.Errorf("List by family = %v, %v", members, err)
	}

	if err := repo.RenameFamily(ctx, f.ID(), "Ríos y más"); err != nil {
		t.Fatalf("RenameFamily: %v", err)
	}
	third := seedPupil(t, famTenantA, "Mora", "Ríos")
	if err := repo.AddStudentsToFamily(ctx, f.ID(), []string{third}, nil); err != nil {
		t.Fatalf("AddStudentsToFamily: %v", err)
	}
	got, _ = repo.FamilyByID(ctx, f.ID())
	if got.Name() != "Ríos y más" || !slices.Equal(got.StudentIDs(), []string{alba, third, zapata}) {
		t.Errorf("after rename+add = %+v", got)
	}

	all, err := repo.ListFamilies(ctx)
	if err != nil || !slices.ContainsFunc(all, func(x domain.Family) bool { return x.ID() == f.ID() && len(x.StudentIDs()) == 3 }) {
		t.Errorf("ListFamilies = %+v, %v", all, err)
	}

	dissolved, err := repo.RemoveStudentFromFamily(ctx, f.ID(), zapata)
	if err != nil || dissolved {
		t.Fatalf("Remove = %v, %v", dissolved, err)
	}
	if _, err := repo.RemoveStudentFromFamily(ctx, f.ID(), zapata); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("removing a non-member: %v", err)
	}
	if s, _ := repo.FindByID(ctx, zapata); s.FamilyID() != "" {
		t.Error("the removed pupil kept the family")
	}
	if _, err := repo.RemoveStudentFromFamily(ctx, f.ID(), alba); err != nil {
		t.Fatal(err)
	}
	if dissolved, err := repo.RemoveStudentFromFamily(ctx, f.ID(), third); err != nil || !dissolved {
		t.Fatalf("removing the last one: %v, %v", dissolved, err)
	}
	if _, err := repo.FamilyByID(ctx, f.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the emptied family survived: %v", err)
	}
}

func TestFamilyUnknownIsNotFound(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, famTenantA)
	unknown := uuid.NewString()
	if _, err := repo.FamilyByID(ctx, unknown); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("FamilyByID: %v", err)
	}
	if err := repo.RenameFamily(ctx, unknown, "X"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("RenameFamily: %v", err)
	}
	if err := repo.AddStudentsToFamily(ctx, unknown, []string{uuid.NewString()}, nil); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("AddStudentsToFamily: %v", err)
	}
	if err := repo.DissolveFamily(ctx, unknown); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("DissolveFamily: %v", err)
	}
}

func TestFamilyDissolveClearsEveryMember(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, famTenantA)
	a, b := seedPupil(t, famTenantA, "Paz", "Gil"), seedPupil(t, famTenantA, "Paz", "Gil")
	f := createFamily(t, repo, famTenantA, "Paz Gil", a, b)
	if err := repo.DissolveFamily(ctx, f.ID()); err != nil {
		t.Fatalf("DissolveFamily: %v", err)
	}
	for _, id := range []string{a, b} {
		if s, _ := repo.FindByID(ctx, id); s.FamilyID() != "" {
			t.Errorf("pupil %s kept the family", id)
		}
	}
	if _, err := repo.FamilyByID(ctx, f.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the family survived: %v", err)
	}
}

func TestFamilyRefusesAPupilAlreadyInOne(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, famTenantA)
	a, b, c := seedPupil(t, famTenantA, "Cruz", "Luna"), seedPupil(t, famTenantA, "Cruz", "Luna"), seedPupil(t, famTenantA, "Cruz", "Luna")
	first := createFamily(t, repo, famTenantA, "Cruz", a)

	second := newFamily(t, famTenantA, "Cruz bis", b, a)
	if err := repo.CreateFamily(ctx, second, nil); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("create over a member: want conflict, got %v", err)
	}
	// Nothing of the refused create was written: no family, b still free.
	if _, err := repo.FamilyByID(ctx, second.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the refused family exists: %v", err)
	}
	if s, _ := repo.FindByID(ctx, b); s.FamilyID() != "" {
		t.Error("the refused create moved a pupil")
	}

	other := createFamily(t, repo, famTenantA, "Otra", c)
	if err := repo.AddStudentsToFamily(ctx, other.ID(), []string{b, a}, nil); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("add a member of another family: want conflict, got %v", err)
	}
	if s, _ := repo.FindByID(ctx, b); s.FamilyID() != "" {
		t.Error("the refused add moved a pupil")
	}
	if got, _ := repo.FamilyByID(ctx, first.ID()); !slices.Equal(got.StudentIDs(), []string{a}) {
		t.Errorf("first family = %v", got.StudentIDs())
	}
}

func TestFamilyRefusesAWithdrawnPupil(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, famTenantA)
	gone := seedPupil(t, famTenantA, "Vera", "Sol")
	s, _ := repo.FindByID(ctx, gone)
	if _, err := s.Withdraw("se fue"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateFamily(ctx, newFamily(t, famTenantA, "Vera", gone), nil); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("a withdrawn pupil joined a family: %v", err)
	}
}

// Two clerks confirming two families over the same pupil at once: exactly one
// wins, the other is a conflict and writes nothing.
func TestFamilyConcurrentCreatesOverOnePupil(t *testing.T) {
	ctx := context.Background()
	shared := seedPupil(t, famTenantA, "Rey", "Mar")
	families := []domain.Family{
		newFamily(t, famTenantA, "Rey uno", shared, seedPupil(t, famTenantA, "Rey", "Mar")),
		newFamily(t, famTenantA, "Rey dos", shared, seedPupil(t, famTenantA, "Rey", "Mar")),
	}
	repos := []*pgadapter.Repository{appRepo(t, famTenantA), appRepo(t, famTenantA)}
	errs := race(t, "students", shared,
		func() error { return repos[0].CreateFamily(ctx, families[0], nil) },
		func() error { return repos[1].CreateFamily(ctx, families[1], nil) },
	)

	wins, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case glerrors.KindOf(err) == glerrors.KindConflict:
			conflicts++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d, want exactly one of each (%v)", wins, conflicts, errs)
	}
	winner, loser := families[0], families[1]
	if errs[0] != nil {
		winner, loser = families[1], families[0]
	}
	if s, _ := repos[0].FindByID(ctx, shared); s.FamilyID() != winner.ID() {
		t.Errorf("the shared pupil is in %q, want the winner %q", s.FamilyID(), winner.ID())
	}
	if _, err := repos[0].FamilyByID(ctx, loser.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the losing family was stored: %v", err)
	}
}

// Adding a pupil while the family is dissolved: whichever goes first, the
// family ends gone and nobody points at it.
func TestFamilyRaceAddVersusDissolve(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, famTenantA)
	a, b := seedPupil(t, famTenantA, "Toro", "Paz"), seedPupil(t, famTenantA, "Toro", "Paz")
	f := createFamily(t, repo, famTenantA, "Toro", a)
	other := appRepo(t, famTenantA)
	errs := race(t, "families", f.ID(),
		func() error { return repo.AddStudentsToFamily(ctx, f.ID(), []string{b}, nil) },
		func() error { return other.DissolveFamily(ctx, f.ID()) },
	)
	if errs[0] != nil && glerrors.KindOf(errs[0]) != glerrors.KindNotFound {
		t.Errorf("Add: %v (want success or NotFound)", errs[0])
	}
	if errs[1] != nil {
		t.Errorf("Dissolve: %v", errs[1])
	}
	if _, err := repo.FamilyByID(ctx, f.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the family survived its dissolution: %v", err)
	}
	for _, id := range []string{a, b} {
		if s, _ := repo.FindByID(ctx, id); s.FamilyID() != "" {
			t.Errorf("pupil %s points at a dissolved family", id)
		}
	}
}

// Removing the last pupil while another is added: either the family is
// dissolved and the add finds nothing, or the add lands first and the family
// keeps the new pupil. Never a family without members, never a dangling one.
func TestFamilyRaceRemoveLastVersusAdd(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, famTenantA)
	a, b := seedPupil(t, famTenantA, "Lobo", "Rey"), seedPupil(t, famTenantA, "Lobo", "Rey")
	f := createFamily(t, repo, famTenantA, "Lobo", a)
	other := appRepo(t, famTenantA)
	var dissolved bool
	errs := race(t, "families", f.ID(),
		func() error {
			var err error
			dissolved, err = repo.RemoveStudentFromFamily(ctx, f.ID(), a)
			return err
		},
		func() error { return other.AddStudentsToFamily(ctx, f.ID(), []string{b}, nil) },
	)
	if errs[0] != nil {
		t.Fatalf("Remove: %v", errs[0])
	}
	got, getErr := repo.FamilyByID(ctx, f.ID())
	sb, _ := repo.FindByID(ctx, b)
	switch {
	case errs[1] == nil: // the add went first
		if dissolved || getErr != nil || !slices.Equal(got.StudentIDs(), []string{b}) || sb.FamilyID() != f.ID() {
			t.Errorf("add first: dissolved=%v family=%+v (%v) b in %q", dissolved, got, getErr, sb.FamilyID())
		}
	case glerrors.KindOf(errs[1]) == glerrors.KindNotFound: // the removal went first
		if !dissolved || glerrors.KindOf(getErr) != glerrors.KindNotFound || sb.FamilyID() != "" {
			t.Errorf("remove first: dissolved=%v family err=%v b in %q", dissolved, getErr, sb.FamilyID())
		}
	default:
		t.Errorf("Add: %v (want success or NotFound)", errs[1])
	}
}

// A reception phone on 25 pupils already in families and 5 without one is
// still common: confirming families must not let it start chaining the rest.
// A value on exactly six pupils is not common.
func TestFamilySuggestionCommonValuesCountTheWholeSchool(t *testing.T) {
	// A tenant of its own per run: the suggestion reads see the whole school,
	// so a fixed tenant would read the pupils an earlier run (-count>1) left.
	tenant := uuid.NewString()
	const reception, six = "4437770000", "seis@x.mx"
	ctx := context.Background()
	repo := appRepo(t, tenant)
	contact := func(studentID string, person domain.ContactPerson) {
		t.Helper()
		person.FullName = "Recepción"
		c, err := domain.NewContact(uuid.NewString(), tenant, studentID, domain.RelationshipOther, person)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Link(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	var free, inFamily []string
	for i := range 30 {
		id := seedPupil(t, tenant, fmt.Sprintf("Apellido%02d", i), "Único")
		contact(id, domain.ContactPerson{Phone: reception})
		if i < 5 {
			free = append(free, id)
		} else {
			inFamily = append(inFamily, id)
		}
	}
	createFamily(t, repo, tenant, "Uno", inFamily[:domain.MaxFamilyPupils]...)
	createFamily(t, repo, tenant, "Dos", inFamily[domain.MaxFamilyPupils:]...)
	// six@: two free pupils + four in the first family.
	for _, id := range append(slices.Clone(free[:2]), inFamily[:4]...) {
		contact(id, domain.ContactPerson{Email: six})
	}

	in, err := repo.SuggestionInputs(ctx)
	if err != nil {
		t.Fatalf("SuggestionInputs: %v", err)
	}
	if !in.Common.Phones[reception] {
		t.Errorf("the reception phone is not common: %+v", in.Common)
	}
	if in.Common.Emails[six] {
		t.Errorf("a value on six pupils counted as common: %+v", in.Common)
	}
	got := domain.SuggestFamilies(in.Pupils, in.Contacts, in.Dismissed, in.Common)
	if len(got) != 1 || !slices.Equal(got[0].Reasons, []domain.SuggestionReason{domain.ReasonSharedContactEmail}) ||
		len(got[0].StudentIDs) != 2 {
		t.Errorf("suggestions = %+v, want only the two free pupils sharing the email", got)
	}
}

func TestFamilyAccountCount(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, famTenantA)
	a, b := seedPupil(t, famTenantA, "Nava", "Ruiz"), seedPupil(t, famTenantA, "Nava", "Ruiz")
	mom, dad := uuid.NewString(), uuid.NewString()
	for _, l := range []struct{ user, student string }{{mom, a}, {mom, b}, {dad, a}} {
		link, err := domain.NewGuardianLink(uuid.NewString(), famTenantA, l.user, l.student, domain.RelationshipMother)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Link(ctx, link); err != nil {
			t.Fatal(err)
		}
	}
	// An account-less contact does not count.
	contact, err := domain.NewContact(uuid.NewString(), famTenantA, b, domain.RelationshipOther, domain.ContactPerson{FullName: "Abuela", Phone: "4431234567"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Link(ctx, contact); err != nil {
		t.Fatal(err)
	}
	f := createFamily(t, repo, famTenantA, "Nava", a, b)
	got, err := repo.FamilyByID(ctx, f.ID())
	if err != nil || got.AccountCount() != 2 {
		t.Errorf("account count = %d, %v; want 2 distinct accounts", got.AccountCount(), err)
	}
	all, _ := repo.ListFamilies(ctx)
	for _, x := range all {
		if x.ID() == f.ID() && x.AccountCount() != 2 {
			t.Errorf("listed account count = %d", x.AccountCount())
		}
	}
}

func TestFamilySuggestionInputs(t *testing.T) {
	ctx := context.Background()
	// A tenant of its own per run, as above: these reads see the whole school.
	tenant := uuid.NewString()
	repo := appRepo(t, tenant)
	free := seedPupil(t, tenant, "Ortiz", "Lara")
	member := seedPupil(t, tenant, "Ortiz", "Lara")
	gone := seedPupil(t, tenant, "Ortiz", "Lara")
	createFamily(t, repo, tenant, "Ortiz", member)
	s, _ := repo.FindByID(ctx, gone)
	if _, err := s.Withdraw("baja"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, s); err != nil {
		t.Fatal(err)
	}
	user := uuid.NewString()
	for _, sid := range []string{free, member} {
		link, err := domain.NewGuardianLink(uuid.NewString(), tenant, user, sid, domain.RelationshipFather)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Link(ctx, link.WithPerson(domain.ContactPerson{Phone: "4435550000", Email: "papa@x.mx"})); err != nil {
			t.Fatal(err)
		}
	}
	sig := domain.SuggestionSignature([]string{free, uuid.NewString()})
	if err := repo.DismissSuggestion(ctx, ports.FamilyDismissal{ID: uuid.NewString(), Signature: sig, DismissedBy: user}); err != nil {
		t.Fatalf("DismissSuggestion: %v", err)
	}

	in, err := repo.SuggestionInputs(ctx)
	if err != nil {
		t.Fatalf("SuggestionInputs: %v", err)
	}
	if len(in.Pupils) != 1 || in.Pupils[0] != (domain.SuggestionPupil{ID: free, LastName: "Ortiz", MaternalLastName: "Lara"}) {
		t.Errorf("pupils = %+v, want only the free one", in.Pupils)
	}
	want := domain.SuggestionContact{StudentID: free, Email: "papa@x.mx", Phone: "4435550000", GuardianUserID: user}
	if len(in.Contacts) != 1 || in.Contacts[0] != want {
		t.Errorf("contacts = %+v, want %+v", in.Contacts, want)
	}
	if len(in.Dismissed) != 1 || !in.Dismissed[sig] {
		t.Errorf("dismissed = %v", in.Dismissed)
	}
}

func TestFamilyDismissalIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, famTenantA)
	sig := domain.SuggestionSignature([]string{uuid.NewString(), uuid.NewString()})
	first := uuid.NewString()
	for range 2 {
		if err := repo.DismissSuggestion(ctx, ports.FamilyDismissal{ID: uuid.NewString(), Signature: sig, DismissedBy: first}); err != nil {
			t.Fatalf("DismissSuggestion: %v", err)
		}
	}
	// No user recorded is fine too.
	if err := repo.DismissSuggestion(ctx, ports.FamilyDismissal{ID: uuid.NewString(), Signature: sig}); err != nil {
		t.Fatalf("DismissSuggestion without user: %v", err)
	}
	var rows []struct{ DismissedBy *string }
	err := withAppTx(t, famTenantA, func(tx *gorm.DB) error {
		return tx.Raw("SELECT dismissed_by FROM family_suggestion_dismissals WHERE signature = ?", sig).Scan(&rows).Error
	})
	if err != nil || len(rows) != 1 || rows[0].DismissedBy == nil || *rows[0].DismissedBy != first {
		t.Errorf("rows = %+v, %v; want one, by the first dismisser", rows, err)
	}
}

// RLS as the app role: tenant B cannot read, rename, grow, dissolve or empty
// tenant A's families, cannot point its own pupil at one, and cannot see or
// delete A's dismissals.
func TestRLSIsolatesFamilies(t *testing.T) {
	ctx := context.Background()
	repoA, repoB := appRepo(t, famTenantA), appRepo(t, famTenantB)
	pupilA := seedPupil(t, famTenantA, "Islas", "Mena")
	famA := createFamily(t, repoA, famTenantA, "Islas", pupilA)
	pupilB := seedPupil(t, famTenantB, "Islas", "Mena")

	if _, err := repoB.FamilyByID(ctx, famA.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("B read A's family: %v", err)
	}
	if all, _ := repoB.ListFamilies(ctx); slices.ContainsFunc(all, func(f domain.Family) bool { return f.ID() == famA.ID() }) {
		t.Error("B listed A's family")
	}
	if err := repoB.RenameFamily(ctx, famA.ID(), "Robada"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("B renamed A's family: %v", err)
	}
	if err := repoB.AddStudentsToFamily(ctx, famA.ID(), []string{pupilB}, nil); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("B added to A's family: %v", err)
	}
	if _, err := repoB.RemoveStudentFromFamily(ctx, famA.ID(), pupilA); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("B removed from A's family: %v", err)
	}
	if err := repoB.DissolveFamily(ctx, famA.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("B dissolved A's family: %v", err)
	}
	// Even a raw write cannot point B's pupil at A's family: the composite FK
	// (tenant_id, family_id) has no such pair.
	err := withAppTx(t, famTenantB, func(tx *gorm.DB) error {
		return tx.Exec("UPDATE students SET family_id = ? WHERE id = ?", famA.ID(), pupilB).Error
	})
	if sqlState(err) != "23503" {
		t.Errorf("B pointing its pupil at A's family: want FK violation 23503, got %v", err)
	}
	// Nor delete A's family row directly. The family has NO members, so no FK
	// can refuse the delete: only RLS hides the row.
	empty := uuid.NewString()
	if err := superDB.Exec("INSERT INTO families (id, tenant_id, name) VALUES (?, ?, 'Vacía')", empty, famTenantA).Error; err != nil {
		t.Fatal(err)
	}
	var deleted int64
	err = withAppTx(t, famTenantB, func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM families WHERE id = ?", empty)
		deleted = res.RowsAffected
		return res.Error
	})
	if err != nil || deleted != 0 {
		t.Errorf("B deleting A's empty family: err=%v rows=%d, want no error and 0 rows", err, deleted)
	}
	if err := withAppTx(t, famTenantA, func(tx *gorm.DB) error {
		var n int64
		if err := tx.Raw("SELECT count(*) FROM families WHERE id = ?", empty).Scan(&n).Error; err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("A's empty family is gone")
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
	// Nor create a family in A's name.
	err = withAppTx(t, famTenantB, func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO families (id, tenant_id, name) VALUES (?, ?, 'Intrusa')", uuid.NewString(), famTenantA).Error
	})
	if sqlState(err) != "42501" {
		t.Errorf("B inserting a family for A: want RLS violation 42501, got %v", err)
	}

	sig := domain.SuggestionSignature([]string{pupilA, uuid.NewString()})
	if err := repoA.DismissSuggestion(ctx, ports.FamilyDismissal{ID: uuid.NewString(), Signature: sig}); err != nil {
		t.Fatal(err)
	}
	if in, err := repoB.SuggestionInputs(ctx); err != nil || in.Dismissed[sig] {
		t.Errorf("B saw A's dismissal: %v", err)
	}
	var gone int64
	err = withAppTx(t, famTenantB, func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM family_suggestion_dismissals WHERE signature = ?", sig)
		gone = res.RowsAffected
		return res.Error
	})
	if err != nil || gone != 0 {
		t.Errorf("B deleting A's dismissal: err=%v rows=%d", err, gone)
	}
	// The same signature is B's own decision to make.
	if err := repoB.DismissSuggestion(ctx, ports.FamilyDismissal{ID: uuid.NewString(), Signature: sig}); err != nil {
		t.Errorf("B dismissing its own: %v", err)
	}

	got, err := repoA.FamilyByID(ctx, famA.ID())
	if err != nil || got.Name() != "Islas" || !slices.Equal(got.StudentIDs(), []string{pupilA}) {
		t.Errorf("A's family after B's attempts = %+v, %v", got, err)
	}
	if in, _ := repoA.SuggestionInputs(ctx); !in.Dismissed[sig] {
		t.Error("A's dismissal is gone")
	}
}
