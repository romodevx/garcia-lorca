package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// Period closing (T7). The director closes a period — one level term of one
// cycle, standing since T15 for the evaluation period of the same ordinal in
// each grade of the level — once its grades are captured; the grades service
// freezes a report card per pupil in the same transaction and locks those
// periods for everyone (PERIOD_LOCKED). Reopening unlocks them; closing again
// issues the next version of every report card.
//
// Reads (the periods and their progress) answer to grades:read; closing and
// reopening to grades:close, which only the owner holds (iam 0023). Refusals
// are 409 with X-Error-Code: MISSING_GRADES (captures missing and force unset),
// PERIOD_NOT_CONFIGURED, ALREADY_CLOSED, LEVEL_NOT_GRADED, ALREADY_OPEN.

// PeriodsService is the grades surface the periods handler consumes. The
// Connect GradesClient satisfies it.
type PeriodsService interface {
	ListPeriods(ctx context.Context, cycleID string) ([]client.Period, error)
	GetPeriodProgress(ctx context.Context, termID string) (client.PeriodProgress, error)
	ClosePeriod(ctx context.Context, in client.ClosePeriodInput) (client.ClosePeriodResult, error)
	ReopenPeriod(ctx context.Context, in client.ReopenPeriodInput) (string, error)
}

// GradesPeriodsHandler serves /api/v1/grades/periods*.
type GradesPeriodsHandler struct {
	grades PeriodsService
	names  UserNamer
}

// NewGradesPeriodsHandler wires the handler. names resolves the closing
// director's display name for the snapshot (nil: the token's email).
func NewGradesPeriodsHandler(grades PeriodsService, names UserNamer) *GradesPeriodsHandler {
	return &GradesPeriodsHandler{grades: grades, names: names}
}

const msgCycleRequired = "cycleId is required"

type periodProgressResponse struct {
	Captured int `json:"captured"`
	Expected int `json:"expected"`
}

type periodResponse struct {
	TermID       string                 `json:"termId"`
	TermName     string                 `json:"termName"`
	Ordinal      int                    `json:"ordinal"`
	LevelID      string                 `json:"levelId"`
	LevelName    string                 `json:"levelName"`
	StartsOn     *string                `json:"startsOn"`
	EndsOn       *string                `json:"endsOn"`
	Status       string                 `json:"status"`
	ClosedAt     *string                `json:"closedAt,omitempty"`
	ClosedByName string                 `json:"closedByName,omitempty"`
	Version      int                    `json:"version"`
	Progress     periodProgressResponse `json:"progress"`
}

type missingGradesResponse struct {
	GroupID      string `json:"groupId"`
	GroupLabel   string `json:"groupLabel"`
	SubjectID    string `json:"subjectId"`
	SubjectName  string `json:"subjectName"`
	TeacherName  string `json:"teacherName"`
	MissingCount int    `json:"missingCount"`
}

type progressResponse struct {
	TermID string `json:"termId"`
	// Unfinished is true while a grade's period of the term has not ended:
	// still expected (closing now needs force), so the screen warns "Aún no
	// termina el periodo".
	Unfinished bool    `json:"unfinished"`
	EndsOn     *string `json:"endsOn"`
	// GroupsWithoutPeriod are the groups whose grade lacks the term's period:
	// the term cannot close (PERIOD_NOT_CONFIGURED).
	GroupsWithoutPeriod []string                `json:"groupsWithoutPeriod"`
	Expected            int                     `json:"expected"`
	Captured            int                     `json:"captured"`
	Missing             []missingGradesResponse `json:"missing"`
	// ReportCards / SkippedWithoutGrades let the close dialog say beforehand
	// how many cards it publishes and how many pupils it skips (T10), and
	// WithdrawnReportCards how many published cards it withdraws unreplaced.
	ReportCards          int `json:"reportCards"`
	SkippedWithoutGrades int `json:"skippedWithoutGrades"`
	WithdrawnReportCards int `json:"withdrawnReportCards"`
}

type closePeriodRequest struct {
	Force bool `json:"force"`
}

type closePeriodResponse struct {
	Status               string `json:"status"`
	Version              int    `json:"version"`
	ReportCards          int    `json:"reportCards"`
	SkippedWithoutGrades int    `json:"skippedWithoutGrades"`
	WithdrawnReportCards int    `json:"withdrawnReportCards"`
}

type periodStatusResponse struct {
	Status string `json:"status"`
}

