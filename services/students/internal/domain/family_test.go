package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

var familyCreatedAt = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func pupilIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("p%02d", i)
	}
	return out
}

func TestNewFamily(t *testing.T) {
	tests := []struct {
		name     string
		famName  string
		students []string
		wantKind glerrors.Kind
		wantName string
	}{
		{name: "trims the name", famName: "  García López ", students: []string{"a", "b"}, wantName: "García López"},
		{name: "one pupil is a family", famName: "Ruiz", students: []string{"a"}, wantName: "Ruiz"},
		{name: "twenty pupils", famName: "Grande", students: pupilIDs(domain.MaxFamilyPupils), wantName: "Grande"},
		{name: "blank name", famName: "   ", students: []string{"a"}, wantKind: glerrors.KindInvalid},
		{name: "120 runes is fine", famName: strings.Repeat("ñ", domain.MaxFamilyNameRunes), students: []string{"a"}, wantName: strings.Repeat("ñ", domain.MaxFamilyNameRunes)},
		{name: "121 runes is too long", famName: strings.Repeat("ñ", domain.MaxFamilyNameRunes+1), students: []string{"a"}, wantKind: glerrors.KindInvalid},
		{name: "no pupils", famName: "Ruiz", students: nil, wantKind: glerrors.KindInvalid},
		{name: "twenty-one pupils", famName: "Ruiz", students: pupilIDs(domain.MaxFamilyPupils + 1), wantKind: glerrors.KindInvalid},
		{name: "a pupil twice", famName: "Ruiz", students: []string{"a", "a"}, wantKind: glerrors.KindInvalid},
		{name: "a blank pupil id", famName: "Ruiz", students: []string{"a", " "}, wantKind: glerrors.KindInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := domain.NewFamily("fam-1", "t1", tc.famName, tc.students, familyCreatedAt)
			if tc.wantKind != glerrors.KindUnknown {
				if glerrors.KindOf(err) != tc.wantKind {
					t.Fatalf("err = %v, want kind %v", err, tc.wantKind)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewFamily: %v", err)
			}
			if f.Name() != tc.wantName || f.ID() != "fam-1" || f.TenantID() != "t1" ||
				!reflect.DeepEqual(f.StudentIDs(), tc.students) || !f.CreatedAt().Equal(familyCreatedAt) || f.AccountCount() != 0 {
				t.Errorf("family = %+v", f)
			}
		})
	}
}

func TestNewFamilyRequiresIdentity(t *testing.T) {
	if _, err := domain.NewFamily("", "t1", "Ruiz", []string{"a"}, familyCreatedAt); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing id: %v", err)
	}
	if _, err := domain.NewFamily("f", "", "Ruiz", []string{"a"}, familyCreatedAt); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing tenant: %v", err)
	}
}

func TestFamilyStudentIDsIsACopy(t *testing.T) {
	ids := []string{"a", "b"}
	f, err := domain.NewFamily("f", "t1", "Ruiz", ids, familyCreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	ids[0] = "x"
	got := f.StudentIDs()
	got[1] = "y"
	if !reflect.DeepEqual(f.StudentIDs(), []string{"a", "b"}) {
		t.Errorf("the family shares its slice: %v", f.StudentIDs())
	}
}

func TestFamilyRename(t *testing.T) {
	f, err := domain.NewFamily("f", "t1", "Ruiz", []string{"a"}, familyCreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := f.Rename("  Ruiz Pérez ")
	if err != nil || renamed.Name() != "Ruiz Pérez" {
		t.Fatalf("Rename = %q, %v", renamed.Name(), err)
	}
	if f.Name() != "Ruiz" {
		t.Errorf("Rename mutated the receiver: %q", f.Name())
	}
	if _, err := f.Rename(" "); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("blank rename: %v", err)
	}
}

func TestFamilyAddStudents(t *testing.T) {
	f, err := domain.NewFamily("f", "t1", "Ruiz", []string{"a", "b"}, familyCreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	grown, err := f.AddStudents([]string{"c"})
	if err != nil || !reflect.DeepEqual(grown.StudentIDs(), []string{"a", "b", "c"}) {
		t.Fatalf("AddStudents = %v, %v", grown.StudentIDs(), err)
	}
	if _, err := f.AddStudents([]string{"b"}); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("adding a member again: want conflict, got %v", err)
	}
	if _, err := f.AddStudents(nil); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("adding nobody: %v", err)
	}
	if _, err := f.AddStudents(pupilIDs(domain.MaxFamilyPupils - 1)); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("growing past the cap: %v", err)
	}
}

