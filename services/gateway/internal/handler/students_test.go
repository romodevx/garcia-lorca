package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakeCreator struct {
	got         client.CreateApplicantInput
	gotListStat string
	gotGetID    string
	res         client.StudentResult
	list        []client.StudentResult
	err         error
}

func (f *fakeCreator) CreateApplicant(_ context.Context, in client.CreateApplicantInput) (client.StudentResult, error) {
	f.got = in
	return f.res, f.err
}

func (f *fakeCreator) ListStudents(_ context.Context, status string) ([]client.StudentResult, error) {
	f.gotListStat = status
	return f.list, f.err
}

func (f *fakeCreator) GetStudent(_ context.Context, id string) (client.StudentResult, error) {
	f.gotGetID = id
	return f.res, f.err
}

func postStudents(t *testing.T, h *handler.StudentsHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/students", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	return rec
}

func TestCreateStudentReturns201AndBody(t *testing.T) {
	svc := &fakeCreator{res: client.StudentResult{
		ID:              "stu-1",
		CURP:            "HEGA050312MDFRNN08",
		FirstName:       "Ana",
		Level:           "primaria",
		AdmissionStatus: "pending",
	}}
	h := handler.NewStudentsHandler(svc)

	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","level":"primaria","gradeGroup":"4° A","guardianName":"Rosa"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.got.CURP != "HEGA050312MDFRNN08" || svc.got.Level != "primaria" {
		t.Errorf("forwarded input = %+v", svc.got)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["id"] != "stu-1" || out["admissionStatus"] != "pending" {
		t.Errorf("body = %v", out)
	}
}

