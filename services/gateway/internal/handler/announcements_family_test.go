package handler_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// ADR 0041 (T5): "Familias" is an announcement audience, resolved here to the
// pupils of each family, like a group.

const (
	annFamA = "0b1e5f0a-0000-4000-8000-00000000000a"
	annFamB = "0b1e5f0a-0000-4000-8000-00000000000b"
	annFamC = "0b1e5f0a-0000-4000-8000-00000000000c"
)

// familyPupils answers ListStudentsInFamily from a fixed map and records the
// ids it was asked for.
type familyPupils struct {
	byFamily map[string][]client.StudentResult
	asked    []string
}

func (f *familyPupils) ListStudentsInFamily(_ context.Context, familyID, _ string) ([]client.StudentResult, error) {
	f.asked = append(f.asked, familyID)
	return f.byFamily[familyID], nil
}

func newFamilyAnnouncementHandler(on bool) (*handler.AnnouncementHandler, *fakeAnnouncer, *familyPupils) {
	comms := &fakeAnnouncer{created: 1}
	audience := &fakeAudience{}
	pupils := &familyPupils{byFamily: map[string][]client.StudentResult{
		annFamA: {
			{ID: "s1", AdmissionStatus: "enrolled"},
			{ID: "s2", AdmissionStatus: "withdrawn"},
			{ID: "s3", AdmissionStatus: "enrolled"},
		},
		// s3 sits in both: a pupil the office put in two families hears it once.
		annFamB: {
			{ID: "s3", AdmissionStatus: "enrolled"},
			{ID: "s4", AdmissionStatus: "rejected"},
			{ID: "s5", AdmissionStatus: "accepted"},
		},
		annFamC: {{ID: "s6", AdmissionStatus: "withdrawn"}},
	}}
	enabled := func(context.Context) bool { return on }
	h := handler.NewAnnouncementHandler(comms, audience, audience, handler.WithFamilyAudience(enabled, pupils))
	return h, comms, pupils
}

func familyBody(ids ...string) string {
	return `{"scope":"family","targetIds":["` + strings.Join(ids, `","`) +
		`"],"cycleId":"c1","title":"Junta","body":"Jueves 8:00","idempotencyKey":"k1"}`
}

func TestAnnounceToFamiliesResolvesTheirCurrentPupilsOnce(t *testing.T) {
	h, comms, pupils := newFamilyAnnouncementHandler(true)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements", familyBody(annFamA, annFamB))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	// Withdrawn and rejected pupils left; s3 is in both families and appears once,
	// in the order the families were named.
	if want := []string{"s1", "s3", "s5"}; !slices.Equal(comms.studentIDs, want) {
		t.Errorf("audience = %v, want %v", comms.studentIDs, want)
	}
	if want := []string{annFamA, annFamB}; !slices.Equal(pupils.asked, want) {
		t.Errorf("families read = %v, want %v", pupils.asked, want)
	}
	if !strings.Contains(rec.Body.String(), `"audience":3`) {
		t.Errorf("body = %s, want audience 3", rec.Body.String())
	}
}

// Upper-case ids are the same family: canonicalised before the lookup.
func TestAnnounceToFamilyCanonicalisesTheID(t *testing.T) {
	h, comms, _ := newFamilyAnnouncementHandler(true)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements", familyBody(strings.ToUpper(annFamA)))

	if rec.Code != http.StatusCreated || len(comms.studentIDs) != 2 {
		t.Fatalf("status = %d, audience = %v", rec.Code, comms.studentIDs)
	}
}

func TestAnnounceToFamilyIsRefusedWhileFamiliesAreOff(t *testing.T) {
	h, comms, pupils := newFamilyAnnouncementHandler(false)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements", familyBody(annFamA))

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "families are not enabled") {
		t.Fatalf("status = %d, body = %s; want 400 families are not enabled", rec.Code, rec.Body.String())
	}
	if comms.studentIDs != nil || len(pupils.asked) != 0 {
		t.Errorf("nothing may be read or sent while families are off")
	}
}

// Without the option at all (a gateway that never wired families) the scope is
// refused the same way, never resolved to somebody else.
func TestAnnounceToFamilyWithoutTheOptionIsRefused(t *testing.T) {
	h, comms, _ := newAnnouncementHandler(1)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements", familyBody(annFamA))

	if rec.Code != http.StatusBadRequest || comms.studentIDs != nil {
		t.Fatalf("status = %d, audience = %v", rec.Code, comms.studentIDs)
	}
}

