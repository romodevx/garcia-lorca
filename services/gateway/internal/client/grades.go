package client

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/grades/v1/gradesv1connect"
)

// GradesClient is the gateway's typed client for the grades service over the
// generated Connect client. Handlers depend on its small methods and the plain
// structs below, never on the generated proto types.
type GradesClient struct {
	rpc gradesv1connect.GradesServiceClient
}

// GradeEntry is the edge's view of a captured grade. subject_id and teacher_id
// are DERIVED server-side from the teaching assignment; the client never sends
// them, but the read surface still exposes them.
type GradeEntry struct {
	ID           string
	EnrollmentID string
	SubjectID    string
	TermID       string
	TeacherID    string
	Value        float64
}

// RosterRow is the edge's view of one student's slot in a roster: the
// enrollment, the student it belongs to, and the current grade value. HasValue
// false ⇒ not captured yet and Value is meaningless.
type RosterRow struct {
	EnrollmentID string
	StudentID    string
	Value        float64
	HasValue     bool
}

// NewGradesClient builds a GradesClient targeting the grades base URL over
// httpClient.
func NewGradesClient(baseURL string, httpClient connect.HTTPClient) *GradesClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &GradesClient{rpc: gradesv1connect.NewGradesServiceClient(httpClient, baseURL)}
}

// CaptureGrade records or edits a student's numeric grade for a term. Only the
// client-supplied edges of the knot are sent ({assignment_id, enrollment_id,
// term_id, value}); subject_id and teacher_id are derived server-side from the
// assignment. Connect error codes propagate (InvalidArgument for a bad
// enrollment/term or an out-of-range value) so the handler maps them to HTTP.
func (c *GradesClient) CaptureGrade(ctx context.Context, assignmentID, enrollmentID, termID string, value float64) (GradeEntry, error) {
	res, err := c.rpc.CaptureGrade(ctx, connect.NewRequest(&gradesv1.CaptureGradeRequest{
		AssignmentId: assignmentID,
		EnrollmentId: enrollmentID,
		TermId:       termID,
		Value:        value,
	}))
	if err != nil {
		return GradeEntry{}, err
	}
	return toGradeEntry(res.Msg.GetEntry()), nil
}

// GetRoster returns one row per active enrollment in the assignment's group,
// each carrying the current grade value for the term (if any).
func (c *GradesClient) GetRoster(ctx context.Context, assignmentID, termID string) ([]RosterRow, error) {
	res, err := c.rpc.GetRoster(ctx, connect.NewRequest(&gradesv1.GetRosterRequest{
		AssignmentId: assignmentID,
		TermId:       termID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]RosterRow, 0, len(res.Msg.GetRows()))
	for _, row := range res.Msg.GetRows() {
		out = append(out, toRosterRow(row))
	}
	return out, nil
}

// ListGradeEntries lists grade entries, filtered by any of enrollmentID, termID
// ("" = no filter on that dimension).
func (c *GradesClient) ListGradeEntries(ctx context.Context, enrollmentID, termID string) ([]GradeEntry, error) {
	res, err := c.rpc.ListGradeEntries(ctx, connect.NewRequest(&gradesv1.ListGradeEntriesRequest{
		EnrollmentId: enrollmentID,
		TermId:       termID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]GradeEntry, 0, len(res.Msg.GetEntries()))
	for _, e := range res.Msg.GetEntries() {
		out = append(out, toGradeEntry(e))
	}
	return out, nil
}

func toGradeEntry(e *gradesv1.GradeEntry) GradeEntry {
	return GradeEntry{
		ID:           e.GetId(),
		EnrollmentID: e.GetEnrollmentId(),
		SubjectID:    e.GetSubjectId(),
		TermID:       e.GetTermId(),
		TeacherID:    e.GetTeacherId(),
		Value:        e.GetValue(),
	}
}

func toRosterRow(r *gradesv1.RosterRow) RosterRow {
	return RosterRow{
		EnrollmentID: r.GetEnrollmentId(),
		StudentID:    r.GetStudentId(),
		Value:        r.GetValue(),
		HasValue:     r.GetHasValue(),
	}
}
