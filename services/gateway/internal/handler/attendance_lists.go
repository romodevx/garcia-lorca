package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

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

// The refusals the attendance surfaces answer with (text/plain, like the rest of
// the edge).
const (
	msgListIsPerClass    = "Esta lista es por clase: elige la asignatura"
	msgListIsDaily       = "Esta lista es por día"
	msgOutsideWindow     = "Solo puedes corregir hasta 7 días atrás"
	msgFutureDate        = "No puedes registrar una fecha futura"
	msgInvalidDate       = "La fecha debe tener el formato AAAA-MM-DD"
	msgSubjectNotInGroup = "La asignatura no se imparte en este grupo"
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
// names, read ONCE per request (four list reads, concurrently) so resolving any
// number of groups never fans out into a read per group.
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
// A level with no mode reads as daily, the column's default.
func (c attendanceCatalog) group(id string) (listGroup, bool) {
	g, ok := c.groups[id]
	if !ok {
		return listGroup{}, false
	}
	grade := c.grades[g.GradeLevelID]
	level := c.levels[grade.LevelID]
	mode := level.AttendanceMode
	if mode != attendanceModePerClass {
		mode = attendanceModeDaily
	}
	label := g.Name
	if grade.Name != "" {
		label = grade.Name + groupLabelSeparator + g.Name
	}
	return listGroup{Mode: mode, Label: label, LevelName: level.Name}, true
}

// listModeRefusal is the 400 sentence for a subject that does not fit the
// group's mode, or "" when it fits.
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

// schoolToday is the system's school day: the UTC date. It is a known,
// fleet-wide offset (the school is UTC-6, so after 18:00 local the system is
// already on tomorrow — see finance's resolveAsOf); attendance follows it rather
// than diverging from every other date the school sees.
func schoolToday(now time.Time) time.Time {
	y, m, d := now.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// teacherWindowRefusal is the 400 sentence for a date a teacher may not capture
// (a future day, or more than attendanceWindowDays back), or "" when it is
// inside the window.
func teacherWindowRefusal(date, now time.Time) string {
	today := schoolToday(now)
	switch {
	case date.After(today):
		return msgFutureDate
	case date.Before(today.AddDate(0, 0, -attendanceWindowDays)):
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
// name, else the principal's email, else a generic label. Best-effort by design —
// a directory outage never blocks taking attendance.
func takerName(ctx context.Context, users AttendanceUserReader, p middleware.Principal) string {
	if users != nil {
		if u, err := users.GetUser(ctx, p.UserID); err == nil {
			if name := strings.TrimSpace(u.DisplayName); name != "" {
				return name
			}
		}
	}
	if email := strings.TrimSpace(p.Email); email != "" {
		return email
	}
	return fallbackTakerName
}

// --- roster JSON ---------------------------------------------------------------

// attendanceListResponse is the roster both surfaces answer with.
type attendanceListResponse struct {
	Mode       string                     `json:"mode"`
	Date       string                     `json:"date"`
	Editable   bool                       `json:"editable"`
	WindowDays int                        `json:"windowDays"`
	Session    *attendanceSessionResponse `json:"session"` // null for an untaken list
	Rows       []attendanceListRow        `json:"rows"`
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

func toAttendanceListResponse(mode, date string, editable bool, roster client.AttendanceRoster, names map[string]string) attendanceListResponse {
	out := attendanceListResponse{
		Mode:       mode,
		Date:       date,
		Editable:   editable,
		WindowDays: attendanceWindowDays,
		Rows:       make([]attendanceListRow, 0, len(roster.Rows)),
	}
	if s := roster.Session; s != nil {
		out.Session = &attendanceSessionResponse{TakenAt: s.TakenAt, TakenByName: s.TakenByName, UpdatedAt: s.UpdatedAt}
	}
	for _, r := range roster.Rows {
		out.Rows = append(out.Rows, attendanceListRow{
			EnrollmentID: r.EnrollmentID,
			StudentID:    r.StudentID,
			Name:         names[r.StudentID],
			Status:       r.Status,
			Note:         r.Note,
		})
	}
	return out
}

// studentNamesFrom builds the studentId→"First Last" map from ONE list read,
// best-effort: no rows to enrich or a failed read yields an empty map.
func studentNamesFrom(ctx context.Context, students TeachingStudentReader, enrich bool) map[string]string {
	names := map[string]string{}
	if !enrich || students == nil {
		return names
	}
	list, err := students.ListStudents(ctx, "")
	if err != nil {
		return names
	}
	for _, s := range list {
		names[s.ID] = strings.TrimSpace(s.FirstName + " " + s.LastName)
	}
	return names
}

// writeBadRequest answers a 400 with a sentence for the user.
func writeBadRequest(w http.ResponseWriter, msg string) {
	http.Error(w, msg, http.StatusBadRequest)
}
