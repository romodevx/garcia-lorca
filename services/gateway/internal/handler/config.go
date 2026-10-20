package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// ConfigService is the academic-catalog surface the edge needs (consumer-defined,
// so the handler is testable with a fake). The Connect ConfigClient implements it.
type ConfigService interface {
	CreateCycle(ctx context.Context, in client.CreateCycleInput) (client.CycleResult, error)
	ListCycles(ctx context.Context) ([]client.CycleResult, error)
	ActivateCycle(ctx context.Context, id string) (client.CycleResult, error)
	RenameCycle(ctx context.Context, id, name string) (client.CycleResult, error)
	CreateLevel(ctx context.Context, in client.CreateLevelInput) (client.LevelResult, error)
	ListLevels(ctx context.Context) ([]client.LevelResult, error)
	ReorderLevels(ctx context.Context, levelIDs []string) ([]client.LevelResult, error)
	UpdateLevel(ctx context.Context, id string, p client.LevelPatch) (client.LevelResult, error)
	CreateGradeLevel(ctx context.Context, in client.CreateGradeLevelInput) (client.GradeLevelResult, error)
	ListGradeLevels(ctx context.Context, levelID string) ([]client.GradeLevelResult, error)
	RenameGradeLevel(ctx context.Context, id, name string) (client.GradeLevelResult, error)
	DeleteGradeLevel(ctx context.Context, id string) error
	CreateSubject(ctx context.Context, in client.CreateSubjectInput) (client.SubjectResult, error)
	ListSubjects(ctx context.Context, gradeLevelID string) ([]client.SubjectResult, error)
	RenameSubject(ctx context.Context, id, name string) (client.SubjectResult, error)
	DeleteSubject(ctx context.Context, id string) error
	CreateTerm(ctx context.Context, in client.CreateTermInput) (client.TermResult, error)
	ListTerms(ctx context.Context, cycleID, levelID string) ([]client.TermResult, error)
	SetTermMonths(ctx context.Context, termID string, months []string) (client.TermResult, error)
	CopyTermMonths(ctx context.Context, cycleID, fromLevelID string, toLevelIDs []string) ([]client.TermResult, error)
	ListEvaluationPeriods(ctx context.Context, cycleID, levelID, gradeLevelID string) ([]client.EvaluationPeriod, error)
	UpsertEvaluationPeriods(ctx context.Context, cycleID, gradeLevelID string, in []client.EvaluationPeriodInput) ([]client.EvaluationPeriod, error)
	CopyEvaluationPeriods(ctx context.Context, cycleID, fromGradeLevelID string, toGradeLevelIDs []string) ([]client.EvaluationPeriod, error)
	CreateGroup(ctx context.Context, in client.CreateGroupInput) (client.GroupResult, error)
	ListGroups(ctx context.Context, cycleID, gradeLevelID string) ([]client.GroupResult, error)
	CreateTeacher(ctx context.Context, firstName, lastName, email, userID string) (client.Teacher, error)
	ListTeachers(ctx context.Context) ([]client.Teacher, error)
	CreateNonWorkingPeriod(ctx context.Context, cycleID, label, kind, startsOn, endsOn string) (client.NonWorkingPeriod, error)
	ListNonWorkingPeriods(ctx context.Context, cycleID string) ([]client.NonWorkingPeriod, error)
	DeleteNonWorkingPeriod(ctx context.Context, id string) error
	CreateSchoolEvent(ctx context.Context, in client.SchoolEvent) (client.SchoolEvent, error)
	ListSchoolEvents(ctx context.Context, cycleID string) ([]client.SchoolEvent, error)
	GetSchoolProfile(ctx context.Context) (client.SchoolProfile, bool, error)
	SetSchoolProfile(ctx context.Context, in client.SchoolProfile) (client.SchoolProfile, error)
	DeleteSchoolEvent(ctx context.Context, id string) error
	CreateTeachingAssignment(ctx context.Context, groupID, subjectID, teacherID string) (client.TeachingAssignment, error)
	ListTeachingAssignments(ctx context.Context, cycleID, groupID, teacherID string) ([]client.TeachingAssignment, error)
	AssignTeachingLoad(ctx context.Context, in client.TeachingLoadInput) (client.TeachingLoadResult, error)
	DeleteTeachingAssignment(ctx context.Context, id string) error
	ReassignTeachingAssignment(ctx context.Context, id, teacherID string) (client.TeachingAssignment, error)
}

// ConfigHandler exposes the front-facing (HTTP/JSON) academic-catalog endpoints,
// proxying to the config service. It translates JSON ↔ the ConfigClient and
// Connect error codes ↔ HTTP status. Every route must run behind the module
// guard (module_config) plus RequireAuth + rotation-enforcement, with writes
// gated on config:write and reads on config:read.
type ConfigHandler struct {
	svc    ConfigService
	scale  ScaleGuard
	lock   TermLockGuard
	graded PeriodGradesGuard
}

