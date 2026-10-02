package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// The edge's bounds on an evaluation-period request (T14). config decides the
// business rules; these only keep a malformed or oversized body from leaving
// the gateway.
const (
	maxPeriodsPerGrade = 12 // mirrors config's domain.MaxEvaluationPeriods
	maxPeriodNameLen   = 60 // mirrors config's domain.MaxPeriodNameLen
	maxCopyTargets     = 50
	codePeriodDate     = "PERIOD_DATE_INVALID"
	codeTooManyPeriods = "TOO_MANY_PERIODS"
)

// PeriodGradesGuard answers which evaluation periods already hold grades
// (grades, T15). The Connect GradesClient satisfies it.
type PeriodGradesGuard interface {
	PeriodsWithGrades(ctx context.Context, periodIDs []string) ([]string, error)
}

// WithPeriodGradesGuard makes the evaluation-period writes refuse removing a
// period that already holds grades with 409 PERIOD_HAS_GRADES (T14's pending
// guard, T15). Config must not depend on grades, so the gateway asks first —
// as it does for a scale change (ADR 0019): the check and the write are two
// calls, and a grade captured between them is refused by grades only if its
// period no longer exists then. The composition root always wires it.
func (h *ConfigHandler) WithPeriodGradesGuard(g PeriodGradesGuard) *ConfigHandler {
	h.graded = g
	return h
}

// The refusal for removing a period with grades.
const (
	codePeriodHasGrades = "PERIOD_HAS_GRADES"
	msgPeriodHasGrades  = "«%s» ya tiene calificaciones; no se puede quitar"
)

// periodsRemovable asks grades whether any of the periods a write would
// remove holds grades; it writes the refusal itself (409 PERIOD_HAS_GRADES
// naming the first, or the grades failure — an unknown answer never lets the
// removal through) and reports whether to go on.
func (h *ConfigHandler) periodsRemovable(w http.ResponseWriter, r *http.Request, removed []client.EvaluationPeriod) bool {
	if h.graded == nil || len(removed) == 0 {
		return true
	}
	ids := make([]string, 0, len(removed))
	for _, p := range removed {
		ids = append(ids, p.ID)
	}
	graded, err := h.graded.PeriodsWithGrades(r.Context(), ids)
	if err != nil {
		writeConnectError(w, err)
		return false
	}
	hasGrades := make(map[string]bool, len(graded))
	for _, id := range graded {
		hasGrades[id] = true
	}
	for _, p := range removed {
		if hasGrades[p.ID] {
			writeCodedError(w, http.StatusConflict, codePeriodHasGrades, fmt.Sprintf(msgPeriodHasGrades, p.Name))
			return false
		}
	}
	return true
}

// removedBySave lists the grade's current periods a save keeps no row for
// (rows name the period they edit by id).
func (h *ConfigHandler) removedBySave(ctx context.Context, cycleID, gradeID string, in []client.EvaluationPeriodInput) ([]client.EvaluationPeriod, error) {
	if h.graded == nil {
		return nil, nil
	}
	current, err := h.svc.ListEvaluationPeriods(ctx, cycleID, "", gradeID)
	if err != nil {
		return nil, err
	}
	kept := make(map[string]bool, len(in))
	for _, p := range in {
		kept[p.ID] = true
	}
	var removed []client.EvaluationPeriod
	for _, p := range current {
		if !kept[p.ID] {
			removed = append(removed, p)
		}
	}
	return removed, nil
}

