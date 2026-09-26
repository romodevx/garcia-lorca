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
	scanCodeForeign  = "GLCARD:CCCCCCCCCCCCCCCCCCCCCC" // stu-9, another group
	scanCodeForeignR = "GLCARD:DDDDDDDDDDDDDDDDDDDDDD" // stu-9, another group, suspended
	scanCodeUnknown  = "GLCARD:ZZZZZZZZZZZZZZZZZZZZZZ" // no card ever carried it
	scanCodeGarbage  = "hola"                          // not a card code
)

type scannedCard struct {
	student client.StudentResult
	valid   bool
}

// fakeScanCards doubles the students' card verification the way the service
// answers: InvalidArgument for something that is not a card code, NotFound for
// a code no card carried.
type fakeScanCards struct {
	cards map[string]scannedCard
	err   error
	calls int
}

func (f *fakeScanCards) VerifyCard(_ context.Context, code string) (client.StudentResult, bool, error) {
	f.calls++
	if f.err != nil {
		return client.StudentResult{}, false, f.err
	}
	if code == scanCodeGarbage || code == "" {
		return client.StudentResult{}, false, connect.NewError(connect.CodeInvalidArgument, errString("not a card code"))
	}
	c, ok := f.cards[code]
	if !ok {
		return client.StudentResult{}, false, connect.NewError(connect.CodeNotFound, errString("card not found"))
	}
	return c.student, c.valid, nil
}

func scanCards() *fakeScanCards {
	ana := client.StudentResult{ID: "stu-1", FirstName: "Ana", LastName: "García"}
	luis := client.StudentResult{ID: "stu-2", FirstName: "Luis", LastName: "Pérez"}
	zoe := client.StudentResult{ID: "stu-9", FirstName: "Zoe", LastName: "Villalobos"}
	return &fakeScanCards{cards: map[string]scannedCard{
		scanCodeAna:      {student: ana, valid: true},
		scanCodeRevoked:  {student: luis, valid: false},
		scanCodeForeign:  {student: zoe, valid: true},
		scanCodeForeignR: {student: zoe, valid: false},
	}}
}

// scanRoster is the group's active enrollments: stu-1 and stu-2.
func scanRoster() *fakeAttendance {
	return &fakeAttendance{fakeTeachingAttendance: fakeTeachingAttendance{roster: []client.AttendanceRosterRow{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Status: "present"},
		{EnrollmentID: "enr-2", StudentID: "stu-2", Status: "absent"},
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

func scanBody(groupID, subjectID, code string) string {
	b, _ := json.Marshal(map[string]string{"groupId": groupID, "subjectId": subjectID, "code": code})
	return string(b)
}

type scanCase struct {
	name        string
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
)

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
			if tc.wantCode == http.StatusBadRequest {
				att.roster = nil // no list exists yet, so the mode refusal holds
			}
			rec := httptest.NewRecorder()
			newScanTeacher(att, cards).Scan(rec, teacherPOST("/api/v1/teaching/attendance/scan", "usr-t", scanBody(tc.groupID, tc.subjectID, tc.code)))
			checkScan(t, tc, rec, att, cards)
			if tc.wantCode == http.StatusOK {
				assert.Equal(t, "2026-09-15", att.gotRosterDate, "the group's enrollments are read on the school day")
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
		{name: "a card of another group", groupID: "grp-prim", code: scanCodeForeign, wantCode: 422, wantResult: "foreign", wantBody: wantForeign},
		{name: "an unknown group", groupID: "ghost", code: scanCodeAna, wantCode: 404, wantNoCards: true},
		{name: "a subject not taught in the group", groupID: "grp-sec", subjectID: "sub-esp", code: scanCodeAna, wantCode: 400, wantBody: "La asignatura no se imparte en este grupo\n", wantNoCards: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			att, cards := scanRoster(), scanCards()
			rec := httptest.NewRecorder()
			newScanDirector(att, cards).Scan(rec, directorReq(http.MethodPost, "/api/v1/attendance/scan", scanBody(tc.groupID, tc.subjectID, tc.code)))
			checkScan(t, tc, rec, att, cards)
			if tc.wantCode == http.StatusOK {
				assert.Equal(t, "2026-09-15", att.gotRosterDate)
			}
		})
	}
}
