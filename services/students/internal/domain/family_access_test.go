package domain_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

const (
	accTenant = "11111111-1111-1111-1111-111111111111"
	accMom    = "60000000-0000-0000-0000-0000000000a1"
	accDad    = "60000000-0000-0000-0000-0000000000a2"
	accThird  = "60000000-0000-0000-0000-0000000000a3"
	accPupil1 = "5a000000-0000-0000-0000-000000000001"
	accPupil2 = "5a000000-0000-0000-0000-000000000002"
	accPupil3 = "5a000000-0000-0000-0000-000000000003"
	accPupil4 = "5a000000-0000-0000-0000-000000000004"
)

// seqIDs mints "new-1", "new-2", ... so a plan's inserts are predictable.
func seqIDs() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("new-%d", n) }
}

func accountLink(t *testing.T, id, user, student string, rel domain.Relationship, person domain.ContactPerson) domain.GuardianLink {
	t.Helper()
	l, err := domain.NewGuardianLink(id, accTenant, user, student, rel)
	if err != nil {
		t.Fatal(err)
	}
	return l.WithPerson(person)
}

func plainContact(t *testing.T, id, student string, person domain.ContactPerson) domain.GuardianLink {
	t.Helper()
	c, err := domain.NewContact(id, accTenant, student, domain.RelationshipOther, person)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func member(id string, status domain.AdmissionStatus, contacts ...domain.GuardianLink) domain.FamilyMember {
	return domain.FamilyMember{StudentID: id, Status: status, Contacts: contacts}
}

var rosa = domain.ContactPerson{FullName: "Rosa Pérez", Phone: "4431112233", Email: "rosa@example.com", Emergency: true}

func TestPlanFamilyAccessCapsTheFamilyAtTwoAccounts(t *testing.T) {
	cases := []struct {
		name    string
		members []domain.FamilyMember
		wantErr bool
	}{
		{"no account", []domain.FamilyMember{member(accPupil1, domain.AdmissionEnrolled)}, false},
		{"two accounts", []domain.FamilyMember{
			member(accPupil1, domain.AdmissionEnrolled, accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, rosa)),
			member(accPupil2, domain.AdmissionEnrolled, accountLink(t, "l2", accDad, accPupil2, domain.RelationshipFather, domain.ContactPerson{})),
		}, false},
		{"one account on two siblings counts once", []domain.FamilyMember{
			member(accPupil1, domain.AdmissionEnrolled, accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, rosa)),
			member(accPupil2, domain.AdmissionEnrolled,
				accountLink(t, "l2", accMom, accPupil2, domain.RelationshipMother, rosa),
				accountLink(t, "l3", accDad, accPupil2, domain.RelationshipFather, domain.ContactPerson{})),
		}, false},
		{"a third account is refused", []domain.FamilyMember{
			member(accPupil1, domain.AdmissionEnrolled,
				accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, rosa),
				accountLink(t, "l2", accDad, accPupil1, domain.RelationshipFather, domain.ContactPerson{})),
			member(accPupil2, domain.AdmissionEnrolled, accountLink(t, "l3", accThird, accPupil2, domain.RelationshipOther, domain.ContactPerson{})),
		}, true},
		{"a withdrawn member's accounts still count", []domain.FamilyMember{
			member(accPupil1, domain.AdmissionEnrolled,
				accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, rosa),
				accountLink(t, "l2", accDad, accPupil1, domain.RelationshipFather, domain.ContactPerson{})),
			member(accPupil2, domain.AdmissionWithdrawn, accountLink(t, "l3", accThird, accPupil2, domain.RelationshipOther, domain.ContactPerson{})),
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.PlanFamilyAccess(accTenant, tc.members, seqIDs())
			if tc.wantErr {
				if !errors.Is(err, domain.ErrFamilyAccountLimit) {
					t.Fatalf("err = %v, want ErrFamilyAccountLimit", err)
				}
				if glerrors.KindOf(err) != glerrors.KindConflict {
					t.Errorf("kind = %v, want KindConflict", glerrors.KindOf(err))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	if domain.MaxFamilyAccounts != 2 {
		t.Errorf("MaxFamilyAccounts = %d, want 2 (ADR 0041)", domain.MaxFamilyAccounts)
	}
}

func TestPlanFamilyAccessCopiesTheSourceContactOntoASibling(t *testing.T) {
	members := []domain.FamilyMember{
		// Unsorted on purpose: the source is the lowest-id member's contact.
		member(accPupil3, domain.AdmissionEnrolled,
			accountLink(t, "l9", accMom, accPupil3, domain.RelationshipGuardian, domain.ContactPerson{FullName: "Otra Rosa"})),
		member(accPupil1, domain.AdmissionEnrolled, accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, rosa)),
		member(accPupil2, domain.AdmissionPending),
	}
	plan, err := domain.PlanFamilyAccess(accTenant, members, seqIDs())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Attach) != 0 || len(plan.Insert) != 1 {
		t.Fatalf("plan = %+v, want one insert", plan)
	}
	got := plan.Insert[0]
	if got.ID() != "new-1" || got.TenantID() != accTenant || got.StudentID() != accPupil2 || got.GuardianUserID() != accMom ||
		got.Relationship() != domain.RelationshipMother || got.FullName() != rosa.FullName || got.Phone() != rosa.Phone ||
		got.Email() != rosa.Email || got.IsEmergency() {
		t.Errorf("inserted %+v, want a copy of pupil 1's contact without the emergency mark", got)
	}
}