// removedByCopy lists the targets' periods a copy removes: a copy keeps each
// target period whose ordinal the source has (matched by ordinal, ADR 0029)
// and removes the rest.
func (h *ConfigHandler) removedByCopy(ctx context.Context, cycleID, fromID string, toIDs []string) ([]client.EvaluationPeriod, error) {
	if h.graded == nil {
		return nil, nil
	}
	all, err := h.svc.ListEvaluationPeriods(ctx, cycleID, "", "")
	if err != nil {
		return nil, err
	}
	sourceOrdinals := map[int]bool{}
	for _, p := range all {
		if p.GradeLevelID == fromID {
			sourceOrdinals[p.Ordinal] = true
		}
	}
	targets := make(map[string]bool, len(toIDs))
	for _, id := range toIDs {
		targets[id] = true
	}
	var removed []client.EvaluationPeriod
	for _, p := range all {
		if targets[p.GradeLevelID] && !sourceOrdinals[p.Ordinal] {
			removed = append(removed, p)
		}
	}
	return removed, nil
}

// evaluationPeriodResponse is one period on the wire; dates are YYYY-MM-DD,
// both inclusive; termId is null unless the period was migrated from a term.
type evaluationPeriodResponse struct {
	ID           string  `json:"id"`
	CycleID      string  `json:"cycleId"`
	GradeLevelID string  `json:"gradeLevelId"`
	Ordinal      int     `json:"ordinal"`
	Name         string  `json:"name"`
	StartsOn     string  `json:"startsOn"`
	EndsOn       string  `json:"endsOn"`
	TermID       *string `json:"termId"`
}

func toEvaluationPeriodResponses(ps []client.EvaluationPeriod) map[string]any {
	out := make([]evaluationPeriodResponse, 0, len(ps))
	for _, p := range ps {
		r := evaluationPeriodResponse{
			ID: p.ID, CycleID: p.CycleID, GradeLevelID: p.GradeLevelID, Ordinal: p.Ordinal, Name: p.Name,
			StartsOn: formatDate(p.StartsOn), EndsOn: formatDate(p.EndsOn),
		}
		if p.TermID != "" {
			termID := p.TermID
			r.TermID = &termID
		}
		out = append(out, r)
	}
	return map[string]any{"periods": out}
}

// ListEvaluationPeriods handles GET /api/v1/config/evaluation-periods
// ?cycleId=&levelId=&gradeLevelId= (cycleId required).
func (h *ConfigHandler) ListEvaluationPeriods(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cycleID, levelID, gradeID := q.Get("cycleId"), q.Get("levelId"), q.Get("gradeLevelId")
	if !uuidShape.MatchString(cycleID) {
		http.Error(w, "cycleId is required", http.StatusBadRequest)
		return
	}
	if (levelID != "" && !uuidShape.MatchString(levelID)) || (gradeID != "" && !uuidShape.MatchString(gradeID)) {
		http.Error(w, "levelId and gradeLevelId must be ids", http.StatusBadRequest)
		return
	}
	res, err := h.svc.ListEvaluationPeriods(r.Context(), cycleID, levelID, gradeID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEvaluationPeriodResponses(res))
}

type evaluationPeriodInput struct {
	// ID is the grade's period the row edits; omitted for a new period.
	ID       string `json:"id"`
	Ordinal  int    `json:"ordinal"`
	Name     string `json:"name"`
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
}

type putEvaluationPeriodsRequest struct {
	CycleID string `json:"cycleId"`
	// A pointer, so a body without "periods" is told apart from [] (clears).
	Periods *[]evaluationPeriodInput `json:"periods"`
}

