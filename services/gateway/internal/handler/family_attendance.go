package handler

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// family_attendance.go is the family app's per-day attendance view (T5): one row
// per school day the child had a list taken (or a stored exception), plus today,
// instead of one row per stored exception — so a per-class school no longer
// counts six "faltas" for one day.

// The day statuses the view answers with. present/absent/late/excused are the
// attendance vocabulary; partial is a day absent in some lists but not all, and
// not_taken is today before any list was taken.
const (
	dayPresent  = "present"
	dayAbsent   = "absent"
	dayLate     = "late"
	dayExcused  = "excused"
	dayPartial  = "partial"
	dayNotTaken = "not_taken"
)

// monthLayout is the ?month= form (YYYY-MM).
const monthLayout = "2006-01"

// msgInvalidMonth is the 400 sentence for a malformed ?month=.
const msgInvalidMonth = "month must be YYYY-MM"

type familyClassResponse struct {
	SubjectName string `json:"subjectName"`
	Status      string `json:"status"`
	Note        string `json:"note,omitempty"`
}

type familyDayResponse struct {
	Date    string                `json:"date"`
	Status  string                `json:"status"`
	Mode    string                `json:"mode"`
	Note    string                `json:"note,omitempty"`
	Classes []familyClassResponse `json:"classes"`
}

type familyAttendanceSummary struct {
	Present int `json:"present"`
	Absent  int `json:"absent"`
	Late    int `json:"late"`
	Excused int `json:"excused"`
	Partial int `json:"partial"`
}

type familyAttendanceResponse struct {
	Month   string                  `json:"month"`
	Today   string                  `json:"today"`
	Days    []familyDayResponse     `json:"days"`
	Summary familyAttendanceSummary `json:"summary"`
}

// maxSessionGroups is how many groups one ListSessions call may name (the
// attendance service's documented bound); more are read in chunks.
const maxSessionGroups = 20

// firstServedMonth is the earliest ?month= the view answers; later than the
// month after today's is refused too (review #11).
var firstServedMonth = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

// Attendance handles GET /api/v1/family/students/{studentId}/attendance?month=
// — the child's month, one row per day, newest first (see buildAttendanceDays).
// month defaults to the current UTC month. The school day is the UTC date on
// purpose (schoolToday): it is the same fleet-wide offset finance and the
// teachers' lists use, so every date the school sees agrees. Another family's
// child is 404, like every by-child route.
//
// Reads: the child's enrollments, the school years and the child's transfer
// history (once each) to place the group windows; then, only for the windows
// overlapping the month, the enrollments' exceptions (concurrently), the lists
// taken for those groups (in chunks of maxSessionGroups, concurrently) and —
// only when a per-class list is in the month — the subject names once.
func (h *FamilyHandler) Attendance(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	studentID := r.PathValue("studentId")
	if !h.guardOwnsChild(w, r, p.UserID, studentID) {
		return
	}
	today := schoolToday(h.now())
	first, ok := parseMonth(r.URL.Query().Get("month"), today)
	if !ok {
		writeBadRequest(w, msgInvalidMonth)
		return
	}
	windows, err := h.groupWindows(r.Context(), studentID, today)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	view, err := h.readMonth(r.Context(), windows, first)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	view.today = today
	view.subjectNames = h.subjectNames(r.Context(), view.hasPerClass())
	days, summary := buildAttendanceDays(view)
	writeJSON(w, http.StatusOK, familyAttendanceResponse{
		Month:   first.Format(monthLayout),
		Today:   today.Format(attendanceDateLayout),
		Days:    days,
		Summary: summary,
	})
}

// parseMonth reads ?month=YYYY-MM as its first day; empty is today's month.
// Months before firstServedMonth or after the one following today's are
// refused: nothing is recorded there, and an absurd year is a client bug.
func parseMonth(value string, today time.Time) (time.Time, bool) {
	current := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	if value == "" {
		return current, true
	}
	first, err := time.Parse(monthLayout, value)
	if err != nil || first.Before(firstServedMonth) || first.After(current.AddDate(0, 1, 0)) {
		return time.Time{}, false
	}
	return first, true
}

// monthView is everything the day derivation needs, already read.
type monthView struct {
	first, last  time.Time
	today        time.Time
	windows      []enrollmentWindow
	sessions     []client.TakenList
	records      []client.AttendanceRecord
	subjectNames map[string]string
}

