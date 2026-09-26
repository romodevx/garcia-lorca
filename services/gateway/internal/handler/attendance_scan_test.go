package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// T4 — attendance by QR. The scan RESOLVES a card to a row of the list; it
// never writes attendance (the list is saved by the T3 routes).

const (
	scanCodeAna      = "GLCARD:AAAAAAAAAAAAAAAAAAAAAA" // stu-1, in the list
	scanCodeRevoked  = "GLCARD:BBBBBBBBBBBBBBBBBBBBBB" // stu-2, in the list, suspended
	scanCodePending  = "GLCARD:EEEEEEEEEEEEEEEEEEEEEE" // stu-3, in the list, admission pending, card active
	scanCodeForeign  = "GLCARD:CCCCCCCCCCCCCCCCCCCCCC" // stu-9, another group
	scanCodeForeignR = "GLCARD:DDDDDDDDDDDDDDDDDDDDDD" // stu-9, another group, suspended
	scanCodeUnknown  = "GLCARD:ZZZZZZZZZZZZZZZZZZZZZZ" // no card ever carried it
	scanCodeGarbage  = "hola"                          // not a card code
)

// fakeScanCards doubles the students' card read the way the service answers:
// InvalidArgument for something that is not a card code, NotFound for a code
// no card carried. It carries only the card's revocation — the pupil's
// admission status never reaches the scan (QA ISSUE-004).
type fakeScanCards struct {
	cards map[string]client.ScannedCard
	err   error
	calls int
}

func (f *fakeScanCards) ScanCard(_ context.Context, code string) (client.ScannedCard, error) {
	f.calls++
	if f.err != nil {
		return client.ScannedCard{}, f.err
	}
	if code == scanCodeGarbage || code == "" {
		return client.ScannedCard{}, connect.NewError(connect.CodeInvalidArgument, errString("not a card code"))
	}
	c, ok := f.cards[code]
	if !ok {
		return client.ScannedCard{}, connect.NewError(connect.CodeNotFound, errString("card not found"))
	}
	return c, nil
}

func scanCards() *fakeScanCards {
	ana := client.StudentResult{ID: "stu-1", FirstName: "Ana", LastName: "García"}
	luis := client.StudentResult{ID: "stu-2", FirstName: "Luis", LastName: "Pérez"}
	eva := client.StudentResult{ID: "stu-3", FirstName: "Eva", LastName: "Ruiz", AdmissionStatus: "pending"}
	zoe := client.StudentResult{ID: "stu-9", FirstName: "Zoe", LastName: "Villalobos"}
	return &fakeScanCards{cards: map[string]client.ScannedCard{
		scanCodeAna:      {Student: ana},
		scanCodeRevoked:  {Student: luis, Revoked: true},
		scanCodePending:  {Student: eva},
		scanCodeForeign:  {Student: zoe},
		scanCodeForeignR: {Student: zoe, Revoked: true},
	}}
}

// scanRoster is the group's active enrollments: stu-1, stu-2 and stu-3.
func scanRoster() *fakeAttendance {
	return &fakeAttendance{fakeTeachingAttendance: fakeTeachingAttendance{roster: []client.AttendanceRosterRow{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Status: "present"},
		{EnrollmentID: "enr-2", StudentID: "stu-2", Status: "absent"},
		{EnrollmentID: "enr-3", StudentID: "stu-3", Status: "present"},
	}}}
}

func newScanTeacher(att *fakeAttendance, cards *fakeScanCards) *handler.TeachingAttendanceHandler {
	scope := handler.NewTeachingHandler(listTeacher(), &fakeTeachingGrades{}, &fakeTeachingStudents{})
	return handler.NewTeachingAttendanceHandler(scope, att, teacherUsers,
		handler.WithAttendanceClock(fixedClock), handler.WithScanCards(cards))
}

func newScanDirector(att *fakeAttendance, cards *fakeScanCards) *handler.AttendanceHandler {
	return handler.NewAttendanceHandler(att, listTeacher(), &fakeTeachingStudents{}, directorUsers,
		handler.WithDirectorAttendanceClock(fixedClock), handler.WithDirectorScanCards(cards))
}

// scanToday is fixedClock's school day.
const scanToday = "2026-09-15"

func scanBody(groupID, subjectID, code string) string {
	return scanBodyOn(scanToday, groupID, subjectID, code)
}

