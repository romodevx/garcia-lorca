package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	connect "connectrpc.com/connect"
	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// attendance_lists.go holds what the director (/attendance/*) and teacher
// (/teaching/attendance/*) pass-list surfaces share (T3): resolving a group's
// attendance mode from its level, the mode rules, the teacher's date window, the
// taker's display name and the roster JSON both answer with.
//
// A pass list is keyed by (group, date, subject). The group's LEVEL decides its
// mode: "daily" lists carry no subject (one per group per day), "per_class" lists
// carry the subject being taught (one per group, subject and day).

// attendanceWindowDays is how many days back a teacher may still capture or
// correct a list (today included, so eight calendar days in all). The director
// has no window.
const attendanceWindowDays = 7

// maxTakerNameRunes is the taken_by_name column's CHECK (1..200 characters);
// the attendance service refuses a longer name, so the edge cuts it to fit.
const maxTakerNameRunes = 200

// The refusals the attendance surfaces answer with (text/plain, like the rest of
// the edge).
const (
	msgListIsPerClass    = "Esta lista es por clase: elige la asignatura"
	msgListIsDaily       = "Esta lista es por día"
	msgFutureDate        = "No puedes registrar una fecha futura"
	msgInvalidDate       = "La fecha debe tener el formato AAAA-MM-DD"
	msgSubjectNotInGroup = "La asignatura no se imparte en este grupo"
)

// msgOutsideWindow is built from attendanceWindowDays so the sentence can never
// disagree with the rule it explains.
var msgOutsideWindow = fmt.Sprintf("Solo puedes corregir hasta %d días atrás", attendanceWindowDays)

// The read-only reasons a roster names (readOnlyReason): codes, not sentences —
// the web maps each to its own text.
const (
	readOnlyFuture = "future"
	readOnlyWindow = "window"
	// readOnlyNoClasses: the school calendar has no classes that day (T14);
	// the attendance service refuses its save with NO_CLASSES.
	readOnlyNoClasses = "no_classes"
)

const (
	attendanceModeDaily    = client.AttendanceModeDaily
	attendanceModePerClass = client.AttendanceModePerClass

	// attendanceDateLayout is the ISO school day every list date crosses in.
	attendanceDateLayout = "2006-01-02"
	// groupLabelSeparator joins a group's grade and letter: "1° SECUNDARIA · A".
	groupLabelSeparator = " · "
	// fallbackTakerName names a taker with neither a display name nor an email.
	fallbackTakerName = "Usuario"
	// attendanceStatusPresent is a roster row with no stored mark.
	attendanceStatusPresent = "present"
)

// AttendanceUserReader reads the caller's display name from the iam directory,
// to stamp on the list's session. The Connect AuthClient satisfies it.
type AttendanceUserReader interface {
	GetUser(ctx context.Context, id string) (client.User, error)
}

// AttendanceCatalogReader is the config surface that resolves a group's level
// (and so its attendance mode), its display label and its subjects. The Connect
// ConfigClient satisfies it.
type AttendanceCatalogReader interface {
	ListGroups(ctx context.Context, cycleID, gradeLevelID string) ([]client.GroupResult, error)
	ListGradeLevels(ctx context.Context, levelID string) ([]client.GradeLevelResult, error)
	ListLevels(ctx context.Context) ([]client.LevelResult, error)
	ListSubjects(ctx context.Context, gradeLevelID string) ([]client.SubjectResult, error)
}

// attendanceCatalog is the tenant's group → grade → level chain plus subject
// names, read ONCE per request (four list reads, concurrently) so resolving
// MANY groups (the teacher's classes) never fans out into a read per group.
// One list resolves its single group with resolveListGroup instead.
type attendanceCatalog struct {
	groups   map[string]client.GroupResult
	grades   map[string]client.GradeLevelResult
	levels   map[string]client.LevelResult
	subjects map[string]string
}

// listGroup is what the attendance surfaces need to know about one group.
type listGroup struct {
	Mode      string
	Label     string // "1° SECUNDARIA · A"
	LevelName string
	GradeName string // "1° SECUNDARIA"; "" when the grade is unknown
	Name      string // the group's own name, its letter: "A"
}

