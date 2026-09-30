package studentsclient_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/services/grades/internal/adapters/studentsclient"
)

// directoryStub answers ListStudents by ids, refusing more than 200 at once
// like the real service, and records every page it was asked for.
type directoryStub struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	pages [][]string
}

func (s *directoryStub) ListStudents(_ context.Context, r *connect.Request[studentsv1.ListStudentsRequest]) (*connect.Response[studentsv1.ListStudentsResponse], error) {
	ids := r.Msg.GetIds()
	if len(ids) > 200 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("too many ids"))
	}
	s.pages = append(s.pages, ids)
	out := make([]*studentsv1.Student, 0, len(ids))
	for _, id := range ids {
		if id == "ghost" {
			continue
		}
		out = append(out, &studentsv1.Student{Id: id, FirstName: "Ana", LastName: "López", MaternalLastName: id, EnrollmentNumber: "2026-" + id})
	}
	return connect.NewResponse(&studentsv1.ListStudentsResponse{Students: out}), nil
}

func newDirectory(t *testing.T, svc *directoryStub) *studentsclient.Client {
	t.Helper()
	path, handler := studentsv1connect.NewStudentsServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return studentsclient.New(srv.URL, srv.Client())
}

// T7: a level's pupils are named in pages of at most 200 ids (the students
// service's cap), never one read per pupil; an unknown pupil is absent.
func TestStudentsByIDsPagesBy200(t *testing.T) {
	stub := &directoryStub{}
	ids := make([]string, 0, 450)
	for i := 0; i < 449; i++ {
		ids = append(ids, fmt.Sprintf("s%03d", i))
	}
	ids = append(ids, "ghost")

	got, err := newDirectory(t, stub).StudentsByIDs(context.Background(), ids)
	require.NoError(t, err)
	assert.Len(t, stub.pages, 3, "450 ids = 3 reads")
	assert.Len(t, got, 449)
	assert.Equal(t, "López s007 Ana", got["s007"].Name, "surnames first (T3)")
	assert.Equal(t, "2026-s007", got["s007"].EnrollmentNumber)
	_, ok := got["ghost"]
	assert.False(t, ok)

	empty, err := newDirectory(t, &directoryStub{}).StudentsByIDs(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