// scanBodyOn is a scan against the list of date — the list on screen.
func scanBodyOn(date, groupID, subjectID, code string) string {
	b, _ := json.Marshal(map[string]string{"date": date, "groupId": groupID, "subjectId": subjectID, "code": code})
	return string(b)
}

type scanCase struct {
	name        string
	date        string // "" = scanToday
	groupID     string
	subjectID   string
	code        string
	wantCode    int
	wantResult  string // X-Scan-Result; "" = none
	wantBody    string // exact text/plain refusal, or the JSON of a 200
	wantNoCards bool   // the card must never be looked up (no probing)
}

// The refusals the web tells apart by X-Scan-Result.
const (
	wantInvalid   = "Código no válido\n"
	wantSuspended = "Credencial suspendida\n"
	wantForeign   = "Este alumno no pertenece a este grupo\n"
	wantAnaJSON   = `{"enrollmentId":"enr-1","studentId":"stu-1","name":"Ana García"}`
	wantEvaJSON   = `{"enrollmentId":"enr-3","studentId":"stu-3","name":"Eva Ruiz"}`
	wantBadDate   = "La fecha debe tener el formato AAAA-MM-DD\n"
	wantFuture    = "No puedes registrar una fecha futura\n"
	wantWindow    = "Solo puedes corregir hasta 7 días atrás\n"
)

func (tc scanCase) body() string {
	date := tc.date
	if date == "" {
		date = scanToday
	}
	return scanBodyOn(date, tc.groupID, tc.subjectID, tc.code)
}

func checkScan(t *testing.T, tc scanCase, rec *httptest.ResponseRecorder, att *fakeAttendance, cards *fakeScanCards) {
	t.Helper()
	require.Equal(t, tc.wantCode, rec.Code, rec.Body.String())
	assert.Equal(t, tc.wantResult, rec.Header().Get("X-Scan-Result"))
	if tc.wantCode == http.StatusOK {
		assert.JSONEq(t, tc.wantBody, rec.Body.String())
	} else if tc.wantBody != "" {
		assert.Equal(t, tc.wantBody, rec.Body.String())
	}
	if tc.wantResult != "" {
		assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		assert.Equal(t, 1, att.rosterCalls, "one roster read per scan: the gate's read is reused")
	}
	if tc.wantNoCards {
		assert.Zero(t, cards.calls, "a list the caller may not scan must never look the card up")
		assert.Zero(t, att.rosterCalls)
	}
	if tc.wantResult == "foreign" {
		for _, leak := range []string{"Zoe", "Villalobos", "stu-9"} {
			assert.NotContains(t, rec.Body.String(), leak, "a foreign pupil's data must never leave")
		}
	}
	assert.Zero(t, att.saveCalls, "a scan resolves; it never writes attendance")
}

