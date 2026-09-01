package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

type fakeAnnouncer struct {
	studentIDs []string
	title      string
	body       string
	key        string
	created    int
	err        error
}

func (f *fakeAnnouncer) Announce(_ context.Context, studentIDs []string, title, body, key string) (int, error) {
	f.studentIDs, f.title, f.body, f.key = studentIDs, title, body, key
	return f.created, f.err
}

type fakeAudience struct {
	enrollments []client.Enrollment
	groups      []client.GroupResult
	gradeLevels []client.GradeLevelResult
	gotGroupID  string
	gotLevelID  string
}

func (f *fakeAudience) ListEnrollments(_ context.Context, _, _, groupID string) ([]client.Enrollment, error) {
	f.gotGroupID = groupID
	out := make([]client.Enrollment, 0, len(f.enrollments))
	for _, e := range f.enrollments {
		if groupID == "" || e.GroupID == groupID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeAudience) ListGradeLevels(_ context.Context, levelID string) ([]client.GradeLevelResult, error) {
	f.gotLevelID = levelID
	return f.gradeLevels, nil
}

func (f *fakeAudience) ListGroups(_ context.Context, _, gradeLevelID string) ([]client.GroupResult, error) {
	out := make([]client.GroupResult, 0, len(f.groups))
	for _, g := range f.groups {
		if gradeLevelID == "" || g.GradeLevelID == gradeLevelID {
			out = append(out, g)
		}
	}
	return out, nil
}

func newAnnouncementHandler(created int) (*handler.AnnouncementHandler, *fakeAnnouncer, *fakeAudience) {
	comms := &fakeAnnouncer{created: created}
	audience := &fakeAudience{
		enrollments: []client.Enrollment{
			{StudentID: "s1", GroupID: "g1", CycleID: "c1", Status: "active"},
			{StudentID: "s2", GroupID: "g1", CycleID: "c1", Status: "withdrawn"},
			{StudentID: "s3", GroupID: "g2", CycleID: "c1", Status: "active"},
		},
		groups:      []client.GroupResult{{ID: "g1", GradeLevelID: "gl1"}, {ID: "g2", GradeLevelID: "gl2"}},
		gradeLevels: []client.GradeLevelResult{{ID: "gl1", LevelID: "lv1"}, {ID: "gl2", LevelID: "lv1"}},
	}
	return handler.NewAnnouncementHandler(comms, audience, audience), comms, audience
}

// One message, one group. The school writes it once instead of twenty-eight
// times and missing two.
func TestAnnounceToAGroup(t *testing.T) {
	h, comms, _ := newAnnouncementHandler(1)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements",
		`{"scope":"group","targetId":"g1","cycleId":"c1","title":"Junta","body":"Jueves 8:00","idempotencyKey":"k1"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	// A pupil who left the group mid-year is not in it. Sending them the junta
	// is how a school looks like it does not know who its pupils are.
	if len(comms.studentIDs) != 1 || comms.studentIDs[0] != "s1" {
		t.Errorf("audience = %v, want only the active pupil of g1", comms.studentIDs)
	}
	if comms.key != "k1" {
		t.Errorf("idempotency key = %q, want it forwarded", comms.key)
	}
}

// A level is every group under every grade of that level. Making the office
// send it per group is the same problem one level up.
func TestAnnounceToALevelReachesEveryGroupUnderIt(t *testing.T) {
	h, comms, _ := newAnnouncementHandler(2)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements",
		`{"scope":"level","targetId":"lv1","cycleId":"c1","title":"Suspensión","body":"Viernes no hay clases","idempotencyKey":"k2"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if len(comms.studentIDs) != 2 {
		t.Errorf("audience = %v, want the active pupils of both groups", comms.studentIDs)
	}
}

// The whole school, for the days when that is the message.
func TestAnnounceToTheWholeCycle(t *testing.T) {
	h, comms, _ := newAnnouncementHandler(2)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements",
		`{"scope":"cycle","cycleId":"c1","title":"Aviso","body":"Mañana","idempotencyKey":"k3"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if len(comms.studentIDs) != 2 {
		t.Errorf("audience = %v, want every active pupil", comms.studentIDs)
	}
}

// A group that matched nobody is a mistake worth hearing about, not a silent
// success the office reads as "sent".
func TestAnnounceToAnEmptyAudienceIsRefused(t *testing.T) {
	h, comms, _ := newAnnouncementHandler(0)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements",
		`{"scope":"group","targetId":"g9","cycleId":"c1","title":"Junta","body":"Jueves","idempotencyKey":"k4"}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if comms.studentIDs != nil {
		t.Error("comms was asked to send to nobody")
	}
}

func TestAnnounceNeedsAKnownScope(t *testing.T) {
	h, _, _ := newAnnouncementHandler(0)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements",
		`{"scope":"planeta","cycleId":"c1","title":"Junta","body":"Jueves","idempotencyKey":"k5"}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// The note that goes to one family and nobody else. A school sends more of
// these than it sends circulars.
func TestAnnounceToOneStudent(t *testing.T) {
	h, comms, _ := newAnnouncementHandler(1)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements",
		`{"scope":"student","targetId":"s1","cycleId":"c1","title":"Recordatorio","body":"Traer uniforme","idempotencyKey":"k6"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if len(comms.studentIDs) != 1 || comms.studentIDs[0] != "s1" {
		t.Errorf("audience = %v, want the one pupil named", comms.studentIDs)
	}
}

// "1° A y 1° B" is one message the school writes once — not two it writes twice
// and sends at different times.
func TestAnnounceToSeveralGroupsAtOnce(t *testing.T) {
	h, comms, _ := newAnnouncementHandler(2)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements",
		`{"scope":"group","targetIds":["g1","g2"],"cycleId":"c1","title":"Junta","body":"Jueves","idempotencyKey":"k7"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if len(comms.studentIDs) != 2 {
		t.Errorf("audience = %v, want the active pupils of both groups", comms.studentIDs)
	}
}

// A pupil in two of the chosen groups hears it once. Two notifications for one
// announcement is how a school teaches parents to ignore them.
func TestAPupilInTwoChosenGroupsHearsItOnce(t *testing.T) {
	h, comms, audience := newAnnouncementHandler(1)
	audience.enrollments = append(audience.enrollments,
		client.Enrollment{StudentID: "s1", GroupID: "g2", CycleID: "c1", Status: "active"})

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements",
		`{"scope":"group","targetIds":["g1","g2"],"cycleId":"c1","title":"Junta","body":"Jueves","idempotencyKey":"k8"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	seen := map[string]int{}
	for _, id := range comms.studentIDs {
		seen[id]++
	}
	if seen["s1"] != 1 {
		t.Errorf("s1 appears %d times in the audience, want 1", seen["s1"])
	}
}
