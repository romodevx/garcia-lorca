package handler

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The school timetable (T16, ADR 0023): the edge of config's timetable. config
// decides every rule (assignments only, no double-booking on the base or on a
// date's effective timetable, reasons once the cycle started); the edge checks
// the shape of a request, turns "HH:MM" into minutes and back, and names the
// caller for the history.

// The edge's bounds and codes.
const (
	maxScheduleReasonLen   = 200 // mirrors config's domain.MaxScheduleReasonLen
	maxScheduleEvents      = 500
	codeScheduleTime       = "SCHEDULE_TIME_INVALID"
	codeScheduleWeekday    = "SCHEDULE_WEEKDAY_INVALID"
	minutesPerHour         = 60
	minutesPerDay          = 24 * minutesPerHour
	firstSchoolDay         = 1
	lastSchoolDay          = 5
	noTeacherID            = "00000000-0000-0000-0000-000000000000"
	overrideKindSubstitute = "substitute"
)

// clockShape is "HH:MM", 00:00 … 24:00.
var clockShape = regexp.MustCompile(`^([01][0-9]|2[0-4]):([0-5][0-9])$`)

// overrideKinds are the date-specific changes config knows.
var overrideKinds = map[string]bool{overrideKindSubstitute: true, "swap": true, "cancel": true}

// ScheduleService is the config timetable surface the handler consumes; the
// Connect ConfigClient satisfies it.
type ScheduleService interface {
	GetScheduleBoard(ctx context.Context, cycleID, teacherID string) (client.ScheduleBoard, error)
	GetScheduleDay(ctx context.Context, cycleID string, date time.Time, teacherID string) (client.ScheduleDay, error)
	CreateScheduleBlock(ctx context.Context, in client.ScheduleBlockInput, by client.ScheduleActor) (client.ScheduleBlock, error)
	UpdateScheduleBlock(ctx context.Context, id string, in client.ScheduleBlockInput, by client.ScheduleActor) (client.ScheduleBlock, error)
	DeleteScheduleBlock(ctx context.Context, id, reason string, by client.ScheduleActor) error
	CreateScheduleOverride(ctx context.Context, in client.ScheduleOverrideInput, by client.ScheduleActor) (client.ScheduleOverride, error)
	DeleteScheduleOverride(ctx context.Context, id, reason string, by client.ScheduleActor) error
	ListAvailableTeachers(ctx context.Context, q client.AvailabilityQuery) ([]client.AvailableTeacher, error)
	SetScheduleGrid(ctx context.Context, levelID string, start, end int) (client.ScheduleGrid, error)
	ListScheduleEvents(ctx context.Context, cycleID string, limit int) ([]client.ScheduleEvent, error)
	// T19 — teacher absences.
	CreateTeacherAbsence(ctx context.Context, in client.TeacherAbsenceInput, by client.ScheduleActor) (client.TeacherAbsenceSaved, error)
	UpdateTeacherAbsence(ctx context.Context, id string, in client.TeacherAbsenceInput, by client.ScheduleActor) (client.TeacherAbsenceSaved, error)
	CancelTeacherAbsence(ctx context.Context, id string, by client.ScheduleActor) (client.TeacherAbsence, error)
	ListTeacherAbsences(ctx context.Context, q client.TeacherAbsenceQuery) ([]client.TeacherAbsence, error)
	ListUncoveredClasses(ctx context.Context, cycleID string, from, to time.Time) ([]client.UncoveredClass, error)
	// The teacher's own timetable resolves the caller's teacher record and the
	// active cycle.
	TeacherDirectoryReader
	TeachingCycleReader
}

// ScheduleHandler serves /api/v1/schedule* and the teacher's own
// /api/v1/teaching/schedule*.
type ScheduleHandler struct {
	svc   ScheduleService
	names UserNamer
}

// NewScheduleHandler wires the handler; names resolves the caller's display
// name for the history (nil: the token email).
func NewScheduleHandler(svc ScheduleService, names UserNamer) *ScheduleHandler {
	return &ScheduleHandler{svc: svc, names: names}
}

