package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// Family access (ADR 0041, T2): the cap of two accounts and the sync of every
// account to every placeable sibling, through GrantContactAccess,
// LinkGuardian, CreateFamily and AddStudentsToFamily, plus AddFamilyContact and
// RevokeFamilyAccess. The world is famWorld (family_test.go): pupils A
// (enrolled), B (pending), C (accepted), Gone (withdrawn), Draft.

const (
	accMom   = "60000000-0000-0000-0000-0000000000a1"
	accDad   = "60000000-0000-0000-0000-0000000000a2"
	accThird = "60000000-0000-0000-0000-0000000000a3"
)

// accWorld is famWorld plus the services T2 drives, with unique minted ids.
type accWorld struct {
	*famWorld
	users     *fakeUsers
	guardians *usecase.GuardianService
	families  *usecase.FamilyService
}

func newAccWorld() *accWorld {
	w := newFamWorld()
	users := newUsers()
	for _, id := range []string{accMom, accDad, accThird} {
		seedGuardianUser(users, id, "guardian")
	}
	n := 0
	newID := func() string { n++; return fmt.Sprintf("1d000000-0000-0000-0000-%012d", n) }
	return &accWorld{
		famWorld:  w,
		users:     users,
		guardians: usecase.NewGuardianService(w, w.links, w, users, famTenant, newID),
		families:  usecase.NewFamilyService(w, w, famTenant, newID, func() time.Time { return famNow }),
	}
}

