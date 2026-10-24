package usecase_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Families from an import (PLAN-familias-importacion.md): the plan is the
// domain's; these tests are about executing it through the guarded paths.

const famBatch = "ba000000-0000-0000-0000-000000000001"

// siblingsInput is A and B (García López) sharing a phone, both from the
// batch, as the repository would read them.
func siblingsInput() domain.ImportFamilyInput {
	return domain.ImportFamilyInput{
		Pupils: []domain.ImportPupil{
			{ID: pupilA, LastName: "García", MaternalLastName: "López", InBatch: true},
			{ID: pupilB, LastName: "García", MaternalLastName: "López", InBatch: true},
		},
		Contacts: []domain.SuggestionContact{
			{StudentID: pupilA, Phone: "4435550000"},
			{StudentID: pupilB, Phone: "4435550000"},
		},
	}
}

func TestBuildFamiliesFromImportCreates(t *testing.T) {
	w := newAccWorld()
	w.importInput = siblingsInput()
	res, err := w.families.BuildFamiliesFromImport(context.Background(), strings.ToUpper(famBatch))
	if err != nil {
		t.Fatalf("BuildFamiliesFromImport: %v", err)
	}
	if len(res.Created) != 1 || len(res.Joined) != 0 || res.Skipped != 0 {
		t.Fatalf("result = %+v, want one created", res)
	}
	f := res.Created[0]
	if f.Name() != "García López" || !slices.Equal(f.StudentIDs(), []string{pupilA, pupilB}) {
		t.Errorf("created = %+v", f)
	}
	if w.importBatch != famBatch {
		t.Errorf("inputs read for batch %q, want the canonical %q", w.importBatch, famBatch)
	}
	want := []ports.FamilyImportLink{
		{BatchID: famBatch, StudentID: pupilA, CreatedFamily: true},
		{BatchID: famBatch, StudentID: pupilB, CreatedFamily: true},
	}
	if got := w.recordedLinks(f.ID()); !slices.Equal(got, want) {
		t.Errorf("links = %+v, want %+v", got, want)
	}
}

func TestBuildFamiliesFromImportJoins(t *testing.T) {
	w := newAccWorld()
	w.group(t, pupilB)
	in := siblingsInput()
	in.Pupils[1].FamilyID, in.Pupils[1].InBatch = famID, false
	in.Families = map[string]domain.ImportFamilyState{famID: {Pupils: 1}}
	w.importInput = in
	res, err := w.families.BuildFamiliesFromImport(context.Background(), famBatch)
	if err != nil {
		t.Fatalf("BuildFamiliesFromImport: %v", err)
	}
	if len(res.Created) != 0 || len(res.Joined) != 1 || res.Skipped != 0 {
		t.Fatalf("result = %+v, want one joined", res)
	}
	if !slices.Equal(res.Joined[0].StudentIDs(), []string{pupilB, pupilA}) || w.pupils[pupilA].FamilyID() != famID {
		t.Errorf("joined = %+v", res.Joined[0])
	}
	want := []ports.FamilyImportLink{{BatchID: famBatch, StudentID: pupilA}}
	if got := w.recordedLinks(famID); !slices.Equal(got, want) {
		t.Errorf("links = %+v, want only the joiner, not created", got)
	}
}

