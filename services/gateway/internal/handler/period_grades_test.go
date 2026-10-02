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
	mPer      = "0e000000-0000-0000-0000-000000000001"
	mUser     = "u0000000-0000-0000-0000-000000000001"
)

var mUpdated = time.Date(2026, 10, 3, 15, 4, 5, 0, time.UTC)

type fakePeriodGrades struct {
	sheet      client.PeriodSheet
	sheetErr   error
	gotSheetQ  client.PeriodSheetQuery
	sheetCalls int

	upsertIn    client.PeriodGradeInput
	upsertErr   error
	upsertCalls int

	clearCell  client.PeriodCell
	clearErr   error
	clearCalls int
}

func (f *fakePeriodGrades) GetPeriodSheet(_ context.Context, q client.PeriodSheetQuery) (client.PeriodSheet, error) {
	f.sheetCalls++
	f.gotSheetQ = q
	return f.sheet, f.sheetErr
}

func (f *fakePeriodGrades) UpsertPeriodGrade(_ context.Context, in client.PeriodGradeInput) (client.PeriodGrade, error) {
	f.upsertCalls++
	f.upsertIn = in
	if f.upsertErr != nil {
		return client.PeriodGrade{}, f.upsertErr
	}
	return client.PeriodGrade{EnrollmentID: in.EnrollmentID, SubjectID: "sub-1", PeriodID: in.PeriodID, Value: "9", UpdatedAt: mUpdated}, nil
}

func (f *fakePeriodGrades) ClearPeriodGrade(_ context.Context, cell client.PeriodCell) error {
	f.clearCalls++
	f.clearCell = cell
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

func sheetFixture() client.PeriodSheet {
	return client.PeriodSheet{
		AssignmentID: mAsg, PeriodID: mPer, CurrentPeriodID: mPer, Today: "2026-10-01",
		Periods: []client.EvaluationPeriodRef{
			{ID: mPer, Ordinal: 1, Name: "1.er trimestre", StartsOn: "2026-08-24", EndsOn: "2026-11-30"},
		},
		Scale:          client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, Editable: true,
		Rows: []client.PeriodSheetRow{
			{EnrollmentID: "enr-z", StudentID: "stu-z", Editable: true},
			{EnrollmentID: "enr-a", StudentID: "stu-a", Value: "9", UpdatedAt: mUpdated, Editable: true},
			{EnrollmentID: "enr-n", StudentID: "stu-n", ReadOnlyReason: "Ese periodo corresponde al grupo anterior del alumno"},
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

// denyGate stands in for the teacher's ownership check refusing.
func denyGate(w http.ResponseWriter, _ *http.Request, _ string) bool {
	http.Error(w, "not found", http.StatusNotFound)
	return false
}

func principalReq(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: mUser, TenantID: "t1", Email: "dir@colegio.mx"}))
}

func directorHandler(grades handler.PeriodGradesService) *handler.PeriodGradesHandler {
	return handler.NewPeriodGradesHandler(grades, sheetStudents(), nil, handler.AnyTenantAssignment, handler.ScopeSchool)
}

func putBody(value string) string {
	return `{"assignmentId":"` + mAsg + `","periodId":"` + mPer + `","enrollmentId":"` + mEnr + `","value":` + value + `}`
}

// --- sheet -------------------------------------------------------------------

func TestPeriodSheetShapeAndSpanishOrder(t *testing.T) {
	grades := &fakePeriodGrades{sheet: sheetFixture()}
	rec := httptest.NewRecorder()
	directorHandler(grades).Sheet(rec, principalReq(http.MethodGet, "/api/v1/grades/period-sheet?assignmentId="+strings.ToUpper(mAsg), ""))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, mAsg, grades.gotSheetQ.AssignmentID, "the id is canonicalised before it goes downstream")
	assert.Equal(t, "", grades.gotSheetQ.PeriodID, "no period ⇒ the service picks today's")
	assert.True(t, grades.gotSheetQ.SchoolWide, "the director's sheet is school-wide")
	assert.JSONEq(t, `{
		"assignmentId": "`+mAsg+`", "periodId": "`+mPer+`", "currentPeriodId": "`+mPer+`", "today": "2026-10-01",
		"periods": [{"id":"`+mPer+`","ordinal":1,"name":"1.er trimestre","startsOn":"2026-08-24","endsOn":"2026-11-30"}],
		"scale": {"min":5,"max":10,"decimals":1,"passing":6},
		"gradingEnabled": true, "editable": true, "closedPeriodIds": [],
		"rows": [
			{"enrollmentId":"enr-a","studentId":"stu-a","name":"Ávila Ana","value":9,"updatedAt":"2026-10-03T15:04:05Z","rowEditable":true},
			{"enrollmentId":"enr-n","studentId":"stu-n","name":"Núñez Nico","value":null,"updatedAt":null,"rowEditable":false,
			 "rowReadOnlyReason":"Ese periodo corresponde al grupo anterior del alumno"},
			{"enrollmentId":"enr-z","studentId":"stu-z","name":"Zúñiga Zoe","value":null,"updatedAt":null,"rowEditable":true}
		]}`, rec.Body.String(), "rows sorted by last name, Spanish collation (Á with A, Ñ after N)")
}

