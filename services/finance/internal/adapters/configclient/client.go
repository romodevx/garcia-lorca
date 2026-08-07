// Package configclient implements the finance CycleReader port over a Connect
// client to the config (academic catalog) bounded context. Every charge is
// stamped with the school cycle it belongs to, so the ledger validates that
// reference by RPC while staying testable with a fake, and this adapter is the
// only place in finance the generated config proto types are touched.
//
// Error mapping mirrors the port contract: a config NotFound (the cycle does not
// exist) becomes glerrors.ErrNotFound so the use case can turn it into a
// KindInvalid ("cycle not found"); any other transport failure is wrapped as
// KindInternal (no config-internal detail leaks upward).
package configclient

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Compile-time proof the adapter satisfies the consumer-defined port.
var _ ports.CycleReader = (*Client)(nil)

// Client reads school cycles from config by RPC. It depends only on the
// generated Connect client and translates to the small, finance-owned CycleRef
// view the port speaks in.
type Client struct {
	rpc configv1connect.ConfigServiceClient
}

// New builds a Client targeting the config base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors the grades config
// client).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: configv1connect.NewConfigServiceClient(httpClient, baseURL)}
}

// ByID fetches a school cycle by id and projects it to the finance-owned view —
// its identity is all the ledger needs to stamp a charge. A missing cycle is
// glerrors.ErrNotFound.
func (c *Client) ByID(ctx context.Context, id string) (ports.CycleRef, error) {
	res, err := c.rpc.GetCycle(ctx, connect.NewRequest(&configv1.GetCycleRequest{Id: id}))
	if err != nil {
		return ports.CycleRef{}, mapConfigErr(err, "cycle")
	}
	return ports.CycleRef{ID: res.Msg.GetCycle().GetId()}, nil
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
