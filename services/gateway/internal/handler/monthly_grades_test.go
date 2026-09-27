package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// Canonical uuids: the edge parses and lowercases every id it is handed.
const (
	mAsg      = "a0000000-0000-0000-0000-000000000001"
	mAsgOther = "a0000000-0000-0000-0000-000000000009"
	mEnr      = "e0000000-0000-0000-0000-000000000001"
	mUser     = "u0000000-0000-0000-0000-000000000001"
)

var mUpdated = time.Date(2026, 9, 3, 15, 4, 5, 0, time.UTC)

type fakeMonthlyGrades struct {
	sheet      client.MonthlySheet
	sheetErr   error
	gotSheetID string
	gotMonth   string
	gotSheetQ  client.MonthlySheetQuery
	sheetCalls int

	upsertIn    client.MonthlyGradeInput
	upsertErr   error
	upsertCalls int

	clearAsg, clearMonth, clearEnr string
	clearCell                      client.MonthlyCell
	clearErr                       error
	clearCalls                     int
}

func (f *fakeMonthlyGrades) GetMonthlySheet(_ context.Context, q client.MonthlySheetQuery) (client.MonthlySheet, error) {
	f.sheetCalls++
	f.gotSheetID, f.gotMonth, f.gotSheetQ = q.AssignmentID, q.Month, q
	return f.sheet, f.sheetErr
}

func (f *fakeMonthlyGrades) UpsertMonthlyGrade(_ context.Context, in client.MonthlyGradeInput) (client.MonthlyGrade, error) {
	f.upsertCalls++
	f.upsertIn = in
	if f.upsertErr != nil {
		return client.MonthlyGrade{}, f.upsertErr
	}
	return client.MonthlyGrade{EnrollmentID: in.EnrollmentID, SubjectID: "sub-1", Month: in.Month, Value: "9.5", UpdatedAt: mUpdated}, nil
}

func (f *fakeMonthlyGrades) ClearMonthlyGrade(_ context.Context, cell client.MonthlyCell) error {
	f.clearCalls++
	f.clearAsg, f.clearMonth, f.clearEnr, f.clearCell = cell.AssignmentID, cell.Month, cell.EnrollmentID, cell
	return f.clearErr
}

// fakeSheetStudents names students by id and records which ids it was asked
// for: a sheet must never read the whole school.
type fakeSheetStudents struct {
	students []client.StudentResult
	gotIDs   []string
	calls    int
}

func (f *fakeSheetStudents) StudentsByIDs(_ context.Context, ids []string) ([]client.StudentResult, error) {
	f.calls++
	f.gotIDs = append([]string(nil), ids...)
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []client.StudentResult
	for _, s := range f.students {
		if want[s.ID] {
			out = append(out, s)
		}
	}
	return out, nil
}

func sheetFixture() client.MonthlySheet {
	return client.MonthlySheet{
		AssignmentID: mAsg, Month: "2026-09", Months: []string{"2026-08", "2026-09"},
		Scale:          client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, Editable: true,
		Rows: []client.MonthlySheetRow{
			{EnrollmentID: "enr-z", StudentID: "stu-z", Editable: true},
			{EnrollmentID: "enr-a", StudentID: "stu-a", Value: "9.5", UpdatedAt: mUpdated, Editable: true},
			{EnrollmentID: "enr-n", StudentID: "stu-n", ReadOnlyReason: "Ese mes corresponde al grupo anterior del alumno"},
		},
	}
}

func sheetStudents() *fakeSheetStudents {
	return &fakeSheetStudents{students: []client.StudentResult{
		{ID: "stu-other", FirstName: "Otro", LastName: "Grupo"},
		{ID: "stu-z", FirstName: "Zoe", LastName: "Zúñiga"},
		{ID: "stu-a", FirstName: "Ana", LastName: "Ávila"},
		{ID: "stu-n", FirstName: "Nico", LastName: "Núñez"},
	}}
}

