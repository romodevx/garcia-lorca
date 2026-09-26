package client

import (
	"context"
	"net/http"
	"time"

	connect "connectrpc.com/connect"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/attendance/v1/attendancev1connect"
)

// AttendanceClient is the gateway's typed client for the attendance service over
// the generated Connect client. Handlers depend on its small methods and the
// plain structs below, never on the generated proto types.
//
// The model is DEFAULT-PRESENT: only the exceptions (absent|late|excused) are
// stored, and saving a day is a reconcile of the whole group·day, not an append.
type AttendanceClient struct {
	rpc attendancev1connect.AttendanceServiceClient
}

// AttendanceRosterRow is the edge's view of one student's slot in a day's pass
// list: the enrollment, the student it belongs to, and the effective status for
// that day (present|absent|late|excused — "present" when nothing is stored).
// It is named apart from the grades RosterRow: the two edges carry different
// payloads and must not be conflated.
type AttendanceRosterRow struct {
	EnrollmentID string
	StudentID    string
	Status       string
	Note         string
}

// AttendanceRoster is one list's rows plus its session; Session is nil for a
// list nobody has taken.
type AttendanceRoster struct {
	Rows    []AttendanceRosterRow
	Session *AttendanceSession
}

// AttendanceSession records that a list was taken: TakenAt/TakenByName are the
// first save, UpdatedAt the latest correction.
type AttendanceSession struct {
	TakenAt     time.Time
	TakenByName string
	UpdatedAt   time.Time
}

// SaveAttendanceInput is one save of a list: the group·day, the subject of a
// per-class list ("" for daily), the COMPLETE exception set, and who saves it
// (iam user id + display name, stamped on the list's session).
type SaveAttendanceInput struct {
	GroupID     string
	Date        string
	SubjectID   string
	Marks       []Mark
	TakenBy     string
	TakenByName string
}

// Mark is one exception submitted for a group·day. Status is normally
// absent|late|excused; "present" is accepted too and CLEARS the row stored for
// that enrollment (an explicit revert to default-present, equivalent to omitting
// it). Note is an optional free-text justification, capped at 500 characters.
type Mark struct {
	EnrollmentID string
	Status       string
	Note         string
}

// AttendanceRecord is the edge's view of one stored attendance exception. Date
// is an ISO date, "YYYY-MM-DD" (attendance is per school day, never a time).
type AttendanceRecord struct {
	ID           string
	EnrollmentID string
	Date         string
	Status       string
	Note         string
	SubjectID    string
}

// NewAttendanceClient builds an AttendanceClient targeting the attendance base
// URL over httpClient.
func NewAttendanceClient(baseURL string, httpClient connect.HTTPClient) *AttendanceClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &AttendanceClient{rpc: attendancev1connect.NewAttendanceServiceClient(httpClient, baseURL)}
}

// GetDailyRoster returns one list — the group's daily list (subjectID "") or a
// subject's per-class list — on the given date (ISO "YYYY-MM-DD"): one row per
// active enrollment with its effective status and note ("present" when nothing
// is stored), plus the list's session when it has been taken.
func (c *AttendanceClient) GetDailyRoster(ctx context.Context, groupID, date, subjectID string) (AttendanceRoster, error) {
	res, err := c.rpc.GetDailyRoster(ctx, connect.NewRequest(&attendancev1.GetDailyRosterRequest{
		GroupId:   groupID,
		Date:      date,
		SubjectId: subjectID,
	}))
	if err != nil {
		return AttendanceRoster{}, err
	}
	out := AttendanceRoster{Rows: make([]AttendanceRosterRow, 0, len(res.Msg.GetRows()))}
	for _, row := range res.Msg.GetRows() {
		out.Rows = append(out.Rows, toAttendanceRosterRow(row))
	}
	if s := res.Msg.GetSession(); s != nil {
		out.Session = &AttendanceSession{
			TakenAt:     s.GetTakenAt().AsTime(),
			TakenByName: s.GetTakenByName(),
			UpdatedAt:   s.GetUpdatedAt().AsTime(),
		}
	}
	return out, nil
}

// SaveDailyAttendance reconciles a whole list in one call: in.Marks is the
// COMPLETE set of exceptions, so every enrollment absent from it is reset to
// present. It returns how many stored marks are "absent" and how many
// exceptions are stored for the day in total. Saving the same day again is
// idempotent.
func (c *AttendanceClient) SaveDailyAttendance(ctx context.Context, in SaveAttendanceInput) (int32, int32, error) {
	res, err := c.rpc.SaveDailyAttendance(ctx, connect.NewRequest(&attendancev1.SaveDailyAttendanceRequest{
		GroupId:     in.GroupID,
		Date:        in.Date,
		SubjectId:   in.SubjectID,
		Marks:       fromMarks(in.Marks),
		TakenBy:     in.TakenBy,
		TakenByName: in.TakenByName,
	}))
	if err != nil {
		return 0, 0, err
	}
	return res.Msg.GetAbsentCount(), res.Msg.GetMarkedCount(), nil
}

// ListAttendance lists one enrollment's stored exceptions within an inclusive
// date range (both ISO "YYYY-MM-DD"). Present days are absent from the result by
// construction: they are never stored.
func (c *AttendanceClient) ListAttendance(ctx context.Context, enrollmentID, from, to string) ([]AttendanceRecord, error) {
	res, err := c.rpc.ListAttendance(ctx, connect.NewRequest(&attendancev1.ListAttendanceRequest{
		EnrollmentId: enrollmentID,
		FromDate:     from,
		ToDate:       to,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]AttendanceRecord, 0, len(res.Msg.GetRecords()))
	for _, r := range res.Msg.GetRecords() {
		out = append(out, toAttendanceRecord(r))
	}
	return out, nil
}

func toAttendanceRosterRow(r *attendancev1.RosterRow) AttendanceRosterRow {
	return AttendanceRosterRow{
		EnrollmentID: r.GetEnrollmentId(),
		StudentID:    r.GetStudentId(),
		Status:       r.GetStatus(),
		Note:         r.GetNote(),
	}
}

func toAttendanceRecord(r *attendancev1.AttendanceRecord) AttendanceRecord {
	return AttendanceRecord{
		ID:           r.GetId(),
		EnrollmentID: r.GetEnrollmentId(),
		Date:         r.GetDate(),
		Status:       r.GetStatus(),
		Note:         r.GetNote(),
		SubjectID:    r.GetSubjectId(),
	}
}

func fromMarks(in []Mark) []*attendancev1.Mark {
	out := make([]*attendancev1.Mark, 0, len(in))
	for _, m := range in {
		out = append(out, &attendancev1.Mark{
			EnrollmentId: m.EnrollmentID,
			Status:       m.Status,
			Note:         m.Note,
		})
	}
	return out
}