// The plan is read without locks. A family whose pupils changed before the
// write is skipped, and the rest still runs.
func TestBuildFamiliesFromImportSkipsWhatChangedMeanwhile(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, w *accWorld)
	}{
		{
			name:  "a pupil joined a family meanwhile",
			setup: func(_ *testing.T, w *accWorld) { w.pupils[pupilB].AssignFamily(famOther) },
		},
		{
			name: "an app account reached a pupil meanwhile (H1)",
			setup: func(t *testing.T, w *accWorld) {
				w.link(t, "1e000000-0000-0000-0000-000000000001", accMom, pupilA, domain.ContactPerson{})
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newAccWorld()
			in := siblingsInput()
			// A second family the race does not touch: Luna Paz (Draft) and C.
			w.put(pupilC, "Luna", "Paz", domain.AdmissionAccepted)
			in.Pupils = append(in.Pupils,
				domain.ImportPupil{ID: pupilDraft, LastName: "Luna", MaternalLastName: "Paz", InBatch: true},
				domain.ImportPupil{ID: pupilC, LastName: "Luna", MaternalLastName: "Paz"},
			)
			in.Contacts = append(in.Contacts,
				domain.SuggestionContact{StudentID: pupilDraft, Email: "luna@x.mx"},
				domain.SuggestionContact{StudentID: pupilC, Email: "luna@x.mx"},
			)
			w.importInput = in
			tc.setup(t, w)

			res, err := w.families.BuildFamiliesFromImport(context.Background(), famBatch)
			if err != nil {
				t.Fatalf("BuildFamiliesFromImport: %v", err)
			}
			if len(res.Created) != 1 || res.Skipped != 1 {
				t.Fatalf("result = %+v, want the other family created and one skipped", res)
			}
			if got := res.Created[0].StudentIDs(); !slices.Equal(got, []string{pupilDraft, pupilC}) {
				t.Errorf("created = %v", got)
			}
			if w.pupils[pupilA].FamilyID() != "" {
				t.Error("the raced family was written")
			}
			if n := len(w.allRecordedLinks()); n != 2 {
				t.Errorf("links = %d, want only the created family's two", n)
			}
		})
	}
}

func TestBuildFamiliesFromImportStopsOnAFailedRead(t *testing.T) {
	w := newAccWorld()
	boom := errors.New("db down")
	w.importErr = boom
	if _, err := w.families.BuildFamiliesFromImport(context.Background(), famBatch); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the read's", err)
	}
}

// A failure that is not the office's doing stops the build, but the families
// already committed come back with the error: they exist, and the summary
// must be able to show (and undo) them.
func TestBuildFamiliesFromImportReturnsWhatItWroteOnFailure(t *testing.T) {
	w := newAccWorld()
	in := siblingsInput()
	w.put(pupilC, "Luna", "Paz", domain.AdmissionAccepted)
	in.Pupils = append(in.Pupils,
		domain.ImportPupil{ID: pupilDraft, LastName: "Luna", MaternalLastName: "Paz", InBatch: true},
		domain.ImportPupil{ID: pupilC, LastName: "Luna", MaternalLastName: "Paz"},
	)
	in.Contacts = append(in.Contacts,
		domain.SuggestionContact{StudentID: pupilDraft, Email: "luna@x.mx"},
		domain.SuggestionContact{StudentID: pupilC, Email: "luna@x.mx"},
	)
	w.importInput = in
	w.createFailAt, w.createFailErr = 2, glerrors.New(glerrors.KindInternal, "db down")

	res, err := w.families.BuildFamiliesFromImport(context.Background(), famBatch)
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("err = %v, want the internal failure", err)
	}
	if len(res.Created) != 1 || !slices.Equal(res.Created[0].StudentIDs(), []string{pupilA, pupilB}) {
		t.Errorf("partial result = %+v, want the family written before the failure", res)
	}
}

func TestFamiliesFromImportRefuseAMalformedBatch(t *testing.T) {
	w := newAccWorld()
	if _, err := w.families.BuildFamiliesFromImport(context.Background(), "nope"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("build: %v", err)
	}
	if _, _, err := w.families.UndoFamiliesFromImport(context.Background(), ""); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("undo: %v", err)
	}
	if w.importBatch != "" || w.undoBatch != "" {
		t.Error("a malformed batch reached the repository")
	}
}

func TestUndoFamiliesFromImport(t *testing.T) {
	w := newAccWorld()
	w.undo = ports.ImportUndo{Removed: 3, Dissolved: 1}
	removed, dissolved, err := w.families.UndoFamiliesFromImport(context.Background(), strings.ToUpper(famBatch))
	if err != nil || removed != 3 || dissolved != 1 {
		t.Errorf("undo = %d, %d, %v; want 3, 1", removed, dissolved, err)
	}
	if w.undoBatch != famBatch {
		t.Errorf("undo batch = %q, want the canonical %q", w.undoBatch, famBatch)
	}
}