func TestTeachingAttendanceScan(t *testing.T) {
	tests := []scanCase{
		{name: "a pupil of the caller's daily group", groupID: "grp-prim", code: scanCodeAna, wantCode: 200, wantResult: "ok", wantBody: wantAnaJSON},
		{name: "a pupil of the caller's per-class list", groupID: "grp-sec", subjectID: "sub-mat1", code: scanCodeAna, wantCode: 200, wantResult: "ok", wantBody: wantAnaJSON},
		{name: "an unknown code", groupID: "grp-prim", code: scanCodeUnknown, wantCode: 422, wantResult: "invalid", wantBody: wantInvalid},
		{name: "not a card code", groupID: "grp-prim", code: scanCodeGarbage, wantCode: 422, wantResult: "invalid", wantBody: wantInvalid},
		{name: "an empty code", groupID: "grp-prim", code: "", wantCode: 422, wantResult: "invalid", wantBody: wantInvalid},
		{name: "a suspended card of the group", groupID: "grp-prim", code: scanCodeRevoked, wantCode: 409, wantResult: "suspended", wantBody: wantSuspended},
		{name: "a pupil of the group whose admission is pending counts", groupID: "grp-prim", code: scanCodePending, wantCode: 200, wantResult: "ok", wantBody: wantEvaJSON},
		{name: "a day inside the window is scanned on that day's list", date: "2026-09-10", groupID: "grp-prim", code: scanCodeAna, wantCode: 200, wantResult: "ok", wantBody: wantAnaJSON},
		{name: "the window's last day", date: "2026-09-08", groupID: "grp-prim", code: scanCodeAna, wantCode: 200, wantResult: "ok", wantBody: wantAnaJSON},
		{name: "a future day", date: "2026-09-16", groupID: "grp-prim", code: scanCodeAna, wantCode: 400, wantBody: wantFuture, wantNoCards: true},
		{name: "a day beyond the window", date: "2026-09-07", groupID: "grp-prim", code: scanCodeAna, wantCode: 400, wantBody: wantWindow, wantNoCards: true},
		{name: "a malformed date", date: "15/09/2026", groupID: "grp-prim", code: scanCodeAna, wantCode: 400, wantBody: wantBadDate, wantNoCards: true},
		{name: "a valid card of another group", groupID: "grp-prim", code: scanCodeForeign, wantCode: 422, wantResult: "foreign", wantBody: wantForeign},
		{name: "a suspended card of another group reads as foreign", groupID: "grp-prim", code: scanCodeForeignR, wantCode: 422, wantResult: "foreign", wantBody: wantForeign},
		{name: "another teacher's class in the caller's group", groupID: "grp-sec", subjectID: "sub-hist", code: scanCodeAna, wantCode: 404, wantNoCards: true},
		{name: "a group the caller does not teach", groupID: "grp-other", code: scanCodeAna, wantCode: 404, wantNoCards: true},
		{name: "no group", groupID: "", code: scanCodeAna, wantCode: 404, wantNoCards: true},
		{name: "a per-class list without its subject", groupID: "grp-sec", code: scanCodeAna, wantCode: 400, wantBody: "Esta lista es por clase: elige la asignatura\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			att, cards := scanRoster(), scanCards()
			if tc.wantCode == http.StatusBadRequest && tc.date == "" {
				att.roster = nil // no list exists yet, so the mode refusal holds
			}
			rec := httptest.NewRecorder()
			newScanTeacher(att, cards).Scan(rec, teacherPOST("/api/v1/teaching/attendance/scan", "usr-t", tc.body()))
			checkScan(t, tc, rec, att, cards)
			if tc.wantCode == http.StatusOK {
				assert.Equal(t, orToday(tc.date), att.gotRosterDate, "the enrollments are read on the list's own day")
				assert.Equal(t, tc.groupID, att.gotRosterGroupID)
			}
		})
	}
}

