package studentsclient

import (
	"context"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter satisfies the directory port too.
var _ ports.StudentDirectory = (*Client)(nil)

// idsPerRead is the most ids the students service accepts in one ListStudents
// (one page), so a class list never drains the whole school.
const idsPerRead = 200

// StudentsByIDs names pupils by id in pages of idsPerRead — a level of 450
// pupils is three reads, never one per pupil. A pupil the directory does not
// return is absent from the map.
func (c *Client) StudentsByIDs(ctx context.Context, ids []string) (map[string]ports.StudentRef, error) {
	out := make(map[string]ports.StudentRef, len(ids))
	for start := 0; start < len(ids); start += idsPerRead {
		page := ids[start:min(start+idsPerRead, len(ids))]
		res, err := c.rpc.ListStudents(ctx, connect.NewRequest(&studentsv1.ListStudentsRequest{Ids: page, PageSize: int32(len(page))}))
		if err != nil {
			return nil, mapStudentsErr(err, "students")
		}
		for _, s := range res.Msg.GetStudents() {
			out[s.GetId()] = ports.StudentRef{ID: s.GetId(), Name: fullName(s.GetFirstName(), s.GetLastName()), EnrollmentNumber: s.GetEnrollmentNumber()}
		}
	}
	return out, nil
}

func fullName(first, last string) string {
	switch {
	case first == "":
		return last
	case last == "":
		return first
	default:
		return first + " " + last
	}
}
