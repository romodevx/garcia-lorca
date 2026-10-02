package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

const (
	scCycle   = "00000000-0000-4000-8000-0000000005c1"
	scBlock   = "00000000-0000-4000-8000-0000000005b1"
	scAssign  = "00000000-0000-4000-8000-0000000005a1"
	scTeacher = "00000000-0000-4000-8000-0000000005d1"
	scGroup   = "00000000-0000-4000-8000-0000000005e1"
	scSubject = "00000000-0000-4000-8000-0000000005f1"
	scLevel   = "00000000-0000-4000-8000-000000000511"
	scOvr     = "00000000-0000-4000-8000-000000000521"
)

var scMon = time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)

// fakeScheduleSvc records what the edge hands config.
type fakeScheduleSvc struct {
	boardCycle, boardTeacher string
	boardCalls               int
	dayDate                  time.Time
	dayTeacher               string
	blockIn                  client.ScheduleBlockInput
	actor                    client.ScheduleActor
	updatedID, deletedID     string
	deleteReason             string
	ovrIn                    client.ScheduleOverrideInput
	avail                    client.AvailabilityQuery
	grid                     [3]any
	eventsLimit              int
	teachers                 []client.Teacher
	err                      error
}

func scBlockResult() client.ScheduleBlock {
	return client.ScheduleBlock{ID: scBlock, CycleID: scCycle, AssignmentID: scAssign, GroupID: scGroup, SubjectID: scSubject, TeacherID: scTeacher, Weekday: 1, StartsAt: 480, EndsAt: 530}
}

func (f *fakeScheduleSvc) GetScheduleBoard(_ context.Context, cycleID, teacherID string) (client.ScheduleBoard, error) {
	f.boardCalls++
	f.boardCycle, f.boardTeacher = cycleID, teacherID
	return client.ScheduleBoard{
		Cycle:       client.CycleResult{ID: scCycle, Name: "2026-2027", Status: "active", StartsOn: scMon, EndsOn: scMon},
		Cycles:      []client.CycleResult{{ID: scCycle, Name: "2026-2027", Status: "active", StartsOn: scMon, EndsOn: scMon}},
		Started:     true,
		Grids:       []client.ScheduleGrid{{LevelID: scLevel, GridStart: 420, GridEnd: 900}},
		Levels:      []client.LevelResult{{ID: scLevel, Name: "Primaria"}},
		Grades:      []client.GradeLevelResult{{ID: "g", LevelID: scLevel, Name: "3.º", Ordinal: 3}},
		Groups:      []client.GroupResult{{ID: scGroup, GradeLevelID: "g", Name: "A"}},
		Subjects:    []client.SubjectResult{{ID: scSubject, GradeLevelID: "g", Name: "Música"}},
		Teachers:    []client.Teacher{{ID: scTeacher, FirstName: "Ana", LastName: "Ruiz", Email: "ana@x.mx"}},
		Assignments: []client.TeachingAssignment{{ID: scAssign, GroupID: scGroup, SubjectID: scSubject, TeacherID: scTeacher}, {ID: "other", GroupID: scGroup, SubjectID: "s2", TeacherID: "t2"}},
		Blocks:      []client.ScheduleBlock{scBlockResult()},
	}, f.err
}

func (f *fakeScheduleSvc) GetScheduleDay(_ context.Context, cycleID string, date time.Time, teacherID string) (client.ScheduleDay, error) {
	f.dayDate, f.dayTeacher = date, teacherID
	o := client.ScheduleOverride{ID: scOvr, CycleID: cycleID, BlockID: scBlock, Date: date, Kind: "substitute", TeacherID: "t2", Reason: "tarde",
		CreatedBy: client.ScheduleActor{UserID: "u", Name: "D"}, CreatedAt: scMon}
	return client.ScheduleDay{Date: date, Weekday: 1, Classes: []client.ScheduledClass{
		{Block: scBlockResult(), TeacherID: "t2", StartsAt: 480, EndsAt: 530, Status: "substitute", Override: &o},
		{Block: scBlockResult(), TeacherID: scTeacher, StartsAt: 530, EndsAt: 580, Status: "regular"},
	}}, f.err
}