// group puts pupils in family famID directly (no sync), as T1 left them.
func (w *accWorld) group(t *testing.T, ids ...string) {
	t.Helper()
	f, err := domain.NewFamily(famID, famTenant, "García", ids, famNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.CreateFamily(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}
}

// link stores a link with an account directly (as before T2, unsynced).
func (w *accWorld) link(t *testing.T, id, user, student string, person domain.ContactPerson) {
	t.Helper()
	l, err := domain.NewGuardianLink(id, famTenant, user, student, domain.RelationshipMother)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.links.Link(context.Background(), l.WithPerson(person)); err != nil {
		t.Fatal(err)
	}
}

// contact stores an account-less contact directly.
func (w *accWorld) contact(t *testing.T, id, student string, person domain.ContactPerson) {
	t.Helper()
	c, err := domain.NewContact(id, famTenant, student, domain.RelationshipOther, person)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.links.Link(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

// accountsOf is the set of accounts linked to a pupil, sorted.
func (w *accWorld) accountsOf(student string) []string {
	var out []string
	for _, l := range w.links.links {
		if l.StudentID() == student && l.HasAccount() {
			out = append(out, l.GuardianUserID())
		}
	}
	slices.Sort(out)
	return out
}

var (
	rosaPerson = domain.ContactPerson{FullName: "Rosa Pérez", Phone: "4431112233", Email: "rosa@example.com"}
	luisPerson = domain.ContactPerson{FullName: "Luis García", Phone: "4435556677"}
)

func TestGrantInAFamilySyncsTheAccountToEverySibling(t *testing.T) {
	w := newAccWorld()
	w.group(t, pupilA, pupilB, pupilGone, pupilDraft)
	w.contact(t, "c-a", pupilA, rosaPerson)
	// B already has Rosa as a contact (same email, other capitalisation).
	w.contact(t, "c-b", pupilB, domain.ContactPerson{FullName: "Rosa", Email: "ROSA@example.com"})

	got, err := w.guardians.GrantContactAccess(context.Background(), "c-a", pupilA, accMom)
	if err != nil {
		t.Fatalf("GrantContactAccess: %v", err)
	}
	if got.ID() != "c-a" || got.GuardianUserID() != accMom {
		t.Errorf("granted %+v", got)
	}
	if b, _ := w.links.ByID(context.Background(), "c-b"); b.GuardianUserID() != accMom {
		t.Errorf("B's own contact with Rosa's email did not receive the account: %+v", b)
	}
	if len(w.links.links) != 2 {
		t.Errorf("links = %d, want 2 (no second Rosa on B)", len(w.links.links))
	}
	for _, p := range []string{pupilGone, pupilDraft} {
		if acc := w.accountsOf(p); len(acc) != 0 {
			t.Errorf("pupil %s got accounts %v; drafts and pupils who left get none", p, acc)
		}
	}
	if w.onPupilCalls != 1 {
		t.Errorf("OnPupil calls = %d, want the grant under the family lock", w.onPupilCalls)
	}
}

func TestGrantRefusesAThirdAccountAndWritesNothing(t *testing.T) {
	w := newAccWorld()
	w.group(t, pupilA, pupilB)
	w.link(t, "l-mom", accMom, pupilA, rosaPerson)
	w.link(t, "l-dad", accDad, pupilB, luisPerson)
	w.contact(t, "c-b", pupilB, domain.ContactPerson{FullName: "Tía Ana"})
	before := maps.Clone(w.links.links)

	_, err := w.guardians.GrantContactAccess(context.Background(), "c-b", pupilB, accThird)
	if !errors.Is(err, domain.ErrFamilyAccountLimit) {
		t.Fatalf("err = %v, want ErrFamilyAccountLimit", err)
	}
	if !maps.EqualFunc(before, w.links.links, func(a, b domain.GuardianLink) bool { return a == b }) {
		t.Error("a refused grant left writes behind (attach or sync not rolled back)")
	}
}

func TestGrantOfAnAccountTheFamilyHasCountsOnce(t *testing.T) {
	w := newAccWorld()
	w.group(t, pupilA, pupilB)
	w.link(t, "l-mom", accMom, pupilA, rosaPerson)
	w.link(t, "l-dad", accDad, pupilA, luisPerson)
	w.contact(t, "c-b", pupilB, domain.ContactPerson{FullName: "Rosa (otra captura)"})

	if _, err := w.guardians.GrantContactAccess(context.Background(), "c-b", pupilB, accMom); err != nil {
		t.Fatalf("re-granting the family's own account: %v", err)
	}
	if got := w.accountsOf(pupilB); !slices.Equal(got, []string{accMom, accDad}) {
		t.Errorf("B accounts = %v, want mom (granted) and dad (synced)", got)
	}
}

func TestGrantOnAPupilWithoutFamilyHasNoCap(t *testing.T) {
	w := newAccWorld()
	w.link(t, "l-mom", accMom, pupilC, rosaPerson)
	w.link(t, "l-dad", accDad, pupilC, luisPerson)
	w.contact(t, "c-c", pupilC, domain.ContactPerson{FullName: "Abuela"})

	if _, err := w.guardians.GrantContactAccess(context.Background(), "c-c", pupilC, accThird); err != nil {
		t.Fatalf("a family-less pupil keeps today's behaviour: %v", err)
	}
	if w.onPupilCalls != 1 {
		t.Errorf("OnPupil calls = %d: a family-less grant must still lock the pupil", w.onPupilCalls)
	}
}

func TestLinkGuardianInAFamilySyncsAndCaps(t *testing.T) {
	w := newAccWorld()
	w.group(t, pupilA, pupilB)
	ctx := context.Background()

	if _, err := w.guardians.LinkGuardian(ctx, accMom, pupilA, "mother"); err != nil {
		t.Fatalf("LinkGuardian: %v", err)
	}
	if got := w.accountsOf(pupilB); !slices.Equal(got, []string{accMom}) {
		t.Errorf("B accounts = %v, want mom synced", got)
	}
	if _, err := w.guardians.LinkGuardian(ctx, accDad, pupilB, "father"); err != nil {
		t.Fatalf("second account: %v", err)
	}
	before := maps.Clone(w.links.links)
	if _, err := w.guardians.LinkGuardian(ctx, accThird, pupilA, "other"); !errors.Is(err, domain.ErrFamilyAccountLimit) {
		t.Fatalf("third account: err = %v, want ErrFamilyAccountLimit", err)
	}
	if len(w.links.links) != len(before) {
		t.Errorf("links %d → %d: the refused link was stored", len(before), len(w.links.links))
	}
}

func TestCreateFamilySyncsTheMergedAccounts(t *testing.T) {
	w := newAccWorld()
	w.link(t, "l-mom", accMom, pupilA, rosaPerson)
	w.link(t, "l-dad", accDad, pupilB, luisPerson)

	f, err := w.families.CreateFamily(context.Background(), "", []string{pupilA, pupilB, pupilDraft})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	for _, p := range []string{pupilA, pupilB} {
		if got := w.accountsOf(p); !slices.Equal(got, []string{accMom, accDad}) {
			t.Errorf("%s accounts = %v, want both parents", p, got)
		}
	}
	if got := w.accountsOf(pupilDraft); len(got) != 0 {
		t.Errorf("the draft got %v", got)
	}
	if f.ID() == "" {
		t.Error("no family returned")
	}
}

func TestCreateFamilyRefusesMergingMoreThanTwoAccounts(t *testing.T) {
	w := newAccWorld()
	w.link(t, "l-mom", accMom, pupilA, rosaPerson)
	w.link(t, "l-dad", accDad, pupilA, luisPerson)
	w.link(t, "l-3", accThird, pupilB, domain.ContactPerson{FullName: "Otro"})
	before := maps.Clone(w.links.links)

	_, err := w.families.CreateFamily(context.Background(), "", []string{pupilA, pupilB})
	if !errors.Is(err, domain.ErrFamilyAccountLimit) {
		t.Fatalf("err = %v, want ErrFamilyAccountLimit", err)
	}
	if len(w.famWorld.families) != 0 || w.pupils[pupilA].FamilyID() != "" || len(w.links.links) != len(before) {
		t.Error("a refused merge left a family, a membership or a link behind")
	}
}

func TestAddStudentsToFamilySyncsAndCaps(t *testing.T) {
	w := newAccWorld()
	w.group(t, pupilA)
	w.link(t, "l-mom", accMom, pupilA, rosaPerson)
	ctx := context.Background()

	if _, err := w.families.AddStudentsToFamily(ctx, famID, []string{pupilB}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := w.accountsOf(pupilB); !slices.Equal(got, []string{accMom}) {
		t.Errorf("B accounts = %v, want mom synced", got)
	}
	w.link(t, "l-dad", accDad, pupilA, luisPerson)
	w.link(t, "l-3", accThird, pupilC, domain.ContactPerson{FullName: "Otro"})
	if _, err := w.families.AddStudentsToFamily(ctx, famID, []string{pupilC}); !errors.Is(err, domain.ErrFamilyAccountLimit) {
		t.Fatalf("over the cap: err = %v", err)
	}
	if w.pupils[pupilC].FamilyID() != "" {
		t.Error("the refused pupil joined anyway")
	}
}

func TestAddFamilyContact(t *testing.T) {
	w := newAccWorld()
	w.group(t, pupilA, pupilB, pupilGone, pupilDraft)
	w.contact(t, "c-a", pupilA, domain.ContactPerson{FullName: "Rosa", Phone: "443-111-2233"})
	ctx := context.Background()

	got, err := w.families.AddFamilyContact(ctx, famID, "mother", rosaPerson)
	if err != nil {
		t.Fatalf("AddFamilyContact: %v", err)
	}
	students := make([]string, 0, len(got))
	for _, c := range got {
		students = append(students, c.StudentID())
		if c.HasAccount() {
			t.Errorf("a family contact came with an account: %+v", c)
		}
	}
	if !slices.Equal(students, []string{pupilA, pupilB, pupilDraft}) {
		t.Errorf("contacts for %v, want A, B and the draft (not the withdrawn pupil)", students)
	}
	if got[0].ID() != "c-a" {
		t.Errorf("A already had Rosa by phone: got %q, want the existing c-a", got[0].ID())
	}
	if n := len(w.links.links); n != 3 {
		t.Errorf("links = %d, want 3", n)
	}

	if _, err := w.families.AddFamilyContact(ctx, famUnknown, "mother", rosaPerson); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown family: %v", err)
	}
	if _, err := w.families.AddFamilyContact(ctx, famID, "mother", domain.ContactPerson{Phone: "4431112233"}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("nameless person: %v", err)
	}
	if _, err := w.families.AddFamilyContact(ctx, famID, "aunt", rosaPerson); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unknown relationship: %v", err)
	}
	if _, err := w.families.AddFamilyContact(ctx, "nope", "mother", rosaPerson); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("malformed family id: %v", err)
	}
}

func TestRevokeFamilyAccess(t *testing.T) {
	w := newAccWorld()
	w.group(t, pupilA, pupilB)
	w.link(t, "l-a", accMom, pupilA, rosaPerson)
	w.link(t, "l-b", accMom, pupilB, domain.ContactPerson{}) // legacy: account only
	w.link(t, "l-c", accMom, pupilC, rosaPerson)             // outside the family
	ctx := context.Background()

	n, err := w.families.RevokeFamilyAccess(ctx, famID, accMom)
	if err != nil || n != 2 {
		t.Fatalf("revoke = %d, %v; want 2", n, err)
	}
	if a, err := w.links.ByID(ctx, "l-a"); err != nil || a.HasAccount() || a.FullName() != rosaPerson.FullName {
		t.Errorf("A's contact = %+v, %v; want it kept, without the account", a, err)
	}
	if _, err := w.links.ByID(ctx, "l-b"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the nameless legacy link must be deleted: %v", err)
	}
	if got := w.accountsOf(pupilC); !slices.Equal(got, []string{accMom}) {
		t.Errorf("a pupil outside the family lost the account: %v", got)
	}
	if _, err := w.families.RevokeFamilyAccess(ctx, famID, accMom); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("revoking again: %v, want NotFound", err)
	}
	if _, err := w.families.RevokeFamilyAccess(ctx, famUnknown, accMom); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown family: %v", err)
	}
	if _, err := w.families.RevokeFamilyAccess(ctx, famID, "nope"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("malformed account: %v", err)
	}
}
