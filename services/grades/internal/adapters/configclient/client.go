// Package configclient implements the grades AssignmentReader port over a
// Connect client to the config (academic catalog) bounded context. It is one of
// the two cross-service seams of the capture knot: the use case reads the
// teaching assignment (which yields group/subject/teacher/cycle) and validates
// the term by RPC while staying testable with a fake, and this adapter is the
// only place the generated config proto types are touched.
//
// Error mapping mirrors the port contract: a config NotFound (the assignment or
// term does not exist) becomes glerrors.ErrNotFound so the use case can turn it
// into a KindInvalid ("assignment not found"); any other transport failure is
// wrapped as KindInternal (no config-internal detail leaks upward).
package configclient

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter satisfies the consumer-defined port.
var _ ports.AssignmentReader = (*Client)(nil)

// Client reads teaching assignments and terms from config by RPC. It depends
// only on the generated Connect client and translates to the small, grades-owned
// AssignmentRef / TermRef views the port speaks in.
type Client struct {
	rpc configv1connect.ConfigServiceClient
}

// New builds a Client targeting the config base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors the students config
// client).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: configv1connect.NewConfigServiceClient(httpClient, baseURL)}
}

// ByID fetches a teaching assignment by id and projects it to the grades-owned
// view (its identity plus the group/subject/teacher/cycle it binds — subject and
// teacher are DERIVED from here by the use case). A missing assignment is
// glerrors.ErrNotFound.
func (c *Client) ByID(ctx context.Context, id string) (ports.AssignmentRef, error) {
	res, err := c.rpc.GetTeachingAssignment(ctx, connect.NewRequest(&configv1.GetTeachingAssignmentRequest{Id: id}))
	if err != nil {
		return ports.AssignmentRef{}, mapConfigErr(err, "teaching assignment")
	}
	a := res.Msg.GetTeachingAssignment()
	return ports.AssignmentRef{
		ID:        a.GetId(),
		GroupID:   a.GetGroupId(),
		SubjectID: a.GetSubjectId(),
		TeacherID: a.GetTeacherId(),
		CycleID:   a.GetCycleId(),
	}, nil
}

// TermByID fetches a term by id and projects it to the grades-owned view (its
// identity and cycle). A missing term is glerrors.ErrNotFound.
func (c *Client) TermByID(ctx context.Context, id string) (ports.TermRef, error) {
	res, err := c.rpc.GetTerm(ctx, connect.NewRequest(&configv1.GetTermRequest{Id: id}))
	if err != nil {
		return ports.TermRef{}, mapConfigErr(err, "term")
	}
	t := res.Msg.GetTerm()
	return ports.TermRef{
		ID:      t.GetId(),
		CycleID: t.GetCycleId(),
	}, nil
}

// mapConfigErr translates a Connect transport error to the port's error
// vocabulary: NotFound → the shared glerrors.ErrNotFound sentinel; anything else
// → KindInternal wrapping the cause (the caller never sees config-internal
// detail beyond the wrapped error's own message).
func mapConfigErr(err error, what string) error {
	if connect.CodeOf(err) == connect.CodeNotFound {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "config "+what+" lookup failed", err)
}