// loadAttendanceCatalog reads the catalog. Every read is a hard dependency: a
// group's mode decides which list exists, so it is never guessed.
func loadAttendanceCatalog(ctx context.Context, r AttendanceCatalogReader) (attendanceCatalog, error) {
	var (
		groups   []client.GroupResult
		grades   []client.GradeLevelResult
		levels   []client.LevelResult
		subjects []client.SubjectResult
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { groups, err = r.ListGroups(gctx, "", ""); return err })
	g.Go(func() (err error) { grades, err = r.ListGradeLevels(gctx, ""); return err })
	g.Go(func() (err error) { levels, err = r.ListLevels(gctx); return err })
	g.Go(func() (err error) { subjects, err = r.ListSubjects(gctx, ""); return err })
	if err := g.Wait(); err != nil {
		return attendanceCatalog{}, err
	}
	c := attendanceCatalog{
		groups:   make(map[string]client.GroupResult, len(groups)),
		grades:   make(map[string]client.GradeLevelResult, len(grades)),
		levels:   make(map[string]client.LevelResult, len(levels)),
		subjects: make(map[string]string, len(subjects)),
	}
	for _, x := range groups {
		c.groups[x.ID] = x
	}
	for _, x := range grades {
		c.grades[x.ID] = x
	}
	for _, x := range levels {
		c.levels[x.ID] = x
	}
	for _, x := range subjects {
		c.subjects[x.ID] = x.Name
	}
	return c, nil
}

// group resolves one group; ok is false for a group the catalog does not hold.
func (c attendanceCatalog) group(id string) (listGroup, bool) {
	g, ok := c.groups[id]
	if !ok {
		return listGroup{}, false
	}
	grade := c.grades[g.GradeLevelID]
	return newListGroup(g, grade, c.levels[grade.LevelID]), true
}

// newListGroup is the ONE place a group's attendance view is built — its mode
// and its label — whichever way the group, grade and level were read. A level
// with no mode reads as daily, the column's default; an unknown grade or level
// is a zero value (daily, label = the group's letter).
func newListGroup(g client.GroupResult, grade client.GradeLevelResult, level client.LevelResult) listGroup {
	mode := level.AttendanceMode
	if mode != attendanceModePerClass {
		mode = attendanceModeDaily
	}
	gradeName := gradeWithLevel(grade.Name, level.Name)
	return listGroup{Mode: mode, Label: groupLabel(gradeName, g.Name), LevelName: level.Name, GradeName: gradeName, Name: g.Name}
}

// gradeWithLevel is a grade's name as a screen shows it. The config screen asks
// for the grade "as the school names it", so "1°" under PRIMARIA and "1°" under
// SECUNDARIA are both normal: when the grade's name does not already carry its
// level, the level is added ("1° PRIMARIA"). An unknown grade or level keeps
// what is known.
func gradeWithLevel(gradeName, levelName string) string {
	if gradeName == "" || levelName == "" || strings.Contains(strings.ToUpper(gradeName), strings.ToUpper(levelName)) {
		return gradeName
	}
	return gradeName + " " + levelName
}

// groupLabel is how a group is named on screen: "1° SECUNDARIA · A", or the
// bare letter when its grade is unknown.
func groupLabel(gradeName, groupName string) string {
	if gradeName == "" {
		return groupName
	}
	return gradeName + groupLabelSeparator + groupName
}

// AttendanceGroupReader reads ONE group, grade and level by id: all a single
// pass list needs to know its mode, instead of listing the whole catalog. The
// Connect ConfigClient satisfies it.
type AttendanceGroupReader interface {
	GetGroup(ctx context.Context, id string) (client.GroupResult, error)
	GetGradeLevel(ctx context.Context, id string) (client.GradeLevelResult, error)
	GetLevel(ctx context.Context, id string) (client.LevelResult, error)
}

// resolveListGroup reads groupID → its grade → its level (three point reads,
// each depending on the one before). found is false for a group that does not
// exist; a grade or level that no longer exists reads as unknown (daily), as
// the catalog view does. Any other failure is an error — a group's mode decides
// which list exists, so it is never guessed.
func resolveListGroup(ctx context.Context, r AttendanceGroupReader, groupID string) (listGroup, bool, error) {
	if groupID == "" {
		return listGroup{}, false, nil
	}
	g, err := r.GetGroup(ctx, groupID)
	if isNotFound(err) {
		return listGroup{}, false, nil
	}
	if err != nil {
		return listGroup{}, false, err
	}
	var grade client.GradeLevelResult
	if g.GradeLevelID != "" {
		if grade, err = r.GetGradeLevel(ctx, g.GradeLevelID); err != nil && !isNotFound(err) {
			return listGroup{}, false, err
		}
	}
	var level client.LevelResult
	if grade.LevelID != "" {
		if level, err = r.GetLevel(ctx, grade.LevelID); err != nil && !isNotFound(err) {
			return listGroup{}, false, err
		}
	}
	return newListGroup(g, grade, level), true, nil
}