// --- wire shapes ------------------------------------------------------------------

type scheduleCycleJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
	Active   bool   `json:"active"`
}

type scheduleSettingsJSON struct {
	LevelID   string `json:"levelId"`
	GridStart string `json:"gridStart"`
	GridEnd   string `json:"gridEnd"`
}

type scheduleBlockJSON struct {
	ID           string `json:"id"`
	CycleID      string `json:"cycleId"`
	AssignmentID string `json:"assignmentId"`
	GroupID      string `json:"groupId"`
	SubjectID    string `json:"subjectId"`
	TeacherID    string `json:"teacherId"`
	Weekday      int    `json:"weekday"`
	StartsAt     string `json:"startsAt"`
	EndsAt       string `json:"endsAt"`
}

type scheduleLevelJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type scheduleGradeJSON struct {
	ID      string `json:"id"`
	LevelID string `json:"levelId"`
	Name    string `json:"name"`
	Ordinal int    `json:"ordinal"`
}

type scheduleNamedJSON struct {
	ID           string `json:"id"`
	GradeLevelID string `json:"gradeLevelId"`
	Name         string `json:"name"`
}

type scheduleTeacherJSON struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

type scheduleAssignmentJSON struct {
	ID        string `json:"id"`
	GroupID   string `json:"groupId"`
	SubjectID string `json:"subjectId"`
	TeacherID string `json:"teacherId"`
}

type scheduleBoardJSON struct {
	Cycle       scheduleCycleJSON        `json:"cycle"`
	Cycles      []scheduleCycleJSON      `json:"cycles"`
	Started     bool                     `json:"started"`
	Settings    []scheduleSettingsJSON   `json:"settings"`
	Levels      []scheduleLevelJSON      `json:"levels"`
	Grades      []scheduleGradeJSON      `json:"grades"`
	Groups      []scheduleNamedJSON      `json:"groups"`
	Subjects    []scheduleNamedJSON      `json:"subjects"`
	Teachers    []scheduleTeacherJSON    `json:"teachers"`
	Assignments []scheduleAssignmentJSON `json:"assignments"`
	Blocks      []scheduleBlockJSON      `json:"blocks"`
}

type scheduleClassJSON struct {
	BlockID       string  `json:"blockId"`
	GroupID       string  `json:"groupId"`
	SubjectID     string  `json:"subjectId"`
	TeacherID     string  `json:"teacherId"`
	StartsAt      string  `json:"startsAt"`
	EndsAt        string  `json:"endsAt"`
	Status        string  `json:"status"`
	BaseTeacherID string  `json:"baseTeacherId"`
	BaseStartsAt  string  `json:"baseStartsAt"`
	BaseEndsAt    string  `json:"baseEndsAt"`
	OverrideID    *string `json:"overrideId"`
	Reason        *string `json:"reason"`
	// Absence is why an "uncovered" class has nobody (T19), shaped by caller.
	Absence *teacherAbsenceJSON `json:"absence"`
}

type scheduleDayJSON struct {
	Date       string              `json:"date"`
	Weekday    int                 `json:"weekday"`
	NonWorking *string             `json:"nonWorking"`
	Classes    []scheduleClassJSON `json:"classes"`
}

type scheduleOverrideJSON struct {
	ID            string    `json:"id"`
	CycleID       string    `json:"cycleId"`
	BlockID       string    `json:"blockId"`
	Date          string    `json:"date"`
	Kind          string    `json:"kind"`
	TeacherID     *string   `json:"teacherId"`
	SwapBlockID   *string   `json:"swapBlockId"`
	Reason        string    `json:"reason"`
	CreatedBy     string    `json:"createdBy"`
	CreatedByName string    `json:"createdByName"`
	CreatedAt     time.Time `json:"createdAt"`
}

