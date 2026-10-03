package handler

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// Teacher absences in the timetable (T19, ADR 0032). config decides every
// rule (dates inside the cycle, no overlapping absences, who may be put in
// class); the edge checks the request's shape, refuses a teacher registering
// their own absence, and SHAPES what each caller reads: an absence's kind,
// note and who registered or cancelled it reach only schedule:write holders.
// Everyone else — schedule:read, and every teacher in "Mi horario" — reads
// only that the teacher is away and when.

const (
	permScheduleWrite = "schedule:write"
	maxAbsenceNoteLen = 500 // mirrors config's domain.MaxAbsenceNoteLen
	maxAbsenceListDay = 366 // a list read spans at most a school year
	codeAbsenceOwn    = "SCHEDULE_ABSENCE_OWN"
	hoursPerDay       = 24
)

var absenceKinds = map[string]bool{"sick": true, "vacation": true, "personal": true, "other": true}

// teacherAbsenceJSON is an absence on the wire. The pointer fields tagged
// omitempty are the private ones: absent from the JSON unless the caller holds
// schedule:write.
type teacherAbsenceJSON struct {
	ID              string     `json:"id"`
	CycleID         string     `json:"cycleId"`
	TeacherID       string     `json:"teacherId"`
	StartsOn        string     `json:"startsOn"`
	EndsOn          string     `json:"endsOn"`
	AllDay          bool       `json:"allDay"`
	StartsAt        *string    `json:"startsAt"`
	EndsAt          *string    `json:"endsAt"`
	Cancelled       bool       `json:"cancelled"`
	Kind            *string    `json:"kind,omitempty"`
	Note            *string    `json:"note,omitempty"`
	CreatedByName   *string    `json:"createdByName,omitempty"`
	CreatedAt       *time.Time `json:"createdAt,omitempty"`
	CancelledAt     *time.Time `json:"cancelledAt,omitempty"`
	CancelledByName *string    `json:"cancelledByName,omitempty"`
}

type uncoveredClassJSON struct {
	Date  string            `json:"date"`
	Class scheduleClassJSON `json:"class"`
}

type absenceRequest struct {
	CycleID   string `json:"cycleId"`
	TeacherID string `json:"teacherId"`
	Kind      string `json:"kind"`
	StartsOn  string `json:"startsOn"`
	EndsOn    string `json:"endsOn"`
	AllDay    bool   `json:"allDay"`
	StartsAt  string `json:"startsAt"`
	EndsAt    string `json:"endsAt"`
	Note      string `json:"note"`
}

// toAbsenceJSON shapes a for its reader: private carries the kind, the note
// and who registered or cancelled it.
func toAbsenceJSON(a client.TeacherAbsence, private bool) teacherAbsenceJSON {
	out := teacherAbsenceJSON{
		ID: a.ID, CycleID: a.CycleID, TeacherID: a.TeacherID, StartsOn: formatDate(a.StartsOn), EndsOn: formatDate(a.EndsOn),
		AllDay: a.AllDay, Cancelled: !a.CancelledAt.IsZero(),
	}
	if !a.AllDay {
		out.StartsAt, out.EndsAt = optional(formatClock(a.StartMinute)), optional(formatClock(a.EndMinute))
	}
	if !private {
		return out
	}
	kind, note, by, at := a.Kind, a.Note, a.CreatedBy.Name, a.CreatedAt
	out.Kind, out.Note, out.CreatedByName, out.CreatedAt = &kind, &note, &by, &at
	if out.Cancelled {
		cancelledAt, cancelledBy := a.CancelledAt, a.CancelledBy.Name
		out.CancelledAt, out.CancelledByName = &cancelledAt, &cancelledBy
	}
	return out
}