func TestRestoreFamilyKeepsTheCount(t *testing.T) {
	f := domain.RestoreFamily(domain.FamilyParams{
		ID: "f", TenantID: "t1", Name: "Ruiz", StudentIDs: []string{"a"}, AccountCount: 2, CreatedAt: familyCreatedAt,
	})
	if f.AccountCount() != 2 || f.Name() != "Ruiz" || len(f.StudentIDs()) != 1 {
		t.Errorf("restored = %+v", f)
	}
}

func TestSurnameKey(t *testing.T) {
	tests := []struct {
		name, last, maternal, want string
	}{
		{name: "accents and case fold", last: "García", maternal: "López", want: "GARCIA LOPEZ"},
		{name: "diaeresis and eñe", last: "Agüero", maternal: "Peña", want: "AGUERO PENA"},
		{name: "legacy both in last_name", last: "GARCÍA LÓPEZ", maternal: "", want: "GARCIA LOPEZ"},
		{name: "extra spaces collapse", last: "  garcia   ", maternal: "  lopez ", want: "GARCIA LOPEZ"},
		{name: "one surname is too weak", last: "García", maternal: "", want: ""},
		{name: "no surname", last: "", maternal: "", want: ""},
		{name: "particles do not count as a surname", last: "de la", maternal: "Cruz", want: ""},
		{name: "particles stay in a real key", last: "García", maternal: "de la Cruz", want: "GARCIA DE LA CRUZ"},
		{name: "y joins two surnames but is not one", last: "Pérez y", maternal: "", want: ""},
		{name: "a hyphen is a space", last: "Pérez-Gil", maternal: "", want: "PEREZ GIL"},
		{name: "all vowels", last: "Ávila Íñiguez", maternal: "Úrsulo Éboli Ó", want: "AVILA INIGUEZ URSULO EBOLI O"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.SurnameKey(tc.last, tc.maternal); got != tc.want {
				t.Errorf("SurnameKey(%q, %q) = %q, want %q", tc.last, tc.maternal, got, tc.want)
			}
		})
	}
	if domain.SurnameKey("GARCÍA LÓPEZ", "") != domain.SurnameKey("Garcia", "Lopez") {
		t.Error("a legacy pupil must key like a captured one")
	}
}

func TestProposedFamilyName(t *testing.T) {
	if got := domain.ProposedFamilyName("  García ", " López "); got != "García López" {
		t.Errorf("got %q", got)
	}
	if got := domain.ProposedFamilyName("GARCÍA LÓPEZ", ""); got != "GARCÍA LÓPEZ" {
		t.Errorf("legacy got %q", got)
	}
}

func TestSuggestionSignature(t *testing.T) {
	sum := sha256.Sum256([]byte("a,b,c"))
	want := hex.EncodeToString(sum[:])
	if got := domain.SuggestionSignature([]string{"c", "a", "b", "a"}); got != want {
		t.Errorf("signature = %s, want %s (sorted, deduplicated, comma-joined)", got, want)
	}
	if domain.SuggestionSignature([]string{"b", "a"}) != domain.SuggestionSignature([]string{"a", "b"}) {
		t.Error("the signature depends on order")
	}
}

// --- SuggestFamilies ---------------------------------------------------------

func pupil(id, last, maternal string) domain.SuggestionPupil {
	return domain.SuggestionPupil{ID: id, LastName: last, MaternalLastName: maternal}
}

