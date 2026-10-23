package usecase_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

const (
	famTenant  = "11111111-1111-1111-1111-111111111111"
	famID      = "fa000000-0000-0000-0000-000000000001"
	famOther   = "fa000000-0000-0000-0000-000000000002"
	famUnknown = "fa000000-0000-0000-0000-0000000000ff"
	pupilA     = "5a000000-0000-0000-0000-00000000000a"
	pupilB     = "5a000000-0000-0000-0000-00000000000b"
	pupilC     = "5a000000-0000-0000-0000-00000000000c"
	pupilGone  = "5a000000-0000-0000-0000-0000000000dd"
	pupilNope  = "5a000000-0000-0000-0000-0000000000ee"
	pupilDraft = "5a000000-0000-0000-0000-0000000000df"
	famActor   = "a0000000-0000-0000-0000-000000000001"
)

var famNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// famWorld is the pupils and families in memory: the StudentRepository's List
// (the rest of the port is never called by FamilyService) and the whole
// FamilyRepository, sharing the membership the way students.family_id does.
type famWorld struct {
	ports.StudentRepository // nil: FamilyService only lists (and FindByID)
	*memAccess              // the account writes, over pupils and links
	pupils                  map[string]*domain.Student
	links                   *fakeGuardianRepo
	families                map[string]domain.Family
	dismissals              map[string]ports.FamilyDismissal
	inputs                  ports.SuggestionInputs
}

func newFamWorld() *famWorld {
	w := &famWorld{
		pupils:     map[string]*domain.Student{},
		families:   map[string]domain.Family{},
		dismissals: map[string]ports.FamilyDismissal{},
		links:      newGuardianRepo(),
	}
	w.memAccess = &memAccess{pupils: w.pupils, links: w.links, familyExists: func(id string) bool {
		_, ok := w.families[id]
		return ok
	}}
	w.put(pupilA, "García", "López", domain.AdmissionEnrolled)
	w.put(pupilB, "García", "López", domain.AdmissionPending)
	w.put(pupilC, "Ruiz", "", domain.AdmissionAccepted)
	w.put(pupilGone, "Soto", "Gil", domain.AdmissionWithdrawn)
	w.put(pupilDraft, "Luna", "Paz", domain.AdmissionDraft)
	return w
}

func (w *famWorld) put(id, last, maternal string, status domain.AdmissionStatus) {
	w.pupils[id] = domain.Rehydrate(domain.NewApplicantParams{
		ID: id, TenantID: famTenant, FirstName: "Ana", LastName: last, MaternalLastName: maternal,
	}, status)
}

func (w *famWorld) List(_ context.Context, f ports.ListFilter) ([]*domain.Student, string, error) {
	var out []*domain.Student
	for _, id := range f.IDs {
		if s, ok := w.pupils[id]; ok {
			out = append(out, s)
		}
	}
	return out, "", nil
}

func (w *famWorld) ListFamilies(context.Context) ([]domain.Family, error) {
	var out []domain.Family
	for _, f := range w.families {
		out = append(out, f)
	}
	return out, nil
}

func (w *famWorld) FamilyByID(_ context.Context, id string) (domain.Family, error) {
	f, ok := w.families[id]
	if !ok {
		return domain.Family{}, glerrors.ErrNotFound
	}
	return f, nil
}

func (w *famWorld) FindByID(_ context.Context, id string) (*domain.Student, error) {
	if s, ok := w.pupils[id]; ok {
		return s, nil
	}
	return nil, glerrors.ErrNotFound
}

func (w *famWorld) CreateFamily(ctx context.Context, f domain.Family, sync ports.FamilyAccessFunc) error {
	w.families[f.ID()] = f
	for _, id := range f.StudentIDs() {
		w.pupils[id].AssignFamily(f.ID())
	}
	if err := w.syncFamily(ctx, f.ID(), sync); err != nil {
		for _, id := range f.StudentIDs() {
			w.pupils[id].AssignFamily("")
		}
		delete(w.families, f.ID())
		return err
	}
	return nil
}

// syncFamily runs a write's sync in the family's "transaction" (links roll
// back on error; the caller reverts the membership).
func (w *famWorld) syncFamily(ctx context.Context, familyID string, sync ports.FamilyAccessFunc) error {
	if sync == nil {
		return nil
	}
	return w.run(ctx, &memAccessTx{m: w.memAccess, familyID: familyID}, sync)
}