// ownerGate / denyGate stand in for the teacher's ownership check.
func denyGate(w http.ResponseWriter, _ *http.Request, _ string) bool {
	http.Error(w, "not found", http.StatusNotFound)
	return false
}

func principalReq(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: mUser, TenantID: "t1"}))
}

// --- sheet -------------------------------------------------------------------

func TestMonthlySheetShapeAndSpanishOrder(t *testing.T) {
	grades := &fakeMonthlyGrades{sheet: sheetFixture()}
	clock := func() time.Time { return time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC) }
	h := handler.NewMonthlyGradesHandler(grades, sheetStudents(), handler.AnyTenantAssignment, handler.ScopeSchool, handler.WithMonthlyClock(clock))

	rec := httptest.NewRecorder()
	h.Sheet(rec, principalReq(http.MethodGet, "/api/v1/grades/sheet?assignmentId="+strings.ToUpper(mAsg), ""))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, mAsg, grades.gotSheetID, "the id is canonicalised before it goes downstream")
	assert.Equal(t, "", grades.gotMonth, "no month ⇒ the service picks the current one")
	assert.JSONEq(t, `{
		"assignmentId": "`+mAsg+`", "month": "2026-09", "months": ["2026-08","2026-09"],
		"scale": {"min":5,"max":10,"decimals":1,"passing":6},
		"gradingEnabled": true, "currentMonth": "2026-09", "editable": true, "closedMonths": [],
		"rows": [
			{"enrollmentId":"enr-a","studentId":"stu-a","name":"Ana Ávila","value":9.5,"updatedAt":"2026-09-03T15:04:05Z","rowEditable":true},
			{"enrollmentId":"enr-n","studentId":"stu-n","name":"Nico Núñez","value":null,"updatedAt":null,"rowEditable":false,
			 "rowReadOnlyReason":"Ese mes corresponde al grupo anterior del alumno"},
			{"enrollmentId":"enr-z","studentId":"stu-z","name":"Zoe Zúñiga","value":null,"updatedAt":null,"rowEditable":true}
		]}`, rec.Body.String(), "rows sorted by last name, Spanish collation (Á with A, Ñ after N)")
	assert.True(t, grades.gotSheetQ.SchoolWide, "the director's sheet is school-wide")
}

