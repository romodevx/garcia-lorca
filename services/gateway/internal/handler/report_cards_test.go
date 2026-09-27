package handler_test

import (
	"bytes"
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

const (
	rcID       = "7c000000-0000-0000-0000-000000000001"
	rcStudent  = "57000000-0000-0000-0000-000000000001"
	rcStranger = "57000000-0000-0000-0000-0000000000ff"
	rcGuardian = "90000000-0000-0000-0000-000000000001"
	rcGroup    = "40000000-0000-0000-0000-000000000001"
)

var rcIssued = time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)

type fakeReportCards struct {
	list      []client.ReportCardSummary
	card      client.ReportCard
	err       error
	gotFilter client.ReportCardFilter
	gotID     string
	calls     int
}

func (f *fakeReportCards) ListReportCards(_ context.Context, filter client.ReportCardFilter) ([]client.ReportCardSummary, error) {
	f.calls++
	f.gotFilter = filter
	return f.list, f.err
}

func (f *fakeReportCards) GetReportCard(_ context.Context, id string) (client.ReportCard, error) {
	f.calls++
	f.gotID = id
	return f.card, f.err
}

type fakeProfiles struct{ profile client.SchoolProfile }

func (f fakeProfiles) GetSchoolProfile(context.Context) (client.SchoolProfile, bool, error) {
	return f.profile, true, nil
}

type fakeLinks struct{ children []string }

func (f fakeLinks) ListStudentsForGuardian(_ context.Context, _ string) ([]client.GuardianLink, error) {
	out := make([]client.GuardianLink, 0, len(f.children))
	for _, c := range f.children {
		out = append(out, client.GuardianLink{StudentID: c})
	}
	return out, nil
}

func issuedCard() client.ReportCard {
	return client.ReportCard{
		ID: rcID, TermID: pTerm, CycleID: pCycle, LevelID: "lvl-1", GroupID: rcGroup, EnrollmentID: "enr-1", StudentID: rcStudent,
		Version: 2, Status: "published", IssuedAt: rcIssued, IssuedBy: pDir, StudentName: "Ana López", EnrollmentNumber: "2026-00001",
		GroupLabel: "1° · A", LevelName: "Primaria", CycleName: "2026-2027", TermName: "1.er trimestre",
		Months: []string{"2026-09", "2026-10"}, GeneralAverage: "8.5",
		Scale: client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		Lines: []client.ReportCardLine{
			{SubjectID: "sub-1", SubjectName: "Matemáticas", Position: 1, MonthValues: map[string]string{"2026-09": "8.4", "2026-10": "8.5"}, TermAverage: "8.5"},
			{SubjectID: "sub-2", SubjectName: "Artes", Position: 2, MonthValues: map[string]string{}, TermAverage: ""},
		},
	}
}

func newReportCardsHandler(f *fakeReportCards, children ...string) *handler.ReportCardsHandler {
	return handler.NewReportCardsHandler(f, fakeProfiles{profile: client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA", CCT: "09PPR0001A"}}, fakeLinks{children: children})
}

func guardianReq(target string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: rcGuardian, TenantID: "t1"}))
}

// --- director ----------------------------------------------------------------

