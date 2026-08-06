package client

import (
	"context"
	"net/http"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
)

// StudentsClient is the gateway's typed client for the students service over the
// generated Connect client. Handlers depend on its small methods and the plain
// structs below, never on the generated proto types.
type StudentsClient struct {
	rpc studentsv1connect.StudentsServiceClient
}

// CreateApplicantInput is the edge's view of a new-applicant request. Level is a
// canonical lowercase name ("kinder", "primaria", …); an unknown value maps to
// the unspecified wire enum, which the students service rejects as invalid.
type CreateApplicantInput struct {
	CURP         string
	FirstName    string
	LastName     string
	Level        string
	GradeGroup   string
	GuardianName string
}

// StudentResult is the edge's view of a student, with enums flattened to their
// canonical lowercase names for JSON.
type StudentResult struct {
	ID              string
	CURP            string
	FirstName       string
	LastName        string
	Level           string
	GradeGroup      string
	GuardianName    string
	AdmissionStatus string
}

// Enrollment is the edge's view of a student's placement in a group of a cycle,
// with the wire status enum flattened to its canonical lowercase name
// ("active"/"transferred"/"withdrawn") and the timestamp as time.Time. CycleID
// is DERIVED server-side from the group at enroll time — it is never a client
// argument.
type Enrollment struct {
	ID         string
	StudentID  string
	CycleID    string
	GroupID    string
	TermID     string
	Status     string
	EnrolledAt time.Time
}

// NewStudentsClient builds a StudentsClient targeting the students base URL over
// httpClient.
func NewStudentsClient(baseURL string, httpClient connect.HTTPClient) *StudentsClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &StudentsClient{rpc: studentsv1connect.NewStudentsServiceClient(httpClient, baseURL)}
}

// CreateApplicant registers a new applicant via the students service. Connect
// error codes propagate (AlreadyExists for a duplicate CURP, InvalidArgument for
// bad input) so the handler maps them to HTTP status.
func (c *StudentsClient) CreateApplicant(ctx context.Context, in CreateApplicantInput) (StudentResult, error) {
	res, err := c.rpc.CreateApplicant(ctx, connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp:         in.CURP,
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		Level:        levelFromName(in.Level),
		GradeGroup:   in.GradeGroup,
		GuardianName: in.GuardianName,
	}))
	if err != nil {
		return StudentResult{}, err
	}
	return toStudentResult(res.Msg.GetStudent()), nil
}

// --- enrollments ----------------------------------------------------------

// Enroll places a student into a group of a cycle. The server derives the cycle
// from the group, so cycleID is never sent; the returned Enrollment carries it.
// termID may be empty. Connect error codes propagate (InvalidArgument for a
// missing group / term-not-in-cycle, AlreadyExists for a duplicate enrollment).
func (c *StudentsClient) Enroll(ctx context.Context, studentID, groupID, termID string) (Enrollment, error) {
	res, err := c.rpc.Enroll(ctx, connect.NewRequest(&studentsv1.EnrollRequest{
		StudentId: studentID,
		GroupId:   groupID,
		TermId:    termID,
	}))
	if err != nil {
		return Enrollment{}, err
	}
	return toEnrollment(res.Msg.GetEnrollment()), nil
}

// ListEnrollments lists enrollments filtered by any of studentID, cycleID,
// groupID ("" = no filter on that dimension).
func (c *StudentsClient) ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]Enrollment, error) {
	res, err := c.rpc.ListEnrollments(ctx, connect.NewRequest(&studentsv1.ListEnrollmentsRequest{
		StudentId: studentID,
		CycleId:   cycleID,
		GroupId:   groupID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]Enrollment, 0, len(res.Msg.GetEnrollments()))
	for _, e := range res.Msg.GetEnrollments() {
		out = append(out, toEnrollment(e))
	}
	return out, nil
}

// GetEnrollment fetches a single enrollment by id.
func (c *StudentsClient) GetEnrollment(ctx context.Context, id string) (Enrollment, error) {
	res, err := c.rpc.GetEnrollment(ctx, connect.NewRequest(&studentsv1.GetEnrollmentRequest{Id: id}))
	if err != nil {
		return Enrollment{}, err
	}
	return toEnrollment(res.Msg.GetEnrollment()), nil
}

// TransferStudent moves an active enrollment to another group in the same cycle,
// recording a dated, reasoned history row. Connect error codes propagate
// (InvalidArgument for a non-active enrollment / cross-cycle or same group).
func (c *StudentsClient) TransferStudent(ctx context.Context, enrollmentID, toGroupID, reason string) (Enrollment, error) {
	res, err := c.rpc.TransferStudent(ctx, connect.NewRequest(&studentsv1.TransferStudentRequest{
		EnrollmentId: enrollmentID,
		ToGroupId:    toGroupID,
		Reason:       reason,
	}))
	if err != nil {
		return Enrollment{}, err
	}
	return toEnrollment(res.Msg.GetEnrollment()), nil
}

// WithdrawEnrollment marks an active enrollment as withdrawn.
func (c *StudentsClient) WithdrawEnrollment(ctx context.Context, id string) (Enrollment, error) {
	res, err := c.rpc.WithdrawEnrollment(ctx, connect.NewRequest(&studentsv1.WithdrawEnrollmentRequest{Id: id}))
	if err != nil {
		return Enrollment{}, err
	}
	return toEnrollment(res.Msg.GetEnrollment()), nil
}

func toEnrollment(e *studentsv1.Enrollment) Enrollment {
	return Enrollment{
		ID:         e.GetId(),
		StudentID:  e.GetStudentId(),
		CycleID:    e.GetCycleId(),
		GroupID:    e.GetGroupId(),
		TermID:     e.GetTermId(),
		Status:     enrollmentStatusName(e.GetStatus()),
		EnrolledAt: timeFromTS(e.GetEnrolledAt()),
	}
}

func toStudentResult(s *studentsv1.Student) StudentResult {
	return StudentResult{
		ID:              s.GetId(),
		CURP:            s.GetCurp(),
		FirstName:       s.GetFirstName(),
		LastName:        s.GetLastName(),
		Level:           levelName(s.GetLevel()),
		GradeGroup:      s.GetGradeGroup(),
		GuardianName:    s.GetGuardianName(),
		AdmissionStatus: admissionName(s.GetAdmissionStatus()),
	}
}

var levelNames = map[studentsv1.Level]string{
	studentsv1.Level_LEVEL_KINDER:       "kinder",
	studentsv1.Level_LEVEL_PRIMARIA:     "primaria",
	studentsv1.Level_LEVEL_SECUNDARIA:   "secundaria",
	studentsv1.Level_LEVEL_PREPARATORIA: "preparatoria",
}

func levelName(l studentsv1.Level) string { return levelNames[l] }

func levelFromName(name string) studentsv1.Level {
	for level, n := range levelNames {
		if n == name {
			return level
		}
	}
	return studentsv1.Level_LEVEL_UNSPECIFIED
}

var admissionNames = map[studentsv1.AdmissionStatus]string{
	studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING:   "pending",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED:  "accepted",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED:  "enrolled",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED:  "rejected",
	studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN: "withdrawn",
}

func admissionName(s studentsv1.AdmissionStatus) string { return admissionNames[s] }

var enrollmentStatusNames = map[studentsv1.EnrollmentStatus]string{
	studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE:      "active",
	studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_TRANSFERRED: "transferred",
	studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN:   "withdrawn",
}

func enrollmentStatusName(s studentsv1.EnrollmentStatus) string { return enrollmentStatusNames[s] }