// A sheet names only its own pupils, in ONE read by id — never the whole school.
func TestMonthlySheetNamesOnlyItsOwnPupils(t *testing.T) {
	students := sheetStudents()
	h := handler.NewMonthlyGradesHandler(&fakeMonthlyGrades{sheet: sheetFixture()}, students, handler.AnyTenantAssignment, handler.ScopeSchool)
	rec := httptest.NewRecorder()
	h.Sheet(rec, principalReq(http.MethodGet, "/api/v1/grades/sheet?assignmentId="+mAsg, ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, students.calls)
	assert.ElementsMatch(t, []string{"stu-z", "stu-a", "stu-n"}, students.gotIDs)
	assert.NotContains(t, rec.Body.String(), "Otro Grupo")
}

// The teacher's routes are never school-wide: the previous-group rule applies.
func TestTeacherMonthlyRoutesAreNotSchoolWide(t *testing.T) {
	grades := &fakeMonthlyGrades{sheet: sheetFixture()}
	h := handler.NewMonthlyGradesHandler(grades, sheetStudents(), handler.AnyTenantAssignment, handler.ScopeTeacher)
	h.Sheet(httptest.NewRecorder(), principalReq(http.MethodGet, "/api/v1/teaching/grades/sheet?assignmentId="+mAsg, ""))
	h.Put(httptest.NewRecorder(), principalReq(http.MethodPut, "/api/v1/teaching/grades/monthly",
		`{"assignmentId":"`+mAsg+`","month":"2026-09","enrollmentId":"`+mEnr+`","value":9}`))
	h.Clear(httptest.NewRecorder(), principalReq(http.MethodDelete, "/api/v1/teaching/grades/monthly?assignmentId="+mAsg+"&month=2026-09&enrollmentId="+mEnr, ""))
	assert.False(t, grades.gotSheetQ.SchoolWide)
	assert.False(t, grades.upsertIn.SchoolWide)
	assert.False(t, grades.clearCell.SchoolWide)
	assert.Equal(t, 1, grades.upsertCalls)
	assert.Equal(t, 1, grades.clearCalls)
}

// Every monthly refusal names its machine code in X-Error-Code: the edge's own
// and the grades service's (forwarded from the error metadata).
func TestMonthlyRefusalsCarryErrorCodes(t *testing.T) {
	coded := func(code, msg string) error {
		e := connect.NewError(connect.CodeInvalidArgument, errString(msg))
		e.Meta().Set("X-Error-Code", code)
		return e
	}
	body := func(value string) string {
		return `{"assignmentId":"` + mAsg + `","month":"2026-09","enrollmentId":"` + mEnr + `","value":` + value + `}`
	}
	tests := []struct {
		name, body, wantCode string
		svcErr               error
	}{
		{"string value", body(`"9.5"`), "GRADE_NOT_NUMBER", nil},
		{"malformed enrollment", `{"assignmentId":"` + mAsg + `","month":"2026-09","enrollmentId":"x","value":9}`, "ENROLLMENT_NOT_IN_GROUP", nil},
		{"previous group", body(`9`), "MONTH_PREVIOUS_GROUP", coded("MONTH_PREVIOUS_GROUP", "Ese mes corresponde al grupo anterior del alumno")},
		{"out of range", body(`11`), "GRADE_OUT_OF_RANGE", coded("GRADE_OUT_OF_RANGE", "La calificación debe estar entre 5 y 10")},
		{"a code that is not a code is dropped", body(`9`), "", coded("bad code\r\nX-Evil: 1", "x")},
		{"uncoded refusal", body(`9`), "", connect.NewError(connect.CodeInvalidArgument, errString("x"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h := handler.NewMonthlyGradesHandler(&fakeMonthlyGrades{upsertErr: tt.svcErr}, sheetStudents(), handler.AnyTenantAssignment, handler.ScopeSchool)
			h.Put(rec, principalReq(http.MethodPut, "/api/v1/grades/monthly", tt.body))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, tt.wantCode, rec.Header().Get("X-Error-Code"))
		})
	}
}

// A future month is refused by the grades service on BOTH audiences' writes;
// the edge forwards it as 400 + MONTH_IN_FUTURE with the sentence.
func TestMonthlyFutureMonthIsRefusedOnEveryRoute(t *testing.T) {
	future := connect.NewError(connect.CodeInvalidArgument, errString("Aún no llega ese mes"))
	future.Meta().Set("X-Error-Code", "MONTH_IN_FUTURE")
	put := `{"assignmentId":"` + mAsg + `","month":"2026-11","enrollmentId":"` + mEnr + `","value":9}`
	del := "?assignmentId=" + mAsg + "&month=2026-11&enrollmentId=" + mEnr
	for _, scope := range []handler.CaptureScope{handler.ScopeTeacher, handler.ScopeSchool} {
		h := handler.NewMonthlyGradesHandler(&fakeMonthlyGrades{upsertErr: future, clearErr: future}, sheetStudents(), handler.AnyTenantAssignment, scope)
		putRec, delRec := httptest.NewRecorder(), httptest.NewRecorder()
		h.Put(putRec, principalReq(http.MethodPut, "/api/v1/grades/monthly", put))
		h.Clear(delRec, principalReq(http.MethodDelete, "/api/v1/grades/monthly"+del, ""))
		for _, rec := range []*httptest.ResponseRecorder{putRec, delRec} {
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "MONTH_IN_FUTURE", rec.Header().Get("X-Error-Code"))
			assert.Contains(t, rec.Body.String(), "Aún no llega ese mes")
		}
	}
}

