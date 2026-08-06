// Package configclient implements the students CatalogValidator port over a
// Connect client to the config (academic catalog) bounded context. It is the
// cross-service seam of the enroll knot: the use case validates group/term
// references by RPC while staying testable with a fake, and this adapter is the
// only place the generated config proto types are touched.
//
// Error mapping mirrors the port contract: a config NotFound (the group or term
// does not exist) becomes glerrors.ErrNotFound so the use case can turn it into a
// KindInvalid ("group not found"); any other transport failure is wrapped as
// KindInternal (no config-internal detail leaks upward).
package configclient

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter satisfies the consumer-defined port.
var _ ports.CatalogValidator = (*Client)(nil)

// Client validates cross-service catalog references against config by RPC. It
// depends only on the generated Connect client and translates to the small,
// students-owned GroupRef / TermRef views the port speaks in.
type Client struct {
	rpc configv1connect.ConfigServiceClient
}

// New builds a Client targeting the config base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors the gateway config
// client).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: configv1connect.NewConfigServiceClient(httpClient, baseURL)}
}

// GroupByID fetches a group by id and projects it to the students-owned view
// (its identity, the cycle it derives from, and its grade level). A missing
// group is glerrors.ErrNotFound.
func (c *Client) GroupByID(ctx context.Context, id string) (ports.GroupRef, error) {
	res, err := c.rpc.GetGroup(ctx, connect.NewRequest(&configv1.GetGroupRequest{Id: id}))
	if err != nil {
		return ports.GroupRef{}, mapCatalogErr(err, "group")
	}
	g := res.Msg.GetGroup()
	return ports.GroupRef{
		ID:           g.GetId(),
		CycleID:      g.GetCycleId(),
		GradeLevelID: g.GetGradeLevelId(),
	}, nil
}

// TermByID fetches a term by id and projects it to the students-owned view (its
// identity and cycle). A missing term is glerrors.ErrNotFound.
func (c *Client) TermByID(ctx context.Context, id string) (ports.TermRef, error) {
	res, err := c.rpc.GetTerm(ctx, connect.NewRequest(&configv1.GetTermRequest{Id: id}))
	if err != nil {
		return ports.TermRef{}, mapCatalogErr(err, "term")
	}
	t := res.Msg.GetTerm()
	return ports.TermRef{
		ID:      t.GetId(),
		CycleID: t.GetCycleId(),
	}, nil
}

// mapCatalogErr translates a Connect transport error to the port's error
// vocabulary: NotFound → the shared glerrors.ErrNotFound sentinel; anything else
// → KindInternal wrapping the cause (the caller never sees config-internal
// detail beyond the wrapped error's own message).
func mapCatalogErr(err error, what string) error {
	if connect.CodeOf(err) == connect.CodeNotFound {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "config "+what+" lookup failed", err)
}