func TestPlanFamilyAccessAttachesToTheSiblingsContactWithTheSameEmail(t *testing.T) {
	sameEmail := plainContact(t, "c2", accPupil2, domain.ContactPerson{FullName: "Rosa P.", Email: "ROSA@example.com"})
	other := plainContact(t, "c1", accPupil2, domain.ContactPerson{FullName: "Abuela", Phone: "4439998877"})
	members := []domain.FamilyMember{
		member(accPupil1, domain.AdmissionEnrolled, accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, rosa)),
		member(accPupil2, domain.AdmissionAccepted, other, sameEmail),
	}
	plan, err := domain.PlanFamilyAccess(accTenant, members, seqIDs())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Insert) != 0 || len(plan.Attach) != 1 {
		t.Fatalf("plan = %+v, want one attach", plan)
	}
	if got := plan.Attach[0]; got.ID() != "c2" || got.GuardianUserID() != accMom || got.FullName() != "Rosa P." {
		t.Errorf("attached %+v, want the account on the sibling's own contact c2", got)
	}
}

func TestPlanFamilyAccessLeavesAlreadyLinkedSiblingsAlone(t *testing.T) {
	members := []domain.FamilyMember{
		member(accPupil1, domain.AdmissionEnrolled, accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, rosa)),
		member(accPupil2, domain.AdmissionEnrolled, accountLink(t, "l2", accMom, accPupil2, domain.RelationshipMother, rosa)),
	}
	plan, err := domain.PlanFamilyAccess(accTenant, members, seqIDs())
	if err != nil || len(plan.Attach)+len(plan.Insert) != 0 {
		t.Fatalf("plan = %+v, %v; want nothing to do", plan, err)
	}
}

func TestPlanFamilyAccessGivesNoAccountToDraftsNorPupilsWhoLeft(t *testing.T) {
	members := []domain.FamilyMember{
		member(accPupil1, domain.AdmissionEnrolled, accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, rosa)),
		member(accPupil2, domain.AdmissionDraft, plainContact(t, "c2", accPupil2, rosa)),
		member(accPupil3, domain.AdmissionWithdrawn, plainContact(t, "c3", accPupil3, rosa)),
		member(accPupil4, domain.AdmissionRejected),
	}
	plan, err := domain.PlanFamilyAccess(accTenant, members, seqIDs())
	if err != nil || len(plan.Attach)+len(plan.Insert) != 0 {
		t.Fatalf("plan = %+v, %v; want nothing for a draft, a withdrawn or a rejected pupil", plan, err)
	}
}