func (f *fakeScheduleSvc) CreateScheduleBlock(_ context.Context, in client.ScheduleBlockInput, by client.ScheduleActor) (client.ScheduleBlock, error) {
	f.blockIn, f.actor = in, by
	return scBlockResult(), f.err
}

func (f *fakeScheduleSvc) UpdateScheduleBlock(_ context.Context, id string, in client.ScheduleBlockInput, by client.ScheduleActor) (client.ScheduleBlock, error) {
	f.updatedID, f.blockIn, f.actor = id, in, by
	return scBlockResult(), f.err
}

func (f *fakeScheduleSvc) DeleteScheduleBlock(_ context.Context, id, reason string, by client.ScheduleActor) error {
	f.deletedID, f.deleteReason, f.actor = id, reason, by
	return f.err
}

func (f *fakeScheduleSvc) CreateScheduleOverride(_ context.Context, in client.ScheduleOverrideInput, by client.ScheduleActor) (client.ScheduleOverride, error) {
	f.ovrIn, f.actor = in, by
	return client.ScheduleOverride{ID: scOvr, CycleID: scCycle, BlockID: in.BlockID, Date: in.Date, Kind: in.Kind, TeacherID: in.TeacherID,
		SwapBlockID: in.SwapBlockID, Reason: in.Reason, CreatedBy: by, CreatedAt: scMon}, f.err
}

func (f *fakeScheduleSvc) DeleteScheduleOverride(_ context.Context, id, reason string, by client.ScheduleActor) error {
	f.deletedID, f.deleteReason, f.actor = id, reason, by
	return f.err
}

func (f *fakeScheduleSvc) ListAvailableTeachers(_ context.Context, q client.AvailabilityQuery) ([]client.AvailableTeacher, error) {
	f.avail = q
	return []client.AvailableTeacher{{Teacher: client.Teacher{ID: "t2", FirstName: "Eva", LastName: "Soto"}, Match: "subject"}}, f.err
}

func (f *fakeScheduleSvc) SetScheduleGrid(_ context.Context, levelID string, start, end int) (client.ScheduleGrid, error) {
	f.grid = [3]any{levelID, start, end}
	return client.ScheduleGrid{LevelID: levelID, GridStart: start, GridEnd: end}, f.err
}

func (f *fakeScheduleSvc) ListScheduleEvents(_ context.Context, _ string, limit int) ([]client.ScheduleEvent, error) {
	f.eventsLimit = limit
	return []client.ScheduleEvent{
		{ID: "00000000-0000-4000-8000-000000000531", At: scMon, By: client.ScheduleActor{UserID: "u", Name: "D"}, Action: "override_created", BlockID: scBlock, OverrideID: scOvr, Date: scMon, Reason: "tarde", Summary: "Suplencia"},
		{ID: "00000000-0000-4000-8000-000000000532", At: scMon, By: client.ScheduleActor{UserID: "u", Name: "D"}, Action: "block_created", BlockID: scBlock, Summary: "Clase nueva"},
	}, f.err
}

func (f *fakeScheduleSvc) ListTeachers(context.Context) ([]client.Teacher, error) {
	return f.teachers, nil
}

func (f *fakeScheduleSvc) ListCycles(context.Context) ([]client.CycleResult, error) {
	return []client.CycleResult{{ID: "old", Status: "closed"}, {ID: scCycle, Status: "active"}}, nil
}

type scheduleNamer struct{}

func (scheduleNamer) GetUser(context.Context, string) (client.User, error) {
	return client.User{DisplayName: "Directora Ruiz"}, nil
}

func scheduleReq(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	return r.WithContext(middleware.WithPrincipal(r.Context(), middleware.Principal{UserID: "u-dir", Email: "dir@x.mx"}))
}