func (v monthView) hasPerClass() bool {
	for _, s := range v.sessions {
		if s.SubjectID != "" {
			return true
		}
	}
	for _, r := range v.records {
		if r.SubjectID != "" {
			return true
		}
	}
	return false
}

// groupWindows reads what places the child's group windows — the enrollments,
// the school years and the transfer history — and derives them (see
// buildGroupWindows). A child with no enrollment reads nothing more.
func (h *FamilyHandler) groupWindows(ctx context.Context, studentID string, today time.Time) ([]enrollmentWindow, error) {
	enrollments, err := h.linkedEnrollments(ctx, studentID)
	if err != nil || len(enrollments) == 0 {
		return nil, err
	}
	cycles, err := h.cycleSpans(ctx)
	if err != nil {
		return nil, err
	}
	transfers, err := h.students.ListTransfers(ctx, studentID)
	if err != nil {
		return nil, err
	}
	return buildGroupWindows(enrollments, transfers, cycles, today), nil
}

// cycleSpans reads the school years by id. A handler wired without a catalog
// knows none, and every enrollment's cycle is then unbounded.
func (h *FamilyHandler) cycleSpans(ctx context.Context) (map[string]client.CycleResult, error) {
	out := map[string]client.CycleResult{}
	if h.catalog == nil {
		return out, nil
	}
	cycles, err := h.catalog.ListCycles(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range cycles {
		out[c.ID] = c
	}
	return out, nil
}

// readMonth reads the month's exceptions and taken lists, only for the windows
// overlapping the month: a month none overlaps costs no attendance read.
func (h *FamilyHandler) readMonth(ctx context.Context, windows []enrollmentWindow, first time.Time) (monthView, error) {
	view := monthView{first: first, last: first.AddDate(0, 1, -1)}
	from, to := view.first.Format(attendanceDateLayout), view.last.Format(attendanceDateLayout)
	view.windows = overlapping(windows, from, to)
	if len(view.windows) == 0 {
		return view, nil
	}
	enrollmentIDs := distinct(view.windows, func(w enrollmentWindow) string { return w.enrollmentID })
	records, err := fanOut(ctx, len(enrollmentIDs), func(ctx context.Context, i int) ([]client.AttendanceRecord, error) {
		return h.attendance.ListAttendance(ctx, enrollmentIDs[i], from, to)
	})
	if err != nil {
		return monthView{}, err
	}
	chunks := chunk(distinct(view.windows, func(w enrollmentWindow) string { return w.groupID }), maxSessionGroups)
	sessions, err := fanOut(ctx, len(chunks), func(ctx context.Context, i int) ([]client.TakenList, error) {
		return h.attendance.ListSessions(ctx, chunks[i], from, to)
	})
	if err != nil {
		return monthView{}, err
	}
	view.records, view.sessions = records, sessions
	return view, nil
}

// subjectNames resolves the per-class labels once; best-effort like the grades
// enrichment (a failed catalog read leaves names empty, never fails the view —
// but it is logged, so a broken catalog does not hide behind blank labels).
func (h *FamilyHandler) subjectNames(ctx context.Context, needed bool) map[string]string {
	names := map[string]string{}
	if !needed || h.catalog == nil {
		return names
	}
	subjects, err := h.catalog.ListSubjects(ctx, "")
	if err != nil {
		slog.WarnContext(ctx, "subject catalog lookup failed; attendance classes go unnamed", "error", err)
		return names
	}
	for _, s := range subjects {
		names[s.ID] = s.Name
	}
	return names
}

// dayMarks is what one day holds while it is being built.
type dayMarks struct {
	daily   bool
	status  string // the daily list's status ("" = present)
	note    string
	classes map[string]familyClassResponse // subject id → class
}

// buildAttendanceDays derives the month's rows. A day appears when a list was
// taken for the child's group that day (inside an enrollment window) or an
// exception is stored for it; today always appears (not_taken before any list).
//
//   - Daily day: the list's status — present when taken with no exception.
//   - Per-class day: classes = every list taken that day with the child's status;
//     the day's status is dayStatus over them (late > excused > present with no
//     absence; partial when absent in some classes; absent when absent in all).
//
// Rows are newest first; the summary counts DAYS (a per-class day is one day),
// with partial days in their own counter. not_taken counts nowhere.
func buildAttendanceDays(v monthView) ([]familyDayResponse, familyAttendanceSummary) {
	byDay := map[string]*dayMarks{}
	get := func(date string) *dayMarks {
		d, ok := byDay[date]
		if !ok {
			d = &dayMarks{classes: map[string]familyClassResponse{}}
			byDay[date] = d
		}
		return d
	}
	for _, s := range v.sessions {
		if !coveredByAny(v.windows, s.GroupID, s.Date) {
			continue
		}
		d := get(s.Date)
		if s.SubjectID == "" {
			d.daily = true
			continue
		}
		if _, ok := d.classes[s.SubjectID]; !ok {
			d.classes[s.SubjectID] = familyClassResponse{SubjectName: v.subjectNames[s.SubjectID], Status: dayPresent}
		}
	}
	for _, r := range v.records {
		d := get(r.Date)
		if r.SubjectID == "" {
			d.daily, d.status, d.note = true, r.Status, r.Note
			continue
		}
		d.classes[r.SubjectID] = familyClassResponse{SubjectName: v.subjectNames[r.SubjectID], Status: r.Status, Note: r.Note}
	}

	days := make([]familyDayResponse, 0, len(byDay)+1)
	var summary familyAttendanceSummary
	for date, d := range byDay {
		day := d.row(date)
		summary.count(day.Status)
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date > days[j].Date })
	return withToday(days, v), summary
}

