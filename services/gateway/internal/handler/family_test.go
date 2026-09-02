package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// fakeFamilyStudents doubles the students surface the family handler consumes:
// the guardian-link scoping primitive plus the per-child profile/enrollment
// reads. It records the ids it was asked for, which is the assertion that
// matters — every by-child read must be scoped to the authenticated principal's
// own links, never to a request field.
type fakeFamilyStudents struct {
	gotGuardianID string
	links         []client.GuardianLink
	err           error

	student       client.StudentResult
	studentsByID  map[string]client.StudentResult
	getStudentErr error
	gotStudentID  string
	gotStudentIDs []string

	enrollments             []client.Enrollment
	listEnrollmentsErr      error
	gotEnrollmentsStudentID string
}

func (f *fakeFamilyStudents) ListStudentsForGuardian(_ context.Context, guardianUserID string) ([]client.GuardianLink, error) {
	f.gotGuardianID = guardianUserID
	return f.links, f.err
}

func (f *fakeFamilyStudents) GetStudent(_ context.Context, id string) (client.StudentResult, error) {
	f.gotStudentID = id
	f.gotStudentIDs = append(f.gotStudentIDs, id)
	if f.getStudentErr != nil {
		return client.StudentResult{}, f.getStudentErr
	}
	if f.studentsByID != nil {
		return f.studentsByID[id], nil
	}
	return f.student, nil
}

func (f *fakeFamilyStudents) ListEnrollments(_ context.Context, studentID, _, _ string) ([]client.Enrollment, error) {
	f.gotEnrollmentsStudentID = studentID
	return f.enrollments, f.listEnrollmentsErr
}

// fakeFamilyComms doubles the notification inbox (keyed by student so the
// aggregation across several children is observable) and the mark-read
// acknowledgement (recording the id it was asked to mark).
type fakeFamilyComms struct {
	gotStudentIDs []string
	gotUnreadOnly bool
	byStudent     map[string][]client.Notification
	err           error

	marked     string
	markResult client.Notification
	markErr    error
}

func (f *fakeFamilyComms) ListNotifications(_ context.Context, studentID string, unreadOnly bool) ([]client.Notification, error) {
	f.gotStudentIDs = append(f.gotStudentIDs, studentID)
	f.gotUnreadOnly = unreadOnly
	if f.err != nil {
		return nil, f.err
	}
	return f.byStudent[studentID], nil
}

func (f *fakeFamilyComms) MarkNotificationRead(_ context.Context, id string) (client.Notification, error) {
	f.marked = id
	if f.markErr != nil {
		return client.Notification{}, f.markErr
	}
	if f.markResult.ID == "" {
		return client.Notification{ID: id, ReadAt: "2026-09-01T18:00:00Z"}, nil
	}
	return f.markResult, nil
}

// fakeFamilyFinance doubles the account statement read, recording the student id
// it was keyed on.
type fakeFamilyFinance struct {
	gotStudentID string
	account      client.AccountStatement
	err          error
}

func (f *fakeFamilyFinance) GetAccount(_ context.Context, studentID string) (client.AccountStatement, error) {
	f.gotStudentID = studentID
	return f.account, f.err
}

// fakeFamilyGrades doubles the grade-entries read, keyed by enrollment so the
// aggregation across a child's enrollments is observable.
type fakeFamilyGrades struct {
	gotEnrollmentIDs []string
	gotTermID        string
	byEnrollment     map[string][]client.GradeEntry
	err              error
}

func (f *fakeFamilyGrades) ListGradeEntries(_ context.Context, enrollmentID, termID string) ([]client.GradeEntry, error) {
	f.gotEnrollmentIDs = append(f.gotEnrollmentIDs, enrollmentID)
	f.gotTermID = termID
	if f.err != nil {
		return nil, f.err
	}
	return f.byEnrollment[enrollmentID], nil
}

// fakeFamilyAttendance doubles the attendance range read, keyed by enrollment.
type fakeFamilyAttendance struct {
	gotEnrollmentIDs []string
	gotFrom, gotTo   string
	byEnrollment     map[string][]client.AttendanceRecord
	err              error
}