type scheduleEventJSON struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	ActorID    string    `json:"actorId"`
	ActorName  string    `json:"actorName"`
	Action     string    `json:"action"`
	BlockID    *string   `json:"blockId"`
	OverrideID *string   `json:"overrideId"`
	AbsenceID  *string   `json:"absenceId"`
	Date       *string   `json:"date"`
	Reason     string    `json:"reason"`
	Summary    string    `json:"summary"`
}

type availableTeacherJSON struct {
	TeacherID string `json:"teacherId"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Match     string `json:"match"`
}

// --- the director's timetable --------------------------------------------------

// Board handles GET /api/v1/schedule?cycleId= (active cycle when omitted).
func (h *ScheduleHandler) Board(w http.ResponseWriter, r *http.Request) {
	cycleID := r.URL.Query().Get("cycleId")
	if cycleID != "" && !uuidShape.MatchString(cycleID) {
		http.Error(w, "cycleId must be an id", http.StatusBadRequest)
		return
	}
	b, err := h.svc.GetScheduleBoard(r.Context(), strings.ToLower(cycleID), "")
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toBoardJSON(b))
}

// Day handles GET /api/v1/schedule/day?cycleId=&date=.
func (h *ScheduleHandler) Day(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cycleID := q.Get("cycleId")
	date, err := time.Parse(dateLayout, q.Get("date"))
	if !uuidShape.MatchString(cycleID) || err != nil {
		http.Error(w, "cycleId and date (YYYY-MM-DD) are required", http.StatusBadRequest)
		return
	}
	d, err := h.svc.GetScheduleDay(r.Context(), strings.ToLower(cycleID), date, "")
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDayJSON(d, callerHolds(r, permScheduleWrite)))
}

type scheduleBlockRequest struct {
	AssignmentID string `json:"assignmentId"`
	Weekday      int    `json:"weekday"`
	StartsAt     string `json:"startsAt"`
	EndsAt       string `json:"endsAt"`
	Reason       string `json:"reason"`
}

// CreateBlock handles POST /api/v1/schedule/blocks.
func (h *ScheduleHandler) CreateBlock(w http.ResponseWriter, r *http.Request) {
	in, ok := readBlockInput(w, r)
	if !ok {
		return
	}
	b, err := h.svc.CreateScheduleBlock(r.Context(), in, h.actor(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"block": toBlockJSON(b)})
}

// UpdateBlock handles PUT /api/v1/schedule/blocks/{id}.
func (h *ScheduleHandler) UpdateBlock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in, ok := readBlockInput(w, r)
	if !ok {
		return
	}
	b, err := h.svc.UpdateScheduleBlock(r.Context(), id, in, h.actor(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"block": toBlockJSON(b)})
}

// DeleteBlock handles DELETE /api/v1/schedule/blocks/{id}?reason=.
func (h *ScheduleHandler) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	reason, ok := queryReason(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteScheduleBlock(r.Context(), id, reason, h.actor(r)); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type scheduleOverrideRequest struct {
	BlockID     string `json:"blockId"`
	Date        string `json:"date"`
	Kind        string `json:"kind"`
	TeacherID   string `json:"teacherId"`
	SwapBlockID string `json:"swapBlockId"`
	Reason      string `json:"reason"`
}

// CreateOverride handles POST /api/v1/schedule/overrides.
func (h *ScheduleHandler) CreateOverride(w http.ResponseWriter, r *http.Request) {
	var req scheduleOverrideRequest
	if err := decodeJSONBody(w, r, &req); err != nil || !uuidShape.MatchString(req.BlockID) || !overrideKinds[req.Kind] ||
		(req.TeacherID != "" && !uuidShape.MatchString(req.TeacherID)) || (req.SwapBlockID != "" && !uuidShape.MatchString(req.SwapBlockID)) ||
		utf8.RuneCountInString(req.Reason) > maxScheduleReasonLen {
		http.Error(w, `invalid request body (want {"blockId","date","kind":"substitute|swap|cancel","teacherId"?,"swapBlockId"?,"reason"})`, http.StatusBadRequest)
		return
	}
	date, err := time.Parse(dateLayout, req.Date)
	if err != nil {
		http.Error(w, "date must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	o, err := h.svc.CreateScheduleOverride(r.Context(), client.ScheduleOverrideInput{
		BlockID: strings.ToLower(req.BlockID), Date: date, Kind: req.Kind, TeacherID: strings.ToLower(req.TeacherID),
		SwapBlockID: strings.ToLower(req.SwapBlockID), Reason: req.Reason,
	}, h.actor(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"override": toOverrideJSON(o)})
}

// DeleteOverride handles DELETE /api/v1/schedule/overrides/{id}?reason=.
func (h *ScheduleHandler) DeleteOverride(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	reason, ok := queryReason(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteScheduleOverride(r.Context(), id, reason, h.actor(r)); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AvailableTeachers handles GET /api/v1/schedule/available-teachers.
func (h *ScheduleHandler) AvailableTeachers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for _, key := range []string{"cycleId", "groupId", "subjectId"} {
		if !uuidShape.MatchString(q.Get(key)) {
			http.Error(w, key+" is required", http.StatusBadRequest)
			return
		}
	}
	if ex := q.Get("excludeBlockId"); ex != "" && !uuidShape.MatchString(ex) {
		http.Error(w, "excludeBlockId must be an id", http.StatusBadRequest)
		return
	}
	start, end, ok := clockRange(w, q.Get("startsAt"), q.Get("endsAt"))
	if !ok {
		return
	}
	query := client.AvailabilityQuery{
		CycleID: strings.ToLower(q.Get("cycleId")), GroupID: strings.ToLower(q.Get("groupId")),
		SubjectID: strings.ToLower(q.Get("subjectId")), StartsAt: start, EndsAt: end,
		ExcludeBlockID: strings.ToLower(q.Get("excludeBlockId")),
	}
	if raw := q.Get("date"); raw != "" {
		date, err := time.Parse(dateLayout, raw)
		if err != nil {
			http.Error(w, "date must be YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		query.Date = date
	} else {
		wd, err := strconv.Atoi(q.Get("weekday"))
		if err != nil || wd < firstSchoolDay || wd > lastSchoolDay {
			writeCodedError(w, http.StatusBadRequest, codeScheduleWeekday, "Indica la fecha o el día (lunes 1 a viernes 5)")
			return
		}
		query.Weekday = wd
	}
	free, err := h.svc.ListAvailableTeachers(r.Context(), query)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]availableTeacherJSON, 0, len(free))
	for _, a := range free {
		out = append(out, availableTeacherJSON{TeacherID: a.Teacher.ID, FirstName: a.Teacher.FirstName, LastName: a.Teacher.LastName, Match: a.Match})
	}
	writeJSON(w, http.StatusOK, map[string]any{"teachers": out})
}

type scheduleSettingsRequest struct {
	GridStart string `json:"gridStart"`
	GridEnd   string `json:"gridEnd"`
}

// PutSettings handles PUT /api/v1/schedule/settings/{levelId}.
func (h *ScheduleHandler) PutSettings(w http.ResponseWriter, r *http.Request) {
	levelID, ok := pathID(w, r, "levelId")
	if !ok {
		return
	}
	var req scheduleSettingsRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, `invalid request body (want {"gridStart":"HH:MM","gridEnd":"HH:MM"})`, http.StatusBadRequest)
		return
	}
	start, end, ok := clockRange(w, req.GridStart, req.GridEnd)
	if !ok {
		return
	}
	g, err := h.svc.SetScheduleGrid(r.Context(), levelID, start, end)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": toSettingsJSON(g)})
}

// Events handles GET /api/v1/schedule/events?cycleId=&limit=.
func (h *ScheduleHandler) Events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cycleID := q.Get("cycleId")
	if !uuidShape.MatchString(cycleID) {
		http.Error(w, "cycleId is required", http.StatusBadRequest)
		return
	}
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxScheduleEvents {
			http.Error(w, fmt.Sprintf("limit must be 1–%d", maxScheduleEvents), http.StatusBadRequest)
			return
		}
		limit = n
	}
	evs, err := h.svc.ListScheduleEvents(r.Context(), strings.ToLower(cycleID), limit)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]scheduleEventJSON, 0, len(evs))
	for _, e := range evs {
		ev := scheduleEventJSON{
			ID: e.ID, At: e.At, ActorID: e.By.UserID, ActorName: e.By.Name, Action: e.Action,
			BlockID: optional(e.BlockID), OverrideID: optional(e.OverrideID), AbsenceID: optional(e.AbsenceID), Reason: e.Reason, Summary: e.Summary,
		}
		if !e.Date.IsZero() {
			ev.Date = optional(formatDate(e.Date))
		}
		out = append(out, ev)
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}

// --- the teacher's own timetable --------------------------------------------------

// OwnBoard handles GET /api/v1/teaching/schedule: the active cycle's board,
// narrowed to the caller's own classes and assignments. The teacher comes from
// the principal, never from the request; a caller with no teacher record gets
// an empty timetable (config is asked for a teacher no class has, never for
// "everybody").
func (h *ScheduleHandler) OwnBoard(w http.ResponseWriter, r *http.Request) {
	teacherID, ok := h.callerTeacher(w, r)
	if !ok {
		return
	}
	b, err := h.svc.GetScheduleBoard(r.Context(), "", teacherID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// config already narrowed the classes; the edge narrows again (defence in
	// depth) and drops the other teachers' assignments.
	own := b.Assignments[:0:0]
	for _, a := range b.Assignments {
		if a.TeacherID == teacherID {
			own = append(own, a)
		}
	}
	b.Assignments = own
	blocks := b.Blocks[:0:0]
	for _, bl := range b.Blocks {
		if bl.TeacherID == teacherID {
			blocks = append(blocks, bl)
		}
	}
	b.Blocks = blocks
	writeJSON(w, http.StatusOK, toBoardJSON(b))
}

// OwnDay handles GET /api/v1/teaching/schedule/day?date=: the caller's
// classes that date in the active cycle, covers included.
func (h *ScheduleHandler) OwnDay(w http.ResponseWriter, r *http.Request) {
	date, err := time.Parse(dateLayout, r.URL.Query().Get("date"))
	if err != nil {
		http.Error(w, "date must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	teacherID, ok := h.callerTeacher(w, r)
	if !ok {
		return
	}
	cycles, err := h.svc.ListCycles(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	cycleID := ""
	for _, c := range cycles {
		if c.Status == cycleStatusActive {
			cycleID = c.ID
		}
	}
	if cycleID == "" {
		http.Error(w, "No hay un ciclo activo", http.StatusNotFound)
		return
	}
	d, err := h.svc.GetScheduleDay(r.Context(), cycleID, date, teacherID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	mine := d.Classes[:0:0]
	for _, c := range d.Classes {
		if c.TeacherID == teacherID || c.Block.TeacherID == teacherID {
			mine = append(mine, c)
		}
	}
	d.Classes = mine
	// "Mi horario" says only "Ausente": never the kind nor the note.
	writeJSON(w, http.StatusOK, toDayJSON(d, false))
}

// callerTeacher is the teacher record linked to the caller, or noTeacherID.
func (h *ScheduleHandler) callerTeacher(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	teachers, err := h.svc.ListTeachers(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return "", false
	}
	for _, t := range teachers {
		if t.UserID != "" && t.UserID == p.UserID {
			return t.ID, true
		}
	}
	return noTeacherID, true
}

// --- helpers --------------------------------------------------------------------

func (h *ScheduleHandler) actor(r *http.Request) client.ScheduleActor {
	p, _ := middleware.PrincipalFrom(r.Context())
	return client.ScheduleActor{UserID: p.UserID, Name: callerName(r.Context(), h.names, p)}
}

func readBlockInput(w http.ResponseWriter, r *http.Request) (client.ScheduleBlockInput, bool) {
	var req scheduleBlockRequest
	if err := decodeJSONBody(w, r, &req); err != nil || !uuidShape.MatchString(req.AssignmentID) ||
		utf8.RuneCountInString(req.Reason) > maxScheduleReasonLen {
		http.Error(w, `invalid request body (want {"assignmentId","weekday","startsAt":"HH:MM","endsAt":"HH:MM","reason"?})`, http.StatusBadRequest)
		return client.ScheduleBlockInput{}, false
	}
	if req.Weekday < firstSchoolDay || req.Weekday > lastSchoolDay {
		writeCodedError(w, http.StatusBadRequest, codeScheduleWeekday, "El día va de lunes (1) a viernes (5)")
		return client.ScheduleBlockInput{}, false
	}
	start, end, ok := clockRange(w, req.StartsAt, req.EndsAt)
	if !ok {
		return client.ScheduleBlockInput{}, false
	}
	return client.ScheduleBlockInput{
		AssignmentID: strings.ToLower(req.AssignmentID), Weekday: req.Weekday, StartsAt: start, EndsAt: end, Reason: req.Reason,
	}, true
}

// clockRange parses two "HH:MM" times (24:00 the latest); a malformed one is
// 400 SCHEDULE_TIME_INVALID. The order and the 5-minute steps are config's.
func clockRange(w http.ResponseWriter, from, to string) (int, int, bool) {
	start, okStart := parseClock(from)
	end, okEnd := parseClock(to)
	if !okStart || !okEnd {
		writeCodedError(w, http.StatusBadRequest, codeScheduleTime, "Hora inválida (usa HH:MM, de 00:00 a 24:00)")
		return 0, 0, false
	}
	return start, end, true
}

func parseClock(s string) (int, bool) {
	m := clockShape.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	h, _ := strconv.Atoi(m[1])
	mins, _ := strconv.Atoi(m[2])
	total := h*minutesPerHour + mins
	if total > minutesPerDay {
		return 0, false
	}
	return total, true
}

func formatClock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/minutesPerHour, minutes%minutesPerHour)
}

func pathID(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	id := r.PathValue(key)
	if !uuidShape.MatchString(id) {
		http.Error(w, key+" must be an id", http.StatusBadRequest)
		return "", false
	}
	return strings.ToLower(id), true
}

func queryReason(w http.ResponseWriter, r *http.Request) (string, bool) {
	reason := r.URL.Query().Get("reason")
	if utf8.RuneCountInString(reason) > maxScheduleReasonLen {
		http.Error(w, fmt.Sprintf("reason admits up to %d characters", maxScheduleReasonLen), http.StatusBadRequest)
		return "", false
	}
	return reason, true
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toCycleJSON(c client.CycleResult) scheduleCycleJSON {
	return scheduleCycleJSON{ID: c.ID, Name: c.Name, StartsOn: formatDate(c.StartsOn), EndsOn: formatDate(c.EndsOn), Active: c.Status == cycleStatusActive}
}

func toSettingsJSON(g client.ScheduleGrid) scheduleSettingsJSON {
	return scheduleSettingsJSON{LevelID: g.LevelID, GridStart: formatClock(g.GridStart), GridEnd: formatClock(g.GridEnd)}
}

func toBlockJSON(b client.ScheduleBlock) scheduleBlockJSON {
	return scheduleBlockJSON{
		ID: b.ID, CycleID: b.CycleID, AssignmentID: b.AssignmentID, GroupID: b.GroupID, SubjectID: b.SubjectID,
		TeacherID: b.TeacherID, Weekday: b.Weekday, StartsAt: formatClock(b.StartsAt), EndsAt: formatClock(b.EndsAt),
	}
}

func toOverrideJSON(o client.ScheduleOverride) scheduleOverrideJSON {
	return scheduleOverrideJSON{
		ID: o.ID, CycleID: o.CycleID, BlockID: o.BlockID, Date: formatDate(o.Date), Kind: o.Kind,
		TeacherID: optional(o.TeacherID), SwapBlockID: optional(o.SwapBlockID), Reason: o.Reason,
		CreatedBy: o.CreatedBy.UserID, CreatedByName: o.CreatedBy.Name, CreatedAt: o.CreatedAt,
	}
}

// toDayJSON renders a date's classes; private says whether the caller may
// read an absence's kind and note (schedule:write).
func toDayJSON(d client.ScheduleDay, private bool) scheduleDayJSON {
	out := scheduleDayJSON{Date: formatDate(d.Date), Weekday: d.Weekday, NonWorking: optional(d.NonWorking), Classes: make([]scheduleClassJSON, 0, len(d.Classes))}
	for _, c := range d.Classes {
		out.Classes = append(out.Classes, toClassJSON(c, private))
	}
	return out
}

func toClassJSON(c client.ScheduledClass, private bool) scheduleClassJSON {
	cj := scheduleClassJSON{
		BlockID: c.Block.ID, GroupID: c.Block.GroupID, SubjectID: c.Block.SubjectID, TeacherID: c.TeacherID,
		StartsAt: formatClock(c.StartsAt), EndsAt: formatClock(c.EndsAt), Status: c.Status,
		BaseTeacherID: c.Block.TeacherID, BaseStartsAt: formatClock(c.Block.StartsAt), BaseEndsAt: formatClock(c.Block.EndsAt),
	}
	if c.Override != nil {
		cj.OverrideID, cj.Reason = optional(c.Override.ID), optional(c.Override.Reason)
	}
	if c.Absence != nil {
		a := toAbsenceJSON(*c.Absence, private)
		cj.Absence = &a
	}
	return cj
}

func toBoardJSON(b client.ScheduleBoard) scheduleBoardJSON {
	out := scheduleBoardJSON{
		Cycle: toCycleJSON(b.Cycle), Started: b.Started,
		Cycles: make([]scheduleCycleJSON, 0, len(b.Cycles)), Settings: make([]scheduleSettingsJSON, 0, len(b.Grids)),
		Levels: make([]scheduleLevelJSON, 0, len(b.Levels)), Grades: make([]scheduleGradeJSON, 0, len(b.Grades)),
		Groups: make([]scheduleNamedJSON, 0, len(b.Groups)), Subjects: make([]scheduleNamedJSON, 0, len(b.Subjects)),
		Teachers: make([]scheduleTeacherJSON, 0, len(b.Teachers)), Assignments: make([]scheduleAssignmentJSON, 0, len(b.Assignments)),
		Blocks: make([]scheduleBlockJSON, 0, len(b.Blocks)),
	}
	for _, c := range b.Cycles {
		out.Cycles = append(out.Cycles, toCycleJSON(c))
	}
	for _, g := range b.Grids {
		out.Settings = append(out.Settings, toSettingsJSON(g))
	}
	for _, l := range b.Levels {
		out.Levels = append(out.Levels, scheduleLevelJSON{ID: l.ID, Name: l.Name})
	}
	for _, g := range b.Grades {
		out.Grades = append(out.Grades, scheduleGradeJSON{ID: g.ID, LevelID: g.LevelID, Name: g.Name, Ordinal: g.Ordinal})
	}
	for _, g := range b.Groups {
		out.Groups = append(out.Groups, scheduleNamedJSON{ID: g.ID, GradeLevelID: g.GradeLevelID, Name: g.Name})
	}
	for _, s := range b.Subjects {
		out.Subjects = append(out.Subjects, scheduleNamedJSON{ID: s.ID, GradeLevelID: s.GradeLevelID, Name: s.Name})
	}
	for _, t := range b.Teachers {
		out.Teachers = append(out.Teachers, scheduleTeacherJSON{ID: t.ID, FirstName: t.FirstName, LastName: t.LastName})
	}
	for _, a := range b.Assignments {
		out.Assignments = append(out.Assignments, scheduleAssignmentJSON{ID: a.ID, GroupID: a.GroupID, SubjectID: a.SubjectID, TeacherID: a.TeacherID})
	}
	for _, bl := range b.Blocks {
		out.Blocks = append(out.Blocks, toBlockJSON(bl))
	}
	return out
}
