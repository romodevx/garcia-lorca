// Package studentsclient implements the grades EnrollmentReader port over a
// Connect client to the students bounded context. It is the second cross-service
// seam of the capture knot and the roster: the use case reads a single
// enrollment (ByID) to validate it belongs to the assignment's group/cycle and
// is active, and lists a group's enrollments (ByGroup) to build the roster. This
// adapter is the only place the generated students proto types are touched.
//
// Error mapping mirrors the port contract: a students NotFound (the enrollment
// does not exist) becomes glerrors.ErrNotFound so the use case can turn it into
// a KindInvalid ("enrollment not found"); any other transport failure is wrapped
// as KindInternal (no students-internal detail leaks upward).
package studentsclient

import (
	"context"
	"net/http"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter satisfies the consumer-defined port.
var _ ports.EnrollmentReader = (*Client)(nil)

// Client reads enrollments from students by RPC. It depends only on the
// generated Connect client and translates to the small, grades-owned
// EnrollmentRef view the port speaks in.
type Client struct {
	rpc studentsv1connect.StudentsServiceClient
}

// New builds a Client targeting the students base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors the students config
// client).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: studentsv1connect.NewStudentsServiceClient(httpClient, baseURL)}
}

// ByID fetches an enrollment by id and projects it to the grades-owned view
// (enough to place the student in a group of a cycle and know whether the
// enrollment is live). A missing enrollment is glerrors.ErrNotFound.
func (c *Client) ByID(ctx context.Context, id string) (ports.EnrollmentRef, error) {
	res, err := c.rpc.GetEnrollment(ctx, connect.NewRequest(&studentsv1.GetEnrollmentRequest{Id: id}))
	if err != nil {
		return ports.EnrollmentRef{}, mapStudentsErr(err, "enrollment")
	}
	return toRef(res.Msg.GetEnrollment()), nil
}

// ByGroup lists the enrollments of a group and projects each to the grades-owned
// view. The use case filters to active enrollments when building the roster.
func (c *Client) ByGroup(ctx context.Context, groupID string) ([]ports.EnrollmentRef, error) {
	res, err := c.rpc.ListEnrollments(ctx, connect.NewRequest(&studentsv1.ListEnrollmentsRequest{GroupId: groupID}))
	if err != nil {
		return nil, mapStudentsErr(err, "enrollments")
	}
	src := res.Msg.GetEnrollments()
	out := make([]ports.EnrollmentRef, 0, len(src))
	for _, e := range src {
		out = append(out, toRef(e))
	}
	return out, nil
}

// ByCycle lists every enrollment of a cycle (any status) in ONE read.
func (c *Client) ByCycle(ctx context.Context, cycleID string) ([]ports.EnrollmentRef, error) {
	res, err := c.rpc.ListEnrollments(ctx, connect.NewRequest(&studentsv1.ListEnrollmentsRequest{CycleId: cycleID}))
	if err != nil {
		return nil, mapStudentsErr(err, "enrollments")
	}
	src := res.Msg.GetEnrollments()
	out := make([]ports.EnrollmentRef, 0, len(src))
	for _, e := range src {
		out = append(out, toRef(e))
	}
	return out, nil
}

// TransfersIntoGroup lists every move INTO a group in ONE read.
func (c *Client) TransfersIntoGroup(ctx context.Context, groupID string) ([]ports.TransferRef, error) {
	res, err := c.rpc.ListTransfers(ctx, connect.NewRequest(&studentsv1.ListTransfersRequest{ToGroupId: groupID}))
	if err != nil {
		return nil, mapStudentsErr(err, "transfers")
	}
	src := res.Msg.GetTransfers()
	out := make([]ports.TransferRef, 0, len(src))
	for _, t := range src {
		out = append(out, ports.TransferRef{EnrollmentID: t.GetEnrollmentId(), ToGroupID: t.GetToGroupId(), MovedAt: t.GetMovedAt().AsTime().UTC()})
	}
	return out, nil
}

// toRef projects a proto Enrollment to the grades-owned view, mapping the status
// enum to the lowercase string the use case compares against ("active", …).
func toRef(e *studentsv1.Enrollment) ports.EnrollmentRef {
	return ports.EnrollmentRef{
		ID:        e.GetId(),
		StudentID: e.GetStudentId(),
		GroupID:   e.GetGroupId(),
		CycleID:   e.GetCycleId(),
		Status:    statusString(e.GetStatus()),
		// Unset stays the zero time (unknown): the use case then expects every
		// month rather than inventing a start.
		EnrolledAt: enrolledAt(e),
	}
}

func enrolledAt(e *studentsv1.Enrollment) time.Time {
	if e.GetEnrolledAt() == nil {
		return time.Time{}
	}
	return e.GetEnrolledAt().AsTime().UTC()
}

// statusString maps the proto EnrollmentStatus enum to the lowercase status
// string the use case speaks in (mirroring the students domain string form). An
// unspecified/unknown enum maps to the empty string, which the use case treats
// as non-active.
func statusString(s studentsv1.EnrollmentStatus) string {
	switch s {
	case studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE:
		return "active"
	case studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_TRANSFERRED:
		return "transferred"
	case studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN:
		return "withdrawn"
	default:
		return ""
	}
}

// mapStudentsErr translates a Connect transport error to the port's error
// vocabulary: NotFound → the shared glerrors.ErrNotFound sentinel; anything else
// → KindInternal wrapping the cause.
func mapStudentsErr(err error, what string) error {
	if connect.CodeOf(err) == connect.CodeNotFound {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "students "+what+" lookup failed", err)
}
