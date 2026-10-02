// Package studentsclient implements the attendance EnrollmentReader seam over a
// Connect client to the students bounded context. Attendance owns no enrollments:
// a group's roster IS the list of its active enrollments, read from students on
// every call, and this adapter is the only place the generated students proto
// types are touched.
//
// Error mapping mirrors the seam's contract: a students NotFound becomes
// glerrors.ErrNotFound so the service can turn it into a KindInvalid; any other
// transport failure is wrapped as KindInternal (no students-internal detail leaks
// upward).
package studentsclient

import (
	"context"
	"net/http"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Compile-time proof the adapter satisfies the consumer-defined seam.
var _ service.EnrollmentReader = (*Client)(nil)

// Client reads enrollments from students by RPC. It depends only on the generated
// Connect client and translates to the small, attendance-owned EnrollmentRef view
// the seam speaks in.
type Client struct {
	rpc studentsv1connect.StudentsServiceClient
}

// New builds a Client targeting the students base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors grades).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: studentsv1connect.NewStudentsServiceClient(httpClient, baseURL)}
}

// ByGroup lists the enrollments of a group and projects each to the
// attendance-owned view. The service filters to the active ones — that set is
// both the roster and the exact scope a day's reconcile replaces.
func (c *Client) ByGroup(ctx context.Context, groupID string) ([]service.EnrollmentRef, error) {
	res, err := c.rpc.ListEnrollments(ctx, connect.NewRequest(&studentsv1.ListEnrollmentsRequest{GroupId: groupID}))
	if err != nil {
		return nil, mapStudentsErr(err, "enrollments")
	}
	src := res.Msg.GetEnrollments()
	out := make([]service.EnrollmentRef, 0, len(src))
	for _, e := range src {
		out = append(out, toRef(e))
	}
	return out, nil
}

// toRef projects a proto Enrollment to the attendance-owned view, mapping the
// status enum to the lowercase string the service compares against ("active").
// Only the student id travels with it — attendance never carries student PII.
func toRef(e *studentsv1.Enrollment) service.EnrollmentRef {
	return service.EnrollmentRef{
		ID:         e.GetId(),
		StudentID:  e.GetStudentId(),
		GroupID:    e.GetGroupId(),
		Status:     statusString(e.GetStatus()),
		EnrolledAt: timeOf(e.GetEnrolledAt()),
		EndedAt:    timeOf(e.GetEndedAt()),
	}
}

// timeOf is a timestamp as a UTC time; an unset one is the zero time.
func timeOf(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime().UTC()
}

// TransfersInto lists every move INTO the group in ONE read (T12: when each
// pupil joined it, since a transfer rewrites the enrollment's group in place).
func (c *Client) TransfersInto(ctx context.Context, groupID string) ([]service.TransferIn, error) {
	res, err := c.rpc.ListTransfers(ctx, connect.NewRequest(&studentsv1.ListTransfersRequest{ToGroupId: groupID}))
	if err != nil {
		return nil, mapStudentsErr(err, "transfers")
	}
	out := make([]service.TransferIn, 0, len(res.Msg.GetTransfers()))
	for _, t := range res.Msg.GetTransfers() {
		out = append(out, service.TransferIn{EnrollmentID: t.GetEnrollmentId(), MovedAt: timeOf(t.GetMovedAt())})
	}
	return out, nil
}

// statusString maps the proto EnrollmentStatus enum to the lowercase status string
// the service speaks in (mirroring the students domain string form). An
// unspecified/unknown enum maps to the empty string, which the service treats as
// non-active — an unrecognised enum can never sneak onto a roster.
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

// mapStudentsErr translates a Connect transport error to the seam's error
// vocabulary: NotFound → the shared glerrors.ErrNotFound sentinel; anything else
// → KindInternal wrapping the cause.
func mapStudentsErr(err error, what string) error {
	if connect.CodeOf(err) == connect.CodeNotFound {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "students "+what+" lookup failed", err)
}