// Absences handles GET /api/v1/schedule/absences?cycleId=&from=&to=&teacherId=&includeCancelled=.
func (h *ScheduleHandler) Absences(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cycleID, from, to, ok := cycleRange(w, r)
	if !ok {
		return
	}
	if int(to.Sub(from).Hours()/hoursPerDay)+1 > maxAbsenceListDay { // both are midnight UTC: whole days
		http.Error(w, "from and to are at most a year apart", http.StatusBadRequest)
		return
	}
	teacherID := q.Get("teacherId")
	if teacherID != "" && !uuidShape.MatchString(teacherID) {
		http.Error(w, "teacherId must be an id", http.StatusBadRequest)
		return
	}
	list, err := h.svc.ListTeacherAbsences(r.Context(), client.TeacherAbsenceQuery{
		CycleID: cycleID, TeacherID: strings.ToLower(teacherID), From: from, To: to, IncludeCancelled: q.Get("includeCancelled") == "true",
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	private := callerHolds(r, permScheduleWrite)
	out := make([]teacherAbsenceJSON, 0, len(list))
	for _, a := range list {
		out = append(out, toAbsenceJSON(a, private))
	}
	writeJSON(w, http.StatusOK, map[string]any{"absences": out})
}

// CreateAbsence handles POST /api/v1/schedule/absences.
func (h *ScheduleHandler) CreateAbsence(w http.ResponseWriter, r *http.Request) {
	in, ok := h.readAbsence(w, r)
	if !ok {
		return
	}
	saved, err := h.svc.CreateTeacherAbsence(r.Context(), in, h.actor(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, savedAbsenceJSON(saved))
}

// UpdateAbsence handles PUT /api/v1/schedule/absences/{id}.
func (h *ScheduleHandler) UpdateAbsence(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in, ok := h.readAbsence(w, r)
	if !ok {
		return
	}
	saved, err := h.svc.UpdateTeacherAbsence(r.Context(), id, in, h.actor(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, savedAbsenceJSON(saved))
}

// CancelAbsence handles POST /api/v1/schedule/absences/{id}/cancel.
func (h *ScheduleHandler) CancelAbsence(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	// config refuses cancelling the caller's own absence (SCHEDULE_ABSENCE_OWN).
	own, ok := h.callerTeacher(w, r)
	if !ok {
		return
	}
	a, err := h.svc.CancelTeacherAbsence(r.Context(), id, h.actor(r), own)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"absence": toAbsenceJSON(a, true)})
}

// Uncovered handles GET /api/v1/schedule/uncovered?cycleId=&from=&to=.
func (h *ScheduleHandler) Uncovered(w http.ResponseWriter, r *http.Request) {
	cycleID, from, to, ok := cycleRange(w, r)
	if !ok {
		return
	}
	list, err := h.svc.ListUncoveredClasses(r.Context(), cycleID, from, to)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	private := callerHolds(r, permScheduleWrite)
	out := make([]uncoveredClassJSON, 0, len(list))
	for _, u := range list {
		out = append(out, uncoveredClassJSON{Date: formatDate(u.Date), Class: toClassJSON(u.Class, private)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"classes": out})
}

// savedAbsenceJSON answers a write: only schedule:write reaches it, so the
// absence goes whole.
func savedAbsenceJSON(s client.TeacherAbsenceSaved) map[string]any {
	return map[string]any{"absence": toAbsenceJSON(s.Absence, true), "uncoveredCount": s.Uncovered}
}

// readAbsence parses and checks an absence body, and refuses the caller's
// own absence (403 SCHEDULE_ABSENCE_OWN): a teacher does not register theirs.
func (h *ScheduleHandler) readAbsence(w http.ResponseWriter, r *http.Request) (client.TeacherAbsenceInput, bool) {
	var req absenceRequest
	if err := decodeJSONBody(w, r, &req); err != nil || !uuidShape.MatchString(req.TeacherID) || !absenceKinds[req.Kind] ||
		(req.CycleID != "" && !uuidShape.MatchString(req.CycleID)) || utf8.RuneCountInString(req.Note) > maxAbsenceNoteLen {
		http.Error(w, `invalid request body (want {"cycleId"?,"teacherId","kind":"sick|vacation|personal|other","startsOn","endsOn","allDay","startsAt"?,"endsAt"?,"note"?})`, http.StatusBadRequest)
		return client.TeacherAbsenceInput{}, false
	}
	from, errFrom := time.Parse(dateLayout, req.StartsOn)
	to, errTo := time.Parse(dateLayout, req.EndsOn)
	if errFrom != nil || errTo != nil {
		http.Error(w, "startsOn and endsOn must be YYYY-MM-DD", http.StatusBadRequest)
		return client.TeacherAbsenceInput{}, false
	}
	in := client.TeacherAbsenceInput{
		CycleID: strings.ToLower(req.CycleID), TeacherID: strings.ToLower(req.TeacherID), Kind: req.Kind,
		StartsOn: from, EndsOn: to, AllDay: req.AllDay, Note: req.Note,
	}
	if !req.AllDay {
		start, end, ok := clockRange(w, req.StartsAt, req.EndsAt)
		if !ok {
			return client.TeacherAbsenceInput{}, false
		}
		in.StartMinute, in.EndMinute = start, end
	}
	own, ok := h.callerTeacher(w, r)
	if !ok {
		return client.TeacherAbsenceInput{}, false
	}
	if own == in.TeacherID {
		writeCodedError(w, http.StatusForbidden, codeAbsenceOwn, "No puedes registrar tu propia ausencia; pídeselo a la dirección")
		return client.TeacherAbsenceInput{}, false
	}
	return in, true
}

// cycleRange reads ?cycleId=&from=&to= (dates YYYY-MM-DD, from <= to).
func cycleRange(w http.ResponseWriter, r *http.Request) (string, time.Time, time.Time, bool) {
	q := r.URL.Query()
	cycleID := q.Get("cycleId")
	from, errFrom := time.Parse(dateLayout, q.Get("from"))
	to, errTo := time.Parse(dateLayout, q.Get("to"))
	if !uuidShape.MatchString(cycleID) || errFrom != nil || errTo != nil || to.Before(from) {
		http.Error(w, "cycleId, from and to (YYYY-MM-DD, from <= to) are required", http.StatusBadRequest)
		return "", time.Time{}, time.Time{}, false
	}
	return strings.ToLower(cycleID), from, to, true
}
