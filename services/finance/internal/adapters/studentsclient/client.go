// Package studentsclient implements the finance StudentReader and
// EnrollmentReader ports over a Connect client to the students bounded context.
// It is the seam the ledger validates its references through: issuing a charge
// reads the student (ByID) to prove the debtor exists, and bulk generation
// expands a cycle (or one of its groups) into enrollments (ListByCycle) to issue
// one charge per active row. This adapter is the only place in finance the
// generated students proto types are touched.
//
// The projections carry identity and status only — never PII. Error mapping
// mirrors the port contract: a students NotFound becomes glerrors.ErrNotFound so
// the use case can turn it into a KindInvalid ("student not found"); any other
// transport failure is wrapped as KindInternal (no students-internal detail
// leaks upward).
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
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Compile-time proof the adapter satisfies both consumer-defined ports: one
// Connect client serves them because both read from the same bounded context.
var (
	_ ports.StudentReader    = (*Client)(nil)
	_ ports.EnrollmentReader = (*Client)(nil)
)

// Client reads students and their enrollments by RPC. It depends only on the
// generated Connect client and translates to the small, finance-owned
// StudentRef / EnrollmentRef views the ports speak in.
type Client struct {
	rpc studentsv1connect.StudentsServiceClient
}

// New builds a Client targeting the students base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors the grades students
// client).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: studentsv1connect.NewStudentsServiceClient(httpClient, baseURL)}
}

// ByID fetches a student by id and projects it to the finance-owned view. What
// the ledger needs from students is existence — the admission status rides along
// best-effort, so a use case can refuse to charge a withdrawn applicant without
// a second round trip. A missing student is glerrors.ErrNotFound.
func (c *Client) ByID(ctx context.Context, id string) (ports.StudentRef, error) {
	res, err := c.rpc.GetStudent(ctx, connect.NewRequest(&studentsv1.GetStudentRequest{Id: id}))
	if err != nil {
		return ports.StudentRef{}, mapStudentsErr(err, "student")
	}
	s := res.Msg.GetStudent()
	return ports.StudentRef{
		ID:     s.GetId(),
		Status: admissionStatusString(s.GetAdmissionStatus()),
	}, nil
}

// ListByCycle lists the enrollments a bulk charge generation expands over and
// projects each to the finance-owned view. An empty groupID means the whole
// cycle: the request treats an empty filter as "any", so it is passed straight
// through. Filtering by status stays with the use case.
func (c *Client) ListByCycle(ctx context.Context, cycleID, groupID string) ([]ports.EnrollmentRef, error) {
	req := &studentsv1.ListEnrollmentsRequest{CycleId: cycleID, GroupId: groupID}
	res, err := c.rpc.ListEnrollments(ctx, connect.NewRequest(req))
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

// ListByStudent returns one pupil's enrolments across every cycle. A deposit
// needs it to find where the pupil sits: the group leads to the level, and the
// level's tuition table carries the collection rule that decides what a late
// payment costs.
func (c *Client) ListByStudent(ctx context.Context, studentID string) ([]ports.EnrollmentRef, error) {
	req := &studentsv1.ListEnrollmentsRequest{StudentId: studentID}
	res, err := c.rpc.ListEnrollments(ctx, connect.NewRequest(req))
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

// toRef projects a proto Enrollment to the finance-owned view, mapping the
// status enum to the lowercase string the use case compares against ("active",
// …).
func toRef(e *studentsv1.Enrollment) ports.EnrollmentRef {
	return ports.EnrollmentRef{
		ID:        e.GetId(),
		StudentID: e.GetStudentId(),
		GroupID:   e.GetGroupId(),
		CycleID:   e.GetCycleId(),
		Status:    statusString(e.GetStatus()),
		// AsTime() on an absent timestamp is the Unix epoch, not the zero time,
		// and an epoch join date would put every pupil before every cycle — the
		// same answer as zero here, but by accident rather than on purpose.
		EnrolledAt: enrolledAt(e.GetEnrolledAt()),
		Origin:     e.GetOrigin(),
	}
}

// enrolledAt reads the day a pupil took their place, keeping "not recorded" as
// the zero time rather than 1970.
func enrolledAt(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

// statusString maps the proto EnrollmentStatus enum to the lowercase status
// string the use case speaks in (mirroring the students domain string form). An
// unspecified/unknown enum maps to the empty string, which the use case treats
// as non-active — so an enum this build does not know about is never charged.
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

// admissionStatusString maps the proto AdmissionStatus enum to the lowercase
// status string the finance side speaks in (mirroring the students domain string
// form). An unspecified/unknown enum maps to the empty string: the ledger's
// contract is that existence is what matters, so an unmapped status must never
// read as a live admission.
func admissionStatusString(s studentsv1.AdmissionStatus) string {
	switch s {
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING:
		return "pending"
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED:
		return "accepted"
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED:
		return "enrolled"
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED:
		return "rejected"
	case studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN:
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
