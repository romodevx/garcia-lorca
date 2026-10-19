package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/garcia-lorca/pkg/studentname"
	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
	"github.com/garcia-lorca/services/gateway/internal/reportcard"
)

// Report cards (boletas, T7): the frozen snapshots the grades service issues
// when a period closes. The director lists a period's (or a group's) latest
// cards and opens or downloads any of them (grades:read). A family reads only
// its own child's cards (family:read), and only the PUBLISHED version: a card a
// later close superseded is 404, like another family's child.

// ReportCardsService is the grades surface the report-card handler consumes.
// The Connect GradesClient satisfies it.
type ReportCardsService interface {
	ListReportCards(ctx context.Context, filter client.ReportCardFilter) ([]client.ReportCardSummary, error)
	GetReportCard(ctx context.Context, id string) (client.ReportCard, error)
}

// ReportCardsHandler serves the director's /api/v1/grades/report-cards* and the
// family's /api/v1/family/students/{studentId}/report-cards*.
type ReportCardsHandler struct {
	grades   ReportCardsService
	profiles SchoolProfileReader
	links    GuardianLinkReader
	zone     *time.Location
}

// ReportCardsOption configures a ReportCardsHandler.
type ReportCardsOption func(*ReportCardsHandler)

// WithReportCardZone sets the school's time zone, so the PDF's issue date is
// the school's day (ADR 0035). Without it the PDF prints the UTC day.
func WithReportCardZone(zone *time.Location) ReportCardsOption {
	return func(h *ReportCardsHandler) { h.zone = zone }
}

// NewReportCardsHandler wires the handler. links is the family ownership gate.
func NewReportCardsHandler(grades ReportCardsService, profiles SchoolProfileReader, links GuardianLinkReader, opts ...ReportCardsOption) *ReportCardsHandler {
	h := &ReportCardsHandler{grades: grades, profiles: profiles, links: links}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

const (
	msgReportCardFilter = "termId or groupId is required"
	reportCardPublished = "published"
)

type reportCardSummaryResponse struct {
	ID             string       `json:"id"`
	TermID         string       `json:"termId"`
	TermName       string       `json:"termName"`
	StudentID      string       `json:"studentId"`
	StudentName    string       `json:"studentName"`
	GroupID        string       `json:"groupId"`
	GroupLabel     string       `json:"groupLabel"`
	Version        int          `json:"version"`
	IssuedAt       *string      `json:"issuedAt"`
	GeneralAverage *json.Number `json:"generalAverage"`
	HasGrades      bool         `json:"hasGrades"`
}

type familyReportCardSummaryResponse struct {
	ID             string       `json:"id"`
	TermID         string       `json:"termId"`
	TermName       string       `json:"termName"`
	TermOrdinal    int          `json:"termOrdinal"`
	CycleName      string       `json:"cycleName"`
	Version        int          `json:"version"`
	IssuedAt       *string      `json:"issuedAt"`
	GeneralAverage *json.Number `json:"generalAverage"`
	HasGrades      bool         `json:"hasGrades"`
}

// reportCardLineResponse is one subject: its grade per period column (T15)
// or, on a card issued before, per month column.
type reportCardLineResponse struct {
	SubjectID   string                 `json:"subjectId"`
	SubjectName string                 `json:"subjectName"`
	Position    int                    `json:"position"`
	Periods     map[string]json.Number `json:"periods"`
	Months      map[string]json.Number `json:"months"`
	Average     *json.Number           `json:"average"`
}

// reportCardPeriodResponse is one period column of a T15 card.
type reportCardPeriodResponse struct {
	ID      string `json:"id"`
	Ordinal int    `json:"ordinal"`
	Name    string `json:"name"`
}

type reportCardResponse struct {
	ID               string                     `json:"id"`
	TermID           string                     `json:"termId"`
	TermName         string                     `json:"termName"`
	TermOrdinal      int                        `json:"termOrdinal"`
	CycleName        string                     `json:"cycleName"`
	LevelName        string                     `json:"levelName"`
	GroupID          string                     `json:"groupId"`
	GroupLabel       string                     `json:"groupLabel"`
	StudentID        string                     `json:"studentId"`
	StudentName      string                     `json:"studentName"`
	EnrollmentNumber string                     `json:"enrollmentNumber"`
	Version          int                        `json:"version"`
	Status           string                     `json:"status"`
	IssuedAt         *string                    `json:"issuedAt"`
	Periods          []reportCardPeriodResponse `json:"periods"`
	Months           []string                   `json:"months"`
	Scale            gradeScaleResponse         `json:"scale"`
	GeneralAverage   *json.Number               `json:"generalAverage"`
	HasGrades        bool                       `json:"hasGrades"`
	Lines            []reportCardLineResponse   `json:"lines"`
	School           client.SchoolProfile       `json:"school"`
}

// --- director ------------------------------------------------------------------

// List handles GET /api/v1/grades/report-cards?termId=&groupId= — the latest
// published card of each pupil (at least one filter; never the whole school).
func (h *ReportCardsHandler) List(w http.ResponseWriter, r *http.Request) {
	filter, ok := directorFilter(r)
	if !ok {
		http.Error(w, msgReportCardFilter, http.StatusBadRequest)
		return
	}
	cards, err := h.grades.ListReportCards(r.Context(), filter)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]reportCardSummaryResponse, 0, len(cards))
	for _, c := range cards {
		out = append(out, reportCardSummaryResponse{
			ID: c.ID, TermID: c.TermID, TermName: c.TermName, StudentID: c.StudentID, StudentName: c.StudentName,
			GroupID: c.GroupID, GroupLabel: c.GroupLabel, Version: c.Version, IssuedAt: rfc3339OrNil(c.IssuedAt),
			GeneralAverage: numberOrNil(c.GeneralAverage), HasGrades: summaryHasGrades(c),
		})
	}
	studentname.SortByDisplayName(out, func(c reportCardSummaryResponse) string { return c.StudentName })
	writePrivateJSON(w, map[string][]reportCardSummaryResponse{"reportCards": out})
}

