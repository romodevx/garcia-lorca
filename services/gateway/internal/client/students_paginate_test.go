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
	seen  []string   // page tokens the client sent, in order
	ids   [][]string // ids filters the client sent, in order
}

func (f *fakeStudentsRPC) ListStudents(_ context.Context, req *connect.Request[studentsv1.ListStudentsRequest]) (*connect.Response[studentsv1.ListStudentsResponse], error) {
	f.seen = append(f.seen, req.Msg.GetPageToken())
	f.ids = append(f.ids, req.Msg.GetIds())
	if len(f.pages) == 0 {
		out := &studentsv1.ListStudentsResponse{}
		for _, id := range req.Msg.GetIds() {
			out.Students = append(out.Students, &studentsv1.Student{Id: id})
		}
		f.call++
		return connect.NewResponse(out), nil
	}
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

// StudentsByIDs names only the students asked for, in chunks of one page, never
// the whole school.
func TestStudentsByIDsChunksTheIDs(t *testing.T) {
	fake := &fakeStudentsRPC{}
	c := &StudentsClient{rpc: fake}
	ids := make([]string, 250)
	for i := range ids {
		ids[i] = "s" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	got, err := c.StudentsByIDs(context.Background(), ids)
	if err != nil {
		t.Fatalf("StudentsByIDs: %v", err)
	}
	if len(got) != 250 {
		t.Fatalf("got %d students, want 250", len(got))
	}
	if len(fake.ids) != 2 || len(fake.ids[0]) != 200 || len(fake.ids[1]) != 50 {
		t.Errorf("chunks = %d, want [200 50]", len(fake.ids))
	}
	none, err := c.StudentsByIDs(context.Background(), nil)
	if err != nil || len(none) != 0 || fake.call != 2 {
		t.Errorf("no ids must not call students: %v, %d calls", err, fake.call)
	}
}
