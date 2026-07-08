package client

import (
	"context"
	"net/http"

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