func TestTeachingAttendanceScanEdgeCases(t *testing.T) {
	t.Run("without a principal is 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teaching/attendance/scan", bytes.NewBufferString(scanBody("grp-prim", "", scanCodeAna)))
		newScanTeacher(scanRoster(), scanCards()).Scan(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("a malformed body is 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newScanTeacher(scanRoster(), scanCards()).Scan(rec, teacherPOST("/api/v1/teaching/attendance/scan", "usr-t", "{"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("a students outage is an error, never a verdict", func(t *testing.T) {
		cards := scanCards()
		cards.err = connect.NewError(connect.CodeUnavailable, errString("students down"))
		rec := httptest.NewRecorder()
		newScanTeacher(scanRoster(), cards).Scan(rec, teacherPOST("/api/v1/teaching/attendance/scan", "usr-t", scanBody("grp-prim", "", scanCodeAna)))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Empty(t, rec.Header().Get("X-Scan-Result"))
	})
	t.Run("an attendance outage is an error, never a verdict", func(t *testing.T) {
		att := scanRoster()
		att.rosterErr = connect.NewError(connect.CodeUnavailable, errString("attendance down"))
		rec := httptest.NewRecorder()
		newScanTeacher(att, scanCards()).Scan(rec, teacherPOST("/api/v1/teaching/attendance/scan", "usr-t", scanBody("grp-prim", "", scanCodeAna)))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Empty(t, rec.Header().Get("X-Scan-Result"))
	})
	t.Run("a handler without cards wired answers 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newTeachingAttendance(listTeacher(), scanRoster(), &fakeTeachingStudents{}).
			Scan(rec, teacherPOST("/api/v1/teaching/attendance/scan", "usr-t", scanBody("grp-prim", "", scanCodeAna)))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// The raw code is a credential: no log line may keep it, refused or not.
func TestAttendanceScanNeverLogsTheCode(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := newScanTeacher(scanRoster(), scanCards())
	for _, code := range []string{scanCodeAna, scanCodeUnknown, scanCodeRevoked, scanCodeForeign} {
		h.Scan(httptest.NewRecorder(), teacherPOST("/api/v1/teaching/attendance/scan", "usr-t", scanBody("grp-prim", "", code)))
		assert.NotContains(t, logs.String(), code[len("GLCARD:"):])
	}
	assert.NotEmpty(t, logs.String(), "refusals are traceable (by a hash prefix)")
}

func TestDirectorAttendanceScan(t *testing.T) {
	tests := []scanCase{
		{name: "a pupil of a daily group", groupID: "grp-prim", code: scanCodeAna, wantCode: 200, wantResult: "ok", wantBody: wantAnaJSON},
		{name: "any subject taught in a per-class group", groupID: "grp-sec", subjectID: "sub-hist", code: scanCodeAna, wantCode: 200, wantResult: "ok", wantBody: wantAnaJSON},
		{name: "a group nobody teaches in is still the director's", groupID: "grp-other", code: scanCodeAna, wantCode: 200, wantResult: "ok", wantBody: wantAnaJSON},
		{name: "an unknown code", groupID: "grp-prim", code: scanCodeUnknown, wantCode: 422, wantResult: "invalid", wantBody: wantInvalid},
		{name: "a suspended card", groupID: "grp-prim", code: scanCodeRevoked, wantCode: 409, wantResult: "suspended", wantBody: wantSuspended},
		{name: "a pupil whose admission is pending counts", groupID: "grp-prim", code: scanCodePending, wantCode: 200, wantResult: "ok", wantBody: wantEvaJSON},
		{name: "the director has no window: a month back", date: "2026-08-15", groupID: "grp-prim", code: scanCodeAna, wantCode: 200, wantResult: "ok", wantBody: wantAnaJSON},
		{name: "a malformed date", date: "2026-9-1", groupID: "grp-prim", code: scanCodeAna, wantCode: 400, wantBody: wantBadDate, wantNoCards: true},
		{name: "no date", date: "none", groupID: "grp-prim", code: scanCodeAna, wantCode: 400, wantBody: wantBadDate, wantNoCards: true},
		{name: "a card of another group", groupID: "grp-prim", code: scanCodeForeign, wantCode: 422, wantResult: "foreign", wantBody: wantForeign},
		{name: "an unknown group", groupID: "ghost", code: scanCodeAna, wantCode: 404, wantNoCards: true},
		{name: "a subject not taught in the group", groupID: "grp-sec", subjectID: "sub-esp", code: scanCodeAna, wantCode: 400, wantBody: "La asignatura no se imparte en este grupo\n", wantNoCards: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			att, cards := scanRoster(), scanCards()
			rec := httptest.NewRecorder()
			body := tc.body()
			if tc.date == "none" {
				b, _ := json.Marshal(map[string]string{"groupId": tc.groupID, "code": tc.code})
				body = string(b)
			}
			newScanDirector(att, cards).Scan(rec, directorReq(http.MethodPost, "/api/v1/attendance/scan", body))
			checkScan(t, tc, rec, att, cards)
			if tc.wantCode == http.StatusOK {
				assert.Equal(t, orToday(tc.date), att.gotRosterDate)
			}
		})
	}
}

func orToday(date string) string {
	if date == "" {
		return scanToday
	}
	return date
}

// A list taken before its level changed mode is still scannable, and the
// roster the gate read to find it is the one the scan resolves against.
func TestAttendanceScanReusesTheGateRoster(t *testing.T) {
	t.Run("teacher", func(t *testing.T) {
		att, cards := scanRoster(), scanCards() // stu-2 absent: the list exists
		rec := httptest.NewRecorder()
		newScanTeacher(att, cards).Scan(rec, teacherPOST("/api/v1/teaching/attendance/scan", "usr-t", scanBody("grp-sec", "", scanCodeAna)))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, wantAnaJSON, rec.Body.String())
		assert.Equal(t, 1, att.rosterCalls, "the gate's roster is reused, never read twice")
	})
	t.Run("director", func(t *testing.T) {
		att, cards := scanRoster(), scanCards()
		rec := httptest.NewRecorder()
		newScanDirector(att, cards).Scan(rec, directorReq(http.MethodPost, "/api/v1/attendance/scan", scanBody("grp-sec", "", scanCodeAna)))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, 1, att.rosterCalls, "the gate's roster is reused, never read twice")
	})
}