// List handles GET /api/v1/grades/periods?cycleId= — every term of the cycle
// with its closure and capture progress.
func (h *GradesPeriodsHandler) List(w http.ResponseWriter, r *http.Request) {
	cycleID, ok := canonicalUUID(r.URL.Query().Get("cycleId"))
	if !ok {
		http.Error(w, msgCycleRequired, http.StatusBadRequest)
		return
	}
	periods, err := h.grades.ListPeriods(r.Context(), cycleID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]periodResponse, 0, len(periods))
	for _, p := range periods {
		out = append(out, periodResponse{
			TermID: p.TermID, TermName: p.TermName, Ordinal: p.Ordinal, LevelID: p.LevelID, LevelName: p.LevelName,
			StartsOn: stringOrNil(p.StartsOn), EndsOn: stringOrNil(p.EndsOn), Status: p.Status,
			ClosedAt: rfc3339OrNil(p.ClosedAt), ClosedByName: p.ClosedByName,
			Version: p.Version, Progress: periodProgressResponse{Captured: p.Captured, Expected: p.Expected},
		})
	}
	writeJSON(w, http.StatusOK, map[string][]periodResponse{"periods": out})
}

// Progress handles GET /api/v1/grades/periods/{termId}/progress — what the
// period expects, what is captured, and what is missing per group and subject.
func (h *GradesPeriodsHandler) Progress(w http.ResponseWriter, r *http.Request) {
	termID, ok := canonicalUUID(r.PathValue("termId"))
	if !ok {
		writeNotFound(w)
		return
	}
	p, err := h.grades.GetPeriodProgress(r.Context(), termID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	missing := make([]missingGradesResponse, 0, len(p.Missing))
	for _, m := range p.Missing {
		missing = append(missing, missingGradesResponse{
			GroupID: m.GroupID, GroupLabel: m.GroupLabel, SubjectID: m.SubjectID, SubjectName: m.SubjectName,
			TeacherName: m.TeacherName, MissingCount: m.MissingCount,
		})
	}
	writeJSON(w, http.StatusOK, progressResponse{
		TermID: p.TermID, Unfinished: p.Unfinished, EndsOn: stringOrNil(p.EndsOn),
		GroupsWithoutPeriod: nonNilMonths(p.GroupsWithoutPeriod),
		Expected:            p.Expected, Captured: p.Captured, Missing: missing,
		ReportCards: p.ReportCards, SkippedWithoutGrades: p.SkippedWithoutGrades,
		WithdrawnReportCards: p.WithdrawnReportCards,
	})
}

// Close handles POST /api/v1/grades/periods/{termId}/close {force} — close the
// period and issue its report cards. An empty body means force:false.
func (h *GradesPeriodsHandler) Close(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	termID, ok := canonicalUUID(r.PathValue("termId"))
	if !ok {
		writeNotFound(w)
		return
	}
	var req closePeriodRequest
	if err := decodeJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.grades.ClosePeriod(r.Context(), client.ClosePeriodInput{
		TermID: termID, Force: req.Force, ActorID: p.UserID, ActorName: callerName(r.Context(), h.names, p),
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, closePeriodResponse{Status: res.Status, Version: res.Version, ReportCards: res.ReportCards,
		SkippedWithoutGrades: res.SkippedWithoutGrades, WithdrawnReportCards: res.WithdrawnReportCards})
}

// maxReopenReason bounds, in characters, why a period is reopened (grades
// and its column hold the same limit).
const maxReopenReason = 500

type reopenPeriodRequest struct {
	Reason string `json:"reason"`
}

// Reopen handles POST /api/v1/grades/periods/{termId}/reopen {reason}. The
// reason is required (CC-308): missing, blank or over 500 characters is 400
// in the error envelope, and grades stores it with who reopened the period.
func (h *GradesPeriodsHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	termID, ok := canonicalUUID(r.PathValue("termId"))
	if !ok {
		writeNotFound(w)
		return
	}
	var req reopenPeriodRequest
	if err := decodeJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeReopenReasonError(w)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxReopenReason {
		writeReopenReasonError(w)
		return
	}
	status, err := h.grades.ReopenPeriod(r.Context(), client.ReopenPeriodInput{
		TermID: termID, ActorID: p.UserID, ActorName: callerName(r.Context(), h.names, p), Reason: reason,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, periodStatusResponse{Status: status})
}

// writeReopenReasonError answers a reopen without a usable reason.
func writeReopenReasonError(w http.ResponseWriter) {
	middleware.WriteError(w, http.StatusBadRequest, "invalid",
		"Escribe el motivo para reabrir el periodo (máximo 500 caracteres)", map[string]string{"field": "reason"})
}
