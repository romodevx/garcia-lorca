package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

type fakeCreator struct {
	got         client.CreateApplicantInput
	gotListStat string
	gotGetID    string
	gotUpdate   client.UpdateStudentInput
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

func (f *fakeCreator) UpdateStudent(_ context.Context, in client.UpdateStudentInput) (client.StudentResult, error) {
	f.gotUpdate = in
	return f.res, f.err
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
		Level:           "Preescolar",
		LevelID:         "lvl-9",
		AdmissionStatus: "pending",
	}}
	h := handler.NewStudentsHandler(svc)

	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","levelId":"lvl-9","gradeGroup":"4° A","guardianName":"Rosa"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.got.CURP != "HEGA050312MDFRNN08" || svc.got.LevelID != "lvl-9" {
		t.Errorf("forwarded input = %+v", svc.got)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["id"] != "stu-1" || out["admissionStatus"] != "pending" {
		t.Errorf("body = %v", out)
	}
	// "level" is the school's own name for it; "levelId" is what to send back.
	if out["level"] != "Preescolar" || out["levelId"] != "lvl-9" {
		t.Errorf("level in body = (%v, %v)", out["level"], out["levelId"])
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
	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","levelId":"lvl-9"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestCreateStudentInvalidArgumentIs400(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewStudentsHandler(svc)
	rec := postStudents(t, h, `{"curp":"bad","firstName":"Ana","lastName":"Herrera","levelId":"lvl-9"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateStudentInternalErrorIs500(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeUnavailable, nil)}
	h := handler.NewStudentsHandler(svc)
	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","levelId":"lvl-9"}`)
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

// The list backs pickers that only need names: it never carries a pupil's
// address, phones, email or birth date. Those come with GET /students/{id}.
func TestListStudentsLeavesTheProfileOut(t *testing.T) {
	svc := &fakeCreator{list: []client.StudentResult{{
		ID: "stu-1", FirstName: "Ana",
		Profile: client.StudentProfile{Email: "ana@correo.mx", Address: client.StudentAddress{Street: "Juárez"}},
	}}}
	h := handler.NewStudentsHandler(svc)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/students", nil))

	if body := rec.Body.String(); strings.Contains(body, "profile") || strings.Contains(body, "ana@correo.mx") || strings.Contains(body, "Juárez") {
		t.Errorf("list carries the profile: %s", body)
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
	svc := &fakeCreator{res: client.StudentResult{ID: "stu-9", FirstName: "Ana", AdmissionStatus: "accepted", EnrollmentNumber: "2026-00042"}}
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
	if out["id"] != "stu-9" || out["admissionStatus"] != "accepted" || out["enrollmentNumber"] != "2026-00042" {
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
	gradeStudent                                           string
	gradeLevel                                             string
	gotStudentID, gotGroupID, gotTermID                    string
	gotListStudentID, gotListCycleID, gotListGrp           string
	gotGetID                                               string
	gotXferEnrollmentID, gotXferToGroup, gotXferR          string
	gotWithdrawID                                          string
	gotWithdrawnOn, gotWithdrawReason                      string
	gotPreviewStudentID, gotPreviewCycleID, gotPreviewDate string
	res                                                    client.Enrollment
	list                                                   []client.Enrollment
	preview                                                client.WithdrawalPreview
	previewErr                                             error
	err                                                    error
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

func (f *fakeEnrollmentSvc) WithdrawEnrollment(_ context.Context, id, withdrawnOn, reason string) (client.Enrollment, error) {
	f.gotWithdrawID = id
	f.gotWithdrawnOn, f.gotWithdrawReason = withdrawnOn, reason
	return f.res, f.err
}

func (f *fakeEnrollmentSvc) PreviewWithdrawal(_ context.Context, studentID, cycleID, withdrawnOn string) (client.WithdrawalPreview, error) {
	f.gotPreviewStudentID, f.gotPreviewCycleID, f.gotPreviewDate = studentID, cycleID, withdrawnOn
	return f.preview, f.previewErr
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

func TestWithdrawForwardsDateAndReasonAndReturnsContractShape(t *testing.T) {
	svc := &fakeEnrollmentSvc{res: client.Enrollment{ID: "enr-1", StudentID: "stu-1", CycleID: "cyc-1", Status: "withdrawn", WithdrawnOn: "2026-10-15", WithdrawalReason: "Cambio de ciudad"}}
	h := handler.NewEnrollmentsHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments/withdraw",
		strings.NewReader(`{"enrollmentId":"enr-1","withdrawnOn":"2026-10-15","reason":"Cambio de ciudad"}`))
	rec := httptest.NewRecorder()
	h.Withdraw(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotWithdrawID != "enr-1" || svc.gotWithdrawnOn != "2026-10-15" || svc.gotWithdrawReason != "Cambio de ciudad" {
		t.Errorf("forwarded withdrawal = (%q,%q,%q)", svc.gotWithdrawID, svc.gotWithdrawnOn, svc.gotWithdrawReason)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["status"] != "withdrawn" || out["withdrawnOn"] != "2026-10-15" || out["withdrawalReason"] != "Cambio de ciudad" {
		t.Errorf("body = %v", out)
	}
}

func TestWithdrawRequiresFinanceWriteOnlyForBackdatedDates(t *testing.T) {
	for _, tc := range []struct {
		name, date string
		perms      []string
		want       int
	}{
		{name: "backdated without finance write", date: "2026-10-14", want: http.StatusForbidden},
		{name: "backdated with finance write", date: "2026-10-14", perms: []string{"finance:write"}, want: http.StatusOK},
		{name: "today without finance write", date: "2026-10-15", want: http.StatusOK},
		{name: "omitted date without finance write", want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeEnrollmentSvc{res: client.Enrollment{ID: "enr-1", Status: "withdrawn"}}
			h := handler.NewEnrollmentsHandler(svc, handler.WithEnrollmentsClock(func() time.Time {
				return time.Date(2026, 10, 15, 23, 30, 0, 0, time.FixedZone("east", 7*60*60))
			}))
			body := `{"enrollmentId":"enr-1","withdrawnOn":"` + tc.date + `"}`
			if tc.date == "" {
				body = `{"enrollmentId":"enr-1"}`
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments/withdraw", strings.NewReader(body))
			req = req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{Permissions: tc.perms}))
			rec := httptest.NewRecorder()
			h.Withdraw(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusForbidden {
				var envelope map[string]json.RawMessage
				if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || len(envelope) != 1 {
					t.Fatalf("forbidden envelope = %s, err %v", rec.Body.String(), err)
				}
				var payload struct {
					Error struct{ Code, Message string } `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Error.Code != "forbidden" || payload.Error.Message == "" {
					t.Errorf("error = %+v", payload.Error)
				}
				if svc.gotWithdrawID != "" {
					t.Errorf("forbidden request reached service: %+v", svc)
				}
			}
		})
	}
}

func TestWithdrawRejectsBadDateOrLongReasonWithErrorEnvelope(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid date", body: `{"enrollmentId":"enr-1","withdrawnOn":"15-10-2026"}`},
		{name: "impossible date", body: `{"enrollmentId":"enr-1","withdrawnOn":"2026-02-30"}`},
		{name: "reason over 500 runes", body: `{"enrollmentId":"enr-1","reason":"` + strings.Repeat("界", 501) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeEnrollmentSvc{}
			h := handler.NewEnrollmentsHandler(svc, handler.WithWithdrawalPreview(svc, nil))
			rec := doReq(t, h.Withdraw, http.MethodPost, "/api/v1/enrollments/withdraw", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("error envelope JSON: %v", err)
			}
			if len(envelope) != 1 {
				t.Fatalf("error envelope = %v, want single error field", envelope)
			}
			if svc.gotWithdrawID != "" {
				t.Fatalf("invalid request reached service: %+v", svc)
			}
		})
	}
}

func TestWithdrawalPreviewMapsContractAndDefaultsDate(t *testing.T) {
	for _, tc := range []struct{ name, query, wantDate string }{
		{name: "explicit", query: "?withdrawnOn=2026-10-15", wantDate: "2026-10-15"},
		{name: "today UTC", wantDate: time.Now().UTC().Format("2006-01-02")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeEnrollmentSvc{res: client.Enrollment{ID: "enr-1", StudentID: "stu-1", CycleID: "cyc-1", Status: "active"}, preview: client.WithdrawalPreview{
				ToCancel: []client.WithdrawalCharge{{ID: "ch-1", Description: "Colegiatura noviembre", DueDate: "2026-11-01", AmountCents: 450000}}, ToCancelCents: 450000, RemainingDebtCents: 120000,
			}}
			h := handler.NewEnrollmentsHandler(svc, handler.WithWithdrawalPreview(svc, nil))
			req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollments/enr-1/withdrawal-preview"+tc.query, nil)
			req.SetPathValue("id", "enr-1")
			rec := httptest.NewRecorder()
			h.PreviewWithdrawal(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			if svc.gotPreviewStudentID != "stu-1" || svc.gotPreviewCycleID != "cyc-1" || svc.gotPreviewDate != tc.wantDate {
				t.Errorf("preview args = (%q,%q,%q)", svc.gotPreviewStudentID, svc.gotPreviewCycleID, svc.gotPreviewDate)
			}
			var out struct {
				ToCancel           []map[string]any `json:"toCancel"`
				ToCancelCents      int64            `json:"toCancelCents"`
				RemainingDebtCents int64            `json:"remainingDebtCents"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(out.ToCancel) != 1 || out.ToCancel[0]["id"] != "ch-1" || out.ToCancel[0]["description"] != "Colegiatura noviembre" || out.ToCancel[0]["dueDate"] != "2026-11-01" || out.ToCancel[0]["amountCents"] != float64(450000) || out.ToCancelCents != 450000 || out.RemainingDebtCents != 120000 {
				t.Errorf("preview body = %v", out)
			}
		})
	}
}

func TestWithdrawalPreviewRejectsInvalidDateAndNonActiveOrMissingEnrollment(t *testing.T) {
	tests := []struct {
		name, date string
		enrollment client.Enrollment
		err        error
		want       int
	}{
		{name: "invalid date", date: "2026-2-01", enrollment: client.Enrollment{Status: "active"}, want: http.StatusBadRequest},
		{name: "unknown enrollment", date: "2026-02-01", err: connect.NewError(connect.CodeNotFound, nil), want: http.StatusNotFound},
		{name: "withdrawn enrollment", date: "2026-02-01", enrollment: client.Enrollment{Status: "withdrawn"}, want: http.StatusConflict},
		{name: "transferred enrollment", date: "2026-02-01", enrollment: client.Enrollment{Status: "transferred"}, want: http.StatusConflict},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeEnrollmentSvc{res: tc.enrollment, err: tc.err}
			h := handler.NewEnrollmentsHandler(svc)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollments/enr-1/withdrawal-preview?withdrawnOn="+tc.date, nil)
			req.SetPathValue("id", "enr-1")
			rec := httptest.NewRecorder()
			h.PreviewWithdrawal(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want != http.StatusOK && svc.gotPreviewStudentID != "" {
				t.Errorf("finance called after rejection: %+v", svc)
			}
		})
	}
}

func TestWithdrawalPreviewReturnsZerosWhenFinanceModuleIsOff(t *testing.T) {
	svc := &fakeEnrollmentSvc{res: client.Enrollment{ID: "enr-1", StudentID: "stu-1", CycleID: "cyc-1", Status: "active"}}
	h := handler.NewEnrollmentsHandler(svc, handler.WithWithdrawalPreview(svc, flagSet{}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollments/enr-1/withdrawal-preview?withdrawnOn=2026-10-15", nil)
	req.SetPathValue("id", "enr-1")
	rec := httptest.NewRecorder()
	h.PreviewWithdrawal(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		ToCancel           []map[string]any `json:"toCancel"`
		ToCancelCents      int64            `json:"toCancelCents"`
		RemainingDebtCents int64            `json:"remainingDebtCents"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ToCancel == nil || len(out.ToCancel) != 0 || out.ToCancelCents != 0 || out.RemainingDebtCents != 0 {
		t.Errorf("finance-off preview = %+v", out)
	}
	if svc.gotPreviewStudentID != "" {
		t.Errorf("called finance while its module was disabled: %+v", svc)
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

// PUT /api/v1/students/{id} forwards the whole editable file and answers it back.
func TestUpdateStudentForwardsTheFileAndAnswersIt(t *testing.T) {
	svc := &fakeCreator{res: client.StudentResult{
		ID: "stu-1", FirstName: "Ana María",
		Profile: client.StudentProfile{BirthDate: "2019-03-12", Email: "ana@correo.mx",
			Phones:  []client.StudentPhone{{Kind: "celular", Number: "5512345678"}},
			Address: client.StudentAddress{State: "Jalisco"}},
	}}
	h := handler.NewStudentsHandler(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/students/stu-1", strings.NewReader(
		`{"firstName":"Ana María","lastName":"Herrera","curp":"HEGA050312MDFRNN08",`+
			`"profile":{"birthDate":"2019-03-12","sex":"F","email":"ana@correo.mx",`+
			`"phones":[{"kind":"celular","number":"5512345678"}],"address":{"street":"Av. Juárez","state":"Jalisco"}}}`))
	req.SetPathValue("id", "stu-1")
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	got := svc.gotUpdate
	if got.ID != "stu-1" || got.FirstName != "Ana María" || got.Profile.Sex != "F" ||
		len(got.Profile.Phones) != 1 || got.Profile.Address.Street != "Av. Juárez" {
		t.Errorf("forwarded = %+v", got)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	profile, _ := out["profile"].(map[string]any)
	if profile == nil || profile["email"] != "ana@correo.mx" || profile["birthDate"] != "2019-03-12" {
		t.Errorf("profile in body = %v", out["profile"])
	}
}

func TestUpdateStudentInvalidIs400(t *testing.T) {
	svc := &fakeCreator{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewStudentsHandler(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/students/stu-1", strings.NewReader(`{"firstName":"A","lastName":"B","curp":"x"}`))
	req.SetPathValue("id", "stu-1")
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// T3: the maternal surname is captured on create and on edit, and answered back.
func TestStudentsForwardAndAnswerTheMaternalSurname(t *testing.T) {
	svc := &fakeCreator{res: client.StudentResult{ID: "stu-1", FirstName: "Ana", LastName: "Herrera", MaternalLastName: "Ríos"}}
	h := handler.NewStudentsHandler(svc)

	rec := postStudents(t, h, `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","maternalLastName":"Ríos","levelId":"lvl-9"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rec.Code)
	}
	if svc.got.MaternalLastName != "Ríos" {
		t.Errorf("create forwarded maternal = %q, want Ríos", svc.got.MaternalLastName)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["maternalLastName"] != "Ríos" {
		t.Errorf("body maternalLastName = %v, want Ríos", out["maternalLastName"])
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/students/stu-1", strings.NewReader(
		`{"firstName":"Ana","lastName":"Herrera","maternalLastName":"Cruz","curp":"HEGA050312MDFRNN08","profile":{}}`))
	req.SetPathValue("id", "stu-1")
	up := httptest.NewRecorder()
	h.Update(up, req)
	if up.Code != http.StatusOK {
		t.Fatalf("update status = %d", up.Code)
	}
	if svc.gotUpdate.MaternalLastName != "Cruz" {
		t.Errorf("update forwarded maternal = %q, want Cruz", svc.gotUpdate.MaternalLastName)
	}
}

// Review of T3: a caller that does not know the maternal field yet (an older
// cached web page, a script) must not erase the one already on file.
func TestUpdateStudentWithoutTheMaternalFieldKeepsIt(t *testing.T) {
	svc := &fakeCreator{res: client.StudentResult{ID: "stu-1", FirstName: "Ana", LastName: "Herrera", MaternalLastName: "Ríos"}}
	h := handler.NewStudentsHandler(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/students/stu-1", strings.NewReader(
		`{"firstName":"Ana","lastName":"Herrera","curp":"HEGA050312MDFRNN08","profile":{}}`))
	req.SetPathValue("id", "stu-1")
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if svc.gotGetID != "stu-1" || svc.gotUpdate.MaternalLastName != "Ríos" {
		t.Errorf("forwarded maternal = %q (read %q), want the stored Ríos", svc.gotUpdate.MaternalLastName, svc.gotGetID)
	}
}

// ADR 0033: the BBVA reference is shown on the file and edited there. A PUT
// without the field forwards "not sent" (nil), so students keeps the stored one.
func TestStudentsAnswerAndForwardTheBankReference(t *testing.T) {
	svc := &fakeCreator{res: client.StudentResult{ID: "stu-1", FirstName: "Ana", LastName: "Herrera", BankReference: "231076"}}
	h := handler.NewStudentsHandler(svc)

	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/students/stu-1", strings.NewReader(body))
		req.SetPathValue("id", "stu-1")
		rec := httptest.NewRecorder()
		h.Update(rec, req)
		return rec
	}

	rec := put(`{"firstName":"Ana","lastName":"Herrera","curp":"HEGA050312MDFRNN08","profile":{},"bankReference":"999"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if svc.gotUpdate.BankReference == nil || *svc.gotUpdate.BankReference != "999" {
		t.Errorf("forwarded bank reference = %v, want 999", svc.gotUpdate.BankReference)
	}
	var out map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if out["bankReference"] != "231076" {
		t.Errorf("body bankReference = %v, want the stored 231076", out["bankReference"])
	}

	put(`{"firstName":"Ana","lastName":"Herrera","curp":"HEGA050312MDFRNN08","profile":{}}`)
	if svc.gotUpdate.BankReference != nil {
		t.Errorf("a PUT without the field forwarded %q; it must forward nil (keep)", *svc.gotUpdate.BankReference)
	}
}
