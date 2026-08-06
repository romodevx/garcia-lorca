package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// ConfigService is the academic-catalog surface the edge needs (consumer-defined,
// so the handler is testable with a fake). The Connect ConfigClient implements it.
type ConfigService interface {
	CreateCycle(ctx context.Context, in client.CreateCycleInput) (client.CycleResult, error)
	ListCycles(ctx context.Context) ([]client.CycleResult, error)
	CreateLevel(ctx context.Context, in client.CreateLevelInput) (client.LevelResult, error)
	ListLevels(ctx context.Context) ([]client.LevelResult, error)
	CreateGradeLevel(ctx context.Context, in client.CreateGradeLevelInput) (client.GradeLevelResult, error)
	ListGradeLevels(ctx context.Context, levelID string) ([]client.GradeLevelResult, error)
	CreateSubject(ctx context.Context, in client.CreateSubjectInput) (client.SubjectResult, error)
	ListSubjects(ctx context.Context, gradeLevelID string) ([]client.SubjectResult, error)
	CreateTerm(ctx context.Context, in client.CreateTermInput) (client.TermResult, error)
	ListTerms(ctx context.Context, cycleID, levelID string) ([]client.TermResult, error)
	CreateGroup(ctx context.Context, in client.CreateGroupInput) (client.GroupResult, error)
	ListGroups(ctx context.Context, cycleID, gradeLevelID string) ([]client.GroupResult, error)
	CreateTeacher(ctx context.Context, firstName, lastName, email, userID string) (client.Teacher, error)
	ListTeachers(ctx context.Context) ([]client.Teacher, error)
	CreateTeachingAssignment(ctx context.Context, groupID, subjectID, teacherID string) (client.TeachingAssignment, error)
	ListTeachingAssignments(ctx context.Context, cycleID, groupID, teacherID string) ([]client.TeachingAssignment, error)
}

// ConfigHandler exposes the front-facing (HTTP/JSON) academic-catalog endpoints,
// proxying to the config service. It translates JSON ↔ the ConfigClient and
// Connect error codes ↔ HTTP status. Every route must run behind the module
// guard (module_config) plus RequireAuth + rotation-enforcement, with writes
// gated on config:write and reads on config:read.
type ConfigHandler struct {
	svc ConfigService
}

// NewConfigHandler wires the handler to a catalog service.
func NewConfigHandler(svc ConfigService) *ConfigHandler {
	return &ConfigHandler{svc: svc}
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
}

func toLevelResponse(l client.LevelResult) levelResponse {
	return levelResponse{
		ID:                    l.ID,
		Name:                  l.Name,
		SEPCode:               l.SEPCode,
		EnrollmentGranularity: l.Granularity,
	}
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
	}
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

// CreateTeacher handles POST /api/v1/config/teachers.
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

// ListTeachers handles GET /api/v1/config/teachers.
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

// CreateTeachingAssignment handles POST /api/v1/config/teaching-assignments.
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
// GET /api/v1/config/teaching-assignments?cycleId=...&groupId=...&teacherId=...
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
	status := httpStatusFor(err)
	http.Error(w, http.StatusText(status), status)
}