// The sheet names the server's current UTC month, so the screen can lock the
// month chips that come after it before anyone picks one.
func TestMonthlySheetNamesTheCurrentUTCMonth(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 10, 31, 23, 30, 0, 0, time.FixedZone("CST", -6*3600)) }
	h := handler.NewMonthlyGradesHandler(&fakeMonthlyGrades{sheet: sheetFixture()}, sheetStudents(), handler.AnyTenantAssignment, handler.ScopeTeacher, handler.WithMonthlyClock(clock))
	rec := httptest.NewRecorder()
	h.Sheet(rec, principalReq(http.MethodGet, "/api/v1/teaching/grades/sheet?assignmentId="+mAsg, ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"currentMonth":"2026-11"`, "23:30 CST on Oct 31 is already November in UTC")
}

func TestMonthlySheetReadOnlyCarriesReason(t *testing.T) {
	sheet := sheetFixture()
	sheet.GradingEnabled, sheet.Editable, sheet.ReadOnlyReason = false, false, "Este nivel no lleva calificación numérica"
	h := handler.NewMonthlyGradesHandler(&fakeMonthlyGrades{sheet: sheet}, sheetStudents(), handler.AnyTenantAssignment, handler.ScopeSchool)
	rec := httptest.NewRecorder()
	h.Sheet(rec, principalReq(http.MethodGet, "/api/v1/grades/sheet?assignmentId="+mAsg+"&month=2026-09", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"readOnlyReason":"Este nivel no lleva calificación numérica"`)
	assert.Contains(t, rec.Body.String(), `"editable":false`)
}

func TestMonthlySheetRefusals(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		gate     handler.AssignmentGate
		svcErr   error
		wantCode int
		wantBody string
		wantCall bool
	}{
		{"malformed assignment id is 404", "/api/v1/grades/sheet?assignmentId=nope", handler.AnyTenantAssignment, nil, http.StatusNotFound, "", false},
		{"missing assignment id is 404", "/api/v1/grades/sheet", handler.AnyTenantAssignment, nil, http.StatusNotFound, "", false},
		{"not owned is 404 and never reaches grades", "/api/v1/teaching/grades/sheet?assignmentId=" + mAsgOther, denyGate, nil, http.StatusNotFound, "", false},
		{"unknown assignment is 404", "/api/v1/grades/sheet?assignmentId=" + mAsg, handler.AnyTenantAssignment, connect.NewError(connect.CodeNotFound, errString("assignment not found")), http.StatusNotFound, "", true},
		{"month outside the cycle is 400 with the sentence", "/api/v1/grades/sheet?assignmentId=" + mAsg + "&month=2030-01", handler.AnyTenantAssignment,
			connect.NewError(connect.CodeInvalidArgument, errString("Ese mes no pertenece al ciclo")), http.StatusBadRequest, "Ese mes no pertenece al ciclo", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grades := &fakeMonthlyGrades{sheet: sheetFixture(), sheetErr: tt.svcErr}
			rec := httptest.NewRecorder()
			handler.NewMonthlyGradesHandler(grades, sheetStudents(), tt.gate, handler.ScopeTeacher).Sheet(rec, principalReq(http.MethodGet, tt.target, ""))
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), tt.wantBody)
			assert.Equal(t, tt.wantCall, grades.sheetCalls == 1)
		})
	}
}

// --- put ---------------------------------------------------------------------