// directorFilter reads termId / groupId; each present one must be a uuid, and
// at least one must be present.
func directorFilter(r *http.Request) (client.ReportCardFilter, bool) {
	q := r.URL.Query()
	var f client.ReportCardFilter
	for _, p := range []struct {
		raw string
		dst *string
	}{{q.Get("termId"), &f.TermID}, {q.Get("groupId"), &f.GroupID}} {
		if p.raw == "" {
			continue
		}
		id, ok := canonicalUUID(p.raw)
		if !ok {
			return client.ReportCardFilter{}, false
		}
		*p.dst = id
	}
	return f, f.TermID != "" || f.GroupID != ""
}

// Get handles GET /api/v1/grades/report-cards/{id} — the full snapshot and the
// school's details.
func (h *ReportCardsHandler) Get(w http.ResponseWriter, r *http.Request) {
	card, ok := h.directorCard(w, r)
	if !ok {
		return
	}
	h.serveJSON(w, r, card)
}

// PDF handles GET /api/v1/grades/report-cards/{id}/pdf.
func (h *ReportCardsHandler) PDF(w http.ResponseWriter, r *http.Request) {
	card, ok := h.directorCard(w, r)
	if !ok {
		return
	}
	h.servePDF(w, r, card)
}

func (h *ReportCardsHandler) directorCard(w http.ResponseWriter, r *http.Request) (client.ReportCard, bool) {
	id, ok := canonicalUUID(r.PathValue("id"))
	if !ok {
		writeNotFound(w)
		return client.ReportCard{}, false
	}
	card, err := h.grades.GetReportCard(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return client.ReportCard{}, false
	}
	return card, true
}

// --- family --------------------------------------------------------------------

// FamilyList handles GET /api/v1/family/students/{studentId}/report-cards —
// the child's latest published card per period, newest period first (cycle,
// then term ordinal — never issue date, which a reopen moves). grades owns
// that order; the gateway keeps it.
func (h *ReportCardsHandler) FamilyList(w http.ResponseWriter, r *http.Request) {
	studentID, ok := h.ownChild(w, r)
	if !ok {
		return
	}
	cards, err := h.grades.ListReportCards(r.Context(), client.ReportCardFilter{StudentID: studentID})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]familyReportCardSummaryResponse, 0, len(cards))
	for _, c := range cards {
		out = append(out, familyReportCardSummaryResponse{
			ID: c.ID, TermID: c.TermID, TermName: c.TermName, TermOrdinal: c.TermOrdinal, CycleName: c.CycleName, Version: c.Version,
			IssuedAt: rfc3339OrNil(c.IssuedAt), GeneralAverage: numberOrNil(c.GeneralAverage), HasGrades: summaryHasGrades(c),
		})
	}
	writePrivateJSON(w, map[string][]familyReportCardSummaryResponse{"reportCards": out})
}

// FamilyGet handles GET /api/v1/family/students/{studentId}/report-cards/{id}.
func (h *ReportCardsHandler) FamilyGet(w http.ResponseWriter, r *http.Request) {
	card, ok := h.familyCard(w, r)
	if !ok {
		return
	}
	h.serveJSON(w, r, card)
}