func TestPlanFamilyAccessSyncsEveryAccountToEverySibling(t *testing.T) {
	dad := domain.ContactPerson{FullName: "Luis Pérez", Phone: "4435556677"}
	members := []domain.FamilyMember{
		member(accPupil1, domain.AdmissionEnrolled, accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, rosa)),
		member(accPupil2, domain.AdmissionEnrolled, accountLink(t, "l2", accDad, accPupil2, domain.RelationshipFather, dad)),
		member(accPupil3, domain.AdmissionEnrolled),
	}
	plan, err := domain.PlanFamilyAccess(accTenant, members, seqIDs())
	if err != nil {
		t.Fatal(err)
	}
	type pair struct{ user, student string }
	var got []pair
	for _, l := range plan.Insert {
		got = append(got, pair{l.GuardianUserID(), l.StudentID()})
	}
	want := []pair{{accMom, accPupil2}, {accMom, accPupil3}, {accDad, accPupil1}, {accDad, accPupil3}}
	if !slices.Equal(got, want) {
		t.Errorf("inserts = %v, want %v (accounts in id order, then members in id order)", got, want)
	}
}

// One account-less contact matches the email of two accounts' sources: only the
// first account takes it; the second gets a contact of its own.
func TestPlanFamilyAccessNeverAttachesTwoAccountsToOneContact(t *testing.T) {
	shared := domain.ContactPerson{FullName: "Casa", Email: "casa@example.com"}
	members := []domain.FamilyMember{
		member(accPupil1, domain.AdmissionEnrolled,
			accountLink(t, "l1", accMom, accPupil1, domain.RelationshipMother, shared),
			accountLink(t, "l2", accDad, accPupil1, domain.RelationshipFather, shared)),
		member(accPupil2, domain.AdmissionEnrolled, plainContact(t, "c2", accPupil2, shared)),
	}
	plan, err := domain.PlanFamilyAccess(accTenant, members, seqIDs())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Attach) != 1 || plan.Attach[0].GuardianUserID() != accMom || len(plan.Insert) != 1 || plan.Insert[0].GuardianUserID() != accDad {
		t.Errorf("plan = %+v, want mom attached to c2 and dad inserted", plan)
	}
}

func TestPlanFamilyContact(t *testing.T) {
	byEmail := plainContact(t, "c1", accPupil1, domain.ContactPerson{FullName: "Rosa", Email: "rosa@example.com"})
	byPhone := plainContact(t, "c2", accPupil2, domain.ContactPerson{FullName: "R. Pérez", Phone: "4431112233"})
	members := []domain.FamilyMember{
		member(accPupil2, domain.AdmissionEnrolled, byPhone),
		member(accPupil1, domain.AdmissionEnrolled, byEmail),
		member(accPupil3, domain.AdmissionDraft),
		member(accPupil4, domain.AdmissionWithdrawn),
	}
	plan, err := domain.PlanFamilyContact(accTenant, members, domain.RelationshipMother, rosa, seqIDs())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range plan.Contacts {
		ids = append(ids, c.ID())
	}
	if !slices.Equal(ids, []string{"c1", "c2", "new-1"}) {
		t.Errorf("contacts = %v, want the email match, the phone match and one new contact for the draft", ids)
	}
	if len(plan.Insert) != 1 {
		t.Fatalf("inserts = %d, want 1", len(plan.Insert))
	}
	got := plan.Insert[0]
	if got.StudentID() != accPupil3 || got.HasAccount() || got.FullName() != rosa.FullName || got.Relationship() != domain.RelationshipMother || !got.IsEmergency() {
		t.Errorf("new contact = %+v", got)
	}
}

func TestPlanFamilyContactJudgesThePerson(t *testing.T) {
	members := []domain.FamilyMember{member(accPupil1, domain.AdmissionEnrolled)}
	for name, p := range map[string]domain.ContactPerson{
		"no name":   {Phone: "4431112233"},
		"bad phone": {FullName: "Rosa", Phone: "123"},
		"bad email": {FullName: "Rosa", Email: "no-at-sign"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.PlanFamilyContact(accTenant, members, domain.RelationshipMother, p, seqIDs()); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("err = %v, want KindInvalid", err)
			}
		})
	}
	// Judged even when every member already has this person.
	if _, err := domain.PlanFamilyContact(accTenant, nil, domain.RelationshipMother, domain.ContactPerson{}, seqIDs()); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("empty family: err = %v, want KindInvalid", err)
	}
}
