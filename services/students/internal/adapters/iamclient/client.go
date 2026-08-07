// Package iamclient implements the students UserReader port over a Connect client
// to the iam bounded context. It is the cross-service seam of the guardian link:
// the use case validates that a guardian_user_id is a real user holding the
// guardian role, while staying testable with a fake, and this adapter is the only
// place the generated iam proto types are touched.
//
// iam's User carries role IDS, not names, so resolving the port's role NAMES takes
// a second call (ListRoles, the tenant's small role catalog) which is skipped
// entirely for a user with no roles. Guardian links are a rare admin write, so the
// extra round-trip buys a stable, name-based contract at no meaningful cost.
//
// Error mapping mirrors the port contract: an iam NotFound (the user does not
// exist) becomes glerrors.ErrNotFound so the use case can turn it into a
// KindInvalid ("guardian user not found"); any other transport failure is wrapped
// as KindInternal (no iam-internal detail leaks upward).
package iamclient

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/iam/v1/iamv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter satisfies the consumer-defined port.
var _ ports.UserReader = (*Client)(nil)

// Client resolves iam users by RPC. It depends only on the generated Connect
// client and translates to the small, students-owned UserRef view the port speaks
// in.
type Client struct {
	rpc iamv1connect.IamServiceClient
}

// New builds a Client targeting the iam base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors configclient).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: iamv1connect.NewIamServiceClient(httpClient, baseURL)}
}

// ByID fetches a user by id and projects it to the students-owned view (its
// identity, account status, and the names of the roles it holds). A missing user
// is glerrors.ErrNotFound.
func (c *Client) ByID(ctx context.Context, userID string) (ports.UserRef, error) {
	res, err := c.rpc.GetUser(ctx, connect.NewRequest(&iamv1.GetUserRequest{Id: userID}))
	if err != nil {
		return ports.UserRef{}, mapIamErr(err, "user")
	}
	u := res.Msg.GetUser()

	roles, err := c.roleNames(ctx, u.GetRoleIds())
	if err != nil {
		return ports.UserRef{}, err
	}
	return ports.UserRef{
		ID:     u.GetId(),
		Status: u.GetStatus().String(),
		Roles:  roles,
	}, nil
}

// roleNames resolves role ids to their names against iam's role catalog,
// preserving the user's role order. A role id the catalog does not know is
// dropped: the port promises names, and an unresolved uuid would silently fail
// every name comparison downstream.
func (c *Client) roleNames(ctx context.Context, roleIDs []string) ([]string, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	res, err := c.rpc.ListRoles(ctx, connect.NewRequest(&iamv1.ListRolesRequest{}))
	if err != nil {
		return nil, mapIamErr(err, "role")
	}
	byID := make(map[string]string, len(res.Msg.GetRoles()))
	for _, r := range res.Msg.GetRoles() {
		byID[r.GetId()] = r.GetName()
	}
	names := make([]string, 0, len(roleIDs))
	for _, id := range roleIDs {
		if name, ok := byID[id]; ok {
			names = append(names, name)
		}
	}
	return names, nil
}

// mapIamErr translates a Connect transport error to the port's error vocabulary:
// NotFound → the shared glerrors.ErrNotFound sentinel; anything else →
// KindInternal wrapping the cause (the caller never sees iam-internal detail
// beyond the wrapped error's own message).
func mapIamErr(err error, what string) error {
	if connect.CodeOf(err) == connect.CodeNotFound {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "iam "+what+" lookup failed", err)
}