// SubjectReader reads ONE subject by id. The Connect ConfigClient satisfies it.
type SubjectReader interface {
	GetSubject(ctx context.Context, id string) (client.SubjectResult, error)
}

// subjectName is the display name of a per-class list's subject, carried onto
// the events a save publishes so the family's notification names the class
// (T5). It is a courtesy, never a gate: a daily list (no subject), an unknown
// subject or a catalog that cannot answer all yield "" and the save proceeds —
// logged (ids only), so a broken catalog does not hide behind unnamed classes.
func subjectName(ctx context.Context, subjects SubjectReader, subjectID string) string {
	if subjectID == "" {
		return ""
	}
	s, err := subjects.GetSubject(ctx, subjectID)
	if err != nil {
		slog.WarnContext(ctx, "subject lookup failed; the attendance event goes without a class name",
			"subject_id", subjectID, "error", err)
		return ""
	}
	return s.Name
}

// isNotFound reports a Connect NotFound.
func isNotFound(err error) bool {
	return err != nil && connect.CodeOf(err) == connect.CodeNotFound
}

// listModeRefusal is the 400 sentence for a subject that does not fit the
// group's mode, or "" when it fits. The mode governs NEW lists only: see
// listExists for the lists taken before a level changed its mode.
func listModeRefusal(mode, subjectID string) string {
	switch {
	case mode == attendanceModePerClass && subjectID == "":
		return msgListIsPerClass
	case mode == attendanceModeDaily && subjectID != "":
		return msgListIsDaily
	default:
		return ""
	}
}

// keyMode is the shape of a list key itself: a subject makes it per class.
func keyMode(subjectID string) string {
	if subjectID == "" {
		return attendanceModeDaily
	}
	return attendanceModePerClass
}

// ListRosterReader reads one list, which is how the edge learns whether a list
// already exists. Both attendance services satisfy it.
type ListRosterReader interface {
	GetDailyRoster(ctx context.Context, groupID, date, subjectID string) (client.AttendanceRoster, error)
}

// listAccess is what a list gate (assertOwnsList / resolveList) learned: the
// list's mode and, when the gate had to read the list to decide (listExists),
// that roster — so a caller that needs it reuses it instead of reading it
// twice.
type listAccess struct {
	mode   string
	roster *client.AttendanceRoster // nil when the gate did not read the list
}

// rosterOf is the list's roster: the one the gate already read, else a fresh
// read.
func rosterOf(ctx context.Context, r ListRosterReader, access listAccess, groupID, date, subjectID string) (client.AttendanceRoster, error) {
	if access.roster != nil {
		return *access.roster, nil
	}
	return r.GetDailyRoster(ctx, groupID, date, subjectID)
}

// listExists reports whether the exact list (group, date, subject-or-none) was
// already taken — it has a session or any stored mark — and returns the roster
// it read (nil when it read none). Such a list stays readable and correctable
// after its level changes mode: the mode decides which NEW lists may be
// started, it never orphans a day the school already recorded. A malformed
// date names no list.
func listExists(ctx context.Context, r ListRosterReader, groupID, date, subjectID string) (*client.AttendanceRoster, bool, error) {
	if _, ok := parseListDate(date); !ok {
		return nil, false, nil
	}
	roster, err := r.GetDailyRoster(ctx, groupID, date, subjectID)
	if err != nil {
		return nil, false, err
	}
	return &roster, rosterTaken(roster), nil
}

// rosterTaken reports whether a roster was already taken: a session, or any
// row that is not the default present.
func rosterTaken(roster client.AttendanceRoster) bool {
	if roster.Session != nil {
		return true
	}
	for _, row := range roster.Rows {
		if row.Status != attendanceStatusPresent {
			return true
		}
	}
	return false
}

// schoolToday is the school's day (America/Mexico_City, ADR 0035), as the
// midnight-UTC date the lists are keyed by: at 19:30 in Morelia the school is
// still on today, though UTC already turned.
func schoolToday(now time.Time) time.Time {
	return schoolDate(now, defaultSchoolLocation)
}

// teacherReadOnlyReason is why a teacher may not capture date — readOnlyFuture
// or readOnlyWindow (more than attendanceWindowDays back) — or "" when it is
// inside the window. The future is checked first: a future day is never
// described as "too old".
func teacherReadOnlyReason(date, now time.Time) string {
	today := schoolToday(now)
	switch {
	case date.After(today):
		return readOnlyFuture
	case date.Before(today.AddDate(0, 0, -attendanceWindowDays)):
		return readOnlyWindow
	default:
		return ""
	}
}

