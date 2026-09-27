package studentsclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/comms/internal/adapters/studentsclient"
	"github.com/garcia-lorca/services/comms/internal/service"
)

type fakeStudents struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	code connect.Code // 0 = answer
}

func (f *fakeStudents) GetStudent(_ context.Context, req *connect.Request[studentsv1.GetStudentRequest]) (*connect.Response[studentsv1.GetStudentResponse], error) {
	if f.code != 0 {
		return nil, connect.NewError(f.code, errors.New("nope"))
	}
	return connect.NewResponse(&studentsv1.GetStudentResponse{
		Student: &studentsv1.Student{Id: req.Msg.GetId(), FirstName: "Ana Sofía", LastName: "López"},
	}), nil
}

func serve(t *testing.T, f *fakeStudents) *studentsclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(studentsv1connect.NewStudentsServiceHandler(f))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return studentsclient.New(srv.URL, srv.Client())
}

// FirstName carries the pupil's first name(s) only — never the surname.
func TestFirstName(t *testing.T) {
	tests := []struct {
		name     string
		code     connect.Code
		want     string
		wantKind glerrors.Kind
	}{
		{name: "found", want: "Ana Sofía"},
		{name: "not found reads as the seam's NotFound", code: connect.CodeNotFound, wantKind: glerrors.KindNotFound},
		{name: "an outage is internal, never NotFound", code: connect.CodeUnavailable, wantKind: glerrors.KindInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := serve(t, &fakeStudents{code: tt.code}).FirstName(context.Background(), "3f2504e0-4f89-41d3-9a0c-0305e82c3301")
			if tt.wantKind != 0 {
				require.Error(t, err)
				assert.Equal(t, tt.wantKind, glerrors.KindOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Review #6: only Unavailable and DeadlineExceeded are transient — the one
// failure comms asks a redelivery for. Every other code is permanent.
func TestFirstNameClassifiesTransientFailures(t *testing.T) {
	tests := []struct {
		code          connect.Code
		wantTransient bool
	}{
		{connect.CodeUnavailable, true},
		{connect.CodeDeadlineExceeded, true},
		{connect.CodePermissionDenied, false},
		{connect.CodeInvalidArgument, false},
		{connect.CodeInternal, false},
	}
	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			_, err := serve(t, &fakeStudents{code: tt.code}).FirstName(context.Background(), "3f2504e0-4f89-41d3-9a0c-0305e82c3301")
			require.Error(t, err)
			assert.Equal(t, tt.wantTransient, errors.Is(err, service.ErrStudentsTransient))
		})
	}
}