// A grade without periods answers null ids and an empty list.
func TestPeriodSheetOfAGradeWithoutPeriods(t *testing.T) {
	sheet := sheetFixture()
	sheet.PeriodID, sheet.CurrentPeriodID, sheet.Periods = "", "", nil
	sheet.Editable, sheet.ReadOnlyReason = false, "Este grado no tiene periodos de evaluación configurados"
	sheet.ClosedPeriodIDs = []string{mPer}
	rec := httptest.NewRecorder()
	directorHandler(&fakePeriodGrades{sheet: sheet}).Sheet(rec, principalReq(http.MethodGet, "/api/v1/grades/period-sheet?assignmentId="+mAsg+"&periodId="+mPer, ""))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"periodId":null`)
	assert.Contains(t, body, `"currentPeriodId":null`)
	assert.Contains(t, body, `"periods":[]`)
	assert.Contains(t, body, `"closedPeriodIds":["`+mPer+`"]`)
	assert.Contains(t, body, `"readOnlyReason":"Este grado no tiene periodos de evaluación configurados"`)
}

// A sheet names only its own pupils, in ONE read by id — never the whole school.
func TestPeriodSheetNamesOnlyItsOwnPupils(t *testing.T) {
	students := sheetStudents()
	h := handler.NewPeriodGradesHandler(&fakePeriodGrades{sheet: sheetFixture()}, students, nil, handler.AnyTenantAssignment, handler.ScopeSchool)
	rec := httptest.NewRecorder()
	h.Sheet(rec, principalReq(http.MethodGet, "/api/v1/grades/period-sheet?assignmentId="+mAsg, ""))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, students.calls)
	assert.ElementsMatch(t, []string{"stu-z", "stu-a", "stu-n"}, students.gotIDs)
	assert.NotContains(t, rec.Body.String(), "Otro Grupo")
}

func TestPeriodSheetRefusals(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		gate     handler.AssignmentGate
		svcErr   error
		wantCode int
		wantBody string
		wantCall bool
	}{
		{"malformed assignment id is 404", "/api/v1/grades/period-sheet?assignmentId=nope", handler.AnyTenantAssignment, nil, http.StatusNotFound, "", false},
		{"missing assignment id is 404", "/api/v1/grades/period-sheet", handler.AnyTenantAssignment, nil, http.StatusNotFound, "", false},
		{"malformed period is 400", "/api/v1/grades/period-sheet?assignmentId=" + mAsg + "&periodId=x", handler.AnyTenantAssignment, nil, http.StatusBadRequest, "Ese periodo no pertenece al grado del grupo", false},
		{"not owned is 404 and never reaches grades", "/api/v1/teaching/grades/period-sheet?assignmentId=" + mAsgOther, denyGate, nil, http.StatusNotFound, "", false},
		{"unknown assignment is 404", "/api/v1/grades/period-sheet?assignmentId=" + mAsg, handler.AnyTenantAssignment, connect.NewError(connect.CodeNotFound, errString("assignment not found")), http.StatusNotFound, "", true},
		{"a period of another grade keeps the sentence", "/api/v1/grades/period-sheet?assignmentId=" + mAsg + "&periodId=" + mPer, handler.AnyTenantAssignment,
			connect.NewError(connect.CodeInvalidArgument, errString("Ese periodo no pertenece al grado del grupo")), http.StatusBadRequest, "Ese periodo no pertenece al grado del grupo", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grades := &fakePeriodGrades{sheet: sheetFixture(), sheetErr: tt.svcErr}
			rec := httptest.NewRecorder()
			handler.NewPeriodGradesHandler(grades, sheetStudents(), nil, tt.gate, handler.ScopeTeacher).Sheet(rec, principalReq(http.MethodGet, tt.target, ""))
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), tt.wantBody)
			assert.Equal(t, tt.wantCall, grades.sheetCalls == 1)
		})
	}
}

// The teacher's routes are never school-wide: the previous-group rule applies.
func TestTeacherPeriodRoutesAreNotSchoolWide(t *testing.T) {
	grades := &fakePeriodGrades{sheet: sheetFixture()}
	h := handler.NewPeriodGradesHandler(grades, sheetStudents(), nil, handler.AnyTenantAssignment, handler.ScopeTeacher)
	h.Sheet(httptest.NewRecorder(), principalReq(http.MethodGet, "/api/v1/teaching/grades/period-sheet?assignmentId="+mAsg, ""))
	h.Put(httptest.NewRecorder(), principalReq(http.MethodPut, "/api/v1/teaching/grades/period-grades", putBody(`9`)))
	h.Clear(httptest.NewRecorder(), principalReq(http.MethodDelete, "/api/v1/teaching/grades/period-grades?assignmentId="+mAsg+"&periodId="+mPer+"&enrollmentId="+mEnr, ""))
	assert.False(t, grades.gotSheetQ.SchoolWide)
	assert.False(t, grades.upsertIn.SchoolWide)
	assert.False(t, grades.clearCell.SchoolWide)
	assert.Equal(t, 1, grades.upsertCalls)
	assert.Equal(t, 1, grades.clearCalls)
}

// --- put ---------------------------------------------------------------------

// The number's exact text reaches grades (it judges wholeness), with who
// captured it: the caller's id and display name.
func TestPeriodPutForwardsExactTextAndCaller(t *testing.T) {
	grades := &fakePeriodGrades{}
	names := fakeNamer{user: client.User{DisplayName: "Directora Ruiz"}}
	h := handler.NewPeriodGradesHandler(grades, sheetStudents(), names, handler.AnyTenantAssignment, handler.ScopeSchool)
	rec := httptest.NewRecorder()
	h.Put(rec, principalReq(http.MethodPut, "/api/v1/grades/period-grades",
		`{"assignmentId":"`+strings.ToUpper(mAsg)+`","periodId":"`+strings.ToUpper(mPer)+`","enrollmentId":"`+mEnr+`","value":9.0}`))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, client.PeriodGradeInput{AssignmentID: mAsg, PeriodID: mPer, EnrollmentID: mEnr, Value: "9.0",
		CapturedBy: mUser, CapturedByName: "Directora Ruiz", SchoolWide: true}, grades.upsertIn, "never a float")
	assert.JSONEq(t, `{"enrollmentId":"`+mEnr+`","periodId":"`+mPer+`","value":9,"updatedAt":"2026-10-03T15:04:05Z"}`, rec.Body.String())
}

// Every refusal names its machine code in X-Error-Code: the edge's own and the
// grades service's (forwarded from the error metadata).
func TestPeriodRefusalsCarryErrorCodes(t *testing.T) {
	coded := func(code, msg string, status connect.Code) error {
		e := connect.NewError(status, errString(msg))
		e.Meta().Set("X-Error-Code", code)
		return e
	}
	tests := []struct {
		name, body, wantCode string
		wantStatus           int
		svcErr               error
	}{
		{"string value", putBody(`"9"`), "GRADE_NOT_NUMBER", http.StatusBadRequest, nil},
		{"malformed enrollment", `{"assignmentId":"` + mAsg + `","periodId":"` + mPer + `","enrollmentId":"x","value":9}`, "ENROLLMENT_NOT_IN_GROUP", http.StatusBadRequest, nil},
		{"malformed period", `{"assignmentId":"` + mAsg + `","periodId":"p","enrollmentId":"` + mEnr + `","value":9}`, "PERIOD_NOT_IN_GRADE", http.StatusBadRequest, nil},
		{"a decimal", putBody(`9.5`), "GRADE_NOT_INTEGER", http.StatusBadRequest, coded("GRADE_NOT_INTEGER", "Solo se permiten calificaciones enteras", connect.CodeInvalidArgument)},
		{"previous group", putBody(`9`), "PERIOD_PREVIOUS_GROUP", http.StatusBadRequest, coded("PERIOD_PREVIOUS_GROUP", "Ese periodo corresponde al grupo anterior del alumno", connect.CodeInvalidArgument)},
		{"not started", putBody(`9`), "PERIOD_NOT_STARTED", http.StatusBadRequest, coded("PERIOD_NOT_STARTED", "Aún no empieza ese periodo", connect.CodeInvalidArgument)},
		{"a closed term", putBody(`9`), "PERIOD_LOCKED", http.StatusConflict, coded("PERIOD_LOCKED", "El periodo está cerrado", connect.CodeAlreadyExists)},
		{"a code that is not a code is dropped", putBody(`9`), "", http.StatusBadRequest, coded("bad code\r\nX-Evil: 1", "x", connect.CodeInvalidArgument)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			directorHandler(&fakePeriodGrades{upsertErr: tt.svcErr}).Put(rec, principalReq(http.MethodPut, "/api/v1/grades/period-grades", tt.body))
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantCode, rec.Header().Get("X-Error-Code"))
		})
	}
}

func TestPeriodPutRefusals(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		gate     handler.AssignmentGate
		wantCode int
		wantBody string
	}{
		{"null value", putBody(`null`), handler.AnyTenantAssignment, http.StatusBadRequest, "La calificación debe ser un número"},
		{"NaN is not JSON", putBody(`NaN`), handler.AnyTenantAssignment, http.StatusBadRequest, ""},
		{"missing value", `{"assignmentId":"` + mAsg + `","periodId":"` + mPer + `","enrollmentId":"` + mEnr + `"}`, handler.AnyTenantAssignment, http.StatusBadRequest, "La calificación debe ser un número"},
		{"malformed assignment is 404", `{"assignmentId":"x","periodId":"` + mPer + `","enrollmentId":"` + mEnr + `","value":9}`, handler.AnyTenantAssignment, http.StatusNotFound, ""},
		{"not owned is 404", putBody(`9`), denyGate, http.StatusNotFound, ""},
		{"malformed body", `{`, handler.AnyTenantAssignment, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grades := &fakePeriodGrades{}
			rec := httptest.NewRecorder()
			handler.NewPeriodGradesHandler(grades, sheetStudents(), nil, tt.gate, handler.ScopeTeacher).Put(rec, principalReq(http.MethodPut, "/api/v1/grades/period-grades", tt.body))
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), tt.wantBody)
			assert.Zero(t, grades.upsertCalls, "a refused cell must not reach grades")
		})
	}
}

func TestPeriodPutWithoutPrincipalIs401(t *testing.T) {
	grades := &fakePeriodGrades{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/grades/period-grades", strings.NewReader(putBody(`9`)))
	directorHandler(grades).Put(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, grades.upsertCalls)
}

// --- delete ------------------------------------------------------------------

func TestPeriodDelete(t *testing.T) {
	grades := &fakePeriodGrades{}
	rec := httptest.NewRecorder()
	directorHandler(grades).Clear(rec, principalReq(http.MethodDelete, "/api/v1/grades/period-grades?assignmentId="+mAsg+"&periodId="+mPer+"&enrollmentId="+strings.ToUpper(mEnr), ""))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, client.PeriodCell{AssignmentID: mAsg, PeriodID: mPer, EnrollmentID: mEnr, SchoolWide: true}, grades.clearCell)

	for _, tt := range []struct {
		name     string
		target   string
		gate     handler.AssignmentGate
		wantCode int
	}{
		{"not owned", "/api/v1/teaching/grades/period-grades?assignmentId=" + mAsgOther + "&periodId=" + mPer + "&enrollmentId=" + mEnr, denyGate, http.StatusNotFound},
		{"malformed enrollment", "/api/v1/grades/period-grades?assignmentId=" + mAsg + "&periodId=" + mPer + "&enrollmentId=zz", handler.AnyTenantAssignment, http.StatusBadRequest},
		{"missing period", "/api/v1/grades/period-grades?assignmentId=" + mAsg + "&enrollmentId=" + mEnr, handler.AnyTenantAssignment, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			grades := &fakePeriodGrades{}
			rec := httptest.NewRecorder()
			handler.NewPeriodGradesHandler(grades, sheetStudents(), nil, tt.gate, handler.ScopeTeacher).Clear(rec, principalReq(http.MethodDelete, tt.target, ""))
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

	grades := &fakePeriodGrades{sheet: sheetFixture()}
	h := handler.NewPeriodGradesHandler(grades, sheetStudents(), nil, teaching.OwnsAssignment, handler.ScopeTeacher)

	rec := httptest.NewRecorder()
	h.Sheet(rec, teacherGET("/api/v1/teaching/grades/period-sheet?assignmentId="+mAsg, "usr-t"))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = httptest.NewRecorder()
	h.Sheet(rec, teacherGET("/api/v1/teaching/grades/period-sheet?assignmentId="+mAsgOther, "usr-t"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, 1, grades.sheetCalls, "the foreign id never reached grades")

	rec = httptest.NewRecorder()
	h.Sheet(rec, teacherGET("/api/v1/teaching/grades/period-sheet?assignmentId="+mAsg, "usr-other"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "another teacher's assignment is 404")
}