// TermLockGuard answers which terms are closed periods (grades, T7): their
// months are frozen until the director reopens them. The Connect GradesClient
// satisfies it.
type TermLockGuard interface {
	ClosedTerms(ctx context.Context, termIDs []string) ([]string, error)
}

// WithTermLock makes the term-months writes refuse a closed term with 409
// TERM_CLOSED (see termsOpen). The composition root always wires it.
func (h *ConfigHandler) WithTermLock(g TermLockGuard) *ConfigHandler {
	h.lock = g
	return h
}

// The refusal for changing the months of a closed period.
const (
	codeTermClosed = "TERM_CLOSED"
	msgTermClosed  = "El periodo está cerrado; reábrelo para cambiar sus meses"
)

// termsOpen asks grades whether any of termIDs is a closed period; it writes
// the refusal itself (409 TERM_CLOSED, or the grades failure: an unknown
// closure state never lets a change through) and reports whether to go on.
func (h *ConfigHandler) termsOpen(w http.ResponseWriter, r *http.Request, termIDs []string) bool {
	if h.lock == nil || len(termIDs) == 0 {
		return true
	}
	closed, err := h.lock.ClosedTerms(r.Context(), termIDs)
	if err != nil {
		writeConnectError(w, err)
		return false
	}
	if len(closed) > 0 {
		writeCodedError(w, http.StatusConflict, codeTermClosed, msgTermClosed)
		return false
	}
	return true
}

// ScaleGuard counts the grades already captured in a level's active cycle that
// a proposed grading would refuse. The Connect GradesClient satisfies it.
type ScaleGuard interface {
	CountScaleViolations(ctx context.Context, levelID string, g client.LevelGrading) (int, error)
}

// NewConfigHandler wires the handler to a catalog service.
func NewConfigHandler(svc ConfigService) *ConfigHandler {
	return &ConfigHandler{svc: svc}
}

// WithScaleGuard makes PATCH /config/levels/{id} refuse a grading change that
// captured grades would break (see PatchLevel). Without it the change is not
// checked — the composition root always wires it.
func (h *ConfigHandler) WithScaleGuard(g ScaleGuard) *ConfigHandler {
	h.scale = g
	return h
}

// dateLayout is the wire format for catalog dates: a plain calendar day, parsed
// and rendered as UTC midnight so the edge never leaks a timezone.
const dateLayout = "2006-01-02"

// parseDate parses a YYYY-MM-DD calendar day into UTC midnight. An empty string
// yields the zero time (an unset optional date); any other malformed value is an
// error the caller maps to 400.
func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(dateLayout, s)
}

// formatDate renders a time as YYYY-MM-DD, or "" for the zero time.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(dateLayout)
}

// --- cycles ---------------------------------------------------------------

type createCycleRequest struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
}

type cycleResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
}

func toCycleResponse(c client.CycleResult) cycleResponse {
	return cycleResponse{
		ID:       c.ID,
		Name:     c.Name,
		Status:   c.Status,
		StartsOn: formatDate(c.StartsOn),
		EndsOn:   formatDate(c.EndsOn),
	}
}

// CreateCycle handles POST /api/v1/config/cycles.
func (h *ConfigHandler) CreateCycle(w http.ResponseWriter, r *http.Request) {
	var req createCycleRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	startsOn, err := parseDate(req.StartsOn)
	if err != nil {
		http.Error(w, "invalid startsOn (want YYYY-MM-DD)", http.StatusBadRequest)
		return
	}
	endsOn, err := parseDate(req.EndsOn)
	if err != nil {
		http.Error(w, "invalid endsOn (want YYYY-MM-DD)", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateCycle(r.Context(), client.CreateCycleInput{
		Name:     req.Name,
		Status:   req.Status,
		StartsOn: startsOn,
		EndsOn:   endsOn,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCycleResponse(res))
}

type renameCycleRequest struct {
	Name string `json:"name"`
}

// RenameCycle handles PATCH /api/v1/config/cycles/{id} with {name}: the same
// year (id, dates, status) under a new name. A blank name never reaches config;
// config judges the rest (unknown → 404, a name another cycle has → 409).
func (h *ConfigHandler) RenameCycle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "id must be a cycle id", http.StatusBadRequest)
		return
	}
	var req renameCycleRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "Escribe el nombre del ciclo", http.StatusBadRequest)
		return
	}
	res, err := h.svc.RenameCycle(r.Context(), id, req.Name)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCycleResponse(res))
}

