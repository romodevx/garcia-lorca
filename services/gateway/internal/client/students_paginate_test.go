package client

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
)

// fakeStudentsRPC implements only ListStudents; the embedded interface satisfies
// the rest (they panic if called, which they are not in this test).
type fakeStudentsRPC struct {
	studentsv1connect.StudentsServiceClient
	pages []*studentsv1.ListStudentsResponse
	call  int
	seen  []string // page tokens the client sent, in order
}

func (f *fakeStudentsRPC) ListStudents(_ context.Context, req *connect.Request[studentsv1.ListStudentsRequest]) (*connect.Response[studentsv1.ListStudentsResponse], error) {
	f.seen = append(f.seen, req.Msg.GetPageToken())
	resp := f.pages[f.call]
	f.call++
	return connect.NewResponse(resp), nil
}

func TestListStudentsFollowsPageTokenAcrossPages(t *testing.T) {
	fake := &fakeStudentsRPC{pages: []*studentsv1.ListStudentsResponse{
		{Students: []*studentsv1.Student{{Id: "a"}, {Id: "b"}}, NextPageToken: "tok-2"},
		{Students: []*studentsv1.Student{{Id: "c"}}, NextPageToken: ""},
	}}
	c := &StudentsClient{rpc: fake}

	got, err := c.ListStudents(context.Background(), "")
	if err != nil {
		t.Fatalf("ListStudents: %v", err)
	}
	// All students across BOTH pages, not just page 1 — otherwise the web shows
	// raw ids for students beyond the first page.
	if len(got) != 3 {
		t.Fatalf("got %d students, want 3 (both pages)", len(got))
	}
	if fake.call != 2 {
		t.Errorf("rpc called %d times, want 2 (followed the page token)", fake.call)
	}
	if len(fake.seen) != 2 || fake.seen[0] != "" || fake.seen[1] != "tok-2" {
		t.Errorf("page tokens sent = %v, want [\"\", \"tok-2\"]", fake.seen)
	}
}
