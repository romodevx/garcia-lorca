// Package studentsclient implements the comms GuardianReader seam over a Connect
// client to the students bounded context. Comms owns no guardian link: WHICH
// users are a student's guardians is students' knowledge, read on every push, and
// this adapter is the only place the generated students proto types are touched.
//
// It projects each link to its guardian USER id — the id devices are registered
// under here — and nothing else: comms carries no family PII, only the ids it
// needs to reach a phone.
package studentsclient

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/comms/internal/service"
)

// Compile-time proof the adapter satisfies the consumer-defined seam.
var _ service.GuardianReader = (*Client)(nil)

// Client reads guardian links from students by RPC. It depends only on the
// generated Connect client and speaks the seam's vocabulary: a list of user ids.
type Client struct {
	rpc studentsv1connect.StudentsServiceClient
}

// New builds a Client targeting the students base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors attendance and grades).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: studentsv1connect.NewStudentsServiceClient(httpClient, baseURL)}
}

// GuardiansForStudent lists the users linked to studentID as guardians. A student
// with none is an EMPTY list and no error — the read is a list, so "no guardians
// yet" is a normal answer rather than a missing resource, and the caller turns it
// into "nobody to push to".
func (c *Client) GuardiansForStudent(ctx context.Context, studentID string) ([]string, error) {
	res, err := c.rpc.ListGuardiansForStudent(ctx,
		connect.NewRequest(&studentsv1.ListGuardiansForStudentRequest{StudentId: studentID}))
	if err != nil {
		return nil, mapStudentsErr(err)
	}
	return guardianUserIDs(res.Msg.GetLinks()), nil
}

// guardianUserIDs projects the links onto the ids a push is targeted by. A link
// carrying no guardian user is DROPPED rather than passed on: an empty id would
// match no device at best, and every device with an unset owner at worst.
func guardianUserIDs(links []*studentsv1.GuardianLink) []string {
	out := make([]string, 0, len(links))
	for _, l := range links {
		if id := l.GetGuardianUserId(); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// mapStudentsErr translates a Connect transport error to the seam's vocabulary:
// everything is KindInternal wrapping the cause (no students-internal detail
// leaks upward).
//
// There is deliberately no NotFound case. The seam has no "missing" answer —
// a student comms cannot resolve guardians for simply has no push audience — and
// mapping a failure to the empty list here would be indistinguishable from that,
// silently turning a students outage into "this family has nobody to notify".
func mapStudentsErr(err error) error {
	return glerrors.Wrap(glerrors.KindInternal, "students guardian lookup failed", err)
}