func (f *fakeFamilyAttendance) ListAttendance(_ context.Context, enrollmentID, from, to string) ([]client.AttendanceRecord, error) {
	f.gotEnrollmentIDs = append(f.gotEnrollmentIDs, enrollmentID)
	f.gotFrom, f.gotTo = from, to
	if f.err != nil {
		return nil, f.err
	}
	return f.byEnrollment[enrollmentID], nil
}

// fakeFamilyConfig doubles the academic catalog the grades enrichment reads,
// counting the calls so the "maps built once per request" contract is
// observable, and able to fail either lookup to prove the read degrades rather
// than erroring.
type fakeFamilyConfig struct {
	subjects        []client.SubjectResult
	terms           []client.TermResult
	subjectsErr     error
	termsErr        error
	gotListSubjects int
	gotListTerms    int
}

func (f *fakeFamilyConfig) ListSubjects(_ context.Context, _ string) ([]client.SubjectResult, error) {
	f.gotListSubjects++
	return f.subjects, f.subjectsErr
}

func (f *fakeFamilyConfig) ListTerms(_ context.Context, _, _ string) ([]client.TermResult, error) {
	f.gotListTerms++
	return f.terms, f.termsErr
}

// newNotifHandler builds a FamilyHandler for the notification-only tests, where
// the finance/grades/attendance readers are never touched.
func newNotifHandler(students *fakeFamilyStudents, comms *fakeFamilyComms) *handler.FamilyHandler {
	return handler.NewFamilyHandler(students, comms, nil, nil, nil, nil)
}

// guardianRequest builds an authenticated request against the family surface,
// with userID as the principal the middleware would have injected. When pathKey
// is non-empty it sets the path value directly: httptest.NewRequest does NOT run
// the mux, so r.PathValue would otherwise be empty in a direct-handler test.
func guardianRequest(method, target, userID, pathKey, pathVal string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{
		UserID: userID, TenantID: "t1", Email: "mama@example.com", Permissions: []string{"family:read"},
	}))
	if pathKey != "" {
		req.SetPathValue(pathKey, pathVal)
	}
	return req
}

// guardianGET is the read shorthand: an authenticated GET with no path value.
func guardianGET(target, userID string) *http.Request {
	return guardianRequest(http.MethodGet, target, userID, "", "")
}

func TestFamilyNotificationsAggregatesEveryChild(t *testing.T) {
	students := &fakeFamilyStudents{links: []client.GuardianLink{
		{ID: "gl-1", GuardianUserID: "usr-9", StudentID: "stu-1", Relationship: "mother"},
		{ID: "gl-2", GuardianUserID: "usr-9", StudentID: "stu-2", Relationship: "mother"},
	}}
	comms := &fakeFamilyComms{byStudent: map[string][]client.Notification{
		"stu-1": {{ID: "ntf-1", StudentID: "stu-1", Type: "absence", Title: "Ausencia"}},
		"stu-2": {
			{ID: "ntf-2", StudentID: "stu-2", Type: "payment", Title: "Pago recibido"},
			{ID: "ntf-3", StudentID: "stu-2", Type: "debt_overdue", Title: "Adeudo vencido"},
		},
	}}
	h := newNotifHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianGET("/api/v1/family/notifications", "usr-9"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if students.gotGuardianID != "usr-9" {
		t.Errorf("guardian id = %q, want the principal's usr-9", students.gotGuardianID)
	}
	var out struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Notifications) != 3 {
		t.Fatalf("notifications = %d, want 3 (1 + 2 across both children)", len(out.Notifications))
	}
	got := map[string]string{}
	for _, n := range out.Notifications {
		got[n["id"].(string)] = n["studentId"].(string)
	}
	want := map[string]string{"ntf-1": "stu-1", "ntf-2": "stu-2", "ntf-3": "stu-2"}
	for id, studentID := range want {
		if got[id] != studentID {
			t.Errorf("notification %s tagged studentId %q, want %q", id, got[id], studentID)
		}
	}
}

