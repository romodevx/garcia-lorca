package handler_test

import (
	"context"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

const scAbsence = "00000000-0000-4000-8000-000000000541"

func scAbsenceResult() client.TeacherAbsence {
	return client.TeacherAbsence{ID: scAbsence, CycleID: scCycle, TeacherID: scTeacher, Kind: "sick", StartsOn: scMon, EndsOn: scMon.AddDate(0, 0, 2),
		StartMinute: 480, EndMinute: 600, Note: "gripe", CreatedBy: client.ScheduleActor{UserID: "u", Name: "Directora Ruiz"}, CreatedAt: scMon}
}

func (f *fakeScheduleSvc) CreateTeacherAbsence(_ context.Context, in client.TeacherAbsenceInput, by client.ScheduleActor) (client.TeacherAbsenceSaved, error) {
	f.absIn, f.actor = in, by
	return client.TeacherAbsenceSaved{Absence: scAbsenceResult(), Uncovered: 5}, f.err
}

func (f *fakeScheduleSvc) UpdateTeacherAbsence(_ context.Context, id string, in client.TeacherAbsenceInput, by client.ScheduleActor) (client.TeacherAbsenceSaved, error) {
	f.absID, f.absIn, f.actor = id, in, by
	return client.TeacherAbsenceSaved{Absence: scAbsenceResult(), Uncovered: 1}, f.err
}

func (f *fakeScheduleSvc) CancelTeacherAbsence(_ context.Context, id string, by client.ScheduleActor, callerTeacherID string) (client.TeacherAbsence, error) {
	f.absID, f.actor, f.cancelCaller = id, by, callerTeacherID
	a := scAbsenceResult()
	a.CancelledAt, a.CancelledBy = scMon, by
	return a, f.err
}

func (f *fakeScheduleSvc) ListTeacherAbsences(_ context.Context, q client.TeacherAbsenceQuery) ([]client.TeacherAbsence, error) {
	f.absQuery = q
	return []client.TeacherAbsence{scAbsenceResult()}, f.err
}

func (f *fakeScheduleSvc) ListUncoveredClasses(_ context.Context, _ string, from, to time.Time) ([]client.UncoveredClass, error) {
	f.uncFrom, f.uncTo = from, to
	a := scAbsenceResult()
	return []client.UncoveredClass{{Date: scMon, Class: client.ScheduledClass{Block: scBlockResult(), TeacherID: scTeacher, StartsAt: 480, EndsAt: 530, Status: "uncovered", Absence: &a}}}, f.err
}

// asCaller is a request from a principal holding perms.
func asCaller(method, target, body string, userID string, perms ...string) *http.Request {
	r := scheduleReq(method, target, body)
	return r.WithContext(middleware.WithPrincipal(r.Context(), middleware.Principal{UserID: userID, Email: "x@x.mx", Permissions: perms}))
}

func TestCreateAbsenceParsesTheBodyAndAnswersTheUncoveredCount(t *testing.T) {
	h, f := newScheduleH()
	rec := serve(h.CreateAbsence, asCaller(http.MethodPost, "/api/v1/schedule/absences",
		`{"cycleId":"`+scCycle+`","teacherId":"`+scTeacher+`","kind":"personal","startsOn":"2026-10-05","endsOn":"2026-10-07","allDay":false,"startsAt":"08:00","endsAt":"10:00","note":"Cita médica"}`,
		"u-dir", "schedule:read", "schedule:write"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, client.TeacherAbsenceInput{CycleID: scCycle, TeacherID: scTeacher, Kind: "personal", StartsOn: scMon, EndsOn: scMon.AddDate(0, 0, 2),
		StartMinute: 480, EndMinute: 600, Note: "Cita médica"}, f.absIn)
	assert.Equal(t, "Directora Ruiz", f.actor.Name)
	body := decode(t, rec)
	assert.Equal(t, float64(5), body["uncoveredCount"])
	a := body["absence"].(map[string]any)
	assert.Equal(t, "08:00", a["startsAt"])
	assert.Equal(t, "sick", a["kind"], "the writer reads the kind")
	assert.Equal(t, "gripe", a["note"])

	full := `{"teacherId":"` + scTeacher + `","kind":"vacation","startsOn":"2026-10-05","endsOn":"2026-10-09","allDay":true}`
	rec = serve(h.CreateAbsence, asCaller(http.MethodPost, "/api/v1/schedule/absences", full, "u-dir", "schedule:write"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.True(t, f.absIn.AllDay)
	assert.Zero(t, f.absIn.StartMinute)
	assert.Equal(t, "", f.absIn.CycleID, "omitted: config picks the cycle of the dates")
}

func TestCreateAbsenceRefusesMalformedBodies(t *testing.T) {
	h, _ := newScheduleH()
	long := make([]byte, 501)
	for i := range long {
		long[i] = 'a'
	}
	for _, body := range []string{
		`{"teacherId":"bad","kind":"sick","startsOn":"2026-10-05","endsOn":"2026-10-05","allDay":true}`,
		`{"teacherId":"` + scTeacher + `","kind":"holiday","startsOn":"2026-10-05","endsOn":"2026-10-05","allDay":true}`,
		`{"teacherId":"` + scTeacher + `","kind":"sick","startsOn":"05/10/2026","endsOn":"2026-10-05","allDay":true}`,
		`{"teacherId":"` + scTeacher + `","kind":"sick","startsOn":"2026-10-05","endsOn":"2026-10-05","allDay":false}`,
		`{"teacherId":"` + scTeacher + `","kind":"sick","startsOn":"2026-10-05","endsOn":"2026-10-05","allDay":true,"note":"` + string(long) + `"}`,
		`{"cycleId":"x","teacherId":"` + scTeacher + `","kind":"sick","startsOn":"2026-10-05","endsOn":"2026-10-05","allDay":true}`,
	} {
		rec := serve(h.CreateAbsence, asCaller(http.MethodPost, "/api/v1/schedule/absences", body, "u-dir", "schedule:write"))
		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
	}
}

func TestATeacherCannotRegisterTheirOwnAbsence(t *testing.T) {
	h, f := newScheduleH()
	f.teachers = []client.Teacher{{ID: scTeacher, UserID: "u-ana"}}
	body := `{"teacherId":"` + scTeacher + `","kind":"sick","startsOn":"2026-10-05","endsOn":"2026-10-05","allDay":true}`
	rec := serve(h.CreateAbsence, asCaller(http.MethodPost, "/api/v1/schedule/absences", body, "u-ana", "schedule:write"))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "SCHEDULE_ABSENCE_OWN", rec.Header().Get("X-Error-Code"))
	assert.Equal(t, client.TeacherAbsenceInput{}, f.absIn, "config is never asked")

	r := asCaller(http.MethodPut, "/api/v1/schedule/absences/"+scAbsence, body, "u-ana", "schedule:write")
	r.SetPathValue("id", scAbsence)
	assert.Equal(t, http.StatusForbidden, serve(h.UpdateAbsence, r).Code)
}

func TestUpdateAndCancelAbsence(t *testing.T) {
	h, f := newScheduleH()
	r := asCaller(http.MethodPut, "/api/v1/schedule/absences/"+scAbsence,
		`{"teacherId":"`+scTeacher+`","kind":"other","startsOn":"2026-10-05","endsOn":"2026-10-05","allDay":true}`, "u-dir", "schedule:write")
	r.SetPathValue("id", scAbsence)
	rec := serve(h.UpdateAbsence, r)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, scAbsence, f.absID)
	assert.Equal(t, "other", f.absIn.Kind)
	assert.Equal(t, float64(1), decode(t, rec)["uncoveredCount"])

	r = asCaller(http.MethodPost, "/api/v1/schedule/absences/"+scAbsence+"/cancel", "", "u-dir", "schedule:write")
	r.SetPathValue("id", scAbsence)
	rec = serve(h.CancelAbsence, r)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	a := decode(t, rec)["absence"].(map[string]any)
	assert.Equal(t, true, a["cancelled"])
	assert.Equal(t, "Directora Ruiz", a["cancelledByName"])

	assert.Equal(t, "00000000-0000-0000-0000-000000000000", f.cancelCaller, "no teacher record: config is told none")

	f.teachers = []client.Teacher{{ID: scTeacher, UserID: "u-ana"}}
	r = asCaller(http.MethodPost, "/api/v1/schedule/absences/"+scAbsence+"/cancel", "", "u-ana", "schedule:write")
	r.SetPathValue("id", scAbsence)
	require.Equal(t, http.StatusOK, serve(h.CancelAbsence, r).Code)
	assert.Equal(t, scTeacher, f.cancelCaller, "config refuses the caller's own absence")

	r = asCaller(http.MethodPost, "/api/v1/schedule/absences/x/cancel", "", "u-dir", "schedule:write")
	r.SetPathValue("id", "x")
	assert.Equal(t, http.StatusBadRequest, serve(h.CancelAbsence, r).Code)
}

// PRIVACY (owner answer, plan §9): the kind and the note reach only
// schedule:write holders — enforced here, at the edge, not in the screen.
func TestAbsencePrivacyIsShapedAtTheEdge(t *testing.T) {
	private := []string{"kind", "note", "createdByName", "createdAt"}
	list := "/api/v1/schedule/absences?cycleId=" + scCycle + "&from=2026-10-05&to=2026-10-09"
	unc := "/api/v1/schedule/uncovered?cycleId=" + scCycle + "&from=2026-10-05&to=2026-10-09"
	day := "/api/v1/schedule/day?cycleId=" + scCycle + "&date=2026-10-05"
	tests := []struct {
		name  string
		perms []string
		shown bool
	}{
		{"a writer reads everything", []string{"schedule:read", "schedule:write"}, true},
		{"a reader reads only that the teacher is away", []string{"schedule:read"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f := newScheduleH()
			f.absentClass = true

			rec := serve(h.Absences, asCaller(http.MethodGet, list, "", "u", tt.perms...))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assertShaped(t, decode(t, rec)["absences"].([]any)[0].(map[string]any), private, tt.shown)

			rec = serve(h.Uncovered, asCaller(http.MethodGet, unc, "", "u", tt.perms...))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			cls := decode(t, rec)["classes"].([]any)[0].(map[string]any)["class"].(map[string]any)
			assert.Equal(t, "uncovered", cls["status"])
			assertShaped(t, cls["absence"].(map[string]any), private, tt.shown)

			rec = serve(h.Day, asCaller(http.MethodGet, day, "", "u", tt.perms...))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			classes := decode(t, rec)["classes"].([]any)
			assert.Nil(t, classes[0].(map[string]any)["absence"])
			assertShaped(t, classes[1].(map[string]any)["absence"].(map[string]any), private, tt.shown)
		})
	}
}

func TestOwnDayNeverShowsTheKindOrTheNote(t *testing.T) {
	h, f := newScheduleH()
	f.absentClass = true
	f.teachers = []client.Teacher{{ID: scTeacher, UserID: "u-ana"}}
	// Even a teacher who also holds schedule:write reads only "Ausente" in "Mi horario".
	rec := serve(h.OwnDay, asCaller(http.MethodGet, "/api/v1/teaching/schedule/day?date=2026-10-05", "", "u-ana", "teaching:read", "schedule:write"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	for _, c := range decode(t, rec)["classes"].([]any) {
		if a, ok := c.(map[string]any)["absence"].(map[string]any); ok {
			assertShaped(t, a, []string{"kind", "note"}, false)
		}
	}
}

func assertShaped(t *testing.T, absence map[string]any, private []string, shown bool) {
	t.Helper()
	for _, k := range private {
		_, ok := absence[k]
		assert.Equal(t, shown, ok, k)
	}
	assert.Equal(t, "08:00", absence["startsAt"], "the hours are never private")
}

func TestUncoveredAndAbsencesValidateTheRange(t *testing.T) {
	h, f := newScheduleH()
	rec := serve(h.Uncovered, asCaller(http.MethodGet, "/api/v1/schedule/uncovered?cycleId="+scCycle+"&from=2026-10-05&to=2026-10-09", "", "u", "schedule:read"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, scMon, f.uncFrom)
	assert.Equal(t, scMon.AddDate(0, 0, 4), f.uncTo)
	assert.Equal(t, "2026-10-05", decode(t, rec)["classes"].([]any)[0].(map[string]any)["date"])

	for _, target := range []string{
		"/api/v1/schedule/uncovered?cycleId=" + scCycle + "&from=2026-10-05",
		"/api/v1/schedule/uncovered?cycleId=bad&from=2026-10-05&to=2026-10-09",
		"/api/v1/schedule/absences?cycleId=" + scCycle + "&from=2026-10-05&to=2028-10-05",
		"/api/v1/schedule/absences?cycleId=" + scCycle + "&from=2026-10-05&to=2026-10-09&teacherId=bad",
	} {
		hh := h.Uncovered
		if len(target) > 25 && target[:25] == "/api/v1/schedule/absences" {
			hh = h.Absences
		}
		assert.Equal(t, http.StatusBadRequest, serve(hh, asCaller(http.MethodGet, target, "", "u", "schedule:read")).Code, target)
	}

	rec = serve(h.Absences, asCaller(http.MethodGet, "/api/v1/schedule/absences?cycleId="+scCycle+"&from=2026-10-05&to=2026-10-09&teacherId="+scTeacher+"&includeCancelled=true", "", "u", "schedule:read"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, client.TeacherAbsenceQuery{CycleID: scCycle, TeacherID: scTeacher, From: scMon, To: scMon.AddDate(0, 0, 4), IncludeCancelled: true}, f.absQuery)
}

func TestNonWorkingDayRidesOnTheDay(t *testing.T) {
	h, f := newScheduleH()
	f.nonWorking = "Independencia"
	rec := serve(h.Day, scheduleReq(http.MethodGet, "/api/v1/schedule/day?cycleId="+scCycle+"&date=2026-10-05", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Independencia", decode(t, rec)["nonWorking"])
}

// The T19 routes are in the contract and the reader's shape is exactly the schema's required fields.
func TestAbsenceContract(t *testing.T) {
	doc := loadOpenAPI(t)
	for _, op := range []struct{ path, method string }{
		{"/api/v1/schedule/absences", "get"},
		{"/api/v1/schedule/absences", "post"},
		{"/api/v1/schedule/absences/{id}", "put"},
		{"/api/v1/schedule/absences/{id}/cancel", "post"},
		{"/api/v1/schedule/uncovered", "get"},
	} {
		_, ok := doc.Paths[op.path][op.method]
		require.True(t, ok, "%s %s is documented", op.method, op.path)
	}
	h, _ := newScheduleH()
	rec := serve(h.Absences, asCaller(http.MethodGet, "/api/v1/schedule/absences?cycleId="+scCycle+"&from=2026-10-05&to=2026-10-09", "", "u", "schedule:read"))
	got := decode(t, rec)["absences"].([]any)[0].(map[string]any)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{"allDay", "cancelled", "cycleId", "endsAt", "endsOn", "id", "startsAt", "startsOn", "teacherId"}, keys)
}

func TestCreateOverrideForwardsWhatItReplaces(t *testing.T) {
	h, f := newScheduleH()
	rec := serve(h.CreateOverride, scheduleReq(http.MethodPost, "/api/v1/schedule/overrides",
		`{"blockId":"`+scBlock+`","date":"2026-10-05","kind":"substitute","teacherId":"`+scTeacher+`","reason":"x","replaceOverrideId":"`+scOvr+`"}`))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, scOvr, f.ovrIn.ReplaceOverrideID)
	rec = serve(h.CreateOverride, scheduleReq(http.MethodPost, "/api/v1/schedule/overrides",
		`{"blockId":"`+scBlock+`","date":"2026-10-05","kind":"cancel","reason":"x","replaceOverrideId":"bad"}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