// ListCycles handles GET /api/v1/config/cycles.
func (h *ConfigHandler) ListCycles(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListCycles(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]cycleResponse, 0, len(res))
	for _, c := range res {
		out = append(out, toCycleResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"cycles": out})
}

// --- levels ---------------------------------------------------------------

type createLevelRequest struct {
	Name                  string `json:"name"`
	SEPCode               string `json:"sepCode"`
	EnrollmentGranularity string `json:"enrollmentGranularity"`
}

type levelResponse struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	SEPCode               string `json:"sepCode"`
	EnrollmentGranularity string `json:"enrollmentGranularity"`
	// AttendanceMode is "daily" or "per_class" (T3).
	AttendanceMode string `json:"attendanceMode"`
	// Grading is the level's numeric grading scale, in person units (T6).
	Grading levelGradingResponse `json:"grading"`
	// SortOrder is the level's place in the school order (T8; 1 = first).
	SortOrder int `json:"sortOrder"`
}

// levelGradingResponse is a grading scale as a person reads it (5, 9.5).
type levelGradingResponse struct {
	Enabled bool `json:"enabled"`
	gradeScaleResponse
}

// gradeScaleResponse is a scale's numbers as JSON numbers, exact.
type gradeScaleResponse struct {
	Min      json.Number `json:"min"`
	Max      json.Number `json:"max"`
	Decimals int         `json:"decimals"`
	Passing  json.Number `json:"passing"`
}

func toGradeScaleResponse(s client.GradeScale) gradeScaleResponse {
	return gradeScaleResponse{
		Min:      hundredthsNumber(s.MinHundredths),
		Max:      hundredthsNumber(s.MaxHundredths),
		Decimals: s.Decimals,
		Passing:  hundredthsNumber(s.PassingHundredths),
	}
}

func toLevelResponse(l client.LevelResult) levelResponse {
	return levelResponse{
		ID:                    l.ID,
		Name:                  l.Name,
		SEPCode:               l.SEPCode,
		EnrollmentGranularity: l.Granularity,
		AttendanceMode:        l.AttendanceMode,
		Grading:               levelGradingResponse{Enabled: l.Grading.Enabled, gradeScaleResponse: toGradeScaleResponse(l.Grading.Scale)},
		SortOrder:             l.SortOrder,
	}
}

// patchLevelRequest is the body of PATCH /api/v1/config/levels/{id}: either or
// both of the level's attendance mode and its grading scale.
type patchLevelRequest struct {
	AttendanceMode *string              `json:"attendanceMode"`
	Grading        *levelGradingRequest `json:"grading"`
}

// levelGradingRequest is a whole grading scale in the units a person types (5,
// 9.5). Every field is required; the marks stay raw so no float touches them.
type levelGradingRequest struct {
	Enabled  *bool           `json:"enabled"`
	Min      json.RawMessage `json:"min"`
	Max      json.RawMessage `json:"max"`
	Decimals *int            `json:"decimals"`
	Passing  json.RawMessage `json:"passing"`
}

// Sentences the PATCH refuses with before config is called.
const (
	msgPatchLevelEmpty      = "envía attendanceMode, grading o ambos"
	msgAttendanceModeValues = "attendanceMode must be daily or per_class"
	msgGradingIncomplete    = "grading requiere enabled, min, max, decimals y passing"
	msgGradingTwoDecimals   = "La escala admite hasta 2 decimales"
)

// toLevelGrading validates the shape of a grading body and converts its marks
// to exact hundredths. The scale's own rules (min < passing <= max, …) are
// config's to judge; this only refuses what cannot be a scale at all.
func (g levelGradingRequest) toLevelGrading() (client.LevelGrading, string) {
	if g.Enabled == nil || g.Decimals == nil {
		return client.LevelGrading{}, msgGradingIncomplete
	}
	marks := [3]json.RawMessage{g.Min, g.Max, g.Passing}
	var hundredths [3]int
	for i, raw := range marks {
		if _, ok := numberText(raw); !ok {
			return client.LevelGrading{}, msgGradingIncomplete
		}
		h, ok := parseHundredths(raw)
		if !ok {
			return client.LevelGrading{}, msgGradingTwoDecimals
		}
		hundredths[i] = h
	}
	return client.LevelGrading{Enabled: *g.Enabled, Scale: client.GradeScale{
		MinHundredths: hundredths[0], MaxHundredths: hundredths[1], PassingHundredths: hundredths[2], Decimals: *g.Decimals,
	}}, ""
}

// Scale tightening refusal (T6): the sentence and its machine code.
const (
	msgScaleTightening  = "Ya hay calificaciones capturadas en este ciclo que no cumplirían la nueva escala"
	codeScaleTightening = "SCALE_TIGHTENING"
)

