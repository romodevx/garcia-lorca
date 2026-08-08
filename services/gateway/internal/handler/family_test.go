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

// fakeFamilyStudents doubles the guardian-link lookup: it records the guardian
// id the handler asked for, which is the assertion that matters — the child set
// must be derived from the authenticated principal, never from the request.
type fakeFamilyStudents struct {
	gotGuardianID string
	links         []client.GuardianLink
	err           error
}

func (f *fakeFamilyStudents) ListStudentsForGuardian(_ context.Context, guardianUserID string) ([]client.GuardianLink, error) {
	f.gotGuardianID = guardianUserID
	return f.links, f.err
}

// fakeFamilyComms doubles the notification inbox, keyed by student so the
// aggregation across several children is observable.
type fakeFamilyComms struct {
	gotStudentIDs []string
	gotUnreadOnly bool
	byStudent     map[string][]client.Notification
	err           error
}

func (f *fakeFamilyComms) ListNotifications(_ context.Context, studentID string, unreadOnly bool) ([]client.Notification, error) {
	f.gotStudentIDs = append(f.gotStudentIDs, studentID)
	f.gotUnreadOnly = unreadOnly
	if f.err != nil {
		return nil, f.err
	}
	return f.byStudent[studentID], nil
}

// guardianRequest builds an authenticated GET against the family surface, with
// userID as the principal the middleware would have injected.
func guardianRequest(target, userID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{
		UserID: userID, TenantID: "t1", Email: "mama@example.com", Permissions: []string{"family:read"},
	}))
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
	h := handler.NewFamilyHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianRequest("/api/v1/family/notifications", "usr-9"))

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
	h := handler.NewFamilyHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianRequest("/api/v1/family/notifications", "usr-9"))

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
	h := handler.NewFamilyHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianRequest("/api/v1/family/notifications", "usr-9"))

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
	h := handler.NewFamilyHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianRequest("/api/v1/family/notifications", "usr-lonely"))

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
	h := handler.NewFamilyHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianRequest(
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
	h := handler.NewFamilyHandler(students, comms)

	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianRequest("/api/v1/family/notifications?unreadOnly=true", "usr-9"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !comms.gotUnreadOnly {
		t.Error("unreadOnly=true was not forwarded to comms")
	}
}

func TestFamilyNotificationsWithoutPrincipalIs401(t *testing.T) {
	h := handler.NewFamilyHandler(&fakeFamilyStudents{}, &fakeFamilyComms{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/family/notifications", nil)
	h.Notifications(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestFamilyNotificationsPropagatesLinkLookupError(t *testing.T) {
	students := &fakeFamilyStudents{err: connect.NewError(connect.CodePermissionDenied, errString("nope"))}
	h := handler.NewFamilyHandler(students, &fakeFamilyComms{})
	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianRequest("/api/v1/family/notifications", "usr-9"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// One child's inbox failing must fail the whole read: a partial list would look
// like "your child has no news", which is worse than an error.
func TestFamilyNotificationsPropagatesInboxError(t *testing.T) {
	students := &fakeFamilyStudents{links: []client.GuardianLink{{StudentID: "stu-1"}}}
	comms := &fakeFamilyComms{err: connect.NewError(connect.CodeUnavailable, errString("comms down"))}
	h := handler.NewFamilyHandler(students, comms)
	rec := httptest.NewRecorder()
	h.Notifications(rec, guardianRequest("/api/v1/family/notifications", "usr-9"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}