func newScheduleH() (*handler.ScheduleHandler, *fakeScheduleSvc) {
	f := &fakeScheduleSvc{}
	return handler.NewScheduleHandler(f, scheduleNamer{}), f
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return out
}

func TestScheduleBoardDefaultsToTheActiveCycle(t *testing.T) {
	h, f := newScheduleH()
	rec := serve(h.Board, scheduleReq(http.MethodGet, "/api/v1/schedule", ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "", f.boardCycle)
	assert.Equal(t, "", f.boardTeacher)
	body := decode(t, rec)
	assert.Equal(t, true, body["started"])
	assert.Equal(t, true, body["cycle"].(map[string]any)["active"])
	assert.Equal(t, "07:00", body["settings"].([]any)[0].(map[string]any)["gridStart"])
	blk := body["blocks"].([]any)[0].(map[string]any)
	assert.Equal(t, "08:00", blk["startsAt"])
	assert.Equal(t, "08:50", blk["endsAt"])

	rec = serve(h.Board, scheduleReq(http.MethodGet, "/api/v1/schedule?cycleId=nope", ""))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestScheduleCreateBlockParsesClockTimesAndNamesTheActor(t *testing.T) {
	h, f := newScheduleH()
	rec := serve(h.CreateBlock, scheduleReq(http.MethodPost, "/api/v1/schedule/blocks",
		`{"assignmentId":"`+scAssign+`","weekday":1,"startsAt":"08:00","endsAt":"08:50","reason":"Nueva"}`))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, client.ScheduleBlockInput{AssignmentID: scAssign, Weekday: 1, StartsAt: 480, EndsAt: 530, Reason: "Nueva"}, f.blockIn)
	assert.Equal(t, client.ScheduleActor{UserID: "u-dir", Name: "Directora Ruiz"}, f.actor)

	for _, body := range []string{
		`{"assignmentId":"` + scAssign + `","weekday":1,"startsAt":"8:00","endsAt":"08:50"}`,
		`{"assignmentId":"` + scAssign + `","weekday":1,"startsAt":"08:00","endsAt":"24:30"}`,
		`{"assignmentId":"` + scAssign + `","weekday":1,"startsAt":"08:61","endsAt":"09:00"}`,
	} {
		rec = serve(h.CreateBlock, scheduleReq(http.MethodPost, "/api/v1/schedule/blocks", body))
		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
		assert.Equal(t, "SCHEDULE_TIME_INVALID", rec.Header().Get("X-Error-Code"), body)
	}
	rec = serve(h.CreateBlock, scheduleReq(http.MethodPost, "/api/v1/schedule/blocks", `{"assignmentId":"x","weekday":1,"startsAt":"08:00","endsAt":"08:50"}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	rec = serve(h.CreateBlock, scheduleReq(http.MethodPost, "/api/v1/schedule/blocks", `{"assignmentId":"`+scAssign+`","weekday":6,"startsAt":"08:00","endsAt":"08:50"}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "SCHEDULE_WEEKDAY_INVALID", rec.Header().Get("X-Error-Code"))
	long := strings.Repeat("a", 201)
	rec = serve(h.CreateBlock, scheduleReq(http.MethodPost, "/api/v1/schedule/blocks", `{"assignmentId":"`+scAssign+`","weekday":1,"startsAt":"08:00","endsAt":"08:50","reason":"`+long+`"}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestScheduleConflictsForwardConfigsSentenceAndCode(t *testing.T) {
	h, f := newScheduleH()
	ce := connect.NewError(connect.CodeAlreadyExists, errors.New("Ana Ruiz ya da Matemáticas a 3.º A el lunes de 08:00 a 08:50"))
	ce.Meta().Set("X-Error-Code", "SCHEDULE_TEACHER_BUSY")
	f.err = ce
	rec := serve(h.CreateBlock, scheduleReq(http.MethodPost, "/api/v1/schedule/blocks",
		`{"assignmentId":"`+scAssign+`","weekday":1,"startsAt":"08:00","endsAt":"08:50"}`))
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "SCHEDULE_TEACHER_BUSY", rec.Header().Get("X-Error-Code"))
	assert.Contains(t, rec.Body.String(), "Ana Ruiz ya da Matemáticas")
}

func TestScheduleUpdateAndDeleteBlock(t *testing.T) {
	h, f := newScheduleH()
	r := scheduleReq(http.MethodPut, "/api/v1/schedule/blocks/"+scBlock, `{"assignmentId":"`+scAssign+`","weekday":2,"startsAt":"09:00","endsAt":"09:50"}`)
	r.SetPathValue("id", scBlock)
	rec := serve(h.UpdateBlock, r)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, scBlock, f.updatedID)
	assert.Equal(t, 540, f.blockIn.StartsAt)

	r = scheduleReq(http.MethodDelete, "/api/v1/schedule/blocks/"+scBlock+"?reason=Ya+no", "")
	r.SetPathValue("id", scBlock)
	rec = serve(h.DeleteBlock, r)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "Ya no", f.deleteReason)

	r = scheduleReq(http.MethodDelete, "/api/v1/schedule/blocks/x", "")
	r.SetPathValue("id", "x")
	assert.Equal(t, http.StatusBadRequest, serve(h.DeleteBlock, r).Code)
}

func TestScheduleDayAndOverrides(t *testing.T) {
	h, f := newScheduleH()
	rec := serve(h.Day, scheduleReq(http.MethodGet, "/api/v1/schedule/day?cycleId="+scCycle+"&date=2026-10-05", ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, scMon, f.dayDate)
	body := decode(t, rec)
	classes := body["classes"].([]any)
	require.Len(t, classes, 2)
	first := classes[0].(map[string]any)
	assert.Equal(t, "substitute", first["status"])
	assert.Equal(t, scTeacher, first["baseTeacherId"])
	assert.Equal(t, scOvr, first["overrideId"])
	assert.Equal(t, "tarde", first["reason"])
	second := classes[1].(map[string]any)
	assert.Nil(t, second["overrideId"])
	assert.Equal(t, "08:50", second["startsAt"])
	assert.Equal(t, "08:00", second["baseStartsAt"])

	assert.Equal(t, http.StatusBadRequest, serve(h.Day, scheduleReq(http.MethodGet, "/api/v1/schedule/day?cycleId="+scCycle+"&date=05/10/2026", "")).Code)

	rec = serve(h.CreateOverride, scheduleReq(http.MethodPost, "/api/v1/schedule/overrides",
		`{"blockId":"`+scBlock+`","date":"2026-10-05","kind":"substitute","teacherId":"`+scTeacher+`","reason":"La maestra llegó tarde"}`))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, client.ScheduleOverrideInput{BlockID: scBlock, Date: scMon, Kind: "substitute", TeacherID: scTeacher, Reason: "La maestra llegó tarde"}, f.ovrIn)
	o := decode(t, rec)["override"].(map[string]any)
	assert.Equal(t, "2026-10-05", o["date"])
	assert.Nil(t, o["swapBlockId"])
	assert.Equal(t, "Directora Ruiz", o["createdByName"])

	for _, body := range []string{
		`{"blockId":"` + scBlock + `","date":"2026-10-05","kind":"move","reason":"x"}`,
		`{"blockId":"` + scBlock + `","date":"2026-13-05","kind":"cancel","reason":"x"}`,
		`{"blockId":"` + scBlock + `","date":"2026-10-05","kind":"substitute","teacherId":"bad","reason":"x"}`,
	} {
		assert.Equal(t, http.StatusBadRequest, serve(h.CreateOverride, scheduleReq(http.MethodPost, "/api/v1/schedule/overrides", body)).Code, body)
	}

	r := scheduleReq(http.MethodDelete, "/api/v1/schedule/overrides/"+scOvr, "")
	r.SetPathValue("id", scOvr)
	assert.Equal(t, http.StatusNoContent, serve(h.DeleteOverride, r).Code)
	assert.Equal(t, scOvr, f.deletedID)
}

func TestScheduleAvailableTeachersSettingsAndEvents(t *testing.T) {
	h, f := newScheduleH()
	base := "/api/v1/schedule/available-teachers?cycleId=" + scCycle + "&groupId=" + scGroup + "&subjectId=" + scSubject + "&startsAt=08:00&endsAt=08:50"
	rec := serve(h.AvailableTeachers, scheduleReq(http.MethodGet, base+"&date=2026-10-05&excludeBlockId="+scBlock, ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, scMon, f.avail.Date)
	assert.Equal(t, scBlock, f.avail.ExcludeBlockID)
	tc := decode(t, rec)["teachers"].([]any)[0].(map[string]any)
	assert.Equal(t, "subject", tc["match"])
	assert.Equal(t, "Soto", tc["lastName"])

	rec = serve(h.AvailableTeachers, scheduleReq(http.MethodGet, base, ""))
	assert.Equal(t, http.StatusBadRequest, rec.Code, "weekday or date is required")
	rec = serve(h.AvailableTeachers, scheduleReq(http.MethodGet, base+"&weekday=3", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 3, f.avail.Weekday)

	r := scheduleReq(http.MethodPut, "/api/v1/schedule/settings/"+scLevel, `{"gridStart":"06:30","gridEnd":"14:00"}`)
	r.SetPathValue("levelId", scLevel)
	rec = serve(h.PutSettings, r)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, [3]any{scLevel, 390, 840}, f.grid)
	assert.Equal(t, "06:30", decode(t, rec)["settings"].(map[string]any)["gridStart"])

	rec = serve(h.Events, scheduleReq(http.MethodGet, "/api/v1/schedule/events?cycleId="+scCycle+"&limit=20", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 20, f.eventsLimit)
	evs := decode(t, rec)["events"].([]any)
	assert.Equal(t, "2026-10-05", evs[0].(map[string]any)["date"])
	assert.Nil(t, evs[1].(map[string]any)["date"])
	assert.Nil(t, evs[1].(map[string]any)["overrideId"])
	assert.Equal(t, http.StatusBadRequest, serve(h.Events, scheduleReq(http.MethodGet, "/api/v1/schedule/events?cycleId="+scCycle+"&limit=900", "")).Code)
}

func TestOwnScheduleIsScopedToTheCallersTeacher(t *testing.T) {
	h, f := newScheduleH()
	f.teachers = []client.Teacher{{ID: "t-other", UserID: "u-other"}, {ID: scTeacher, UserID: "u-dir"}}
	rec := serve(h.OwnBoard, scheduleReq(http.MethodGet, "/api/v1/teaching/schedule", ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, scTeacher, f.boardTeacher)
	assert.Len(t, decode(t, rec)["assignments"].([]any), 1, "only the caller's own assignments")

	rec = serve(h.OwnDay, scheduleReq(http.MethodGet, "/api/v1/teaching/schedule/day?date=2026-10-05", ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, scTeacher, f.dayTeacher)

	// A caller with no teacher record: an empty timetable, and config is never
	// asked with an empty teacher (which would mean "everybody").
	f2 := &fakeScheduleSvc{teachers: []client.Teacher{{ID: "t-other", UserID: "u-other"}}}
	h2 := handler.NewScheduleHandler(f2, scheduleNamer{})
	rec = serve(h2.OwnBoard, scheduleReq(http.MethodGet, "/api/v1/teaching/schedule", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, decode(t, rec)["blocks"])
	assert.NotEqual(t, "", f2.boardTeacher, "never an unscoped read")
	rec = serve(h2.OwnDay, scheduleReq(http.MethodGet, "/api/v1/teaching/schedule/day?date=2026-10-05", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, decode(t, rec)["classes"])
}

// The timetable routes are in the contract, answer 4xx as text/plain, and the
// wire objects carry exactly the fields their schemas require.
func TestScheduleContract(t *testing.T) {
	doc := loadOpenAPI(t)
	for _, op := range []struct{ path, method string }{
		{"/api/v1/schedule", "get"},
		{"/api/v1/schedule/blocks", "post"},
		{"/api/v1/schedule/blocks/{id}", "put"},
		{"/api/v1/schedule/blocks/{id}", "delete"},
		{"/api/v1/schedule/day", "get"},
		{"/api/v1/schedule/overrides", "post"},
		{"/api/v1/schedule/overrides/{id}", "delete"},
		{"/api/v1/schedule/available-teachers", "get"},
		{"/api/v1/schedule/settings/{levelId}", "put"},
		{"/api/v1/schedule/events", "get"},
		{"/api/v1/teaching/schedule", "get"},
		{"/api/v1/teaching/schedule/day", "get"},
	} {
		node, ok := doc.Paths[op.path][op.method]
		require.True(t, ok, "%s %s is documented", op.method, op.path)
		var operation openAPIOperation
		require.NoError(t, node.Decode(&operation))
		for status, response := range operation.Responses {
			if strings.HasPrefix(status, "4") {
				assert.Contains(t, doc.resolve(t, response).Content, "text/plain", "%s %s %s", op.method, op.path, status)
			}
		}
	}

	raw, err := os.ReadFile(filepath.Clean(openAPIPath))
	require.NoError(t, err)
	var schemas struct {
		Components struct {
			Schemas map[string]struct {
				Required []string `yaml:"required"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &schemas))
	required := func(name string) []string {
		want := append([]string(nil), schemas.Components.Schemas[name].Required...)
		require.NotEmpty(t, want, name)
		sort.Strings(want)
		return want
	}
	keys := func(m map[string]any) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}

	h, _ := newScheduleH()
	board := decode(t, serve(h.Board, scheduleReq(http.MethodGet, "/api/v1/schedule", "")))
	assert.Equal(t, required("ScheduleBoard"), keys(board))
	assert.Equal(t, required("ScheduleBlock"), keys(board["blocks"].([]any)[0].(map[string]any)))
	assert.Equal(t, required("ScheduleCycle"), keys(board["cycle"].(map[string]any)))
	assert.Equal(t, required("ScheduleSettings"), keys(board["settings"].([]any)[0].(map[string]any)))

	day := decode(t, serve(h.Day, scheduleReq(http.MethodGet, "/api/v1/schedule/day?cycleId="+scCycle+"&date=2026-10-05", "")))
	assert.Equal(t, required("ScheduleDay"), keys(day))
	assert.Equal(t, required("ScheduleClass"), keys(day["classes"].([]any)[0].(map[string]any)))

	ovr := decode(t, serve(h.CreateOverride, scheduleReq(http.MethodPost, "/api/v1/schedule/overrides",
		`{"blockId":"`+scBlock+`","date":"2026-10-05","kind":"cancel","reason":"x"}`)))
	assert.Equal(t, required("ScheduleOverride"), keys(ovr["override"].(map[string]any)))

	ev := decode(t, serve(h.Events, scheduleReq(http.MethodGet, "/api/v1/schedule/events?cycleId="+scCycle, "")))
	assert.Equal(t, required("ScheduleEvent"), keys(ev["events"].([]any)[0].(map[string]any)))

	avail := decode(t, serve(h.AvailableTeachers, scheduleReq(http.MethodGet, "/api/v1/schedule/available-teachers?cycleId="+scCycle+"&groupId="+scGroup+"&subjectId="+scSubject+"&weekday=1&startsAt=08:00&endsAt=08:50", "")))
	assert.Equal(t, required("ScheduleAvailableTeacher"), keys(avail["teachers"].([]any)[0].(map[string]any)))
}