func TestCreateStudentInvalidBodyIs400(t *testing.T) {
	h := handler.NewStudentsHandler(&fakeCreator{})
	rec := postStudents(t, h, `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateStudentDuplicateIs409(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeAlreadyExists, nil)}
	h := handler.NewStudentsHandler(svc)
	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","level":"primaria"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestCreateStudentInvalidArgumentIs400(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewStudentsHandler(svc)
	rec := postStudents(t, h, `{"curp":"bad","firstName":"Ana","lastName":"Herrera","level":"primaria"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateStudentInternalErrorIs500(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeUnavailable, nil)}
	h := handler.NewStudentsHandler(svc)
	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","level":"primaria"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestListStudentsForwardsStatusAndWrapsList(t *testing.T) {
	svc := &fakeCreator{list: []client.StudentResult{
		{ID: "stu-1", FirstName: "Ana", AdmissionStatus: "pending"},
		{ID: "stu-2", FirstName: "Beto", AdmissionStatus: "pending"},
	}}
	h := handler.NewStudentsHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/students?status=pending", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotListStat != "pending" {
		t.Errorf("forwarded status = %q, want pending", svc.gotListStat)
	}
	var out struct {
		Students []map[string]any `json:"students"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Students) != 2 || out.Students[0]["id"] != "stu-1" {
		t.Errorf("body = %v", out)
	}
}

func TestListStudentsNoStatusForwardsEmpty(t *testing.T) {
	svc := &fakeCreator{}
	h := handler.NewStudentsHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/students", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotListStat != "" {
		t.Errorf("forwarded status = %q, want empty", svc.gotListStat)
	}
}

func TestListStudentsUpstreamErrorIs500(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeUnavailable, nil)}
	h := handler.NewStudentsHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/students", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestGetStudentByPathValue(t *testing.T) {
	svc := &fakeCreator{res: client.StudentResult{ID: "stu-9", FirstName: "Ana", AdmissionStatus: "accepted"}}
	h := handler.NewStudentsHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/students/stu-9", nil)
	req.SetPathValue("id", "stu-9")
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotGetID != "stu-9" {
		t.Errorf("gotGetID = %q", svc.gotGetID)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "stu-9" || out["admissionStatus"] != "accepted" {
		t.Errorf("body = %v", out)
	}
}

func TestGetStudentNotFoundIs404(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeNotFound, nil)}
	h := handler.NewStudentsHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/students/missing", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// --- enrollments ----------------------------------------------------------

type fakeEnrollmentSvc struct {
	gradeStudent                                  string
	gradeLevel                                    string
	gotStudentID, gotGroupID, gotTermID           string
	gotListStudentID, gotListCycleID, gotListGrp  string
	gotGetID                                      string
	gotXferEnrollmentID, gotXferToGroup, gotXferR string
	gotWithdrawID                                 string
	res                                           client.Enrollment
	list                                          []client.Enrollment
	err                                           error
}

func (f *fakeEnrollmentSvc) Enroll(_ context.Context, studentID, groupID, termID string) (client.Enrollment, error) {
	f.gotStudentID, f.gotGroupID, f.gotTermID = studentID, groupID, termID
	return f.res, f.err
}

func (f *fakeEnrollmentSvc) ListEnrollments(_ context.Context, studentID, cycleID, groupID string) ([]client.Enrollment, error) {
	f.gotListStudentID, f.gotListCycleID, f.gotListGrp = studentID, cycleID, groupID
	return f.list, f.err
}

func (f *fakeEnrollmentSvc) GetEnrollment(_ context.Context, id string) (client.Enrollment, error) {
	f.gotGetID = id
	return f.res, f.err
}

func (f *fakeEnrollmentSvc) TransferStudent(_ context.Context, enrollmentID, toGroupID, reason string) (client.Enrollment, error) {
	f.gotXferEnrollmentID, f.gotXferToGroup, f.gotXferR = enrollmentID, toGroupID, reason
	return f.res, f.err
}

func (f *fakeEnrollmentSvc) WithdrawEnrollment(_ context.Context, id string) (client.Enrollment, error) {
	f.gotWithdrawID = id
	return f.res, f.err
}

func TestEnrollReturns201AndDerivedCycle(t *testing.T) {
	svc := &fakeEnrollmentSvc{res: client.Enrollment{
		ID: "enr-1", StudentID: "stu-1", CycleID: "cyc-1", GroupID: "grp-1", Status: "active",
	}}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{"studentId":"stu-1","groupId":"grp-1"}`))
	rec := httptest.NewRecorder()
	h.Enroll(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.gotStudentID != "stu-1" || svc.gotGroupID != "grp-1" || svc.gotTermID != "" {
		t.Errorf("forwarded (%q,%q,%q)", svc.gotStudentID, svc.gotGroupID, svc.gotTermID)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "enr-1" || out["cycleId"] != "cyc-1" || out["status"] != "active" {
		t.Errorf("body = %v", out)
	}
}

func TestEnrollInvalidBodyIs400(t *testing.T) {
	h := handler.NewEnrollmentsHandler(&fakeEnrollmentSvc{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`not json`))
	rec := httptest.NewRecorder()
	h.Enroll(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestEnrollMissingGroupIs400(t *testing.T) {
	svc := &fakeEnrollmentSvc{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{"studentId":"stu-1","groupId":"missing"}`))
	rec := httptest.NewRecorder()
	h.Enroll(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestEnrollDuplicateIs409(t *testing.T) {
	svc := &fakeEnrollmentSvc{err: connect.NewError(connect.CodeAlreadyExists, nil)}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{"studentId":"stu-1","groupId":"grp-1"}`))
	rec := httptest.NewRecorder()
	h.Enroll(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestListEnrollmentsForwardsFiltersAndWrapsList(t *testing.T) {
	svc := &fakeEnrollmentSvc{list: []client.Enrollment{{ID: "enr-1", Status: "active"}}}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollments?studentId=stu-1&cycleId=cyc-1&groupId=grp-1", nil)
	rec := httptest.NewRecorder()
	h.ListEnrollments(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotListStudentID != "stu-1" || svc.gotListCycleID != "cyc-1" || svc.gotListGrp != "grp-1" {
		t.Errorf("filters (%q,%q,%q)", svc.gotListStudentID, svc.gotListCycleID, svc.gotListGrp)
	}
	var out struct {
		Enrollments []map[string]any `json:"enrollments"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Enrollments) != 1 || out.Enrollments[0]["id"] != "enr-1" {
		t.Errorf("body = %v", out)
	}
}

func TestGetEnrollmentByPathValue(t *testing.T) {
	svc := &fakeEnrollmentSvc{res: client.Enrollment{ID: "enr-9", Status: "active"}}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollments/enr-9", nil)
	req.SetPathValue("id", "enr-9")
	rec := httptest.NewRecorder()
	h.GetEnrollment(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotGetID != "enr-9" {
		t.Errorf("gotGetID = %q", svc.gotGetID)
	}
}

func TestGetEnrollmentNotFoundIs404(t *testing.T) {
	svc := &fakeEnrollmentSvc{err: connect.NewError(connect.CodeNotFound, nil)}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollments/missing", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h.GetEnrollment(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestTransferForwardsAndReturns200(t *testing.T) {
	svc := &fakeEnrollmentSvc{res: client.Enrollment{ID: "enr-1", GroupID: "grp-2", Status: "active"}}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments/transfer",
		strings.NewReader(`{"enrollmentId":"enr-1","toGroupId":"grp-2","reason":"reassigned"}`))
	rec := httptest.NewRecorder()
	h.Transfer(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotXferEnrollmentID != "enr-1" || svc.gotXferToGroup != "grp-2" || svc.gotXferR != "reassigned" {
		t.Errorf("forwarded (%q,%q,%q)", svc.gotXferEnrollmentID, svc.gotXferToGroup, svc.gotXferR)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["groupId"] != "grp-2" {
		t.Errorf("body = %v", out)
	}
}

func TestTransferNonActiveIs400(t *testing.T) {
	svc := &fakeEnrollmentSvc{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments/transfer",
		strings.NewReader(`{"enrollmentId":"enr-1","toGroupId":"grp-2","reason":"x"}`))
	rec := httptest.NewRecorder()
	h.Transfer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWithdrawForwardsAndReturns200(t *testing.T) {
	svc := &fakeEnrollmentSvc{res: client.Enrollment{ID: "enr-1", Status: "withdrawn"}}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments/withdraw",
		strings.NewReader(`{"enrollmentId":"enr-1"}`))
	rec := httptest.NewRecorder()
	h.Withdraw(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotWithdrawID != "enr-1" {
		t.Errorf("gotWithdrawID = %q", svc.gotWithdrawID)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["status"] != "withdrawn" {
		t.Errorf("body = %v", out)
	}
}

func (f *fakeEnrollmentSvc) EnrollIntoGrade(_ context.Context, studentID, gradeLevelID string) (client.Placement, error) {
	f.gradeStudent, f.gradeLevel = studentID, gradeLevelID
	if f.err != nil {
		return client.Placement{}, f.err
	}
	return client.Placement{
		Enrollment: client.Enrollment{ID: "enr-1", StudentID: studentID, GroupID: "grp-b"},
		GroupName:  "1° B",
	}, nil
}

// The panel asks for a grade and is told which group the pupil landed in, by
// name — "quedó en 1° B" is the whole answer a person needs.
func TestEnrollIntoGradeReportsTheGroupCapacityChose(t *testing.T) {
	svc := &fakeEnrollmentSvc{}
	h := handler.NewEnrollmentsHandler(svc)

	rec := doReq(t, h.EnrollIntoGrade, http.MethodPost, "/api/v1/enrollments/by-grade",
		`{"studentId":"stu-1","gradeLevelId":"gl-1"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.gradeStudent != "stu-1" || svc.gradeLevel != "gl-1" {
		t.Errorf("forwarded %q %q", svc.gradeStudent, svc.gradeLevel)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["groupName"] != "1° B" {
		t.Errorf("body = %v; the panel has to be able to say where the pupil landed", out)
	}
}

// The whole API answers in camelCase. Returning the client struct directly would
// serialise Go field names, and a panel reading `enrollment.id` would find
// nothing while `enrollment.ID` quietly worked.
func TestPlacementAnswersInTheSameShapeAsEveryOtherEndpoint(t *testing.T) {
	h := handler.NewEnrollmentsHandler(&fakeEnrollmentSvc{})

	rec := doReq(t, h.EnrollIntoGrade, http.MethodPost, "/api/v1/enrollments/by-grade",
		`{"studentId":"stu-1","gradeLevelId":"gl-1"}`)

	var out struct {
		Enrollment map[string]any `json:"enrollment"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := out.Enrollment["id"]; !ok {
		t.Errorf("enrollment keys = %v; want camelCase like the rest of the API", out.Enrollment)
	}
}