func (w *famWorld) RenameFamily(_ context.Context, id, name string) error {
	f, ok := w.families[id]
	if !ok {
		return glerrors.ErrNotFound
	}
	renamed, err := f.Rename(name)
	if err != nil {
		return err
	}
	w.families[id] = renamed
	return nil
}

func (w *famWorld) AddStudentsToFamily(ctx context.Context, familyID string, ids []string, sync ports.FamilyAccessFunc) error {
	f, ok := w.families[familyID]
	if !ok {
		return glerrors.ErrNotFound
	}
	grown, err := f.AddStudents(ids)
	if err != nil {
		return err
	}
	w.families[familyID] = grown
	for _, id := range ids {
		w.pupils[id].AssignFamily(familyID)
	}
	if err := w.syncFamily(ctx, familyID, sync); err != nil {
		for _, id := range ids {
			w.pupils[id].AssignFamily("")
		}
		w.families[familyID] = f
		return err
	}
	return nil
}

func (w *famWorld) RemoveStudentFromFamily(_ context.Context, familyID, studentID string) (bool, error) {
	f, ok := w.families[familyID]
	if !ok || !slices.Contains(f.StudentIDs(), studentID) {
		return false, glerrors.ErrNotFound
	}
	w.pupils[studentID].AssignFamily("")
	left := slices.DeleteFunc(f.StudentIDs(), func(id string) bool { return id == studentID })
	if len(left) == 0 {
		delete(w.families, familyID)
		return true, nil
	}
	w.families[familyID] = domain.RestoreFamily(domain.FamilyParams{ID: f.ID(), TenantID: f.TenantID(), Name: f.Name(), StudentIDs: left})
	return false, nil
}

func (w *famWorld) DissolveFamily(_ context.Context, id string) error {
	f, ok := w.families[id]
	if !ok {
		return glerrors.ErrNotFound
	}
	for _, sid := range f.StudentIDs() {
		w.pupils[sid].AssignFamily("")
	}
	delete(w.families, id)
	return nil
}

func (w *famWorld) SuggestionInputs(context.Context) (ports.SuggestionInputs, error) {
	return w.inputs, nil
}

func (w *famWorld) DismissSuggestion(_ context.Context, d ports.FamilyDismissal) error {
	if _, ok := w.dismissals[d.Signature]; !ok {
		w.dismissals[d.Signature] = d
	}
	return nil
}

func famService(w *famWorld) *usecase.FamilyService {
	return usecase.NewFamilyService(w, w, newUsers(), famTenant, func() string { return famID }, func() time.Time { return famNow })
}

func TestCreateFamily(t *testing.T) {
	tests := []struct {
		name     string
		famName  string
		ids      []string
		setup    func(w *famWorld)
		wantKind glerrors.Kind
		wantName string
	}{
		{name: "empty name proposes the first pupil's surnames", ids: []string{pupilA, pupilB}, wantName: "García López"},
		{name: "a name the office typed", famName: " Los García ", ids: []string{pupilA, pupilB}, wantName: "Los García"},
		{name: "a draft can be in a family", ids: []string{pupilDraft, pupilA}, wantName: "Luna Paz"},
		{name: "upper-case ids are canonicalized", ids: []string{strings.ToUpper(pupilA)}, wantName: "García López"},
		{name: "unknown pupil", ids: []string{pupilA, pupilNope}, wantKind: glerrors.KindInvalid},
		{name: "malformed id", ids: []string{"nope"}, wantKind: glerrors.KindInvalid},
		{name: "no pupils", ids: nil, wantKind: glerrors.KindInvalid},
		{name: "a withdrawn pupil", ids: []string{pupilA, pupilGone}, wantKind: glerrors.KindInvalid},
		{name: "a pupil twice", ids: []string{pupilA, pupilA}, wantKind: glerrors.KindInvalid},
		{
			name: "a pupil already in a family", ids: []string{pupilA, pupilB},
			setup:    func(w *famWorld) { w.pupils[pupilB].AssignFamily(famOther) },
			wantKind: glerrors.KindConflict,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newFamWorld()
			if tc.setup != nil {
				tc.setup(w)
			}
			f, err := famService(w).CreateFamily(context.Background(), tc.famName, tc.ids)
			if tc.wantKind != glerrors.KindUnknown {
				if glerrors.KindOf(err) != tc.wantKind {
					t.Fatalf("err = %v, want kind %v", err, tc.wantKind)
				}
				if len(w.families) != 0 {
					t.Error("a refused create stored a family")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateFamily: %v", err)
			}
			if f.ID() != famID || f.Name() != tc.wantName || f.TenantID() != famTenant || !f.CreatedAt().Equal(famNow) {
				t.Errorf("family = %+v", f)
			}
			for _, id := range f.StudentIDs() {
				if w.pupils[id].FamilyID() != famID {
					t.Errorf("pupil %s not pointed at the family", id)
				}
			}
		})
	}
}