// FamilyPDF handles GET /api/v1/family/students/{studentId}/report-cards/{id}/pdf.
func (h *ReportCardsHandler) FamilyPDF(w http.ResponseWriter, r *http.Request) {
	card, ok := h.familyCard(w, r)
	if !ok {
		return
	}
	h.servePDF(w, r, card)
}

// ownChild runs the family ownership gate: the child must be the caller's
// (404 otherwise, the answer a missing child gets).
func (h *ReportCardsHandler) ownChild(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	studentID, ok := canonicalUUID(r.PathValue("studentId"))
	if !ok {
		writeNotFound(w)
		return "", false
	}
	owned, err := ownsChild(r.Context(), h.links, p.UserID, studentID)
	if err != nil {
		writeConnectError(w, err)
		return "", false
	}
	if !owned {
		writeNotFound(w)
		return "", false
	}
	return studentID, true
}

// familyCard reads a card of the caller's own child: a card of another pupil,
// or one a later version superseded, is 404.
func (h *ReportCardsHandler) familyCard(w http.ResponseWriter, r *http.Request) (client.ReportCard, bool) {
	studentID, ok := h.ownChild(w, r)
	if !ok {
		return client.ReportCard{}, false
	}
	card, ok := h.directorCard(w, r)
	if !ok {
		return client.ReportCard{}, false
	}
	if card.StudentID != studentID || card.Status != reportCardPublished {
		writeNotFound(w)
		return client.ReportCard{}, false
	}
	return card, true
}

// --- rendering -----------------------------------------------------------------

func (h *ReportCardsHandler) serveJSON(w http.ResponseWriter, r *http.Request, c client.ReportCard) {
	school, _, err := h.profiles.GetSchoolProfile(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	lines := make([]reportCardLineResponse, 0, len(c.Lines))
	for _, l := range c.Lines {
		lines = append(lines, reportCardLineResponse{
			SubjectID: l.SubjectID, SubjectName: l.SubjectName, Position: l.Position,
			Periods: numbers(l.PeriodValues), Months: numbers(l.MonthValues), Average: numberOrNil(l.TermAverage),
		})
	}
	periods := make([]reportCardPeriodResponse, 0, len(c.Periods))
	for _, p := range c.Periods {
		periods = append(periods, reportCardPeriodResponse{ID: p.ID, Ordinal: p.Ordinal, Name: p.Name})
	}
	writePrivateJSON(w, reportCardResponse{
		ID: c.ID, TermID: c.TermID, TermName: c.TermName, TermOrdinal: c.TermOrdinal, CycleName: c.CycleName, LevelName: c.LevelName,
		GroupID: c.GroupID, GroupLabel: c.GroupLabel, StudentID: c.StudentID, StudentName: c.StudentName,
		EnrollmentNumber: c.EnrollmentNumber, Version: c.Version, Status: c.Status, IssuedAt: rfc3339OrNil(c.IssuedAt),
		Periods: periods, Months: nonNilMonths(c.Months), Scale: toGradeScaleResponse(c.Scale), GeneralAverage: numberOrNil(c.GeneralAverage),
		HasGrades: reportcard.HasGrades(c), Lines: lines, School: school,
	})
}

func (h *ReportCardsHandler) servePDF(w http.ResponseWriter, r *http.Request, c client.ReportCard) {
	school, _, err := h.profiles.GetSchoolProfile(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	doc, err := reportcard.PDF(reportcard.Document{School: school, Card: c, Zone: h.zone})
	if err != nil {
		http.Error(w, "the report card could not be rendered", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", reportcard.Filename(c)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(doc) // the status is sent; a client that hung up is not an error to report
}

// writePrivateJSON answers a pupil's grades: no shared cache keeps a copy.
func writePrivateJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, body)
}

// summaryHasGrades reads a summary, which carries no lines: grades computes the
// general average over every subject that has a grade, so it is absent exactly
// when no subject has one.
func summaryHasGrades(c client.ReportCardSummary) bool { return c.GeneralAverage != "" }

// numbers is a map of exact decimal texts as JSON numbers ({} when empty).
func numbers(texts map[string]string) map[string]json.Number {
	out := make(map[string]json.Number, len(texts))
	for k, v := range texts {
		out[k] = json.Number(v)
	}
	return out
}

// numberOrNil is exact decimal text as a JSON number, or null for "".
func numberOrNil(s string) *json.Number {
	if s == "" {
		return nil
	}
	n := json.Number(s)
	return &n
}