func TestMonthlyPutForwardsExactTextAndCaller(t *testing.T) {
	grades := &fakeMonthlyGrades{}
	h := handler.NewMonthlyGradesHandler(grades, sheetStudents(), handler.AnyTenantAssignment, handler.ScopeSchool)
	rec := httptest.NewRecorder()
	h.Put(rec, principalReq(http.MethodPut, "/api/v1/grades/monthly",
		`{"assignmentId":"`+strings.ToUpper(mAsg)+`","month":"2026-09","enrollmentId":"`+mEnr+`","value":9.95}`))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, client.MonthlyGradeInput{AssignmentID: mAsg, Month: "2026-09", EnrollmentID: mEnr, Value: "9.95", CapturedBy: mUser, SchoolWide: true}, grades.upsertIn,
		"the number's exact text reaches grades — never a float")
	assert.JSONEq(t, `{"enrollmentId":"`+mEnr+`","month":"2026-09","value":9.5,"updatedAt":"2026-09-03T15:04:05Z"}`, rec.Body.String())
}

func TestMonthlyPutRefusals(t *testing.T) {
	body := func(value string) string {
		return `{"assignmentId":"` + mAsg + `","month":"2026-09","enrollmentId":"` + mEnr + `","value":` + value + `}`
	}
	tests := []struct {
		name     string
		body     string
		gate     handler.AssignmentGate
		svcErr   error
		wantCode int
		wantBody string
	}{
		{"string value", body(`"9.5"`), handler.AnyTenantAssignment, nil, http.StatusBadRequest, "La calificación debe ser un número"},
		{"null value", body(`null`), handler.AnyTenantAssignment, nil, http.StatusBadRequest, "La calificación debe ser un número"},
		{"NaN is not JSON", body(`NaN`), handler.AnyTenantAssignment, nil, http.StatusBadRequest, ""},
		{"missing value", `{"assignmentId":"` + mAsg + `","month":"2026-09","enrollmentId":"` + mEnr + `"}`, handler.AnyTenantAssignment, nil, http.StatusBadRequest, "La calificación debe ser un número"},
		{"malformed enrollment", `{"assignmentId":"` + mAsg + `","month":"2026-09","enrollmentId":"x","value":9}`, handler.AnyTenantAssignment, nil, http.StatusBadRequest, "El alumno no está inscrito en este grupo"},
		{"malformed assignment is 404", `{"assignmentId":"x","month":"2026-09","enrollmentId":"` + mEnr + `","value":9}`, handler.AnyTenantAssignment, nil, http.StatusNotFound, ""},
		{"not owned is 404", body(`9`), denyGate, nil, http.StatusNotFound, ""},
		{"scale refusal keeps the sentence", body(`11`), handler.AnyTenantAssignment,
			connect.NewError(connect.CodeInvalidArgument, errString("La calificación debe estar entre 5 y 10")), http.StatusBadRequest, "La calificación debe estar entre 5 y 10"},
		{"preescolar refusal", body(`9`), handler.AnyTenantAssignment,
			connect.NewError(connect.CodeInvalidArgument, errString("Este nivel no lleva calificación numérica")), http.StatusBadRequest, "Este nivel no lleva calificación numérica"},
		{"malformed body", `{`, handler.AnyTenantAssignment, nil, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grades := &fakeMonthlyGrades{upsertErr: tt.svcErr}
			rec := httptest.NewRecorder()
			handler.NewMonthlyGradesHandler(grades, sheetStudents(), tt.gate, handler.ScopeTeacher).Put(rec, principalReq(http.MethodPut, "/api/v1/grades/monthly", tt.body))
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), tt.wantBody)
			if tt.svcErr == nil {
				assert.Zero(t, grades.upsertCalls, "a refused cell must not reach grades")
			}
		})
	}
}