func TestDirectorListsTheLatestReportCards(t *testing.T) {
	f := &fakeReportCards{list: []client.ReportCardSummary{{ID: rcID, TermID: pTerm, TermName: "1.er trimestre", StudentID: rcStudent,
		StudentName: "Ana López", GroupID: rcGroup, GroupLabel: "1° · A", Version: 2, IssuedAt: rcIssued, GeneralAverage: "8.5"},
		{ID: "rc-2", StudentID: "stu-2", StudentName: "Beto", Version: 1, IssuedAt: rcIssued}}}
	h := newReportCardsHandler(f)
	rec := httptest.NewRecorder()
	h.List(rec, closerReq(http.MethodGet, "/api/v1/grades/report-cards?termId="+pTerm+"&groupId="+strings.ToUpper(rcGroup), ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, client.ReportCardFilter{TermID: pTerm, GroupID: rcGroup}, f.gotFilter)
	assert.JSONEq(t, `{"reportCards":[
		{"id":"`+rcID+`","termId":"`+pTerm+`","termName":"1.er trimestre","studentId":"`+rcStudent+`","studentName":"Ana López",
		 "groupId":"`+rcGroup+`","groupLabel":"1° · A","version":2,"issuedAt":"2026-12-01T10:00:00Z","generalAverage":8.5,"hasGrades":true},
		{"id":"rc-2","termId":"","termName":"","studentId":"stu-2","studentName":"Beto","groupId":"","groupLabel":"","version":1,
		 "issuedAt":"2026-12-01T10:00:00Z","generalAverage":null,"hasGrades":false}
	]}`, rec.Body.String())
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	for _, bad := range []string{"", "?termId=nope", "?groupId=nope"} {
		rec := httptest.NewRecorder()
		h.List(rec, closerReq(http.MethodGet, "/api/v1/grades/report-cards"+bad, ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code, bad)
	}
}

func TestDirectorReadsAReportCard(t *testing.T) {
	f := &fakeReportCards{card: issuedCard()}
	h := newReportCardsHandler(f)
	req := closerReq(http.MethodGet, "/x", "")
	req.SetPathValue("id", strings.ToUpper(rcID))
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, rcID, f.gotID)
	assert.JSONEq(t, `{
		"id":"`+rcID+`","termId":"`+pTerm+`","termName":"1.er trimestre","cycleName":"2026-2027","levelName":"Primaria",
		"groupId":"`+rcGroup+`","groupLabel":"1° · A","studentId":"`+rcStudent+`","studentName":"Ana López","enrollmentNumber":"2026-00001",
		"version":2,"status":"published","issuedAt":"2026-12-01T10:00:00Z","months":["2026-09","2026-10"],
		"scale":{"min":5,"max":10,"decimals":1,"passing":6},"generalAverage":8.5,"hasGrades":true,
		"lines":[
			{"subjectId":"sub-1","subjectName":"Matemáticas","position":1,"months":{"2026-09":8.4,"2026-10":8.5},"average":8.5},
			{"subjectId":"sub-2","subjectName":"Artes","position":2,"months":{},"average":null}
		],
		"school":{"name":"COLEGIO GARCÍA LORCA","legalName":"","rfc":"","cct":"09PPR0001A","address":"","phone":"","legend":""}
	}`, rec.Body.String())

	f.err = connect.NewError(connect.CodeNotFound, nil)
	rec = httptest.NewRecorder()
	h.Get(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDirectorDownloadsTheReportCardPDF(t *testing.T) {
	h := newReportCardsHandler(&fakeReportCards{card: issuedCard()})
	req := closerReq(http.MethodGet, "/x", "")
	req.SetPathValue("id", rcID)
	rec := httptest.NewRecorder()
	h.PDF(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="boleta-2026-00001-1-er-trimestre-v2.pdf"`, rec.Header().Get("Content-Disposition"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.True(t, bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")))
}

// --- family ------------------------------------------------------------------

func TestFamilyListsOwnChildsReportCards(t *testing.T) {
	f := &fakeReportCards{list: []client.ReportCardSummary{{ID: rcID, TermID: pTerm, TermName: "1.er trimestre", StudentID: rcStudent,
		Version: 2, IssuedAt: rcIssued, GeneralAverage: "8.5"}}}
	h := newReportCardsHandler(f, rcStudent)
	req := guardianReq("/x")
	req.SetPathValue("studentId", rcStudent)
	rec := httptest.NewRecorder()
	h.FamilyList(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, client.ReportCardFilter{StudentID: rcStudent}, f.gotFilter)
	assert.JSONEq(t, `{"reportCards":[{"id":"`+rcID+`","termId":"`+pTerm+`","termName":"1.er trimestre","version":2,
		"issuedAt":"2026-12-01T10:00:00Z","generalAverage":8.5,"hasGrades":true}]}`, rec.Body.String())
}

// Another family's child is 404 on every family route, and grades is never asked.
func TestFamilyRoutesRefuseAnotherChild(t *testing.T) {
	f := &fakeReportCards{card: issuedCard()}
	h := newReportCardsHandler(f, rcStudent)
	for name, serve := range map[string]http.HandlerFunc{"list": h.FamilyList, "json": h.FamilyGet, "pdf": h.FamilyPDF} {
		req := guardianReq("/x")
		req.SetPathValue("studentId", rcStranger)
		req.SetPathValue("id", rcID)
		rec := httptest.NewRecorder()
		serve(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, name)
	}
	assert.Zero(t, f.calls)
}

// A report card of ANOTHER pupil, asked through one's own child, is 404 too.
func TestFamilyCannotReadAnotherPupilsReportCard(t *testing.T) {
	card := issuedCard()
	card.StudentID = rcStranger
	h := newReportCardsHandler(&fakeReportCards{card: card}, rcStudent)
	for name, serve := range map[string]http.HandlerFunc{"json": h.FamilyGet, "pdf": h.FamilyPDF} {
		req := guardianReq("/x")
		req.SetPathValue("studentId", rcStudent)
		req.SetPathValue("id", rcID)
		rec := httptest.NewRecorder()
		serve(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, name)
	}
}

// Only the latest version reaches a family: a superseded card is 404.
func TestFamilySeesOnlyThePublishedVersion(t *testing.T) {
	card := issuedCard()
	card.Status = "superseded"
	h := newReportCardsHandler(&fakeReportCards{card: card}, rcStudent)
	req := guardianReq("/x")
	req.SetPathValue("studentId", rcStudent)
	req.SetPathValue("id", rcID)
	rec := httptest.NewRecorder()
	h.FamilyGet(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFamilyDownloadsOwnChildsPDF(t *testing.T) {
	h := newReportCardsHandler(&fakeReportCards{card: issuedCard()}, rcStudent)
	req := guardianReq("/x")
	req.SetPathValue("studentId", rcStudent)
	req.SetPathValue("id", rcID)
	rec := httptest.NewRecorder()
	h.FamilyPDF(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
	assert.True(t, bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")))
}
