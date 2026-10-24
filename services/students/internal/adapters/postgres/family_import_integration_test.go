//go:build integration

// Integration tests for families from an import (PLAN-familias-importacion.md,
// migration 0024): the pupil's own email and phones in the suggestions, the
// import's inputs, building and undoing a batch's families through the real
// use case, and RLS on family_import_links — as the non-owner `app` role.
//
// Run: go test -tags=integration ./internal/adapters/postgres/ -run 'Import.*Famil|FamilyImport|FileContacts'
package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/students/internal/adapters/postgres"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// setFile writes a pupil's own email and phones as the import stores them.
func setFile(t *testing.T, studentID, email string, phones ...string) {
	t.Helper()
	type phone struct {
		Kind   string `json:"kind"`
		Number string `json:"number"`
	}
	list := make([]phone, 0, len(phones))
	for _, p := range phones {
		list = append(list, phone{Kind: "celular", Number: p})
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if err := superDB.Exec("UPDATE students SET email = ?, phones = ?::jsonb WHERE id = ?", email, string(raw), studentID).Error; err != nil {
		t.Fatalf("set file contacts: %v", err)
	}
}

// markBatch records that the import batch created these pupils.
func markBatch(t *testing.T, batch string, ids ...string) {
	t.Helper()
	if err := superDB.Exec("UPDATE students SET import_batch_id = ? WHERE id IN ?", batch, ids).Error; err != nil {
		t.Fatalf("mark batch: %v", err)
	}
}

// addContact records an account-less contact with these values on a pupil.
func addContact(t *testing.T, repo *pgadapter.Repository, tenant, studentID string, person domain.ContactPerson) {
	t.Helper()
	person.FullName = "Contacto"
	c, err := domain.NewContact(uuid.NewString(), tenant, studentID, domain.RelationshipOther, person)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Link(context.Background(), c); err != nil {
		t.Fatalf("add contact: %v", err)
	}
}

// addAccount links an app account to a pupil.
func addAccount(t *testing.T, repo *pgadapter.Repository, tenant, studentID string) {
	t.Helper()
	l, err := domain.NewGuardianLink(uuid.NewString(), tenant, uuid.NewString(), studentID, domain.RelationshipMother)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Link(context.Background(), l); err != nil {
		t.Fatalf("add account: %v", err)
	}
}

func importLinkCount(t *testing.T, batch string) int64 {
	t.Helper()
	var n int64
	if err := superDB.Raw("SELECT count(*) FROM family_import_links WHERE batch_id = ?", batch).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func familyOf(t *testing.T, repo *pgadapter.Repository, studentID string) string {
	t.Helper()
	s, err := repo.FindByID(context.Background(), studentID)
	if err != nil {
		t.Fatal(err)
	}
	return s.FamilyID()
}

// Imported pupils carry their contact data on their own file: the
// suggestions read it, so siblings sharing a file phone are HIGH confidence,
// and a file email meets a contact's email.
func TestFamilySuggestionsReadTheFileContacts(t *testing.T) {
	tenant := uuid.NewString()
	ctx := context.Background()
	repo := appRepo(t, tenant)
	a, b := seedPupil(t, tenant, "García", "López"), seedPupil(t, tenant, "García", "López")
	setFile(t, a, "", "4435550000")
	setFile(t, b, "", "443 555 0000", "4439990000")
	c, d := seedPupil(t, tenant, "Soto", "Gil"), seedPupil(t, tenant, "Luna", "Paz")
	setFile(t, c, "casa@x.mx")
	addContact(t, repo, tenant, d, domain.ContactPerson{Email: "casa@x.mx"})

	in, err := repo.SuggestionInputs(ctx)
	if err != nil {
		t.Fatalf("SuggestionInputs: %v", err)
	}
	got := domain.SuggestFamilies(in.Pupils, in.Contacts, in.Dismissed, in.Common)
	want := map[string][]domain.SuggestionReason{
		domain.SuggestionSignature([]string{a, b}): {domain.ReasonSameSurnames, domain.ReasonSharedContactPhone},
		domain.SuggestionSignature([]string{c, d}): {domain.ReasonSharedContactEmail},
	}
	if len(got) != len(want) {
		t.Fatalf("suggestions = %+v, want %d", got, len(want))
	}
	for _, s := range got {
		if !slices.Equal(s.Reasons, want[s.Signature]) || s.Confidence != domain.ConfidenceHigh {
			t.Errorf("suggestion %v: reasons %v confidence %v", s.StudentIDs, s.Reasons, s.Confidence)
		}
	}
}

// A file value counts toward the school-wide cap like a contact's, once per
// pupil: seven files with one phone make it common; six pupils, one carrying
// it on its file AND a contact, do not.
func TestFamilyCommonValuesCountFileContactsOncePerPupil(t *testing.T) {
	tenant := uuid.NewString()
	ctx := context.Background()
	repo := appRepo(t, tenant)
	const reception, shared = "4437770000", "4438880000"
	for i := range domain.MaxPupilsPerSharedContact + 1 {
		id := seedPupil(t, tenant, fmt.Sprintf("Recepción%02d", i), "Único")
		setFile(t, id, "", reception)
	}
	for i := range domain.MaxPupilsPerSharedContact {
		id := seedPupil(t, tenant, fmt.Sprintf("Seis%02d", i), "Único")
		setFile(t, id, "", shared)
		if i == 0 {
			addContact(t, repo, tenant, id, domain.ContactPerson{Phone: shared})
		}
	}
	in, err := repo.SuggestionInputs(ctx)
	if err != nil {
		t.Fatalf("SuggestionInputs: %v", err)
	}
	if !in.Common.Phones[reception] {
		t.Errorf("a phone on seven files is not common: %+v", in.Common)
	}
	if in.Common.Phones[shared] {
		t.Errorf("a phone on six pupils counted as common: %+v", in.Common)
	}
}

func TestImportFamilyInputs(t *testing.T) {
	tenant := uuid.NewString()
	ctx := context.Background()
	repo := appRepo(t, tenant)
	batch := uuid.NewString()
	fresh := seedPupil(t, tenant, "Ortiz", "Lara")
	member := seedPupil(t, tenant, "Ortiz", "Lara")
	gone := seedPupil(t, tenant, "Ortiz", "Lara")
	markBatch(t, batch, fresh, gone)
	setFile(t, fresh, "ortiz@x.mx", "4435550000")
	addAccount(t, repo, tenant, member)
	addContact(t, repo, tenant, member, domain.ContactPerson{Phone: "4435550000"})
	f := createFamily(t, repo, tenant, "Ortiz", member)
	s, _ := repo.FindByID(ctx, gone)
	if _, err := s.Withdraw("baja"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, s); err != nil {
		t.Fatal(err)
	}

	in, err := repo.ImportFamilyInputs(ctx, batch)
	if err != nil {
		t.Fatalf("ImportFamilyInputs: %v", err)
	}
	wantPupils := map[string]domain.ImportPupil{
		fresh:  {ID: fresh, LastName: "Ortiz", MaternalLastName: "Lara", InBatch: true},
		member: {ID: member, LastName: "Ortiz", MaternalLastName: "Lara", FamilyID: f.ID(), HasAccount: true},
	}
	if len(in.Pupils) != len(wantPupils) {
		t.Fatalf("pupils = %+v, want the two still at school", in.Pupils)
	}
	for _, p := range in.Pupils {
		if p != wantPupils[p.ID] {
			t.Errorf("pupil = %+v, want %+v", p, wantPupils[p.ID])
		}
	}
	has := func(c domain.SuggestionContact) bool { return slices.Contains(in.Contacts, c) }
	for _, c := range []domain.SuggestionContact{
		{StudentID: fresh, Email: "ortiz@x.mx"},
		{StudentID: fresh, Phone: "4435550000"},
		{StudentID: member, Phone: "4435550000"},
	} {
		if !has(c) {
			t.Errorf("contacts %+v lack %+v", in.Contacts, c)
		}
	}
	if got := in.Families[f.ID()]; got != (domain.ImportFamilyState{Pupils: 1, Accounts: 1}) {
		t.Errorf("family state = %+v", got)
	}
}

// The whole round trip through the use case: a batch builds one family,
// joins another, leaves an account holder to the office; a second build
// does nothing; undo puts everything back, twice.
func TestBuildAndUndoFamiliesFromImport(t *testing.T) {
	tenant := uuid.NewString()
	ctx := context.Background()
	_, families, repo := accessServices(t, tenant)
	batch := uuid.NewString()

	a, b := seedPupil(t, tenant, "García", "López"), seedPupil(t, tenant, "García", "López")
	setFile(t, a, "", "4435550000")
	setFile(t, b, "", "4435550000")
	older := seedPupil(t, tenant, "Soto", "Gil") // an earlier level's import
	addContact(t, repo, tenant, older, domain.ContactPerson{Phone: "4436660000"})
	soto := createFamily(t, repo, tenant, "Soto Gil", older)
	c := seedPupil(t, tenant, "Soto", "Gil")
	setFile(t, c, "", "4436660000")
	e, g := seedPupil(t, tenant, "Ruiz", "Paz"), seedPupil(t, tenant, "Ruiz", "Paz")
	setFile(t, e, "ruiz@x.mx")
	setFile(t, g, "ruiz@x.mx")
	addAccount(t, repo, tenant, e)
	markBatch(t, batch, a, b, c, e, g)

	res, err := families.BuildFamiliesFromImport(ctx, batch)
	if err != nil {
		t.Fatalf("BuildFamiliesFromImport: %v", err)
	}
	if len(res.Created) != 1 || len(res.Joined) != 1 || res.Skipped != 1 {
		t.Fatalf("result = %+v, want 1 created, 1 joined, 1 skipped", res)
	}
	built := res.Created[0]
	if !sameIDs(built.StudentIDs(), []string{a, b}) || built.Name() != "García López" {
		t.Errorf("created = %+v", built)
	}
	if res.Joined[0].ID() != soto.ID() || !sameIDs(res.Joined[0].StudentIDs(), []string{older, c}) {
		t.Errorf("joined = %+v", res.Joined[0])
	}
	if familyOf(t, repo, e) != "" || familyOf(t, repo, g) != "" {
		t.Error("the account holder's group was built")
	}
	if n := importLinkCount(t, batch); n != 3 {
		t.Errorf("links = %d, want 3", n)
	}

	again, err := families.BuildFamiliesFromImport(ctx, batch)
	if err != nil || len(again.Created) != 0 || len(again.Joined) != 0 {
		t.Errorf("second build = %+v, %v; want nothing new", again, err)
	}
	if n := importLinkCount(t, batch); n != 3 {
		t.Errorf("links after a second build = %d, want 3", n)
	}

	undone, err := families.UndoFamiliesFromImport(ctx, batch)
	if err != nil || undone != (usecase.ImportUndoResult{Removed: 3, Dissolved: 1}) {
		t.Fatalf("undo = %+v, %v; want 3 removed, 1 dissolved", undone, err)
	}
	if _, err := repo.FamilyByID(ctx, built.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the built family survived the undo: %v", err)
	}
	kept, err := repo.FamilyByID(ctx, soto.ID())
	if err != nil || !slices.Equal(kept.StudentIDs(), []string{older}) {
		t.Errorf("the joined family after undo = %+v, %v; want only its own pupil", kept, err)
	}
	for _, id := range []string{a, b, c} {
		if familyOf(t, repo, id) != "" {
			t.Errorf("pupil %s still in a family after undo", id)
		}
	}
	if n := importLinkCount(t, batch); n != 0 {
		t.Errorf("links after undo = %d", n)
	}
	if again, err := families.UndoFamiliesFromImport(ctx, batch); err != nil || again != (usecase.ImportUndoResult{}) {
		t.Errorf("second undo = %+v, %v; want nothing", again, err)
	}
}

// Undo leaves alone a pupil the office moved, and a pupil the office added to
// a built family stays in it: a family is dissolved only when emptied.
func TestUndoFamiliesFromImportRespectsTheOffice(t *testing.T) {
	tenant := uuid.NewString()
	ctx := context.Background()
	_, families, repo := accessServices(t, tenant)
	batch := uuid.NewString()
	a, b, c := seedPupil(t, tenant, "Nava", "Ruiz"), seedPupil(t, tenant, "Nava", "Ruiz"), seedPupil(t, tenant, "Nava", "Ruiz")
	for _, id := range []string{a, b, c} {
		setFile(t, id, "nava@x.mx")
	}
	markBatch(t, batch, a, b, c)
	res, err := families.BuildFamiliesFromImport(ctx, batch)
	if err != nil || len(res.Created) != 1 {
		t.Fatalf("build = %+v, %v", res, err)
	}
	built := res.Created[0].ID()

	// The office moves c to a family of its own with a pupil from elsewhere,
	// and adds a stranger x to the built family.
	if _, _, err := families.RemoveStudentFromFamily(ctx, built, c); err != nil {
		t.Fatal(err)
	}
	other := seedPupil(t, tenant, "Vega", "Mora")
	moved := createFamily(t, repo, tenant, "Vega", other, c)
	x := seedPupil(t, tenant, "Paz", "Lima")
	if _, err := families.AddStudentsToFamily(ctx, built, []string{x}); err != nil {
		t.Fatal(err)
	}

	got, err := families.UndoFamiliesFromImport(ctx, batch)
	if err != nil || got != (usecase.ImportUndoResult{Removed: 2}) {
		t.Fatalf("undo = %+v, %v; want 2 removed, none dissolved", got, err)
	}
	if familyOf(t, repo, x) != built {
		t.Error("the pupil the office added lost its family")
	}
	if familyOf(t, repo, c) != moved.ID() {
		t.Error("undo moved a pupil the office had moved")
	}
}

// Undo never leaves an app account behind on pupils it would split off: a
// family any of whose members has an account is kept whole, links and all
// (the office revokes the access first), and the batch's other families are
// still undone.
func TestUndoFamiliesFromImportKeepsAFamilyWithAppAccess(t *testing.T) {
	tenant := uuid.NewString()
	ctx := context.Background()
	_, families, repo := accessServices(t, tenant)
	batch := uuid.NewString()
	a, b := seedPupil(t, tenant, "García", "López"), seedPupil(t, tenant, "García", "López")
	c, d := seedPupil(t, tenant, "Nava", "Ruiz"), seedPupil(t, tenant, "Nava", "Ruiz")
	setFile(t, a, "", "4435550000")
	setFile(t, b, "", "4435550000")
	setFile(t, c, "nava@x.mx")
	setFile(t, d, "nava@x.mx")
	markBatch(t, batch, a, b, c, d)
	res, err := families.BuildFamiliesFromImport(ctx, batch)
	if err != nil || len(res.Created) != 2 {
		t.Fatalf("build = %+v, %v", res, err)
	}
	withAccess := familyOf(t, repo, a)
	addAccount(t, repo, tenant, b)

	got, err := families.UndoFamiliesFromImport(ctx, batch)
	if err != nil || got != (usecase.ImportUndoResult{Removed: 2, Dissolved: 1, KeptWithAccess: 1}) {
		t.Fatalf("undo = %+v, %v; want 2 removed, 1 dissolved, 1 kept", got, err)
	}
	if familyOf(t, repo, a) != withAccess || familyOf(t, repo, b) != withAccess {
		t.Error("undo split a family with an app account")
	}
	if familyOf(t, repo, c) != "" || familyOf(t, repo, d) != "" {
		t.Error("the batch's other family was not undone")
	}
	if n := importLinkCount(t, batch); n != 2 {
		t.Errorf("links = %d, want the kept family's two", n)
	}
}

// An undo reads a bounded number of links; what it did not read stays for
// the next call, and a family is dissolved only once emptied.
func TestUndoFamiliesFromImportInChunks(t *testing.T) {
	t.Cleanup(pgadapter.SetMaxUndoLinksForTest(2))
	tenant := uuid.NewString()
	ctx := context.Background()
	_, families, repo := accessServices(t, tenant)
	batch := uuid.NewString()
	ids := []string{seedPupil(t, tenant, "Mora", "Lis"), seedPupil(t, tenant, "Mora", "Lis"), seedPupil(t, tenant, "Mora", "Lis")}
	for _, id := range ids {
		setFile(t, id, "", "4432223333")
	}
	markBatch(t, batch, ids...)
	if res, err := families.BuildFamiliesFromImport(ctx, batch); err != nil || len(res.Created) != 1 {
		t.Fatalf("build = %+v, %v", res, err)
	}
	built := familyOf(t, repo, ids[0])

	first, err := families.UndoFamiliesFromImport(ctx, batch)
	if err != nil || first != (usecase.ImportUndoResult{Removed: 2}) {
		t.Fatalf("first undo = %+v, %v; want 2 removed", first, err)
	}
	if n := importLinkCount(t, batch); n != 1 {
		t.Errorf("links after the first undo = %d, want the unread one", n)
	}
	second, err := families.UndoFamiliesFromImport(ctx, batch)
	if err != nil || second != (usecase.ImportUndoResult{Removed: 1, Dissolved: 1}) {
		t.Fatalf("second undo = %+v, %v; want 1 removed, 1 dissolved", second, err)
	}
	if _, err := repo.FamilyByID(ctx, built); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the emptied family survived: %v", err)
	}
	if n := importLinkCount(t, batch); n != 0 {
		t.Errorf("links after the second undo = %d", n)
	}
}

// A real race (security H1): an account grant on a pupil is open (OnPupil
// holds the pupil's row FOR SHARE) when the build plans the family without
// seeing it; the build's claim queues on that row, the grant commits, and the
// build's in-transaction re-check sees the account and skips the family —
// nothing written, no links.
func TestBuildFamiliesFromImportRacesAnAccountGrant(t *testing.T) {
	tenant := uuid.NewString()
	ctx := context.Background()
	_, families, repo := accessServices(t, tenant)
	batch := uuid.NewString()
	a, b := seedPupil(t, tenant, "Rangel", "Ochoa"), seedPupil(t, tenant, "Rangel", "Ochoa")
	setFile(t, a, "", "4434445555")
	setFile(t, b, "", "4434445555")
	markBatch(t, batch, a, b)

	held, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseGrant := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseGrant) // never leave the grant's transaction hanging
	granted := make(chan error, 1)
	go func() {
		granted <- repo.OnPupil(ctx, a, func(ctx context.Context, acc ports.FamilyAccess) error {
			l, err := domain.NewGuardianLink(uuid.NewString(), tenant, uuid.NewString(), a, domain.RelationshipMother)
			if err != nil {
				return err
			}
			if err := acc.InsertContact(ctx, l); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-granted:
		t.Fatalf("the grant ended before holding its lock: %v", err)
	}

	type outcome struct {
		res usecase.ImportFamiliesResult
		err error
	}
	built := make(chan outcome, 1)
	go func() {
		res, err := families.BuildFamiliesFromImport(ctx, batch)
		built <- outcome{res, err}
	}()
	awaitLockWaiters(t, 1)
	releaseGrant()
	if err := <-granted; err != nil {
		t.Fatalf("grant: %v", err)
	}
	got := <-built
	if got.err != nil || len(got.res.Created) != 0 || got.res.Skipped != 1 {
		t.Fatalf("build = %+v, %v; want the family skipped", got.res, got.err)
	}
	if familyOf(t, repo, a) != "" || familyOf(t, repo, b) != "" {
		t.Error("the family was built over an account")
	}
	if n := importLinkCount(t, batch); n != 0 {
		t.Errorf("links = %d, want none", n)
	}
}

func TestUndoFamiliesFromImportRefusesAMalformedBatch(t *testing.T) {
	_, families, _ := accessServices(t, uuid.NewString())
	if _, err := families.UndoFamiliesFromImport(context.Background(), "nope"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("malformed batch: %v", err)
	}
	if _, err := families.BuildFamiliesFromImport(context.Background(), "nope"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("malformed batch: %v", err)
	}
}

// RLS as the app role: tenant B cannot see, write, delete or undo tenant A's
// import links, nor point a link of its own at A's family or pupil.
func TestRLSIsolatesFamilyImportLinks(t *testing.T) {
	ctx := context.Background()
	// Tenants of their own: the build reads the whole school, so a fixed
	// tenant would see the pupils an earlier run (-count>1) left.
	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	_, familiesA, repoA := accessServices(t, tenantA)
	_, familiesB, _ := accessServices(t, tenantB)
	batch := uuid.NewString()
	a, b := seedPupil(t, tenantA, "Islas", "Ramos"), seedPupil(t, tenantA, "Islas", "Ramos")
	setFile(t, a, "", "4431112222")
	setFile(t, b, "", "4431112222")
	markBatch(t, batch, a, b)
	res, err := familiesA.BuildFamiliesFromImport(ctx, batch)
	if err != nil || len(res.Created) != 1 {
		t.Fatalf("A's build = %+v, %v", res, err)
	}
	famA := res.Created[0].ID()
	pupilB := seedPupil(t, tenantB, "Islas", "Ramos")

	var seen int64
	if err := withAppTx(t, tenantB, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM family_import_links WHERE batch_id = ?", batch).Scan(&seen).Error
	}); err != nil || seen != 0 {
		t.Errorf("B saw A's links: %d, %v", seen, err)
	}
	if got, err := familiesB.UndoFamiliesFromImport(ctx, batch); err != nil || got != (usecase.ImportUndoResult{}) {
		t.Errorf("B undid A's batch: %+v, %v", got, err)
	}
	var deleted int64
	if err := withAppTx(t, tenantB, func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM family_import_links WHERE batch_id = ?", batch)
		deleted = res.RowsAffected
		return res.Error
	}); err != nil || deleted != 0 {
		t.Errorf("B deleting A's links: %d, %v", deleted, err)
	}
	insert := func(tenant, familyID, studentID string) error {
		return withAppTx(t, tenantB, func(tx *gorm.DB) error {
			return tx.Exec(`INSERT INTO family_import_links (id, tenant_id, batch_id, family_id, student_id, created_family)
			                VALUES (?, ?, ?, ?, ?, true)`, uuid.NewString(), tenant, batch, familyID, studentID).Error
		})
	}
	if err := insert(tenantA, famA, a); sqlState(err) != "42501" {
		t.Errorf("B inserting a link for A: want RLS violation 42501, got %v", err)
	}
	if err := insert(tenantB, famA, pupilB); sqlState(err) != "23503" {
		t.Errorf("B linking to A's family: want FK violation 23503, got %v", err)
	}
	if n := importLinkCount(t, batch); n != 2 {
		t.Errorf("A's links after B's attempts = %d, want 2", n)
	}
	if got, err := repoA.FamilyByID(ctx, famA); err != nil || len(got.StudentIDs()) != 2 {
		t.Errorf("A's family after B's attempts = %+v, %v", got, err)
	}
}

func sameIDs(got, want []string) bool {
	g, w := slices.Clone(got), slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	return slices.Equal(g, w)
}