func TestMonthlyPutWithoutPrincipalIs401(t *testing.T) {
	grades := &fakeMonthlyGrades{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/grades/monthly", strings.NewReader(`{"assignmentId":"`+mAsg+`","month":"2026-09","enrollmentId":"`+mEnr+`","value":9}`))
	handler.NewMonthlyGradesHandler(grades, sheetStudents(), handler.AnyTenantAssignment, handler.ScopeSchool).Put(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, grades.upsertCalls)
}

// --- delete ------------------------------------------------------------------

func TestMonthlyDelete(t *testing.T) {
	grades := &fakeMonthlyGrades{}
	h := handler.NewMonthlyGradesHandler(grades, sheetStudents(), handler.AnyTenantAssignment, handler.ScopeSchool)
	rec := httptest.NewRecorder()
	h.Clear(rec, principalReq(http.MethodDelete, "/api/v1/grades/monthly?assignmentId="+mAsg+"&month=2026-09&enrollmentId="+strings.ToUpper(mEnr), ""))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, []string{mAsg, "2026-09", mEnr}, []string{grades.clearAsg, grades.clearMonth, grades.clearEnr})
	assert.True(t, grades.clearCell.SchoolWide, "the director clears any month")

	for _, tt := range []struct {
		name     string
		target   string
		gate     handler.AssignmentGate
		wantCode int
	}{
		{"not owned", "/api/v1/teaching/grades/monthly?assignmentId=" + mAsgOther + "&month=2026-09&enrollmentId=" + mEnr, denyGate, http.StatusNotFound},
		{"malformed enrollment", "/api/v1/grades/monthly?assignmentId=" + mAsg + "&month=2026-09&enrollmentId=zz", handler.AnyTenantAssignment, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			grades := &fakeMonthlyGrades{}
			rec := httptest.NewRecorder()
			handler.NewMonthlyGradesHandler(grades, sheetStudents(), tt.gate, handler.ScopeTeacher).Clear(rec, principalReq(http.MethodDelete, tt.target, ""))
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Zero(t, grades.clearCalls)
		})
	}
}

// --- teacher gate ------------------------------------------------------------

// The teacher's gate is the existing active-cycle ownership rule: an assignment
// the caller teaches passes, any other id is 404 before grades is called.
func TestTeachingOwnsAssignmentGate(t *testing.T) {
	config := oneTeacher()
	config.assignmentsByTeacher["tch-1"][0].ID = mAsg
	teaching := handler.NewTeachingHandler(config, &fakeTeachingGrades{}, &fakeTeachingStudents{})

	grades := &fakeMonthlyGrades{sheet: sheetFixture()}
	h := handler.NewMonthlyGradesHandler(grades, sheetStudents(), teaching.OwnsAssignment, handler.ScopeTeacher)

	rec := httptest.NewRecorder()
	h.Sheet(rec, teacherGET("/api/v1/teaching/grades/sheet?assignmentId="+mAsg, "usr-t"))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = httptest.NewRecorder()
	h.Sheet(rec, teacherGET("/api/v1/teaching/grades/sheet?assignmentId="+mAsgOther, "usr-t"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, 1, grades.sheetCalls, "the foreign id never reached grades")

	rec = httptest.NewRecorder()
	h.Sheet(rec, teacherGET("/api/v1/teaching/grades/sheet?assignmentId="+mAsg, "usr-other"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "another teacher's assignment is 404")
}

// T7: the sheet names the months a closed period locks, so the screen marks
// them read-only up front.
func TestMonthlySheetCarriesClosedMonths(t *testing.T) {
	sheet := sheetFixture()
	sheet.ClosedMonths = []string{"2026-08", "2026-09"}
	sheet.Editable, sheet.ReadOnlyReason = false, "El periodo está cerrado"
	h := handler.NewMonthlyGradesHandler(&fakeMonthlyGrades{sheet: sheet}, sheetStudents(), handler.AnyTenantAssignment, handler.ScopeSchool)
	rec := httptest.NewRecorder()
	h.Sheet(rec, principalReq(http.MethodGet, "/api/v1/grades/sheet?assignmentId="+mAsg+"&month=2026-09", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"closedMonths":["2026-08","2026-09"]`)
	assert.Contains(t, rec.Body.String(), `"readOnlyReason":"El periodo está cerrado"`)
}
