package handler

import (
	"context"
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
// attendance vocabulary; partial is a per-class day with mixed statuses and
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

// Attendance handles GET /api/v1/family/students/{studentId}/attendance?month=
// — the child's month, one row per day, newest first (see buildAttendanceDays).
// month defaults to the current UTC month (the fleet's school day, see
// schoolToday). Another family's child is 404, like every by-child route.
//
// Reads, all batched: the child's enrollments (students), each enrollment's
// exceptions for the month (attendance; a child has one or two), ONE read of the
// lists taken for the child's groups, and — only when a per-class list is in the
// month — the subject names once.
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
	enrollments, err := h.linkedEnrollments(r.Context(), studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	view, err := h.readMonth(r.Context(), enrollments, first)
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
func parseMonth(value string, today time.Time) (time.Time, bool) {
	if value == "" {
		return time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC), true
	}
	first, err := time.Parse(monthLayout, value)
	if err != nil || first.Year() < 1 {
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

// readMonth reads the month's exceptions and taken lists for the child.
func (h *FamilyHandler) readMonth(ctx context.Context, enrollments []client.Enrollment, first time.Time) (monthView, error) {
	view := monthView{first: first, last: first.AddDate(0, 1, -1), windows: enrollmentWindows(enrollments)}
	from, to := view.first.Format(attendanceDateLayout), view.last.Format(attendanceDateLayout)
	for _, e := range enrollments {
		records, err := h.attendance.ListAttendance(ctx, e.ID, from, to)
		if err != nil {
			return monthView{}, err
		}
		view.records = append(view.records, records...)
	}
	groups := windowGroups(view.windows)
	if len(groups) == 0 {
		return view, nil
	}
	sessions, err := h.attendance.ListSessions(ctx, groups, from, to)
	if err != nil {
		return monthView{}, err
	}
	view.sessions = sessions
	return view, nil
}

// subjectNames resolves the per-class labels once; best-effort like the grades
// enrichment (a failed catalog read leaves names empty, never fails the view).
func (h *FamilyHandler) subjectNames(ctx context.Context, needed bool) map[string]string {
	names := map[string]string{}
	if !needed || h.catalog == nil {
		return names
	}
	subjects, err := h.catalog.ListSubjects(ctx, "")
	if err != nil {
		return names
	}
	for _, s := range subjects {
		names[s.ID] = s.Name
	}
	return names
}

// enrollmentWindow is the span of school days a group's lists are the child's:
// from the enrollment's start up to the day before the next enrollment began.
// to is "" when open-ended. A list outside every window was taken for a group
// the child was not in that day.
type enrollmentWindow struct {
	groupID  string
	from, to string // ISO days, inclusive; "" = unbounded
}

// enrollmentWindows orders the child's enrollments by start and derives each
// one's window. The last one is open-ended only while it is ACTIVE: a withdrawn
// enrollment has no end date recorded, so its lists are not claimed (its stored
// exceptions still show).
func enrollmentWindows(enrollments []client.Enrollment) []enrollmentWindow {
	sorted := append([]client.Enrollment(nil), enrollments...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].EnrolledAt.Before(sorted[j].EnrolledAt) })
	out := make([]enrollmentWindow, 0, len(sorted))
	for i, e := range sorted {
		if e.GroupID == "" {
			continue
		}
		w := enrollmentWindow{groupID: e.GroupID, from: isoDay(e.EnrolledAt)}
		switch {
		case i+1 < len(sorted):
			w.to = isoDay(sorted[i+1].EnrolledAt.AddDate(0, 0, -1))
		case e.Status != "active":
			continue
		}
		out = append(out, w)
	}
	return out
}

// isoDay renders an instant's UTC day, or "" for the zero time (no bound).
func isoDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(attendanceDateLayout)
}

func (w enrollmentWindow) covers(groupID, date string) bool {
	return w.groupID == groupID && (w.from == "" || date >= w.from) && (w.to == "" || date <= w.to)
}

// windowGroups is the distinct group ids of the windows, in order.
func windowGroups(windows []enrollmentWindow) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(windows))
	for _, w := range windows {
		if !seen[w.groupID] {
			seen[w.groupID] = true
			out = append(out, w.groupID)
		}
	}
	return out
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
//     the day is the status all classes share (present, absent, late…), or
//     partial when they differ.
//
// Rows are newest first; the summary counts DAYS (a per-class day is one day),
// with mixed days in their own partial counter. not_taken counts nowhere.
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

// row renders one built day.
func (d *dayMarks) row(date string) familyDayResponse {
	classes := make([]familyClassResponse, 0, len(d.classes))
	for _, c := range d.classes {
		classes = append(classes, c)
	}
	sort.Slice(classes, func(i, j int) bool { return classes[i].SubjectName < classes[j].SubjectName })
	if d.daily {
		status := d.status
		if status == "" {
			status = dayPresent
		}
		return familyDayResponse{Date: date, Status: status, Mode: attendanceModeDaily, Note: d.note, Classes: classes}
	}
	return familyDayResponse{Date: date, Status: sharedStatus(classes), Mode: attendanceModePerClass, Classes: classes}
}

// sharedStatus is the status every class shares, or partial when they differ.
func sharedStatus(classes []familyClassResponse) string {
	for _, c := range classes[1:] {
		if c.Status != classes[0].Status {
			return dayPartial
		}
	}
	return classes[0].Status
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