// PatchLevel handles PATCH /api/v1/config/levels/{id}: change how the level's
// groups take attendance ("daily" | "per_class") and/or its numeric grading
// scale (T6).
//
// Atomic by construction: the body's shape is validated at the edge, then ONE
// config call (UpdateLevel) validates every field again and writes them in one
// transaction — a half-valid body changes nothing; an unknown level is 404.
//
// A grading change first asks grades how many grades captured in the level's
// active cycle the new scale would refuse. Loosening (lower min, higher max,
// more decimals, another passing mark) can never refuse one; tightening or
// disabling grading that would is 409 SCALE_TIGHTENING, and config is never
// called. A failed check fails closed. The check and the write are two calls,
// so a grade captured between them is judged by the next write, not this one.
func (h *ConfigHandler) PatchLevel(w http.ResponseWriter, r *http.Request) {
	var req patchLevelRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	patch, msg := req.toLevelPatch()
	if msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	if patch.Grading != nil && !h.scaleAllowed(w, r, id, *patch.Grading) {
		return
	}
	res, err := h.svc.UpdateLevel(r.Context(), id, patch)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toLevelResponse(res))
}

// toLevelPatch validates the body's shape and converts it; msg is the refusal.
func (req patchLevelRequest) toLevelPatch() (client.LevelPatch, string) {
	if req.AttendanceMode == nil && req.Grading == nil {
		return client.LevelPatch{}, msgPatchLevelEmpty
	}
	if req.AttendanceMode != nil && *req.AttendanceMode != client.AttendanceModeDaily && *req.AttendanceMode != client.AttendanceModePerClass {
		return client.LevelPatch{}, msgAttendanceModeValues
	}
	patch := client.LevelPatch{AttendanceMode: req.AttendanceMode}
	if req.Grading != nil {
		grading, msg := req.Grading.toLevelGrading()
		if msg != "" {
			return client.LevelPatch{}, msg
		}
		patch.Grading = &grading
	}
	return patch, ""
}

// scaleAllowed asks grades whether captured grades would break the new
// grading; it writes the refusal itself and reports whether to go on.
func (h *ConfigHandler) scaleAllowed(w http.ResponseWriter, r *http.Request, levelID string, g client.LevelGrading) bool {
	if h.scale == nil {
		return true
	}
	n, err := h.scale.CountScaleViolations(r.Context(), levelID, g)
	if err != nil {
		writeConnectError(w, err)
		return false
	}
	if n > 0 {
		writeCodedError(w, http.StatusConflict, codeScaleTightening, msgScaleTightening)
		return false
	}
	return true
}

// CreateLevel handles POST /api/v1/config/levels.
func (h *ConfigHandler) CreateLevel(w http.ResponseWriter, r *http.Request) {
	var req createLevelRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateLevel(r.Context(), client.CreateLevelInput{
		Name:        req.Name,
		SEPCode:     req.SEPCode,
		Granularity: req.EnrollmentGranularity,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toLevelResponse(res))
}

// ListLevels handles GET /api/v1/config/levels.
func (h *ConfigHandler) ListLevels(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListLevels(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]levelResponse, 0, len(res))
	for _, l := range res {
		out = append(out, toLevelResponse(l))
	}
	writeJSON(w, http.StatusOK, map[string]any{"levels": out})
}

// --- grade_levels ---------------------------------------------------------

type createGradeLevelRequest struct {
	LevelID string `json:"levelId"`
	Name    string `json:"name"`
	Ordinal int    `json:"ordinal"`
}

type gradeLevelResponse struct {
	ID      string `json:"id"`
	LevelID string `json:"levelId"`
	Name    string `json:"name"`
	Ordinal int    `json:"ordinal"`
}

func toGradeLevelResponse(g client.GradeLevelResult) gradeLevelResponse {
	return gradeLevelResponse{ID: g.ID, LevelID: g.LevelID, Name: g.Name, Ordinal: g.Ordinal}
}