func TestSuggestFamilies(t *testing.T) {
	type want struct {
		ids        []string
		name       string
		reasons    []domain.SuggestionReason
		confidence domain.SuggestionConfidence
	}
	tests := []struct {
		name      string
		pupils    []domain.SuggestionPupil
		contacts  []domain.SuggestionContact
		dismissed []string // groups, as comma-joined ids
		want      []want
	}{
		{
			name:   "accents and case: same surnames",
			pupils: []domain.SuggestionPupil{pupil("a", "García", "López"), pupil("b", "GARCIA", "lopez")},
			want: []want{{ids: []string{"a", "b"}, name: "García López",
				reasons: []domain.SuggestionReason{domain.ReasonSameSurnames}, confidence: domain.ConfidenceMedium}},
		},
		{
			name:   "legacy pupil with both surnames in last_name",
			pupils: []domain.SuggestionPupil{pupil("a", "GARCÍA LÓPEZ", ""), pupil("b", "Garcia", "Lopez")},
			want: []want{{ids: []string{"a", "b"}, name: "GARCÍA LÓPEZ",
				reasons: []domain.SuggestionReason{domain.ReasonSameSurnames}, confidence: domain.ConfidenceMedium}},
		},
		{
			name:   "one-surname pupils are not linked",
			pupils: []domain.SuggestionPupil{pupil("a", "García", ""), pupil("b", "García", "")},
		},
		{
			name:   "half-siblings joined by a shared phone",
			pupils: []domain.SuggestionPupil{pupil("a", "Ruiz", "Pérez"), pupil("b", "Soto", "Pérez")},
			contacts: []domain.SuggestionContact{
				{StudentID: "a", Phone: "443 123 4567"}, {StudentID: "b", Phone: "4431234567"},
			},
			want: []want{{ids: []string{"a", "b"}, name: "Ruiz Pérez",
				reasons: []domain.SuggestionReason{domain.ReasonSharedContactPhone}, confidence: domain.ConfidenceHigh}},
		},
		{
			name:   "a short phone is not a signal",
			pupils: []domain.SuggestionPupil{pupil("a", "Ruiz", "Pérez"), pupil("b", "Soto", "Pérez")},
			contacts: []domain.SuggestionContact{
				{StudentID: "a", Phone: "12345"}, {StudentID: "b", Phone: "12345"},
			},
		},
		{
			name:   "shared email, case-insensitive, and shared account",
			pupils: []domain.SuggestionPupil{pupil("a", "Ruiz", "Pérez"), pupil("b", "Soto", "Gil"), pupil("c", "Luna", "Paz")},
			contacts: []domain.SuggestionContact{
				{StudentID: "a", Email: " Mama@Example.com"}, {StudentID: "b", Email: "mama@example.com "},
				{StudentID: "b", GuardianUserID: "u1"}, {StudentID: "c", GuardianUserID: "u1"},
			},
			want: []want{{ids: []string{"a", "b", "c"}, name: "Ruiz Pérez",
				reasons:    []domain.SuggestionReason{domain.ReasonSharedContactEmail, domain.ReasonSharedAccount},
				confidence: domain.ConfidenceHigh}},
		},
		{
			name: "a value shared by more than six pupils is ignored",
			pupils: []domain.SuggestionPupil{
				pupil("a", "A", "A"), pupil("b", "B", "B"), pupil("c", "C", "C"), pupil("d", "D", "D"),
				pupil("e", "E", "E"), pupil("f", "F", "F"), pupil("g", "G", "G"),
			},
			contacts: func() []domain.SuggestionContact {
				var out []domain.SuggestionContact
				for _, id := range []string{"a", "b", "c", "d", "e", "f", "g"} {
					out = append(out, domain.SuggestionContact{StudentID: id, Phone: "4430000000"})
				}
				return out
			}(),
		},
		{
			name: "six pupils sharing a value are still one family",
			pupils: []domain.SuggestionPupil{
				pupil("a", "A", "A"), pupil("b", "B", "B"), pupil("c", "C", "C"),
				pupil("d", "D", "D"), pupil("e", "E", "E"), pupil("f", "F", "F"),
			},
			contacts: func() []domain.SuggestionContact {
				var out []domain.SuggestionContact
				for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
					out = append(out, domain.SuggestionContact{StudentID: id, Email: "x@y.mx"})
				}
				return out
			}(),
			want: []want{{ids: []string{"a", "b", "c", "d", "e", "f"}, name: "A A",
				reasons: []domain.SuggestionReason{domain.ReasonSharedContactEmail}, confidence: domain.ConfidenceHigh}},
		},
		{
			name:      "a dismissed group is excluded",
			pupils:    []domain.SuggestionPupil{pupil("a", "García", "López"), pupil("b", "García", "López")},
			dismissed: []string{"a,b"},
		},
		{
			name: "a new sibling brings a dismissed group back",
			pupils: []domain.SuggestionPupil{
				pupil("a", "García", "López"), pupil("b", "García", "López"), pupil("c", "García", "López"),
			},
			dismissed: []string{"a,b"},
			want: []want{{ids: []string{"a", "b", "c"}, name: "García López",
				reasons: []domain.SuggestionReason{domain.ReasonSameSurnames}, confidence: domain.ConfidenceMedium}},
		},
		{
			name:   "single pupils are not suggested",
			pupils: []domain.SuggestionPupil{pupil("a", "García", "López"), pupil("b", "Ruiz", "Pérez")},
			contacts: []domain.SuggestionContact{
				{StudentID: "a", Phone: "4431111111"}, {StudentID: "b", Phone: "4432222222"},
			},
		},
		{
			name:   "contacts of unknown pupils are ignored",
			pupils: []domain.SuggestionPupil{pupil("a", "García", "López")},
			contacts: []domain.SuggestionContact{
				{StudentID: "a", Phone: "4431111111"}, {StudentID: "zz", Phone: "4431111111"},
			},
		},
		{
			name: "HIGH first, then by proposed name",
			pupils: []domain.SuggestionPupil{
				pupil("a1", "Álvarez", "Mora"), pupil("a2", "Alvarez", "Mora"),
				pupil("z1", "Zamora", "Ríos"), pupil("z2", "Zapata", "Cruz"),
				pupil("b1", "Bravo", "Sol"), pupil("b2", "Bravo", "Sol"),
			},
			contacts: []domain.SuggestionContact{
				{StudentID: "z1", Phone: "4439999999"}, {StudentID: "z2", Phone: "4439999999"},
			},
			want: []want{
				{ids: []string{"z1", "z2"}, name: "Zamora Ríos",
					reasons: []domain.SuggestionReason{domain.ReasonSharedContactPhone}, confidence: domain.ConfidenceHigh},
				{ids: []string{"a1", "a2"}, name: "Álvarez Mora",
					reasons: []domain.SuggestionReason{domain.ReasonSameSurnames}, confidence: domain.ConfidenceMedium},
				{ids: []string{"b1", "b2"}, name: "Bravo Sol",
					reasons: []domain.SuggestionReason{domain.ReasonSameSurnames}, confidence: domain.ConfidenceMedium},
			},
		},
		{
			name:   "surnames plus a contact is HIGH with both reasons",
			pupils: []domain.SuggestionPupil{pupil("a", "García", "López"), pupil("b", "García", "López")},
			contacts: []domain.SuggestionContact{
				{StudentID: "a", GuardianUserID: "u1"}, {StudentID: "b", GuardianUserID: "u1"},
			},
			want: []want{{ids: []string{"a", "b"}, name: "García López",
				reasons:    []domain.SuggestionReason{domain.ReasonSameSurnames, domain.ReasonSharedAccount},
				confidence: domain.ConfidenceHigh}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dismissed := map[string]bool{}
			for _, group := range tc.dismissed {
				dismissed[domain.SuggestionSignature(strings.Split(group, ","))] = true
			}
			got := domain.SuggestFamilies(tc.pupils, tc.contacts, dismissed, domain.CommonContactValues{})
			if len(got) != len(tc.want) {
				t.Fatalf("got %d suggestions, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				g := got[i]
				if !reflect.DeepEqual(g.StudentIDs, w.ids) || g.ProposedName != w.name ||
					!reflect.DeepEqual(g.Reasons, w.reasons) || g.Confidence != w.confidence {
					t.Errorf("suggestion %d = %+v, want %+v", i, g, w)
				}
				if g.Signature != domain.SuggestionSignature(w.ids) {
					t.Errorf("suggestion %d signature = %s", i, g.Signature)
				}
			}
		})
	}
}

