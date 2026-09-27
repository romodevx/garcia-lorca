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
	sheetCalls int

	upsertIn    client.MonthlyGradeInput
	upsertErr   error
	upsertCalls int

	clearAsg, clearMonth, clearEnr string
	clearErr                       error
	clearCalls                     int
}

func (f *fakeMonthlyGrades) GetMonthlySheet(_ context.Context, assignmentID, month string) (client.MonthlySheet, error) {
	f.sheetCalls++
	f.gotSheetID, f.gotMonth = assignmentID, month
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

func (f *fakeMonthlyGrades) ClearMonthlyGrade(_ context.Context, assignmentID, month, enrollmentID string) error {
	f.clearCalls++
	f.clearAsg, f.clearMonth, f.clearEnr = assignmentID, month, enrollmentID
	return f.clearErr
}

func sheetFixture() client.MonthlySheet {
	return client.MonthlySheet{
		AssignmentID: mAsg, Month: "2026-09", Months: []string{"2026-08", "2026-09"},
		Scale:          client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, Editable: true,
		Rows: []client.MonthlySheetRow{
			{EnrollmentID: "enr-z", StudentID: "stu-z"},
			{EnrollmentID: "enr-a", StudentID: "stu-a", Value: "9.5", UpdatedAt: mUpdated},
			{EnrollmentID: "enr-n", StudentID: "stu-n"},
		},
	}
}

func sheetStudents() *fakeTeachingStudents {
	return &fakeTeachingStudents{students: []client.StudentResult{
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
	h := handler.NewMonthlyGradesHandler(grades, sheetStudents(), handler.AnyTenantAssignment)

	rec := httptest.NewRecorder()
	h.Sheet(rec, principalReq(http.MethodGet, "/api/v1/grades/sheet?assignmentId="+strings.ToUpper(mAsg), ""))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, mAsg, grades.gotSheetID, "the id is canonicalised before it goes downstream")
	assert.Equal(t, "", grades.gotMonth, "no month ⇒ the service picks the current one")
	assert.JSONEq(t, `{
		"assignmentId": "`+mAsg+`", "month": "2026-09", "months": ["2026-08","2026-09"],
		"scale": {"min":5,"max":10,"decimals":1,"passing":6},
		"gradingEnabled": true, "editable": true,
		"rows": [
			{"enrollmentId":"enr-a","studentId":"stu-a","name":"Ana Ávila","value":9.5,"updatedAt":"2026-09-03T15:04:05Z"},
			{"enrollmentId":"enr-n","studentId":"stu-n","name":"Nico Núñez","value":null,"updatedAt":null},
			{"enrollmentId":"enr-z","studentId":"stu-z","name":"Zoe Zúñiga","value":null,"updatedAt":null}
		]}`, rec.Body.String(), "rows sorted by last name, Spanish collation (Á with A, Ñ after N)")
}

func TestMonthlySheetReadOnlyCarriesReason(t *testing.T) {
	sheet := sheetFixture()
	sheet.GradingEnabled, sheet.Editable, sheet.ReadOnlyReason = false, false, "Este nivel no lleva calificación numérica"
	h := handler.NewMonthlyGradesHandler(&fakeMonthlyGrades{sheet: sheet}, sheetStudents(), handler.AnyTenantAssignment)
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
			handler.NewMonthlyGradesHandler(grades, sheetStudents(), tt.gate).Sheet(rec, principalReq(http.MethodGet, tt.target, ""))
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), tt.wantBody)
			assert.Equal(t, tt.wantCall, grades.sheetCalls == 1)
		})
	}
}

// --- put ---------------------------------------------------------------------

func TestMonthlyPutForwardsExactTextAndCaller(t *testing.T) {
	grades := &fakeMonthlyGrades{}
	h := handler.NewMonthlyGradesHandler(grades, sheetStudents(), handler.AnyTenantAssignment)
	rec := httptest.NewRecorder()
	h.Put(rec, principalReq(http.MethodPut, "/api/v1/grades/monthly",
		`{"assignmentId":"`+strings.ToUpper(mAsg)+`","month":"2026-09","enrollmentId":"`+mEnr+`","value":9.95}`))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, client.MonthlyGradeInput{AssignmentID: mAsg, Month: "2026-09", EnrollmentID: mEnr, Value: "9.95", CapturedBy: mUser}, grades.upsertIn,
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
			handler.NewMonthlyGradesHandler(grades, sheetStudents(), tt.gate).Put(rec, principalReq(http.MethodPut, "/api/v1/grades/monthly", tt.body))
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
	handler.NewMonthlyGradesHandler(grades, sheetStudents(), handler.AnyTenantAssignment).Put(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, grades.upsertCalls)
}

// --- delete ------------------------------------------------------------------

func TestMonthlyDelete(t *testing.T) {
	grades := &fakeMonthlyGrades{}
	h := handler.NewMonthlyGradesHandler(grades, sheetStudents(), handler.AnyTenantAssignment)
	rec := httptest.NewRecorder()
	h.Clear(rec, principalReq(http.MethodDelete, "/api/v1/grades/monthly?assignmentId="+mAsg+"&month=2026-09&enrollmentId="+strings.ToUpper(mEnr), ""))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, []string{mAsg, "2026-09", mEnr}, []string{grades.clearAsg, grades.clearMonth, grades.clearEnr})

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
			handler.NewMonthlyGradesHandler(grades, sheetStudents(), tt.gate).Clear(rec, principalReq(http.MethodDelete, tt.target, ""))
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
	h := handler.NewMonthlyGradesHandler(grades, sheetStudents(), teaching.OwnsAssignment)

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