func seedFamily(t *testing.T, w *famWorld, ids ...string) {
	t.Helper()
	if _, err := famService(w).CreateFamily(context.Background(), "", ids); err != nil {
		t.Fatalf("seed family: %v", err)
	}
}

func TestGetAndListFamilies(t *testing.T) {
	w := newFamWorld()
	seedFamily(t, w, pupilA, pupilB)
	svc := famService(w)
	if f, err := svc.GetFamily(context.Background(), famID); err != nil || f.ID() != famID {
		t.Fatalf("GetFamily = %+v, %v", f, err)
	}
	if _, err := svc.GetFamily(context.Background(), famUnknown); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown family: %v", err)
	}
	if _, err := svc.GetFamily(context.Background(), "x"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("malformed id: %v", err)
	}
	all, err := svc.ListFamilies(context.Background())
	if err != nil || len(all) != 1 {
		t.Errorf("ListFamilies = %v, %v", all, err)
	}
}

func TestRenameFamily(t *testing.T) {
	w := newFamWorld()
	seedFamily(t, w, pupilA)
	svc := famService(w)
	f, err := svc.RenameFamily(context.Background(), famID, "  García L. ")
	if err != nil || f.Name() != "García L." {
		t.Fatalf("Rename = %+v, %v", f, err)
	}
	if _, err := svc.RenameFamily(context.Background(), famID, " "); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("blank name: %v", err)
	}
	if _, err := svc.RenameFamily(context.Background(), famUnknown, "X"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown family: %v", err)
	}
}

func TestAddStudentsToFamily(t *testing.T) {
	tests := []struct {
		name     string
		family   string
		ids      []string
		setup    func(w *famWorld)
		wantKind glerrors.Kind
	}{
		{name: "adds a pupil without a family", family: famID, ids: []string{pupilB}},
		{name: "adds a draft", family: famID, ids: []string{pupilDraft}},
		{name: "unknown family", family: famUnknown, ids: []string{pupilB}, wantKind: glerrors.KindNotFound},
		{name: "unknown pupil", family: famID, ids: []string{pupilNope}, wantKind: glerrors.KindInvalid},
		{name: "a withdrawn pupil", family: famID, ids: []string{pupilGone}, wantKind: glerrors.KindInvalid},
		{name: "a member again", family: famID, ids: []string{pupilA}, wantKind: glerrors.KindConflict},
		{
			name: "a pupil of another family", family: famID, ids: []string{pupilB},
			setup:    func(w *famWorld) { w.pupils[pupilB].AssignFamily(famOther) },
			wantKind: glerrors.KindConflict,
		},
		{name: "nobody", family: famID, ids: nil, wantKind: glerrors.KindInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newFamWorld()
			seedFamily(t, w, pupilA)
			if tc.setup != nil {
				tc.setup(w)
			}
			f, err := famService(w).AddStudentsToFamily(context.Background(), tc.family, tc.ids)
			if tc.wantKind != glerrors.KindUnknown {
				if glerrors.KindOf(err) != tc.wantKind {
					t.Fatalf("err = %v, want kind %v", err, tc.wantKind)
				}
				return
			}
			if err != nil {
				t.Fatalf("AddStudentsToFamily: %v", err)
			}
			if want := append([]string{pupilA}, tc.ids...); !reflect.DeepEqual(f.StudentIDs(), want) {
				t.Errorf("members = %v, want %v", f.StudentIDs(), want)
			}
		})
	}
}