// withToday adds today's not_taken row when today is in the month and has no
// row yet; its mode is the latest day's (daily when the month has none).
func withToday(days []familyDayResponse, v monthView) []familyDayResponse {
	today := v.today.Format(attendanceDateLayout)
	if v.today.Before(v.first) || v.today.After(v.last) {
		return days
	}
	mode := attendanceModeDaily
	for _, d := range days {
		if d.Date == today {
			return days
		}
		if d.Date < today {
			mode = d.Mode
			break
		}
	}
	row := familyDayResponse{Date: today, Status: dayNotTaken, Mode: mode, Classes: []familyClassResponse{}}
	i := sort.Search(len(days), func(i int) bool { return days[i].Date < today })
	days = append(days, familyDayResponse{})
	copy(days[i+1:], days[i:])
	days[i] = row
	return days
}

func coveredByAny(windows []enrollmentWindow, groupID, date string) bool {
	for _, w := range windows {
		if w.covers(groupID, date) {
			return true
		}
	}
	return false
}

// row renders one built day. A day with only a daily list carries its status;
// one with only per-class lists takes dayStatus over its classes. A day with
// BOTH: absent on the daily list is an absent day, otherwise dayStatus over
// every list taken — an absence in one class of a present day is partial, as
// in a per-class day.
func (d *dayMarks) row(date string) familyDayResponse {
	classes := make([]familyClassResponse, 0, len(d.classes))
	for _, c := range d.classes {
		classes = append(classes, c)
	}
	sort.Slice(classes, func(i, j int) bool { return classes[i].SubjectName < classes[j].SubjectName })
	statuses := make([]string, 0, len(classes)+1)
	for _, c := range classes {
		statuses = append(statuses, c.Status)
	}
	if !d.daily {
		return familyDayResponse{Date: date, Status: dayStatus(statuses), Mode: attendanceModePerClass, Classes: classes}
	}
	daily := d.status
	if daily == "" {
		daily = dayPresent
	}
	if len(classes) == 0 {
		return familyDayResponse{Date: date, Status: daily, Mode: attendanceModeDaily, Note: d.note, Classes: classes}
	}
	status := dayAbsent
	if daily != dayAbsent {
		status = dayStatus(append(statuses, daily))
	}
	return familyDayResponse{Date: date, Status: status, Mode: attendanceModePerClass, Note: d.note, Classes: classes}
}

// dayStatus is the day's status over every list taken that day (owner rule):
// with no absence, late beats excused beats present; absent in some lists but
// not all is partial; absent in every list is absent.
func dayStatus(statuses []string) string {
	var absent, late, excused int
	for _, s := range statuses {
		switch s {
		case dayAbsent:
			absent++
		case dayLate:
			late++
		case dayExcused:
			excused++
		}
	}
	switch {
	case absent == len(statuses):
		return dayAbsent
	case absent > 0:
		return dayPartial
	case late > 0:
		return dayLate
	case excused > 0:
		return dayExcused
	default:
		return dayPresent
	}
}

// count adds one day to its counter.
func (s *familyAttendanceSummary) count(status string) {
	switch status {
	case dayPresent:
		s.Present++
	case dayAbsent:
		s.Absent++
	case dayLate:
		s.Late++
	case dayExcused:
		s.Excused++
	case dayPartial:
		s.Partial++
	}
}