// CreateGradeLevel handles POST /api/v1/config/grade-levels.
func (h *ConfigHandler) CreateGradeLevel(w http.ResponseWriter, r *http.Request) {
	var req createGradeLevelRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateGradeLevel(r.Context(), client.CreateGradeLevelInput{
		LevelID: req.LevelID,
		Name:    req.Name,
		Ordinal: req.Ordinal,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toGradeLevelResponse(res))
}

// ListGradeLevels handles GET /api/v1/config/grade-levels?levelId=...
func (h *ConfigHandler) ListGradeLevels(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListGradeLevels(r.Context(), r.URL.Query().Get("levelId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]gradeLevelResponse, 0, len(res))
	for _, g := range res {
		out = append(out, toGradeLevelResponse(g))
	}
	writeJSON(w, http.StatusOK, map[string]any{"gradeLevels": out})
}

// --- subjects -------------------------------------------------------------

type createSubjectRequest struct {
	GradeLevelID string `json:"gradeLevelId"`
	Name         string `json:"name"`
}

type subjectResponse struct {
	ID           string `json:"id"`
	GradeLevelID string `json:"gradeLevelId"`
	Name         string `json:"name"`
}

func toSubjectResponse(s client.SubjectResult) subjectResponse {
	return subjectResponse{ID: s.ID, GradeLevelID: s.GradeLevelID, Name: s.Name}
}

// CreateSubject handles POST /api/v1/config/subjects.
func (h *ConfigHandler) CreateSubject(w http.ResponseWriter, r *http.Request) {
	var req createSubjectRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateSubject(r.Context(), client.CreateSubjectInput{
		GradeLevelID: req.GradeLevelID,
		Name:         req.Name,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toSubjectResponse(res))
}

// ListSubjects handles GET /api/v1/config/subjects?gradeLevelId=...
func (h *ConfigHandler) ListSubjects(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListSubjects(r.Context(), r.URL.Query().Get("gradeLevelId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]subjectResponse, 0, len(res))
	for _, s := range res {
		out = append(out, toSubjectResponse(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"subjects": out})
}

type renameSubjectRequest struct {
	Name string `json:"name"`
}

// RenameSubject handles PATCH /api/v1/config/subjects/{id} with {name}: the
// same subject (id and grade) under a new name. A blank name never reaches
// config; config judges the rest (unknown → 404, a name the grade has → 409).
func (h *ConfigHandler) RenameSubject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "id must be a subject id", http.StatusBadRequest)
		return
	}
	var req renameSubjectRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	res, err := h.svc.RenameSubject(r.Context(), id, req.Name)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSubjectResponse(res))
}

// DeleteSubject handles DELETE /api/v1/config/subjects/{id}. A subject that
// was ever in a teacher's load is refused (409): it can only be renamed.
func (h *ConfigHandler) DeleteSubject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "id must be a subject id", http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteSubject(r.Context(), id); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- terms ----------------------------------------------------------------

type createTermRequest struct {
	CycleID  string `json:"cycleId"`
	LevelID  string `json:"levelId"`
	Name     string `json:"name"`
	Ordinal  int    `json:"ordinal"`
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
}

type termResponse struct {
	ID       string `json:"id"`
	CycleID  string `json:"cycleId"`
	LevelID  string `json:"levelId"`
	Name     string `json:"name"`
	Ordinal  int    `json:"ordinal"`
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
	// Months is always a list ("YYYY-MM", ascending), never null.
	Months []string `json:"months"`
}

func toTermResponse(t client.TermResult) termResponse {
	return termResponse{
		ID:       t.ID,
		CycleID:  t.CycleID,
		LevelID:  t.LevelID,
		Name:     t.Name,
		Ordinal:  t.Ordinal,
		StartsOn: formatDate(t.StartsOn),
		EndsOn:   formatDate(t.EndsOn),
		Months:   nonNilMonths(t.Months),
	}
}

// nonNilMonths answers [] for a term with no months, so the JSON is a list.
func nonNilMonths(months []string) []string {
	if months == nil {
		return []string{}
	}
	return months
}

// monthShape is the wire form of a calendar month: "2026-09".
var monthShape = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

type termMonthsRequest struct {
	// A pointer, so a body without "months" is told apart from an empty list
	// (which clears the term).
	Months *[]string `json:"months"`
}

// PutTermMonths handles PUT /api/v1/config/terms/{id}/months: the calendar
// months the term covers, replaced as a set ({"months":[]} clears it). A month
// of another term of the level is 409 MONTH_IN_OTHER_TERM; one outside the
// cycle is 400 MONTH_OUTSIDE_CYCLE; more than six months is 400 TERM_TOO_LONG
// (all decided by the config service). A closed period is 409 TERM_CLOSED,
// asked of grades before config is touched.
func (h *ConfigHandler) PutTermMonths(w http.ResponseWriter, r *http.Request) {
	var req termMonthsRequest
	if err := decodeJSONBody(w, r, &req); err != nil || req.Months == nil {
		http.Error(w, "invalid request body (want {\"months\":[\"YYYY-MM\"]})", http.StatusBadRequest)
		return
	}
	for _, m := range *req.Months {
		if !monthShape.MatchString(m) {
			writeCodedError(w, http.StatusBadRequest, "MONTH_INVALID", "Mes inválido (usa AAAA-MM)")
			return
		}
	}
	termID := r.PathValue("id")
	if !h.termsOpen(w, r, []string{termID}) {
		return
	}
	res, err := h.svc.SetTermMonths(r.Context(), termID, *req.Months)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTermResponse(res))
}

type copyTermMonthsRequest struct {
	CycleID     string   `json:"cycleId"`
	FromLevelID string   `json:"fromLevelId"`
	ToLevelIDs  []string `json:"toLevelIds"`
}

// CopyTermMonths handles POST /api/v1/config/terms/copy-months: each target
// level's terms take the source level's months in one transaction (matched by
// ordinal; missing terms created from the source; unmatched ones emptied).
// Any closed period among the target levels' terms is 409 TERM_CLOSED.
func (h *ConfigHandler) CopyTermMonths(w http.ResponseWriter, r *http.Request) {
	var req copyTermMonthsRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.CycleID) == "" || strings.TrimSpace(req.FromLevelID) == "" || len(req.ToLevelIDs) == 0 {
		http.Error(w, "cycleId, fromLevelId and at least one toLevelIds are required", http.StatusBadRequest)
		return
	}
	if !h.targetTermsOpen(w, r, req.CycleID, req.ToLevelIDs) {
		return
	}
	res, err := h.svc.CopyTermMonths(r.Context(), req.CycleID, req.FromLevelID, req.ToLevelIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]termResponse, 0, len(res))
	for _, t := range res {
		out = append(out, toTermResponse(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"terms": out})
}

// targetTermsOpen reads the cycle's terms once, keeps the target levels' and
// asks grades whether any is closed (see termsOpen).
func (h *ConfigHandler) targetTermsOpen(w http.ResponseWriter, r *http.Request, cycleID string, levelIDs []string) bool {
	if h.lock == nil {
		return true
	}
	terms, err := h.svc.ListTerms(r.Context(), cycleID, "")
	if err != nil {
		writeConnectError(w, err)
		return false
	}
	target := make(map[string]bool, len(levelIDs))
	for _, id := range levelIDs {
		target[strings.TrimSpace(id)] = true
	}
	ids := []string{}
	for _, t := range terms {
		if target[t.LevelID] {
			ids = append(ids, t.ID)
		}
	}
	return h.termsOpen(w, r, ids)
}

// CreateTerm handles POST /api/v1/config/terms.
func (h *ConfigHandler) CreateTerm(w http.ResponseWriter, r *http.Request) {
	var req createTermRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	startsOn, err := parseDate(req.StartsOn)
	if err != nil {
		http.Error(w, "invalid startsOn (want YYYY-MM-DD)", http.StatusBadRequest)
		return
	}
	endsOn, err := parseDate(req.EndsOn)
	if err != nil {
		http.Error(w, "invalid endsOn (want YYYY-MM-DD)", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateTerm(r.Context(), client.CreateTermInput{
		CycleID:  req.CycleID,
		LevelID:  req.LevelID,
		Name:     req.Name,
		Ordinal:  req.Ordinal,
		StartsOn: startsOn,
		EndsOn:   endsOn,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTermResponse(res))
}

// ListTerms handles GET /api/v1/config/terms?cycleId=...&levelId=...
func (h *ConfigHandler) ListTerms(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.ListTerms(r.Context(), q.Get("cycleId"), q.Get("levelId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]termResponse, 0, len(res))
	for _, t := range res {
		out = append(out, toTermResponse(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"terms": out})
}

// --- groups ---------------------------------------------------------------

type createGroupRequest struct {
	CycleID      string `json:"cycleId"`
	GradeLevelID string `json:"gradeLevelId"`
	Name         string `json:"name"`
	Capacity     int    `json:"capacity"`
}

type groupResponse struct {
	ID           string `json:"id"`
	CycleID      string `json:"cycleId"`
	GradeLevelID string `json:"gradeLevelId"`
	Name         string `json:"name"`
	Capacity     int    `json:"capacity"`
}

func toGroupResponse(g client.GroupResult) groupResponse {
	return groupResponse{
		ID:           g.ID,
		CycleID:      g.CycleID,
		GradeLevelID: g.GradeLevelID,
		Name:         g.Name,
		Capacity:     g.Capacity,
	}
}

// CreateGroup handles POST /api/v1/config/groups.
func (h *ConfigHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req createGroupRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateGroup(r.Context(), client.CreateGroupInput{
		CycleID:      req.CycleID,
		GradeLevelID: req.GradeLevelID,
		Name:         req.Name,
		Capacity:     req.Capacity,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toGroupResponse(res))
}

// ListGroups handles GET /api/v1/config/groups?cycleId=...&gradeLevelId=...
func (h *ConfigHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.ListGroups(r.Context(), q.Get("cycleId"), q.Get("gradeLevelId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]groupResponse, 0, len(res))
	for _, g := range res {
		out = append(out, toGroupResponse(g))
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

// --- teachers -------------------------------------------------------------

type createTeacherRequest struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	UserID    string `json:"userId"`
}

type teacherResponse struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	UserID    string `json:"userId"`
}

func toTeacherResponse(t client.Teacher) teacherResponse {
	return teacherResponse{
		ID:        t.ID,
		FirstName: t.FirstName,
		LastName:  t.LastName,
		Email:     t.Email,
		UserID:    t.UserID,
	}
}

// CreateTeacher handles POST /api/v1/teachers.
func (h *ConfigHandler) CreateTeacher(w http.ResponseWriter, r *http.Request) {
	var req createTeacherRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateTeacher(r.Context(), req.FirstName, req.LastName, req.Email, req.UserID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTeacherResponse(res))
}

// ListTeachers handles GET /api/v1/teachers.
func (h *ConfigHandler) ListTeachers(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListTeachers(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]teacherResponse, 0, len(res))
	for _, t := range res {
		out = append(out, toTeacherResponse(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"teachers": out})
}

// --- teaching_assignments -------------------------------------------------

// createTeachingAssignmentRequest carries only the client-supplied edges of the
// knot. cycleId is intentionally absent: the config service derives it from the
// group so it can never drift from the group's cycle.
type createTeachingAssignmentRequest struct {
	GroupID   string `json:"groupId"`
	SubjectID string `json:"subjectId"`
	TeacherID string `json:"teacherId"`
}

type teachingAssignmentResponse struct {
	ID        string `json:"id"`
	CycleID   string `json:"cycleId"`
	GroupID   string `json:"groupId"`
	SubjectID string `json:"subjectId"`
	TeacherID string `json:"teacherId"`
}

func toTeachingAssignmentResponse(a client.TeachingAssignment) teachingAssignmentResponse {
	return teachingAssignmentResponse{
		ID:        a.ID,
		CycleID:   a.CycleID,
		GroupID:   a.GroupID,
		SubjectID: a.SubjectID,
		TeacherID: a.TeacherID,
	}
}

// CreateTeachingAssignment handles POST /api/v1/teaching-assignments.
func (h *ConfigHandler) CreateTeachingAssignment(w http.ResponseWriter, r *http.Request) {
	var req createTeachingAssignmentRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateTeachingAssignment(r.Context(), req.GroupID, req.SubjectID, req.TeacherID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTeachingAssignmentResponse(res))
}

// ListTeachingAssignments handles
// GET /api/v1/teaching-assignments?cycleId=...&groupId=...&teacherId=...
func (h *ConfigHandler) ListTeachingAssignments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.ListTeachingAssignments(r.Context(), q.Get("cycleId"), q.Get("groupId"), q.Get("teacherId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]teachingAssignmentResponse, 0, len(res))
	for _, a := range res {
		out = append(out, toTeachingAssignmentResponse(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"teachingAssignments": out})
}

// writeConnectError maps a Connect error code from the config service to an HTTP
// status and writes the standard status text.
func writeConnectError(w http.ResponseWriter, err error) {
	WriteConnectError(w, err)
}

// WriteConnectError answers a downstream Connect failure, forwarding the
// SERVICE'S OWN sentence when it has one.
//
// It used to answer the bare status text and nothing else, so every refusal from
// every service read "Bad Request" — including the carefully worded ones. A
// director creating a pupil before activating the school year was told "Bad
// Request" rather than "no hay un ciclo escolar activo", and had no way to guess.
//
// Forwarding is safe by construction, not by hope: every service sanitises its
// errors at its own boundary (pkg/errors.SafeMessage), so nothing from a driver
// or a query reaches this point. Exported so the behaviour can be tested for what
// it is — the one thing standing between a written message and a person.
//
// A service's machine code for the refusal (the X-Error-Code error metadata,
// e.g. GRADE_DECIMALS) is forwarded as the X-Error-Code response header — only
// when it is shaped like a code, so nothing else a service sent rides along.
func WriteConnectError(w http.ResponseWriter, err error) {
	status := httpStatusFor(err)
	message := connectMessage(err)
	if message == "" {
		message = http.StatusText(status)
	}
	if code := connectErrorCode(err); code != "" {
		w.Header().Set(errorCodeHeader, code)
	}
	http.Error(w, message, status)
}

// errorCodeHeader carries a refusal's machine code to the client (and is the
// error-metadata key the services put it under).
const errorCodeHeader = "X-Error-Code"

// errorCodeShape is what a machine code looks like: UPPER_SNAKE, bounded.
var errorCodeShape = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// connectErrorCode is the service's machine code for err, or "".
func connectErrorCode(err error) string {
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return ""
	}
	if code := ce.Meta().Get(errorCodeHeader); errorCodeShape.MatchString(code) {
		return code
	}
	return ""
}

// writeCodedError answers an edge refusal with its sentence and machine code.
func writeCodedError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set(errorCodeHeader, code)
	http.Error(w, msg, status)
}

// ActivateCycle: POST /api/v1/config/cycles/{id}/activate
//
// A cycle is created as a draft and used to have no way out, so every school year
// read "Borrador" forever while the panel quietly resolved "the active cycle" to
// whichever came first. This is the way out.
func (h *ConfigHandler) ActivateCycle(w http.ResponseWriter, r *http.Request) {
	cycle, err := h.svc.ActivateCycle(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCycleResponse(cycle))
}

// --- school calendar (M16) -------------------------------------------------

type nonWorkingPeriodRequest struct {
	CycleID  string `json:"cycleId"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
}

// CreateNonWorkingPeriod handles POST /api/v1/config/calendar: a day, or a run
// of days, the school does not operate. The collection rules read it, and so
// will the activity calendar.
func (h *ConfigHandler) CreateNonWorkingPeriod(w http.ResponseWriter, r *http.Request) {
	var req nonWorkingPeriodRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// A single-day holiday may send only startsOn; repeating it is noise at the
	// call site, so the edge fills it in.
	if strings.TrimSpace(req.EndsOn) == "" {
		req.EndsOn = req.StartsOn
	}
	period, err := h.svc.CreateNonWorkingPeriod(r.Context(), req.CycleID, req.Label, req.Kind, req.StartsOn, req.EndsOn)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, period)
}

// ListNonWorkingPeriods handles GET /api/v1/config/calendar?cycleId=…
func (h *ConfigHandler) ListNonWorkingPeriods(w http.ResponseWriter, r *http.Request) {
	cycleID, ok := requireUUIDQuery(w, r.URL.Query(), "cycleId")
	if !ok {
		return
	}
	periods, err := h.svc.ListNonWorkingPeriods(r.Context(), cycleID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"periods": periods})
}

// DeleteNonWorkingPeriod handles DELETE /api/v1/config/calendar/{id}. A day
// entered by mistake has to be removable, or the calendar stops being trusted
// and the school goes back to a sheet on the wall.
func (h *ConfigHandler) DeleteNonWorkingPeriod(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteNonWorkingPeriod(r.Context(), r.PathValue("id")); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- school agenda (M16) ---------------------------------------------------

// CreateSchoolEvent handles POST /api/v1/config/events: something the school
// DOES. It is a different endpoint from the calendar of closed days, and that
// is the point — a festival on the 10th does not mean nobody could pay on the
// 10th, and only the closed days move a due date.
func (h *ConfigHandler) CreateSchoolEvent(w http.ResponseWriter, r *http.Request) {
	var req client.SchoolEvent
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.EndsOn) == "" {
		req.EndsOn = req.StartsOn
	}
	event, err := h.svc.CreateSchoolEvent(r.Context(), req)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

// GetSchoolProfile handles GET /api/v1/config/school-profile. A school that has
// not filled its details answers 200 with found=false: the screen shows an
// empty form, and a receipt falls back.
func (h *ConfigHandler) GetSchoolProfile(w http.ResponseWriter, r *http.Request) {
	p, found, err := h.svc.GetSchoolProfile(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": toSchoolProfileBody(p), "found": found})
}

// PutSchoolProfile handles PUT /api/v1/config/school-profile: every field is
// replaced (task 12).
func (h *ConfigHandler) PutSchoolProfile(w http.ResponseWriter, r *http.Request) {
	var req schoolProfileBody
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	in := req.SchoolProfile
	in.CLABE = req.CLABE
	saved, err := h.svc.SetSchoolProfile(r.Context(), in)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": toSchoolProfileBody(saved)})
}

// schoolProfileBody is the school's details as the configuration screen reads
// and writes them: the printed fields plus the CLABE, which only this screen
// and the family's payment info carry.
type schoolProfileBody struct {
	client.SchoolProfile
	CLABE     string `json:"clabe"`
	CLABEBank string `json:"clabeBank"`
}

func toSchoolProfileBody(p client.SchoolProfile) schoolProfileBody {
	return schoolProfileBody{SchoolProfile: p, CLABE: p.CLABE, CLABEBank: p.CLABEBank}
}

// ListSchoolEvents handles GET /api/v1/config/events?cycleId=…
func (h *ConfigHandler) ListSchoolEvents(w http.ResponseWriter, r *http.Request) {
	cycleID, ok := requireUUIDQuery(w, r.URL.Query(), "cycleId")
	if !ok {
		return
	}
	events, err := h.svc.ListSchoolEvents(r.Context(), cycleID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// DeleteSchoolEvent handles DELETE /api/v1/config/events/{id}.
func (h *ConfigHandler) DeleteSchoolEvent(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteSchoolEvent(r.Context(), r.PathValue("id")); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