// The family inbox is ONE chronological stream, newest first — the same order
// /comms/notifications documents. The fan-out is child-major, so the aggregate
// must be re-sorted at the edge or the second child's news would all trail the
// first child's, however recent it is.
func TestFamilyNotificationsIsNewestFirstAcrossChildren(t *testing.T) {
	students := &fakeFamilyStudents{links: []client.GuardianLink{
		{StudentID: "stu-1"}, {StudentID: "stu-2"},
	}}
	comms := &fakeFamilyComms{byStudent: map[string][]client.Notification{
		"stu-1": {
			{ID: "old-1", StudentID: "stu-1", CreatedAt: "2026-08-01T10:00:00Z"},
			{ID: "old-2", StudentID: "stu-1", CreatedAt: "2026-07-01T10:00:00Z"},
		},
		"stu-2": {
			{ID: "newest", StudentID: "stu-2", CreatedAt: "2026-09-01T10:00:00Z"},
			{ID: "middle", StudentID: "stu-2", CreatedAt: "2026-08-15T10:00:00Z"},
		},
	}}
	h := newNotifHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianGET("/api/v1/family/notifications", "usr-9"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := make([]string, 0, len(out.Notifications))
	for _, n := range out.Notifications {
		got = append(got, n["id"].(string))
	}
	want := []string{"newest", "middle", "old-1", "old-2"}
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v (newest first across both children)", got, want)
		}
	}
}

// Equal instants keep the child order of the fan-out, so the response is
// reproducible rather than dependent on sort internals.
func TestFamilyNotificationsEqualInstantsKeepChildOrder(t *testing.T) {
	students := &fakeFamilyStudents{links: []client.GuardianLink{
		{StudentID: "stu-1"}, {StudentID: "stu-2"},
	}}
	sameInstant := "2026-09-01T10:00:00Z"
	comms := &fakeFamilyComms{byStudent: map[string][]client.Notification{
		"stu-1": {{ID: "ntf-a", StudentID: "stu-1", CreatedAt: sameInstant}},
		"stu-2": {{ID: "ntf-b", StudentID: "stu-2", CreatedAt: sameInstant}},
	}}
	h := newNotifHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianGET("/api/v1/family/notifications", "usr-9"))

	var out struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Notifications) != 2 ||
		out.Notifications[0]["id"] != "ntf-a" || out.Notifications[1]["id"] != "ntf-b" {
		t.Errorf("notifications = %v, want ntf-a then ntf-b", out.Notifications)
	}
}

