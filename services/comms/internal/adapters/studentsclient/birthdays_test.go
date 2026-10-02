package studentsclient_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/services/comms/internal/adapters/studentsclient"
	"github.com/garcia-lorca/services/comms/internal/service"
)

// pagedStudents answers ListStudents in pages of two, recording each request.
type pagedStudents struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	total    int
	requests []*studentsv1.ListStudentsRequest
	fail     bool
	endless  bool
}

func (p *pagedStudents) ListStudents(_ context.Context, req *connect.Request[studentsv1.ListStudentsRequest]) (*connect.Response[studentsv1.ListStudentsResponse], error) {
	p.requests = append(p.requests, req.Msg)
	if p.fail {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("down"))
	}
	if p.endless {
		return connect.NewResponse(&studentsv1.ListStudentsResponse{NextPageToken: "again"}), nil
	}
	start := 0
	if tok := req.Msg.GetPageToken(); tok != "" {
		_, _ = fmt.Sscanf(tok, "p%d", &start)
	}
	res := &studentsv1.ListStudentsResponse{}
	for i := start; i < start+2 && i < p.total; i++ {
		res.Students = append(res.Students, &studentsv1.Student{Id: fmt.Sprintf("s%d", i), FirstName: fmt.Sprintf("N%d", i), LastName: "Secret"})
	}
	if start+2 < p.total {
		res.NextPageToken = fmt.Sprintf("p%d", start+2)
	}
	return connect.NewResponse(res), nil
}

func servePaged(t *testing.T, p *pagedStudents) *studentsclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(studentsv1connect.NewStudentsServiceHandler(p))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return studentsclient.New(srv.URL, srv.Client())
}

// BirthdaysOn asks for ENROLLED pupils born on the day and walks every page.
func TestBirthdaysOnWalksEveryPage(t *testing.T) {
	p := &pagedStudents{total: 5}
	got, err := servePaged(t, p).BirthdaysOn(context.Background(), 2, 28, true)
	require.NoError(t, err)

	require.Len(t, got, 5)
	assert.Equal(t, service.Pupil{ID: "s0", FirstName: "N0"}, got[0])
	assert.Equal(t, "s4", got[4].ID)
	require.Len(t, p.requests, 3)
	first := p.requests[0]
	assert.Equal(t, studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED, first.GetStatus())
	assert.Equal(t, int32(2), first.GetBirthday().GetMonth())
	assert.Equal(t, int32(28), first.GetBirthday().GetDay())
	assert.True(t, first.GetBirthday().GetIncludeFeb_29())
	assert.Equal(t, "p2", p.requests[1].GetPageToken())
}

func TestBirthdaysOnReportsAnOutage(t *testing.T) {
	_, err := servePaged(t, &pagedStudents{fail: true}).BirthdaysOn(context.Background(), 10, 1, false)
	require.Error(t, err)
}

// A directory that never stops paging is a bug there; the walk is bounded here.
func TestBirthdaysOnIsBounded(t *testing.T) {
	p := &pagedStudents{endless: true}
	_, err := servePaged(t, p).BirthdaysOn(context.Background(), 10, 1, false)
	require.Error(t, err)
	assert.LessOrEqual(t, len(p.requests), 100)
}
