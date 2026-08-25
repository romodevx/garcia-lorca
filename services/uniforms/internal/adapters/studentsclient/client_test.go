package studentsclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/adapters/studentsclient"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

// stubStudents is a students service that answers exactly what a test needs.
// Running the real Connect stack in memory means the wire shape is exercised —
// a field renamed in the proto breaks these tests, which is the point.
type stubStudents struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	student     *studentsv1.Student
	enrollments []*studentsv1.Enrollment
	getErr      error
	listErr     error
}

func (s *stubStudents) GetStudent(context.Context, *connect.Request[studentsv1.GetStudentRequest]) (*connect.Response[studentsv1.GetStudentResponse], error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return connect.NewResponse(&studentsv1.GetStudentResponse{Student: s.student}), nil
}

func (s *stubStudents) ListEnrollments(context.Context, *connect.Request[studentsv1.ListEnrollmentsRequest]) (*connect.Response[studentsv1.ListEnrollmentsResponse], error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return connect.NewResponse(&studentsv1.ListEnrollmentsResponse{Enrollments: s.enrollments}), nil
}

func clientFor(t *testing.T, stub *stubStudents) *studentsclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(studentsv1connect.NewStudentsServiceHandler(stub))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return studentsclient.New(srv.URL, srv.Client())
}

func aStudent() *studentsv1.Student {
	return &studentsv1.Student{
		Id: "stu-1", FirstName: " Zoe ", LastName: " Villalobos ",
		Level: studentsv1.Level_LEVEL_PRIMARIA, GradeGroup: "3 B",
		AdmissionStatus: studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED,
	}
}

// TestByIDProjectsWhatTheOrderSnapshots: the panel filters by level and searches
// by name on every page load, so those travel with the pupil rather than being
// resolved per row later.
func TestByIDProjectsWhatTheOrderSnapshots(t *testing.T) {
	c := clientFor(t, &stubStudents{
		student: aStudent(),
		enrollments: []*studentsv1.Enrollment{
			{Id: "enr-1", CycleId: "cyc-2026", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE},
		},
	})

	got, err := c.ByID(context.Background(), "stu-1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.FullName != "Zoe Villalobos" {
		t.Errorf("name = %q; the two halves are joined once, here", got.FullName)
	}
	if got.Level != domain.LevelPrimaria || got.GradeGroup != "3 B" {
		t.Errorf("level %q, group %q", got.Level, got.GradeGroup)
	}
	if got.CycleID != "cyc-2026" {
		t.Errorf("cycle = %q, want cyc-2026", got.CycleID)
	}
}

// TestByIDPicksTheActiveEnrolment is the reason this is two calls and not one. A
// charge has to name a cycle; the cycle lives on the enrolment; and a pupil who
// changed groups has more than one row, only one of which is live. Charging the
// superseded one would attach a uniform to a year the family is not in.
func TestByIDPicksTheActiveEnrolment(t *testing.T) {
	c := clientFor(t, &stubStudents{
		student: aStudent(),
		enrollments: []*studentsv1.Enrollment{
			{Id: "enr-old", CycleId: "cyc-2025", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_TRANSFERRED},
			{Id: "enr-gone", CycleId: "cyc-2024", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN},
			{Id: "enr-live", CycleId: "cyc-2026", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE},
		},
	})

	got, err := c.ByID(context.Background(), "stu-1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.CycleID != "cyc-2026" {
		t.Errorf("cycle = %q; a transferred or withdrawn row is not the one to bill", got.CycleID)
	}
}

// TestByIDWithNoLiveEnrolmentIsNotAnError: the shop decides what to do about a
// pupil with no live enrolment. For a counter sale to a withdrawn pupil the
// answer may well be "sell it anyway, just do not bill a cycle".
func TestByIDWithNoLiveEnrolmentIsNotAnError(t *testing.T) {
	c := clientFor(t, &stubStudents{
		student: aStudent(),
		enrollments: []*studentsv1.Enrollment{
			{Id: "enr-gone", CycleId: "cyc-2024", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_WITHDRAWN},
		},
	})

	got, err := c.ByID(context.Background(), "stu-1")
	if err != nil {
		t.Fatalf("a pupil with no live enrolment must not be an error: %v", err)
	}
	if got.CycleID != "" {
		t.Errorf("cycle = %q, want empty", got.CycleID)
	}
	if got.ID != "stu-1" {
		t.Errorf("the pupil themself is still returned: %+v", got)
	}
}

// TestByIDMissingStudentIsTheSharedSentinel keeps the boundary's vocabulary: the
// use case turns this into a message a cashier can read.
func TestByIDMissingStudentIsTheSharedSentinel(t *testing.T) {
	c := clientFor(t, &stubStudents{
		getErr: connect.NewError(connect.CodeNotFound, errors.New("no such student")),
	})
	_, err := c.ByID(context.Background(), "stu-404")
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v", err)
	}
}

// TestByIDHidesStudentsInternals: whatever the other service was carrying stays
// there. A cashier does not need its stack trace, and neither does an attacker.
func TestByIDHidesStudentsInternals(t *testing.T) {
	c := clientFor(t, &stubStudents{
		getErr: connect.NewError(connect.CodeInternal, errors.New("pq: relation students does not exist")),
	})
	_, err := c.ByID(context.Background(), "stu-1")
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("want KindInternal, got %v", err)
	}
}

// TestByIDFailsWhenTheEnrolmentLookupFails: half an answer is worse than none.
// Returning the pupil with an empty cycle here would silently produce a charge
// belonging to no school year.
func TestByIDFailsWhenTheEnrolmentLookupFails(t *testing.T) {
	c := clientFor(t, &stubStudents{
		student: aStudent(),
		listErr: connect.NewError(connect.CodeUnavailable, errors.New("students is down")),
	})
	if _, err := c.ByID(context.Background(), "stu-1"); err == nil {
		t.Fatal("a failed enrolment lookup was reported as success")
	}
}

// TestUnknownLevelIsNotTreatedAsEveryLevel is a trap worth naming. On a GARMENT
// an unset level means "fits every level"; on a PUPIL it means "we do not know".
// The two share a zero value, and reading the second as the first would offer a
// kinder smock to a prepa student.
func TestUnknownLevelIsNotTreatedAsEveryLevel(t *testing.T) {
	student := aStudent()
	student.Level = studentsv1.Level_LEVEL_UNSPECIFIED
	c := clientFor(t, &stubStudents{
		student: student,
		enrollments: []*studentsv1.Enrollment{
			{Id: "enr-1", CycleId: "cyc-2026", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE},
		},
	})

	got, err := c.ByID(context.Background(), "stu-1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	kinderSmock, err := domain.NewItem("it-1", "t-1", "BAT", "Batita", "", domain.LevelKinder, 41000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	if kinderSmock.AppliesTo(got.Level) {
		t.Error("a pupil of unknown level was offered a kinder-only garment")
	}
}
