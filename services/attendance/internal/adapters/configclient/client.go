// Package configclient implements the attendance GroupReader seam over a Connect
// client to the config (academic catalog) bounded context. Attendance validates
// that the group a teacher is passing a list for actually exists, and nothing
// more: the roster itself comes from students. This adapter is the only place the
// generated config proto types are touched.
//
// Error mapping mirrors the seam's contract: a config NotFound becomes
// glerrors.ErrNotFound so the service can turn it into a KindInvalid ("group not
// found"); any other transport failure is wrapped as KindInternal (no
// config-internal detail leaks upward).
package configclient

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Compile-time proof the adapter satisfies the consumer-defined seam.
var _ service.GroupReader = (*Client)(nil)

// Client checks group references against config by RPC. It depends only on the
// generated Connect client.
type Client struct {
	rpc configv1connect.ConfigServiceClient
}

// New builds a Client targeting the config base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors grades).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: configv1connect.NewConfigServiceClient(httpClient, baseURL)}
}

// ByID reports whether a group exists. Attendance needs existence only, so the
// group itself is deliberately discarded — nothing about a group is copied into
// this context. A missing group is glerrors.ErrNotFound.
func (c *Client) ByID(ctx context.Context, id string) error {
	if _, err := c.rpc.GetGroup(ctx, connect.NewRequest(&configv1.GetGroupRequest{Id: id})); err != nil {
		return mapConfigErr(err, "group")
	}
	return nil
}

// mapConfigErr translates a Connect transport error to the seam's error
// vocabulary: NotFound → the shared glerrors.ErrNotFound sentinel; anything else
// → KindInternal wrapping the cause (the caller never sees config-internal detail
// beyond the wrapped error's own message).
func mapConfigErr(err error, what string) error {
	if connect.CodeOf(err) == connect.CodeNotFound {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "config "+what+" lookup failed", err)
}