func TestAnnounceToFamilyRejectsBadTargets(t *testing.T) {
	tooMany := make([]string, 51)
	for i := range tooMany {
		tooMany[i] = annFamA
	}
	cases := map[string]string{
		"not a uuid": familyBody("family-1"),
		"no target":  `{"scope":"family","cycleId":"c1","title":"Junta","body":"Jueves","idempotencyKey":"k1"}`,
		"over 50":    familyBody(tooMany...),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			h, comms, pupils := newFamilyAnnouncementHandler(true)

			rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements", body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s; want 400", rec.Code, rec.Body.String())
			}
			if comms.studentIDs != nil || len(pupils.asked) != 0 {
				t.Errorf("a bad target must be refused before any read")
			}
		})
	}
}

// A family whose pupils all left is an audience of nobody: refused like an empty
// group, so the office never believes it reached anyone.
func TestAnnounceToAFamilyOfNobodyIsRefused(t *testing.T) {
	h, comms, _ := newFamilyAnnouncementHandler(true)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements", familyBody(annFamC))

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "nadie coincide") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if comms.studentIDs != nil {
		t.Errorf("sent to %v", comms.studentIDs)
	}
}

// brokenFamilies is a students service that is down.
type brokenFamilies struct{}

func (brokenFamilies) ListStudentsInFamily(context.Context, string, string) ([]client.StudentResult, error) {
	return nil, connect.NewError(connect.CodeUnavailable, errors.New("students down"))
}

// A students outage is not the office's mistake: it is answered as the outage
// it is, never as a 400 that reads like a wrong audience.
func TestAnnounceToFamilyReportsAnOutageAsAnOutage(t *testing.T) {
	comms := &fakeAnnouncer{created: 1}
	audience := &fakeAudience{}
	on := func(context.Context) bool { return true }
	h := handler.NewAnnouncementHandler(comms, audience, audience, handler.WithFamilyAudience(on, brokenFamilies{}))

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements", familyBody(annFamA))

	// The gateway's shared mapping (httpStatusFor) answers Unavailable as 500.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if comms.studentIDs != nil {
		t.Errorf("sent to %v during an outage", comms.studentIDs)
	}
}

// Drafts are imported records the school has not confirmed: they have no
// accounts and are not attending, so a family announcement leaves them out.
func TestAnnounceToFamilyLeavesOutDrafts(t *testing.T) {
	comms := &fakeAnnouncer{created: 1}
	audience := &fakeAudience{}
	pupils := &familyPupils{byFamily: map[string][]client.StudentResult{annFamA: {
		{ID: "s1", AdmissionStatus: "pending"},
		{ID: "s2", AdmissionStatus: "draft"},
		{ID: "s3", AdmissionStatus: "accepted"},
		{ID: "s4", AdmissionStatus: "enrolled"},
		{ID: "s5", AdmissionStatus: ""},
	}}}
	on := func(context.Context) bool { return true }
	h := handler.NewAnnouncementHandler(comms, audience, audience, handler.WithFamilyAudience(on, pupils))

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements", familyBody(annFamA))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if want := []string{"s1", "s3", "s4"}; !slices.Equal(comms.studentIDs, want) {
		t.Errorf("audience = %v, want only pending/accepted/enrolled %v", comms.studentIDs, want)
	}
}

// plainFailure is a reader failing with an error that is neither a downstream
// Connect error nor an audience sentence (e.g. the client's page cap).
type plainFailure struct{}

func (plainFailure) ListStudentsInFamily(context.Context, string, string) ([]client.StudentResult, error) {
	return nil, errors.New("students list exceeded 40 pages for family 0b1e…")
}

// Only the handler's own audience sentences are a 400. Any other failure is a
// 500 with a generic text: its wording is internal, not the office's mistake.
func TestAnnounceInternalFailureIsA500WithoutItsText(t *testing.T) {
	comms := &fakeAnnouncer{created: 1}
	audience := &fakeAudience{}
	on := func(context.Context) bool { return true }
	h := handler.NewAnnouncementHandler(comms, audience, audience, handler.WithFamilyAudience(on, plainFailure{}))

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements", familyBody(annFamA))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "pages") {
		t.Errorf("body leaks the internal error: %s", rec.Body.String())
	}
}

// An unknown scope is still the caller's mistake, answered 400 with its sentence.
func TestAnnounceUnknownScopeStaysA400(t *testing.T) {
	h, _, _ := newFamilyAnnouncementHandler(true)

	rec := doReq(t, h.Announce, http.MethodPost, "/api/v1/comms/announcements",
		`{"scope":"everybody","cycleId":"c1","title":"Junta","body":"Jueves","idempotencyKey":"k1"}`)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "scope must be") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
