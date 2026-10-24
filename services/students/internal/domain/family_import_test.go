package domain_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/garcia-lorca/services/students/internal/domain"
)

// importPupil is a family-less batch pupil named García López.
func importPupil(id string) domain.ImportPupil {
	return domain.ImportPupil{ID: id, LastName: "García", MaternalLastName: "López", InBatch: true}
}

func withFamily(p domain.ImportPupil, familyID string) domain.ImportPupil {
	p.FamilyID = familyID
	return p
}

func phone(studentID, number string) domain.SuggestionContact {
	return domain.SuggestionContact{StudentID: studentID, Phone: number}
}

func email(studentID, address string) domain.SuggestionContact {
	return domain.SuggestionContact{StudentID: studentID, Email: address}
}

func TestPlanImportFamilies(t *testing.T) {
	const tel, otherTel, mail = "4435550000", "4436660000", "papa@x.mx"
	cousin := importPupil("c")
	notInBatch := importPupil("b")
	notInBatch.InBatch = false
	withAccount := importPupil("b")
	withAccount.HasAccount = true
	soto := importPupil("b")
	soto.LastName, soto.MaternalLastName = "Soto", "Gil"
	oneSurname := importPupil("b")
	oneSurname.LastName, oneSurname.MaternalLastName = "García", ""
	lone := oneSurname
	lone.ID = "a"

	tests := []struct {
		name string
		in   domain.ImportFamilyInput
		want domain.ImportFamilyPlan
	}{
		{
			name: "same surnames and a shared phone build a family",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), importPupil("b")},
				Contacts: []domain.SuggestionContact{phone("a", "443-555-0000"), phone("b", tel)},
			},
			want: domain.ImportFamilyPlan{Create: [][]string{{"a", "b"}}},
		},
		{
			name: "a shared email counts too, case and spaces aside",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), importPupil("b")},
				Contacts: []domain.SuggestionContact{email("a", " PAPA@x.mx"), email("b", mail)},
			},
			want: domain.ImportFamilyPlan{Create: [][]string{{"a", "b"}}},
		},
		{
			name: "same surnames alone link nobody",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), importPupil("b")},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", otherTel)},
			},
		},
		{
			name: "a shared phone alone links nobody",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), soto},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
			},
		},
		{
			name: "one surname is no key, even with a phone in common",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{lone, oneSurname},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
			},
		},
		{
			name: "a cousin with the surnames but not the phone stays out",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), importPupil("b"), cousin},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel), phone("c", otherTel)},
			},
			want: domain.ImportFamilyPlan{Create: [][]string{{"a", "b"}}},
		},
		{
			name: "pairs chain: a-b by phone, b-c by email",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), importPupil("b"), cousin},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel), email("b", mail), email("c", mail)},
			},
			want: domain.ImportFamilyPlan{Create: [][]string{{"a", "b", "c"}}},
		},
		{
			name: "one batch pupil is enough",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), notInBatch},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
			},
			want: domain.ImportFamilyPlan{Create: [][]string{{"a", "b"}}},
		},
		{
			name: "no batch pupil, nothing to do",
			in: domain.ImportFamilyInput{
				Pupils: []domain.ImportPupil{
					{ID: "a", LastName: "García", MaternalLastName: "López"}, notInBatch,
				},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
			},
		},
		{
			name: "a pupil with an app account is left to the office (H1)",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), withAccount},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
			},
			want: domain.ImportFamilyPlan{Skipped: 1},
		},
		{
			name: "a dismissed group stays dismissed",
			in: domain.ImportFamilyInput{
				Pupils:    []domain.ImportPupil{importPupil("a"), importPupil("b")},
				Contacts:  []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
				Dismissed: map[string]bool{domain.SuggestionSignature([]string{"b", "a"}): true},
			},
			want: domain.ImportFamilyPlan{Skipped: 1},
		},
		{
			name: "a school-wide common value links nobody",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), importPupil("b")},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
				Common:   domain.CommonContactValues{Phones: map[string]bool{tel: true}},
			},
		},
		{
			name: "the family-less join the one family they touch",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), withFamily(importPupil("b"), "F")},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
				Families: map[string]domain.ImportFamilyState{"F": {Pupils: 1}},
			},
			want: domain.ImportFamilyPlan{Join: []domain.ImportJoin{{FamilyID: "F", StudentIDs: []string{"a"}}}},
		},
		{
			name: "a family with app accounts is not joined",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), withFamily(importPupil("b"), "F")},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
				Families: map[string]domain.ImportFamilyState{"F": {Pupils: 1, Accounts: 1}},
			},
			want: domain.ImportFamilyPlan{Skipped: 1},
		},
		{
			name: "a joiner with an app account is not added",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{withFamily(importPupil("a"), "F"), withAccount},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
				Families: map[string]domain.ImportFamilyState{"F": {Pupils: 1}},
			},
			want: domain.ImportFamilyPlan{Skipped: 1},
		},
		{
			name: "touching two families is the office's call",
			in: domain.ImportFamilyInput{
				Pupils: []domain.ImportPupil{
					withFamily(importPupil("a"), "F"), importPupil("b"), withFamily(importPupil("c"), "G"),
				},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel), phone("c", tel)},
				Families: map[string]domain.ImportFamilyState{"F": {Pupils: 1}, "G": {Pupils: 1}},
			},
			want: domain.ImportFamilyPlan{Skipped: 1},
		},
		{
			name: "a family that would outgrow twenty pupils is not joined",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), withFamily(importPupil("b"), "F")},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
				Families: map[string]domain.ImportFamilyState{"F": {Pupils: domain.MaxFamilyPupils}},
			},
			want: domain.ImportFamilyPlan{Skipped: 1},
		},
		{
			name: "a family whose state was not read is not joined",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{importPupil("a"), withFamily(importPupil("b"), "F")},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
			},
			want: domain.ImportFamilyPlan{Skipped: 1},
		},
		{
			name: "everyone already in the one family: nothing left to do (a re-run)",
			in: domain.ImportFamilyInput{
				Pupils:   []domain.ImportPupil{withFamily(importPupil("a"), "F"), withFamily(importPupil("b"), "F")},
				Contacts: []domain.SuggestionContact{phone("a", tel), phone("b", tel)},
				Families: map[string]domain.ImportFamilyState{"F": {Pupils: 2, Accounts: 1}},
			},
		},
		{
			name: "two families, in the order of their first pupil",
			in: domain.ImportFamilyInput{
				Pupils: []domain.ImportPupil{importPupil("a"), soto, importPupil("c"), {ID: "d", LastName: "Soto", MaternalLastName: "Gil", InBatch: true}},
				Contacts: []domain.SuggestionContact{
					phone("d", otherTel), phone("c", tel), phone("b", otherTel), phone("a", tel),
				},
			},
			want: domain.ImportFamilyPlan{Create: [][]string{{"a", "c"}, {"b", "d"}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.PlanImportFamilies(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("plan = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A value is counted per distinct pupil: six pupils link even when one of
// them carries it twice (profile and contact); a seventh drops it.
func TestPlanImportFamiliesSharedContactCap(t *testing.T) {
	const tel = "4435550000"
	build := func(n int) domain.ImportFamilyInput {
		var in domain.ImportFamilyInput
		for i := range n {
			id := fmt.Sprintf("p%02d", i)
			in.Pupils = append(in.Pupils, importPupil(id))
			in.Contacts = append(in.Contacts, phone(id, tel))
		}
		in.Contacts = append(in.Contacts, phone("p00", tel))
		return in
	}
	six := domain.PlanImportFamilies(build(domain.MaxPupilsPerSharedContact))
	if len(six.Create) != 1 || len(six.Create[0]) != domain.MaxPupilsPerSharedContact {
		t.Errorf("six pupils sharing a phone: plan = %+v, want one family of six", six)
	}
	seven := domain.PlanImportFamilies(build(domain.MaxPupilsPerSharedContact + 1))
	if !reflect.DeepEqual(seven, domain.ImportFamilyPlan{}) {
		t.Errorf("seven pupils sharing a phone: plan = %+v, want nothing", seven)
	}
}

// Pairwise distinct phones chain 21 same-surname pupils: no family that big.
func TestPlanImportFamiliesSkipsOversizedGroups(t *testing.T) {
	var in domain.ImportFamilyInput
	for i := range domain.MaxFamilyPupils + 1 {
		id := fmt.Sprintf("p%02d", i)
		in.Pupils = append(in.Pupils, importPupil(id))
		in.Contacts = append(in.Contacts, phone(id, fmt.Sprintf("44300000%02d", i)))
		if i > 0 {
			in.Contacts = append(in.Contacts, phone(id, fmt.Sprintf("44300000%02d", i-1)))
		}
	}
	if got := domain.PlanImportFamilies(in); !reflect.DeepEqual(got, domain.ImportFamilyPlan{Skipped: 1}) {
		t.Errorf("plan = %+v, want one skipped group", got)
	}
}