// A component larger than a family can be, held together only by common
// surnames, is not one family; the contact-based families inside it are.
func TestSuggestFamiliesSplitsOversizedComponentsByContacts(t *testing.T) {
	var pupils []domain.SuggestionPupil
	for i := range domain.MaxFamilyPupils + 5 {
		pupils = append(pupils, pupil(fmt.Sprintf("p%02d", i), "Hernández", "Hernández"))
	}
	contacts := []domain.SuggestionContact{
		{StudentID: "p03", Phone: "4431112222"}, {StudentID: "p07", Phone: "4431112222"},
		{StudentID: "p10", Email: "mama@x.mx"}, {StudentID: "p11", Email: "mama@x.mx"},
		{StudentID: "p11", GuardianUserID: "u9"}, {StudentID: "p12", GuardianUserID: "u9"},
	}
	got := domain.SuggestFamilies(pupils, contacts, nil, domain.CommonContactValues{})
	byFirst := map[string]domain.FamilySuggestion{}
	for _, s := range got {
		byFirst[s.StudentIDs[0]] = s
	}
	if len(got) != 2 {
		t.Fatalf("got %d suggestions, want the 2 contact families: %+v", len(got), got)
	}
	phone, email := byFirst["p03"], byFirst["p10"]
	if !reflect.DeepEqual(phone.StudentIDs, []string{"p03", "p07"}) ||
		!reflect.DeepEqual(phone.Reasons, []domain.SuggestionReason{domain.ReasonSameSurnames, domain.ReasonSharedContactPhone}) ||
		phone.Confidence != domain.ConfidenceHigh {
		t.Errorf("phone family = %+v", phone)
	}
	if !reflect.DeepEqual(email.StudentIDs, []string{"p10", "p11", "p12"}) ||
		!reflect.DeepEqual(email.Reasons, []domain.SuggestionReason{domain.ReasonSameSurnames, domain.ReasonSharedContactEmail, domain.ReasonSharedAccount}) {
		t.Errorf("email family = %+v", email)
	}
}

