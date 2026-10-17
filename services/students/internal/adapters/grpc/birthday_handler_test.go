package grpcadapter_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// T11: the birthday filter reaches the repository as the wire sent it, and an
// unset one stays unset (any day).
func TestListStudentsMapsTheBirthdayFilter(t *testing.T) {
	repo := newFakeRepo()
	svc := usecase.NewStudentService(repo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-1" })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students"))

	if _, err := h.ListStudents(context.Background(), connect.NewRequest(&studentsv1.ListStudentsRequest{
		Birthday: &studentsv1.BirthdayFilter{Month: 2, Day: 28, IncludeFeb_29: true},
	})); err != nil {
		t.Fatalf("ListStudents: %v", err)
	}
	b := repo.lastFilter.Birthday
	if b == nil || b.Month != 2 || b.Day != 28 || !b.IncludeFeb29 {
		t.Errorf("birthday filter = %+v", b)
	}

	if _, err := h.ListStudents(context.Background(), connect.NewRequest(&studentsv1.ListStudentsRequest{})); err != nil {
		t.Fatalf("ListStudents: %v", err)
	}
	if repo.lastFilter.Birthday != nil {
		t.Errorf("no filter sent, got %+v", repo.lastFilter.Birthday)
	}
}

func TestListStudentsRejectsAnImpossibleBirthday(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.ListStudents(context.Background(), connect.NewRequest(&studentsv1.ListStudentsRequest{
		Birthday: &studentsv1.BirthdayFilter{Month: 2, Day: 30},
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestCreateApplicantCarriesTheBirthDate(t *testing.T) {
	h, _ := newHandler(t)
	res, err := h.CreateApplicant(context.Background(), connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp: validCURP, FirstName: "Ana", LastName: "Hernández",
		LevelId: "e1000000-0000-0000-0000-000000000001", GuardianName: "Rosa",
		BirthDate: "2016-02-29",
	}))
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}
	if got := res.Msg.GetStudent().GetProfile().GetBirthDate(); got != "2016-02-29" {
		t.Errorf("birth date = %q, want 2016-02-29", got)
	}
}

// D4 (plan T11): the active-enrollment filter reaches the repository.
func TestListStudentsMapsTheActiveEnrollmentFilter(t *testing.T) {
	repo := newFakeRepo()
	svc := usecase.NewStudentService(repo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-1" })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students"))

	if _, err := h.ListStudents(context.Background(), connect.NewRequest(&studentsv1.ListStudentsRequest{ActiveEnrollment: true})); err != nil {
		t.Fatalf("ListStudents: %v", err)
	}
	if !repo.lastFilter.ActiveEnrollment {
		t.Errorf("active enrollment filter lost: %+v", repo.lastFilter)
	}
}