// teacherWindowRefusal is the 400 sentence for a date a teacher may not save,
// or "" when it is inside the window.
func teacherWindowRefusal(date, now time.Time) string {
	switch teacherReadOnlyReason(date, now) {
	case readOnlyFuture:
		return msgFutureDate
	case readOnlyWindow:
		return msgOutsideWindow
	default:
		return ""
	}
}

// parseListDate parses an ISO school day.
func parseListDate(value string) (time.Time, bool) {
	d, err := time.Parse(attendanceDateLayout, value)
	return d, err == nil
}

// takerName is the display name stamped on a list's session: the iam display
// name, else the principal's email, else a generic label, cut to the column's
// maxTakerNameRunes characters. Best-effort by design — a directory outage (or
// an overlong name) never blocks taking attendance.
func takerName(ctx context.Context, users AttendanceUserReader, p middleware.Principal) string {
	if users != nil {
		if u, err := users.GetUser(ctx, p.UserID); err == nil {
			if name := strings.TrimSpace(u.DisplayName); name != "" {
				return clampRunes(name, maxTakerNameRunes)
			}
		}
	}
	if email := strings.TrimSpace(p.Email); email != "" {
		return clampRunes(email, maxTakerNameRunes)
	}
	return fallbackTakerName
}

// clampRunes cuts s to at most n characters, never inside a multi-byte one.
func clampRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// --- roster JSON ---------------------------------------------------------------

// attendanceListResponse is the roster both surfaces answer with.
// ReadOnlyReason says why a non-editable list is read-only (readOnlyFuture,
// readOnlyWindow, readOnlyNoClasses) and is omitted when editable. Today is the
// school's day (Morelia, see schoolToday), so the web's date picker starts
// from the same day the window is measured against.
type attendanceListResponse struct {
	Mode           string                     `json:"mode"`
	Date           string                     `json:"date"`
	Editable       bool                       `json:"editable"`
	ReadOnlyReason string                     `json:"readOnlyReason,omitempty"`
	Today          string                     `json:"today"`
	WindowDays     int                        `json:"windowDays"`
	Session        *attendanceSessionResponse `json:"session"` // null for an untaken list
	Rows           []attendanceListRow        `json:"rows"`
}

// listView is what a roster answer says about the list itself.
type listView struct {
	Mode           string
	Date           string
	ReadOnlyReason string // "" = editable
	Today          time.Time
}

type attendanceSessionResponse struct {
	TakenAt     time.Time `json:"takenAt"`
	TakenByName string    `json:"takenByName"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// attendanceListRow is one student on a list. A name that cannot be resolved
// degrades to "" — never an error.
type attendanceListRow struct {
	EnrollmentID string `json:"enrollmentId"`
	StudentID    string `json:"studentId"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Note         string `json:"note"`
}

func toAttendanceListResponse(view listView, roster client.AttendanceRoster, students studentDirectory) attendanceListResponse {
	if view.ReadOnlyReason == "" && roster.NoClasses {
		view.ReadOnlyReason = readOnlyNoClasses
	}
	out := attendanceListResponse{
		Mode:           view.Mode,
		Date:           view.Date,
		Editable:       view.ReadOnlyReason == "",
		ReadOnlyReason: view.ReadOnlyReason,
		Today:          view.Today.Format(attendanceDateLayout),
		WindowDays:     attendanceWindowDays,
		Rows:           make([]attendanceListRow, 0, len(roster.Rows)),
	}
	if s := roster.Session; s != nil {
		out.Session = &attendanceSessionResponse{TakenAt: s.TakenAt, TakenByName: s.TakenByName, UpdatedAt: s.UpdatedAt}
	}
	for _, r := range roster.Rows {
		out.Rows = append(out.Rows, attendanceListRow{
			EnrollmentID: r.EnrollmentID,
			StudentID:    r.StudentID,
			Name:         students.name(r.StudentID),
			Status:       r.Status,
			Note:         r.Note,
		})
	}
	sortBySurname(students, out.Rows, func(r attendanceListRow) string { return r.StudentID })
	return out
}

// studentNamesFrom builds the studentId→pupil directory from ONE list read,
// best-effort: no rows to enrich or a failed read yields an empty directory.
func studentNamesFrom(ctx context.Context, students TeachingStudentReader, enrich bool) studentDirectory {
	dir := studentDirectory{}
	if !enrich || students == nil {
		return dir
	}
	list, err := students.ListStudents(ctx, "")
	if err != nil {
		return dir
	}
	for _, s := range list {
		dir[s.ID] = s
	}
	return dir
}

// writeBadRequest answers a 400 with a sentence for the user.
func writeBadRequest(w http.ResponseWriter, msg string) {
	http.Error(w, msg, http.StatusBadRequest)
}
