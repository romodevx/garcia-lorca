package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// --- the labels source (CC-211 / CC-313) ------------------------------------

type fakeRoomCatalog struct {
	cycles    []client.CycleResult
	groups    []client.GroupResult
	grades    []client.GradeLevelResult
	levels    []client.LevelResult
	err       error
	gotCycle  string
	gotGrades string
}

func (f *fakeRoomCatalog) ListCycles(context.Context) ([]client.CycleResult, error) {
	return f.cycles, f.err
}

func (f *fakeRoomCatalog) ListGroups(_ context.Context, cycleID, _ string) ([]client.GroupResult, error) {
	f.gotCycle = cycleID
	return f.groups, f.err
}

func (f *fakeRoomCatalog) ListGradeLevels(_ context.Context, levelID string) ([]client.GradeLevelResult, error) {
	f.gotGrades = levelID
	return f.grades, f.err
}

func (f *fakeRoomCatalog) ListLevels(context.Context) ([]client.LevelResult, error) {
	return f.levels, f.err
}

type fakeCycleEnrollments struct {
	rows     []client.Enrollment
	err      error
	gotCycle string
}

func (f *fakeCycleEnrollments) ListEnrollments(_ context.Context, studentID, cycleID, groupID string) ([]client.Enrollment, error) {
	if studentID != "" || groupID != "" {
		return nil, errors.New("the labels read the whole cycle, never one pupil or group")
	}
	f.gotCycle = cycleID
	return f.rows, f.err
}

func roomSchool() *fakeRoomCatalog {
	return &fakeRoomCatalog{
		cycles: []client.CycleResult{{ID: "c-old", Status: "closed"}, {ID: "c-now", Status: "active"}},
		levels: []client.LevelResult{{ID: "l-pri", Name: "PRIMARIA"}, {ID: "l-sec", Name: "SECUNDARIA"}},
		grades: []client.GradeLevelResult{
			{ID: "g-3", LevelID: "l-pri", Name: "3°"},
			{ID: "g-1s", LevelID: "l-sec", Name: "1° SECUNDARIA"},
		},
		groups: []client.GroupResult{
			{ID: "gr-3b", GradeLevelID: "g-3", Name: "B"},
			{ID: "gr-1a", GradeLevelID: "g-1s", Name: "A"},
			{ID: "gr-orphan", GradeLevelID: "g-gone", Name: "C"},
		},
	}
}

func TestCycleRoomLabels(t *testing.T) {
	tests := []struct {
		name string
		rows []client.Enrollment
		want map[string]string
	}{
		{
			name: "the grade and level are named, once each",
			rows: []client.Enrollment{
				{StudentID: "dos", GroupID: "gr-3b", Status: "active"},
				{StudentID: "uno", GroupID: "gr-1a", Status: "active"},
			},
			want: map[string]string{"dos": "3° PRIMARIA · B", "uno": "1° SECUNDARIA · A"},
		},
		{
			name: "an active enrollment wins over the one the pupil was moved out of",
			rows: []client.Enrollment{
				{StudentID: "dos", GroupID: "gr-1a", Status: "withdrawn"},
				{StudentID: "dos", GroupID: "gr-3b", Status: "active"},
			},
			want: map[string]string{"dos": "3° PRIMARIA · B"},
		},
		{
			name: "a withdrawn pupil keeps the classroom they left",
			rows: []client.Enrollment{{StudentID: "tres", GroupID: "gr-3b", Status: "withdrawn"}},
			want: map[string]string{"tres": "3° PRIMARIA · B"},
		},
		{
			name: "a group the catalog no longer holds, or whose grade is gone, has no label",
			rows: []client.Enrollment{
				{StudentID: "a", GroupID: "gr-missing", Status: "active"},
				{StudentID: "b", GroupID: "gr-orphan", Status: "active"},
			},
			want: map[string]string{},
		},
		{
			name: "a transferred-out enrollment does not name the room",
			rows: []client.Enrollment{{StudentID: "d", GroupID: "gr-3b", Status: "transferred"}},
			want: map[string]string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			catalog := roomSchool()
			enrollments := &fakeCycleEnrollments{rows: tc.rows}
			got, err := handler.NewCycleRoomLabels(catalog, enrollments).RoomLabels(context.Background())
			if err != nil {
				t.Fatalf("RoomLabels: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("labels = %v, want %v", got, tc.want)
			}
			for id, label := range tc.want {
				if got[id] != label {
					t.Errorf("label of %s = %q, want %q", id, got[id], label)
				}
			}
			if catalog.gotCycle != "c-now" || enrollments.gotCycle != "c-now" {
				t.Errorf("read cycle groups=%q enrollments=%q, want the active c-now", catalog.gotCycle, enrollments.gotCycle)
			}
			if catalog.gotGrades != "" {
				t.Errorf("grades read for level %q, want every level", catalog.gotGrades)
			}
		})
	}
}