func TestRemoveStudentFromFamily(t *testing.T) {
	w := newFamWorld()
	seedFamily(t, w, pupilA, pupilB)
	svc := famService(w)

	f, dissolved, err := svc.RemoveStudentFromFamily(context.Background(), famID, pupilB)
	if err != nil || dissolved || !reflect.DeepEqual(f.StudentIDs(), []string{pupilA}) {
		t.Fatalf("Remove = %+v, %v, %v", f, dissolved, err)
	}
	if w.pupils[pupilB].FamilyID() != "" {
		t.Error("the pupil kept its family")
	}
	if _, _, err := svc.RemoveStudentFromFamily(context.Background(), famID, pupilB); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("removing a non-member: %v", err)
	}
	f, dissolved, err = svc.RemoveStudentFromFamily(context.Background(), famID, pupilA)
	if err != nil || !dissolved || f.ID() != "" {
		t.Fatalf("removing the last one: %+v, %v, %v", f, dissolved, err)
	}
	if _, ok := w.families[famID]; ok {
		t.Error("the empty family survived")
	}
}

func TestDissolveFamily(t *testing.T) {
	w := newFamWorld()
	seedFamily(t, w, pupilA, pupilB)
	svc := famService(w)
	if err := svc.DissolveFamily(context.Background(), famID); err != nil {
		t.Fatalf("Dissolve: %v", err)
	}
	if w.pupils[pupilA].FamilyID() != "" || w.pupils[pupilB].FamilyID() != "" {
		t.Error("a member kept the family")
	}
	if err := svc.DissolveFamily(context.Background(), famID); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("dissolving twice: %v", err)
	}
}

func TestSuggestFamiliesReadsTheRepository(t *testing.T) {
	w := newFamWorld()
	w.inputs = ports.SuggestionInputs{
		Pupils: []domain.SuggestionPupil{
			{ID: pupilA, LastName: "García", MaternalLastName: "López"},
			{ID: pupilB, LastName: "García", MaternalLastName: "López"},
		},
	}
	got, err := famService(w).SuggestFamilies(context.Background())
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0].StudentIDs, []string{pupilA, pupilB}) {
		t.Fatalf("SuggestFamilies = %+v, %v", got, err)
	}
	w.inputs.Dismissed = map[string]bool{got[0].Signature: true}
	if again, err := famService(w).SuggestFamilies(context.Background()); err != nil || len(again) != 0 {
		t.Errorf("a dismissed group came back: %+v, %v", again, err)
	}
}

func TestDismissFamilySuggestion(t *testing.T) {
	w := newFamWorld()
	svc := famService(w)
	if err := svc.DismissFamilySuggestion(context.Background(), []string{pupilB, strings.ToUpper(pupilA)}, famActor); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	sig := domain.SuggestionSignature([]string{pupilA, pupilB})
	d, ok := w.dismissals[sig]
	if !ok || d.DismissedBy != famActor || d.ID != famID {
		t.Fatalf("dismissals = %+v (want signature over the canonical ids)", w.dismissals)
	}
	if err := svc.DismissFamilySuggestion(context.Background(), []string{pupilA, pupilB}, famActor); err != nil {
		t.Errorf("dismissing twice must be fine: %v", err)
	}
	for name, ids := range map[string][]string{
		"one pupil": {pupilA}, "malformed": {pupilA, "x"}, "a pupil twice": {pupilA, pupilA},
	} {
		if err := svc.DismissFamilySuggestion(context.Background(), ids, famActor); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := svc.DismissFamilySuggestion(context.Background(), []string{pupilA, pupilB}, "nope"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("malformed user id: %v", err)
	}
}

func TestListStudentsRefusesAMalformedFamilyID(t *testing.T) {
	svc := usecase.NewStudentService(newFamWorld(), &fakePub{}, fakeLevels{}, famTenant, func() string { return famID })
	if _, _, err := svc.ListStudents(context.Background(), ports.ListFilter{FamilyID: "nope"}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("malformed family id: want KindInvalid, got %v", err)
	}
}

func TestSuggestFamiliesHonorsTheSchoolWideCommonValues(t *testing.T) {
	w := newFamWorld()
	w.inputs = ports.SuggestionInputs{
		Pupils: []domain.SuggestionPupil{
			{ID: pupilA, LastName: "Ruiz", MaternalLastName: "Paz"},
			{ID: pupilB, LastName: "Soto", MaternalLastName: "Gil"},
		},
		Contacts: []domain.SuggestionContact{{StudentID: pupilA, Phone: "4430000000"}, {StudentID: pupilB, Phone: "4430000000"}},
		Common:   domain.CommonContactValues{Phones: map[string]bool{"4430000000": true}},
	}
	if got, err := famService(w).SuggestFamilies(context.Background()); err != nil || len(got) != 0 {
		t.Errorf("a school-wide common phone linked pupils: %+v, %v", got, err)
	}
}
