package client_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
)

// T2 — DeleteUser forwards the caller's token and the target id, and a deleted
// account travels as "deleted", never as "unknown".

func (stubIAM) DeleteUser(_ context.Context, req *connect.Request[iamv1.DeleteUserRequest]) (*connect.Response[iamv1.DeleteUserResponse], error) {
	if req.Msg.GetAccessToken() != "caller-token" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errString("invalid access token"))
	}
	if req.Msg.GetUserId() != "usr-1" {
		return nil, connect.NewError(connect.CodeNotFound, errString("user not found"))
	}
	return connect.NewResponse(&iamv1.DeleteUserResponse{}), nil
}

func TestAuthClientDeleteUser(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	if err := c.DeleteUser(context.Background(), "caller-token", "usr-1"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if err := c.DeleteUser(context.Background(), "caller-token", "ghost"); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
	}
	if err := c.DeleteUser(context.Background(), "someone-else", "usr-1"); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want Unauthenticated — the token must be forwarded as given", connect.CodeOf(err))
	}
}

func TestDeletedStatusHasAName(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	u, err := c.SetUserStatus(context.Background(), "usr-1", "deleted")
	// The stub echoes the status it received; "deleted" is not a status a
	// caller may SET (iam refuses it), but it must have a wire name so a
	// deleted row reads as deleted rather than "unknown".
	if err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if u.Status != "deleted" {
		t.Errorf("status = %q, want deleted", u.Status)
	}
}