// Surnames alone over more than a family's size suggest nothing.
func TestSuggestFamiliesDropsOversizedSurnameOnlyComponents(t *testing.T) {
	var pupils []domain.SuggestionPupil
	for i := range domain.MaxFamilyPupils + 1 {
		pupils = append(pupils, pupil(fmt.Sprintf("p%02d", i), "García", "López"))
	}
	if got := domain.SuggestFamilies(pupils, nil, nil, domain.CommonContactValues{}); len(got) != 0 {
		t.Errorf("a %d-pupil surname component was suggested: %+v", len(pupils), got)
	}
}

// A value the school's other pupils (in families already) share too often is
// a reception number: it links nobody, even between few free pupils.
func TestSuggestFamiliesIgnoresCommonContactValues(t *testing.T) {
	pupils := []domain.SuggestionPupil{pupil("a", "A", "A"), pupil("b", "B", "B"), pupil("c", "C", "C")}
	contacts := []domain.SuggestionContact{
		{StudentID: "a", Phone: "443 000 0000"}, {StudentID: "b", Phone: "4430000000"},
		{StudentID: "b", Email: "Recepcion@x.mx"}, {StudentID: "c", Email: "recepcion@x.mx"},
		{StudentID: "a", GuardianUserID: "u1"}, {StudentID: "c", GuardianUserID: "u1"},
	}
	common := domain.CommonContactValues{
		Phones:   map[string]bool{"4430000000": true},
		Emails:   map[string]bool{"recepcion@x.mx": true},
		Accounts: map[string]bool{"u1": true},
	}
	if got := domain.SuggestFamilies(pupils, contacts, nil, common); len(got) != 0 {
		t.Errorf("common values chained pupils: %+v", got)
	}
	if got := domain.SuggestFamilies(pupils, contacts, nil, domain.CommonContactValues{}); len(got) != 1 {
		t.Errorf("without the stoplist the values link them: %+v", got)
	}
}

func TestSuggestFamiliesIsDeterministic(t *testing.T) {
	pupils := []domain.SuggestionPupil{
		pupil("a", "García", "López"), pupil("b", "García", "López"),
		pupil("c", "Ruiz", "Pérez"), pupil("d", "Soto", "Gil"),
		pupil("e", "Luna", "Paz"), pupil("f", "Luna", "Paz"),
	}
	contacts := []domain.SuggestionContact{
		{StudentID: "c", Email: "m@x.mx"}, {StudentID: "d", Email: "m@x.mx"},
		{StudentID: "e", Phone: "4431231234"}, {StudentID: "a", Phone: "4431231234"},
	}
	first := domain.SuggestFamilies(pupils, contacts, nil, domain.CommonContactValues{})
	for range 50 {
		if again := domain.SuggestFamilies(pupils, contacts, nil, domain.CommonContactValues{}); !reflect.DeepEqual(again, first) {
			t.Fatalf("not deterministic:\n%+v\n%+v", first, again)
		}
	}
	// a-b-e-f chain through the shared phone (e↔a) into one family.
	if len(first) != 2 || !reflect.DeepEqual(first[0].StudentIDs, []string{"a", "b", "e", "f"}) {
		t.Errorf("suggestions = %+v", first)
	}
}

func TestStudentCarriesItsFamilyAsRead(t *testing.T) {
	s := domain.Rehydrate(domain.NewApplicantParams{ID: "s", TenantID: "t1", FirstName: "Ana", LastName: "Ruiz"}, domain.AdmissionPending)
	if s.FamilyID() != "" {
		t.Errorf("a pupil starts without a family, got %q", s.FamilyID())
	}
	s.AssignFamily("fam-1")
	if s.FamilyID() != "fam-1" {
		t.Errorf("FamilyID = %q", s.FamilyID())
	}
}
