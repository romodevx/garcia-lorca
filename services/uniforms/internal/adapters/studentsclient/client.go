// Package studentsclient implements the uniform shop's StudentReader port over a
// Connect client to the students bounded context.
//
// The shop asks students two things when a counter sale names a pupil: does this
// child exist, and which cycle is their live enrolment in. The first is
// existence; the second is what the finance charge has to belong to, and it is
// NOT on the student record — it lives on the enrolment, so this adapter reads
// both and answers with one small view.
//
// The projection carries the name, level and group because the order snapshots
// them: the panel filters by level and searches by name on every page load, and
// resolving that per row would make the busiest screen of the busiest fortnight
// depend on students being awake. Nothing beyond that crosses the boundary.
package studentsclient

import (
	"context"
	"net/http"
	"strings"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

// Compile-time proof the adapter satisfies the port.
var _ ports.StudentReader = (*Client)(nil)

// Client reads students and their live enrolment by RPC.
type Client struct {
	rpc studentsv1connect.StudentsServiceClient
}

// New builds a Client targeting the students base URL over httpClient. A nil
// client falls back to http.DefaultClient, which is what local development uses.
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: studentsv1connect.NewStudentsServiceClient(httpClient, baseURL)}
}

// ByID fetches a pupil and the cycle their live enrolment belongs to.
//
// It is two calls, deliberately. A charge has to name a cycle, the cycle lives on
// the enrolment, and guessing it — using "the current one", say — would attach a
// family's uniform to a year they are not enrolled in. A pupil with no live
// enrolment comes back with an empty CycleID rather than an error: the shop
// decides what to do about that, and for a counter sale to a withdrawn pupil the
// answer may well be "sell it anyway, just do not bill the cycle".
func (c *Client) ByID(ctx context.Context, id string) (ports.StudentRef, error) {
	res, err := c.rpc.GetStudent(ctx, connect.NewRequest(&studentsv1.GetStudentRequest{Id: id}))
	if err != nil {
		return ports.StudentRef{}, mapErr(err, "student")
	}
	s := res.Msg.GetStudent()

	cycleID, err := c.liveCycle(ctx, id)
	if err != nil {
		return ports.StudentRef{}, err
	}
	return ports.StudentRef{
		ID:       s.GetId(),
		FullName: fullName(s.GetFirstName(), s.GetLastName()),
		// A pupil with no level id (a legacy row the level migration could not
		// link) is LevelAny, which for a PUPIL means "unknown" rather than "all".
		Level:      domain.Level(s.GetLevelId()),
		LevelName:  s.GetLevelName(),
		GradeGroup: s.GetGradeGroup(),
		CycleID:    cycleID,
		Status:     admissionStatus(s.GetAdmissionStatus()),
	}, nil
}

// liveCycle returns the cycle of the pupil's ACTIVE enrolment, or empty if they
// have none. Transferred and withdrawn rows are skipped: a transfer is
// superseded by the row that replaced it, and a withdrawal is not a year anyone
// should be charged for.
func (c *Client) liveCycle(ctx context.Context, studentID string) (string, error) {
	res, err := c.rpc.ListEnrollments(ctx, connect.NewRequest(&studentsv1.ListEnrollmentsRequest{
		StudentId: studentID,
	}))
	if err != nil {
		return "", mapErr(err, "enrollment")
	}
	for _, e := range res.Msg.GetEnrollments() {
		if e.GetStatus() == studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE {
			return e.GetCycleId(), nil
		}
	}
	return "", nil
}

// fullName joins the two halves the students service keeps apart. The panel
// shows one string, so the join happens once, here, rather than in every caller.
func fullName(first, last string) string {
	return strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
}

var admissionStatuses = map[studentsv1.AdmissionStatus]string{
	studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING:  "pending",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED: "accepted",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED: "rejected",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED: "enrolled",
}

func admissionStatus(s studentsv1.AdmissionStatus) string { return admissionStatuses[s] }

// mapErr keeps the boundary honest: a NotFound becomes the shared sentinel the
// use case knows how to turn into a message, and everything else is internal —
// no students-side detail travels outward.
func mapErr(err error, what string) error {
	if connect.CodeOf(err) == connect.CodeNotFound {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "students "+what+" lookup failed", err)
}