func TestFamilyNotificationsWithoutChildrenIsEmptyArray(t *testing.T) {
	students := &fakeFamilyStudents{}
	comms := &fakeFamilyComms{}
	h := newNotifHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianGET("/api/v1/family/notifications", "usr-lonely"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Notifications == nil || len(out.Notifications) != 0 {
		t.Errorf("notifications = %v, want an empty array (never null)", out.Notifications)
	}
	if len(comms.gotStudentIDs) != 0 {
		t.Errorf("comms was called for %v; a guardian with no children needs no lookup", comms.gotStudentIDs)
	}
}

// The scoping guarantee: the caller cannot widen the child set. A studentId in
// the query (or anywhere in the request) is ignored — only the principal's own
// links decide which inboxes are read.
func TestFamilyNotificationsIgnoresRequestSuppliedStudentID(t *testing.T) {
	students := &fakeFamilyStudents{links: []client.GuardianLink{
		{ID: "gl-1", GuardianUserID: "usr-9", StudentID: "stu-1"},
	}}
	comms := &fakeFamilyComms{byStudent: map[string][]client.Notification{
		"stu-1":        {{ID: "ntf-1", StudentID: "stu-1"}},
		"stu-not-mine": {{ID: "ntf-x", StudentID: "stu-not-mine"}},
		"usr-not-mine": {{ID: "ntf-y", StudentID: "usr-not-mine"}},
	}}
	h := newNotifHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianGET(
		"/api/v1/family/notifications?studentId=stu-not-mine&guardianUserId=usr-not-mine", "usr-9"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if students.gotGuardianID != "usr-9" {
		t.Errorf("guardian id = %q, want the principal's usr-9", students.gotGuardianID)
	}
	if len(comms.gotStudentIDs) != 1 || comms.gotStudentIDs[0] != "stu-1" {
		t.Fatalf("comms called for %v, want only the principal's child stu-1", comms.gotStudentIDs)
	}
	var out struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Notifications) != 1 || out.Notifications[0]["id"] != "ntf-1" {
		t.Errorf("notifications = %v, want only ntf-1", out.Notifications)
	}
}

func TestFamilyNotificationsForwardsUnreadOnly(t *testing.T) {
	students := &fakeFamilyStudents{links: []client.GuardianLink{{StudentID: "stu-1"}}}
	comms := &fakeFamilyComms{}
	h := newNotifHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianGET("/api/v1/family/notifications?unreadOnly=true", "usr-9"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !comms.gotUnreadOnly {
		t.Error("unreadOnly=true was not forwarded to comms")
	}
}

func TestFamilyNotificationsWithoutPrincipalIs401(t *testing.T) {
	h := newNotifHandler(&fakeFamilyStudents{}, &fakeFamilyComms{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/family/notifications", nil)
	h.Notifications(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestFamilyNotificationsPropagatesLinkLookupError(t *testing.T) {
	students := &fakeFamilyStudents{err: connect.NewError(connect.CodePermissionDenied, errString("nope"))}
	h := newNotifHandler(students, &fakeFamilyComms{})
	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianGET("/api/v1/family/notifications", "usr-9"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// One child's inbox failing must fail the whole read: a partial list would look
// like "your child has no news", which is worse than an error.
func TestFamilyNotificationsPropagatesInboxError(t *testing.T) {
	students := &fakeFamilyStudents{links: []client.GuardianLink{{StudentID: "stu-1"}}}
	comms := &fakeFamilyComms{err: connect.NewError(connect.CodeUnavailable, errString("comms down"))}
	h := newNotifHandler(students, comms)
	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianGET("/api/v1/family/notifications", "usr-9"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// --- family students list: the guardian's own linked children, so the mobile
// app can discover the studentIds it then feeds to the by-child routes. Scoped
// by construction like /family/notifications: the set comes only from the
// principal, deduped, never nil.

func TestFamilyListStudentsReturnsLinkedChildrenDeduped(t *testing.T) {
	students := &fakeFamilyStudents{
		links: []client.GuardianLink{
			{StudentID: "stu-1"}, {StudentID: "stu-2"}, {StudentID: "stu-1"}, // duplicate link
		},
		studentsByID: map[string]client.StudentResult{
			"stu-1": {ID: "stu-1", FirstName: "Ana", LastName: "García", Level: "primaria", GradeGroup: "3A"},
			"stu-2": {ID: "stu-2", FirstName: "Luis", LastName: "García", Level: "kinder", GradeGroup: "K2"},
		},
	}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.ListStudents(rec, guardianGET("/api/v1/family/students", "usr-9"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if students.gotGuardianID != "usr-9" {
		t.Errorf("guardian id = %q, want the principal's usr-9", students.gotGuardianID)
	}
	// A duplicate link must not fetch the same child twice.
	if len(students.gotStudentIDs) != 2 {
		t.Errorf("GetStudent called for %v, want each child exactly once (deduped)", students.gotStudentIDs)
	}
	var out struct {
		Students []map[string]any `json:"students"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Students) != 2 {
		t.Fatalf("students = %d, want 2 (deduped)", len(out.Students))
	}
	got := map[string]map[string]any{}
	for _, s := range out.Students {
		got[s["id"].(string)] = s
	}
	if got["stu-1"]["firstName"] != "Ana" || got["stu-1"]["level"] != "primaria" || got["stu-1"]["gradeGroup"] != "3A" {
		t.Errorf("stu-1 = %v, want the core fields", got["stu-1"])
	}
	if got["stu-2"]["firstName"] != "Luis" || got["stu-2"]["lastName"] != "García" {
		t.Errorf("stu-2 = %v, want the core fields", got["stu-2"])
	}
}

func TestFamilyListStudentsWithoutChildrenIsEmptyArray(t *testing.T) {
	students := &fakeFamilyStudents{}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.ListStudents(rec, guardianGET("/api/v1/family/students", "usr-lonely"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		Students []map[string]any `json:"students"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Students == nil || len(out.Students) != 0 {
		t.Errorf("students = %v, want an empty array (never null)", out.Students)
	}
	if len(students.gotStudentIDs) != 0 {
		t.Errorf("GetStudent was called for %v; a guardian with no children needs no lookup", students.gotStudentIDs)
	}
}

func TestFamilyListStudentsWithoutPrincipalIs401(t *testing.T) {
	h := handler.NewFamilyHandler(&fakeFamilyStudents{}, &fakeFamilyComms{}, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/family/students", nil)
	h.ListStudents(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// --- Phase 9b B6.x self-scoped by-child reads -----------------------------
//
// Every by-child route MUST verify {studentId} is in the caller's own linked
// children before reading downstream, and answer 404 (never 403) for an id
// outside that set — so a guardian cannot probe whether another family's child
// id exists.

// oneChild is the common fixture: a guardian linked to exactly one child, stu-1.
func oneChild() *fakeFamilyStudents {
	return &fakeFamilyStudents{links: []client.GuardianLink{
		{ID: "gl-1", GuardianUserID: "usr-9", StudentID: "stu-1", Relationship: "mother"},
	}}
}

func TestFamilyAccountOwnedChildReturnsStatement(t *testing.T) {
	students := oneChild()
	finance := &fakeFamilyFinance{account: client.AccountStatement{
		Charges:           []client.Charge{{ID: "chg-1", StudentID: "stu-1", AmountCents: 150000, RemainingCents: 150000}},
		Payments:          []client.Payment{{ID: "pay-1", ChargeID: "chg-1", AmountCents: 50000, Method: "cash"}},
		TotalBalanceCents: 100000,
	}}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, finance, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.Account(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/account", "usr-9", "studentId", "stu-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if students.gotGuardianID != "usr-9" {
		t.Errorf("ownership check keyed on %q, want the principal usr-9", students.gotGuardianID)
	}
	if finance.gotStudentID != "stu-1" {
		t.Errorf("account read keyed on %q, want stu-1", finance.gotStudentID)
	}
	var out struct {
		Charges           []map[string]any `json:"charges"`
		Payments          []map[string]any `json:"payments"`
		TotalBalanceCents int64            `json:"totalBalanceCents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Charges) != 1 || len(out.Payments) != 1 || out.TotalBalanceCents != 100000 {
		t.Errorf("statement = %+v, want 1 charge, 1 payment, balance 100000", out)
	}
}

func TestFamilyAccountUnlinkedChildIs404AndDoesNotRead(t *testing.T) {
	students := oneChild() // linked only to stu-1
	finance := &fakeFamilyFinance{}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, finance, nil, nil, nil)

	rec := httptest.NewRecorder()
	// The guardian guesses another family's child id in the path.
	h.Account(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-999/account", "usr-9", "studentId", "stu-999"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (never 403 — no existence leak)", rec.Code)
	}
	if finance.gotStudentID != "" {
		t.Errorf("finance was read for %q; an unlinked child must never reach downstream", finance.gotStudentID)
	}
}

func TestFamilyProfileOwnedChildReturnsCoreFields(t *testing.T) {
	students := oneChild()
	students.student = client.StudentResult{
		ID: "stu-1", FirstName: "Ana", LastName: "García", Level: "primaria", GradeGroup: "3A",
	}
	students.enrollments = []client.Enrollment{
		{ID: "enr-1", StudentID: "stu-1", CycleID: "cyc-1", GroupID: "grp-1", TermID: "trm-1", Status: "active"},
	}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.Profile(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/profile", "usr-9", "studentId", "stu-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if students.gotStudentID != "stu-1" {
		t.Errorf("profile read keyed on %q, want stu-1", students.gotStudentID)
	}
	var out struct {
		ID                string          `json:"id"`
		FirstName         string          `json:"firstName"`
		LastName          string          `json:"lastName"`
		Level             string          `json:"level"`
		GradeGroup        string          `json:"gradeGroup"`
		CurrentEnrollment *map[string]any `json:"currentEnrollment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID != "stu-1" || out.FirstName != "Ana" || out.LastName != "García" ||
		out.Level != "primaria" || out.GradeGroup != "3A" {
		t.Errorf("profile = %+v, want the child core fields", out)
	}
	if out.CurrentEnrollment == nil || (*out.CurrentEnrollment)["id"] != "enr-1" {
		t.Errorf("currentEnrollment = %v, want the active enr-1", out.CurrentEnrollment)
	}
}

func TestFamilyProfileUnlinkedChildIs404AndDoesNotRead(t *testing.T) {
	students := oneChild()
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.Profile(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-999/profile", "usr-9", "studentId", "stu-999"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if students.gotStudentID != "" {
		t.Errorf("GetStudent was called for %q; an unlinked child must never reach downstream", students.gotStudentID)
	}
}

func TestFamilyGradesOwnedChildResolvesEnrollmentThenReads(t *testing.T) {
	students := oneChild()
	students.enrollments = []client.Enrollment{
		{ID: "enr-1", StudentID: "stu-1", Status: "active"},
	}
	grades := &fakeFamilyGrades{byEnrollment: map[string][]client.GradeEntry{
		"enr-1": {{ID: "grd-1", EnrollmentID: "enr-1", SubjectID: "sub-1", TermID: "trm-1", Value: 8.5}},
	}}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, grades, nil, nil)

	rec := httptest.NewRecorder()
	h.Grades(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/grades?termId=trm-1", "usr-9", "studentId", "stu-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if students.gotEnrollmentsStudentID != "stu-1" {
		t.Errorf("enrollments resolved for %q, want stu-1", students.gotEnrollmentsStudentID)
	}
	if len(grades.gotEnrollmentIDs) != 1 || grades.gotEnrollmentIDs[0] != "enr-1" {
		t.Errorf("grades read for %v, want [enr-1] (student id must be translated to enrollment id)", grades.gotEnrollmentIDs)
	}
	if grades.gotTermID != "trm-1" {
		t.Errorf("termId passthrough = %q, want trm-1", grades.gotTermID)
	}
	var out struct {
		GradeEntries []map[string]any `json:"gradeEntries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.GradeEntries) != 1 || out.GradeEntries[0]["id"] != "grd-1" {
		t.Errorf("gradeEntries = %v, want grd-1", out.GradeEntries)
	}
}

// A child with more than one enrollment (a mid-cycle transfer) contributes each,
// so the family view aggregates the grades across all of them.
func TestFamilyGradesAggregatesAcrossEnrollments(t *testing.T) {
	students := oneChild()
	students.enrollments = []client.Enrollment{
		{ID: "enr-1", StudentID: "stu-1", Status: "transferred"},
		{ID: "enr-2", StudentID: "stu-1", Status: "active"},
	}
	grades := &fakeFamilyGrades{byEnrollment: map[string][]client.GradeEntry{
		"enr-1": {{ID: "grd-1", EnrollmentID: "enr-1"}},
		"enr-2": {{ID: "grd-2", EnrollmentID: "enr-2"}},
	}}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, grades, nil, nil)

	rec := httptest.NewRecorder()
	h.Grades(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/grades", "usr-9", "studentId", "stu-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		GradeEntries []map[string]any `json:"gradeEntries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.GradeEntries) != 2 {
		t.Fatalf("gradeEntries = %d, want 2 (across both enrollments)", len(out.GradeEntries))
	}
}

// The family grades screen is unrenderable with bare UUIDs, so the family route
// (only) enriches each entry with the subject and term display names, resolved
// once per request from the config catalog.
func TestFamilyGradesEnrichesSubjectAndTermNames(t *testing.T) {
	students := oneChild()
	students.enrollments = []client.Enrollment{
		{ID: "enr-1", StudentID: "stu-1", Status: "active"},
	}
	grades := &fakeFamilyGrades{byEnrollment: map[string][]client.GradeEntry{
		"enr-1": {
			{ID: "grd-1", EnrollmentID: "enr-1", SubjectID: "sub-1", TermID: "trm-1", Value: 8.5},
			{ID: "grd-2", EnrollmentID: "enr-1", SubjectID: "sub-2", TermID: "trm-1", Value: 9.0},
		},
	}}
	config := &fakeFamilyConfig{
		subjects: []client.SubjectResult{
			{ID: "sub-1", Name: "Matemáticas"},
			{ID: "sub-2", Name: "Español"},
		},
		terms: []client.TermResult{{ID: "trm-1", Name: "Bloque 1"}},
	}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, grades, nil, config)

	rec := httptest.NewRecorder()
	h.Grades(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/grades", "usr-9", "studentId", "stu-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// The id→name maps are built once, not per entry: two entries, one catalog read.
	if config.gotListSubjects != 1 || config.gotListTerms != 1 {
		t.Errorf("catalog reads = subjects %d / terms %d, want exactly 1 each per request", config.gotListSubjects, config.gotListTerms)
	}
	var out struct {
		GradeEntries []map[string]any `json:"gradeEntries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.GradeEntries) != 2 {
		t.Fatalf("gradeEntries = %d, want 2", len(out.GradeEntries))
	}
	byID := map[string]map[string]any{}
	for _, e := range out.GradeEntries {
		byID[e["id"].(string)] = e
	}
	// The existing bare-id fields are kept AND the readable names are added.
	if byID["grd-1"]["subjectId"] != "sub-1" || byID["grd-1"]["subjectName"] != "Matemáticas" ||
		byID["grd-1"]["termId"] != "trm-1" || byID["grd-1"]["termName"] != "Bloque 1" {
		t.Errorf("grd-1 = %v, want ids kept and names enriched", byID["grd-1"])
	}
	if byID["grd-2"]["subjectName"] != "Español" || byID["grd-2"]["termName"] != "Bloque 1" {
		t.Errorf("grd-2 = %v, want Español / Bloque 1", byID["grd-2"])
	}
}

// A catalog lookup miss must never fail the grades read: the names degrade to
// empty strings (like the /me enrichment) while the entries still come back 200.
func TestFamilyGradesConfigErrorDegradesToEmptyNames(t *testing.T) {
	students := oneChild()
	students.enrollments = []client.Enrollment{{ID: "enr-1", StudentID: "stu-1", Status: "active"}}
	grades := &fakeFamilyGrades{byEnrollment: map[string][]client.GradeEntry{
		"enr-1": {{ID: "grd-1", EnrollmentID: "enr-1", SubjectID: "sub-1", TermID: "trm-1", Value: 8.5}},
	}}
	config := &fakeFamilyConfig{
		subjectsErr: connect.NewError(connect.CodeUnavailable, errString("config down")),
		termsErr:    connect.NewError(connect.CodeUnavailable, errString("config down")),
	}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, grades, nil, config)

	rec := httptest.NewRecorder()
	h.Grades(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/grades", "usr-9", "studentId", "stu-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the catalog error", rec.Code)
	}
	var out struct {
		GradeEntries []map[string]any `json:"gradeEntries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.GradeEntries) != 1 {
		t.Fatalf("gradeEntries = %d, want 1 (the entry still comes back)", len(out.GradeEntries))
	}
	e := out.GradeEntries[0]
	if e["subjectId"] != "sub-1" || e["subjectName"] != "" || e["termName"] != "" {
		t.Errorf("entry = %v, want ids kept and names degraded to empty strings", e)
	}
}

func TestFamilyGradesUnlinkedChildIs404AndDoesNotRead(t *testing.T) {
	students := oneChild()
	grades := &fakeFamilyGrades{}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, grades, nil, nil)

	rec := httptest.NewRecorder()
	h.Grades(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-999/grades", "usr-9", "studentId", "stu-999"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if students.gotEnrollmentsStudentID != "" || len(grades.gotEnrollmentIDs) != 0 {
		t.Errorf("an unlinked child must never reach enrollment resolution or grades")
	}
}

func TestFamilyAttendanceOwnedChildResolvesEnrollmentThenReads(t *testing.T) {
	students := oneChild()
	students.enrollments = []client.Enrollment{{ID: "enr-1", StudentID: "stu-1", Status: "active"}}
	attendance := &fakeFamilyAttendance{byEnrollment: map[string][]client.AttendanceRecord{
		"enr-1": {{ID: "att-1", EnrollmentID: "enr-1", Date: "2026-09-01", Status: "absent"}},
	}}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, nil, attendance, nil)

	rec := httptest.NewRecorder()
	h.Attendance(rec, guardianRequest(http.MethodGet,
		"/api/v1/family/students/stu-1/attendance?from=2026-09-01&to=2026-09-30", "usr-9", "studentId", "stu-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(attendance.gotEnrollmentIDs) != 1 || attendance.gotEnrollmentIDs[0] != "enr-1" {
		t.Errorf("attendance read for %v, want [enr-1]", attendance.gotEnrollmentIDs)
	}
	if attendance.gotFrom != "2026-09-01" || attendance.gotTo != "2026-09-30" {
		t.Errorf("range = %q..%q, want the from/to passthrough", attendance.gotFrom, attendance.gotTo)
	}
	var out struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Records) != 1 || out.Records[0]["id"] != "att-1" {
		t.Errorf("records = %v, want att-1", out.Records)
	}
}

func TestFamilyAttendanceUnlinkedChildIs404AndDoesNotRead(t *testing.T) {
	students := oneChild()
	attendance := &fakeFamilyAttendance{}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{}, nil, nil, attendance, nil)

	rec := httptest.NewRecorder()
	h.Attendance(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-999/attendance", "usr-9", "studentId", "stu-999"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if students.gotEnrollmentsStudentID != "" || len(attendance.gotEnrollmentIDs) != 0 {
		t.Errorf("an unlinked child must never reach enrollment resolution or attendance")
	}
}

// --- B6.5 guardian mark-read ----------------------------------------------

func TestFamilyMarkReadOwnedNotificationMarks(t *testing.T) {
	students := &fakeFamilyStudents{links: []client.GuardianLink{
		{StudentID: "stu-1"}, {StudentID: "stu-2"},
	}}
	comms := &fakeFamilyComms{byStudent: map[string][]client.Notification{
		"stu-1": {{ID: "ntf-1", StudentID: "stu-1"}},
		"stu-2": {{ID: "ntf-2", StudentID: "stu-2"}},
	}}
	h := handler.NewFamilyHandler(students, comms, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.MarkNotificationRead(rec, guardianRequest(http.MethodPost,
		"/api/v1/family/notifications/ntf-2/read", "usr-9", "notificationId", "ntf-2"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if comms.marked != "ntf-2" {
		t.Errorf("marked %q, want the child's own ntf-2", comms.marked)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "ntf-2" {
		t.Errorf("response id = %v, want ntf-2", out["id"])
	}
}

func TestFamilyMarkReadOtherFamilysNotificationIs404AndDoesNotMark(t *testing.T) {
	students := oneChild() // linked only to stu-1
	comms := &fakeFamilyComms{byStudent: map[string][]client.Notification{
		"stu-1": {{ID: "ntf-1", StudentID: "stu-1"}},
	}}
	h := handler.NewFamilyHandler(students, comms, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	// ntf-x belongs to a child this guardian is not linked to.
	h.MarkNotificationRead(rec, guardianRequest(http.MethodPost,
		"/api/v1/family/notifications/ntf-x/read", "usr-9", "notificationId", "ntf-x"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (a guardian cannot mark another family's notification)", rec.Code)
	}
	if comms.marked != "" {
		t.Errorf("marked %q; a notification outside the caller's children must never be marked", comms.marked)
	}
}

func TestFamilyByChildRoutesWithoutPrincipalAre401(t *testing.T) {
	h := handler.NewFamilyHandler(oneChild(), &fakeFamilyComms{}, &fakeFamilyFinance{}, &fakeFamilyGrades{}, &fakeFamilyAttendance{}, nil)
	routes := map[string]http.HandlerFunc{
		"account":    h.Account,
		"profile":    h.Profile,
		"grades":     h.Grades,
		"attendance": h.Attendance,
		"markRead":   h.MarkNotificationRead,
	}
	for name, fn := range routes {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/family/students/stu-1/"+name, nil)
		req.SetPathValue("studentId", "stu-1")
		req.SetPathValue("notificationId", "ntf-1")
		fn(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without principal = %d, want 401", name, rec.Code)
		}
	}
}

// A parent who overpaid must see the credit the school is holding for them. The
// school's own statement carries it; showing the family only the debt would
// tell them they owe money that is already paid.
func TestFamilyAccountShowsTheCredit(t *testing.T) {
	finance := &fakeFamilyFinance{account: client.AccountStatement{
		TotalBalanceCents: 320000,
		UnallocatedCents:  180000,
	}}
	h := handler.NewFamilyHandler(oneChild(), &fakeFamilyComms{}, finance, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.Account(rec, guardianRequest(http.MethodGet, "/api/v1/family/students/stu-1/account", "usr-9", "studentId", "stu-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["unallocatedCents"] != float64(180000) {
		t.Errorf("unallocatedCents = %v, want 180000", body["unallocatedCents"])
	}
}
