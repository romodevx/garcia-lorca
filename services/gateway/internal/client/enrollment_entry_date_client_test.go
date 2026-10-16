package client_test

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

type stubEntryDateStudents struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	enroll *studentsv1.EnrollRequest
	grade  *studentsv1.EnrollIntoGradeRequest
}

func (s *stubEntryDateStudents) Enroll(_ context.Context, req *connect.Request[studentsv1.EnrollRequest]) (*connect.Response[studentsv1.EnrollResponse], error) {
	s.enroll = req.Msg
	return connect.NewResponse(&studentsv1.EnrollResponse{Enrollment: &studentsv1.Enrollment{Id: "e"}}), nil
}

func (s *stubEntryDateStudents) EnrollIntoGrade(_ context.Context, req *connect.Request[studentsv1.EnrollIntoGradeRequest]) (*connect.Response[studentsv1.EnrollIntoGradeResponse], error) {
	s.grade = req.Msg
	return connect.NewResponse(&studentsv1.EnrollIntoGradeResponse{Enrollment: &studentsv1.Enrollment{Id: "e"}, GroupName: "1° B"}), nil
}

// T9: the entry day reaches students on both enrol RPCs.
func TestEnrollClientsSendTheEntryDay(t *testing.T) {
	ctx := context.Background()
	stub := &stubEntryDateStudents{}
	path, h := studentsv1connect.NewStudentsServiceHandler(stub)
	students := client.NewStudentsClient(serveAt(t, path, h), http.DefaultClient)

	_, err := students.Enroll(ctx, "s", "g", "", "2026-08-24")
	require.NoError(t, err)
	assert.Equal(t, "2026-08-24", stub.enroll.GetEnrolledOn())

	_, err = students.EnrollIntoGrade(ctx, "s", "gl", "2026-09-01")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-01", stub.grade.GetEnrolledOn())
}