func TestCycleRoomLabelsWithoutAnActiveCycleUsesTheFirst(t *testing.T) {
	catalog := roomSchool()
	catalog.cycles = []client.CycleResult{{ID: "c-draft", Status: "draft"}, {ID: "c-x", Status: "closed"}}
	enrollments := &fakeCycleEnrollments{}
	if _, err := handler.NewCycleRoomLabels(catalog, enrollments).RoomLabels(context.Background()); err != nil {
		t.Fatalf("RoomLabels: %v", err)
	}
	if enrollments.gotCycle != "c-draft" {
		t.Errorf("cycle = %q, want the first one, as the web picks it", enrollments.gotCycle)
	}
}

func TestCycleRoomLabelsWithNoCycleIsEmpty(t *testing.T) {
	catalog := roomSchool()
	catalog.cycles = nil
	got, err := handler.NewCycleRoomLabels(catalog, &fakeCycleEnrollments{}).RoomLabels(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("labels = %v, err = %v; want none and no error", got, err)
	}
}

func TestCycleRoomLabelsPassesAFailureUp(t *testing.T) {
	catalog := roomSchool()
	catalog.err = errors.New("config down")
	if _, err := handler.NewCycleRoomLabels(catalog, &fakeCycleEnrollments{}).RoomLabels(context.Background()); err == nil {
		t.Fatal("want the catalog failure, got nil")
	}
}

// --- the list carries them --------------------------------------------------

type fakeRoomLabels struct {
	labels map[string]string
	err    error
}

func (f fakeRoomLabels) RoomLabels(context.Context) (map[string]string, error) {
	return f.labels, f.err
}

type listedRoom struct {
	ID         string  `json:"id"`
	GradeGroup string  `json:"gradeGroup"`
	GroupLabel *string `json:"groupLabel"`
}

func listRooms(t *testing.T, h *handler.StudentsHandler) []listedRoom {
	t.Helper()
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/students", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Students []listedRoom `json:"students"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Students
}

func roomPupils() *fakeCreator {
	return &fakeCreator{list: []client.StudentResult{
		{ID: "dos", Level: "PRIMARIA", GradeGroup: "B"},
		{ID: "nuevo", Level: "PRIMARIA", GradeGroup: "A"},
	}}
}

func TestListStudentsCarriesTheGroupLabelWithItsGrade(t *testing.T) {
	h := handler.NewStudentsHandler(roomPupils(), handler.WithStudentRoomLabels(fakeRoomLabels{labels: map[string]string{"dos": "3° PRIMARIA · B"}}))

	got := listRooms(t, h)

	if got[0].GroupLabel == nil || *got[0].GroupLabel != "3° PRIMARIA · B" {
		t.Errorf("dos groupLabel = %v, want 3° PRIMARIA · B", got[0].GroupLabel)
	}
	if got[1].GroupLabel != nil {
		t.Errorf("a pupil with no classroom has groupLabel %q, want it absent", *got[1].GroupLabel)
	}
	if got[0].GradeGroup != "B" {
		t.Errorf("gradeGroup = %q, want the stored letter kept", got[0].GradeGroup)
	}
}

func TestListStudentsStillAnswersWhenTheLabelsFail(t *testing.T) {
	h := handler.NewStudentsHandler(roomPupils(), handler.WithStudentRoomLabels(fakeRoomLabels{err: errors.New("config down")}))

	got := listRooms(t, h)

	if len(got) != 2 {
		t.Fatalf("listed %d pupils, want 2: a label is a courtesy, never a reason to fail", len(got))
	}
	for _, s := range got {
		if s.GroupLabel != nil {
			t.Errorf("%s groupLabel = %q, want absent", s.ID, *s.GroupLabel)
		}
	}
}

func TestListStudentsWithoutALabelSourceOmitsTheLabel(t *testing.T) {
	for _, s := range listRooms(t, handler.NewStudentsHandler(roomPupils())) {
		if s.GroupLabel != nil {
			t.Errorf("%s groupLabel = %q, want absent", s.ID, *s.GroupLabel)
		}
	}
}