// PutGradeEvaluationPeriods handles PUT /api/v1/config/grade-levels/{id}/
// evaluation-periods: the grade's complete period set in a cycle. Overlaps,
// dates outside the cycle and the like are config's refusals (X-Error-Code
// forwarded); a date not shaped YYYY-MM-DD is 400 PERIOD_DATE_INVALID here.
func (h *ConfigHandler) PutGradeEvaluationPeriods(w http.ResponseWriter, r *http.Request) {
	gradeID := r.PathValue("id")
	if !uuidShape.MatchString(gradeID) {
		http.Error(w, "id must be a grade level id", http.StatusBadRequest)
		return
	}
	var req putEvaluationPeriodsRequest
	if err := decodeJSONBody(w, r, &req); err != nil || req.Periods == nil || !uuidShape.MatchString(req.CycleID) {
		http.Error(w, `invalid request body (want {"cycleId":"…","periods":[…]})`, http.StatusBadRequest)
		return
	}
	if len(*req.Periods) > maxPeriodsPerGrade {
		writeCodedError(w, http.StatusBadRequest, codeTooManyPeriods, fmt.Sprintf("Un grado puede tener hasta %d periodos", maxPeriodsPerGrade))
		return
	}
	// Canonical ids (lower case) from here on: config compares them exactly,
	// and the period-grades guard must see what config will see.
	gradeID, req.CycleID = strings.ToLower(gradeID), strings.ToLower(req.CycleID)
	in := make([]client.EvaluationPeriodInput, 0, len(*req.Periods))
	for _, p := range *req.Periods {
		if (p.ID != "" && !uuidShape.MatchString(p.ID)) || p.Ordinal < 1 || p.Ordinal > maxPeriodsPerGrade ||
			utf8.RuneCountInString(p.Name) > maxPeriodNameLen {
			http.Error(w, fmt.Sprintf("each period needs an ordinal 1–%d, a name of at most %d characters and, if any, a period id", maxPeriodsPerGrade, maxPeriodNameLen), http.StatusBadRequest)
			return
		}
		from, errFrom := time.Parse(dateLayout, p.StartsOn)
		to, errTo := time.Parse(dateLayout, p.EndsOn)
		if errFrom != nil || errTo != nil {
			writeCodedError(w, http.StatusBadRequest, codePeriodDate, "Fecha inválida (usa AAAA-MM-DD)")
			return
		}
		in = append(in, client.EvaluationPeriodInput{ID: strings.ToLower(p.ID), Ordinal: p.Ordinal, Name: p.Name, StartsOn: from, EndsOn: to})
	}
	removed, err := h.removedBySave(r.Context(), req.CycleID, gradeID, in)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if !h.periodsRemovable(w, r, removed) {
		return
	}
	res, err := h.svc.UpsertEvaluationPeriods(r.Context(), req.CycleID, gradeID, in)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEvaluationPeriodResponses(res))
}

type copyEvaluationPeriodsRequest struct {
	CycleID          string   `json:"cycleId"`
	FromGradeLevelID string   `json:"fromGradeLevelId"`
	ToGradeLevelIDs  []string `json:"toGradeLevelIds"`
}

// CopyEvaluationPeriods handles POST /api/v1/config/evaluation-periods/copy:
// each target grade takes the source grade's periods in one transaction.
func (h *ConfigHandler) CopyEvaluationPeriods(w http.ResponseWriter, r *http.Request) {
	var req copyEvaluationPeriodsRequest
	if err := decodeJSONBody(w, r, &req); err != nil || !uuidShape.MatchString(req.CycleID) ||
		!uuidShape.MatchString(req.FromGradeLevelID) || !validCopyTargets(req.ToGradeLevelIDs) {
		http.Error(w, "cycleId, fromGradeLevelId and 1–50 toGradeLevelIds are required", http.StatusBadRequest)
		return
	}
	req.CycleID, req.FromGradeLevelID = strings.ToLower(req.CycleID), strings.ToLower(req.FromGradeLevelID)
	for i, id := range req.ToGradeLevelIDs {
		req.ToGradeLevelIDs[i] = strings.ToLower(id)
	}
	removed, err := h.removedByCopy(r.Context(), req.CycleID, req.FromGradeLevelID, req.ToGradeLevelIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if !h.periodsRemovable(w, r, removed) {
		return
	}
	res, err := h.svc.CopyEvaluationPeriods(r.Context(), req.CycleID, req.FromGradeLevelID, req.ToGradeLevelIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEvaluationPeriodResponses(res))
}

func validCopyTargets(ids []string) bool {
	if len(ids) == 0 || len(ids) > maxCopyTargets {
		return false
	}
	for _, id := range ids {
		if !uuidShape.MatchString(id) {
			return false
		}
	}
	return true
}
