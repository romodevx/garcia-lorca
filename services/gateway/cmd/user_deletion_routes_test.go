package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
)

// T2 through the real mux: module_iam guards it, users:delete (the owner's
// alone) gates it, and the caller's own token is what reaches iam.

const routeUser = "cccccccc-0000-0000-0000-0000000000c1"

// DeleteUser on the stub answers 204 only when the gateway forwarded the
// caller's token: iam derives who is asking from it, never from the request.
func (stubIAM) DeleteUser(_ context.Context, req *connect.Request[iamv1.DeleteUserRequest]) (*connect.Response[iamv1.DeleteUserResponse], error) {
	if req.Msg.GetAccessToken() != "user-deleter" || req.Msg.GetUserId() != routeUser {
		return nil, connect.NewError(connect.CodeInvalidArgument, errStr("token or id not forwarded"))
	}
	return connect.NewResponse(&iamv1.DeleteUserResponse{}), nil
}

func TestUserDeletionRoute(t *testing.T) {
	path := "/api/v1/users/" + routeUser
	cases := []struct {
		name  string
		flags map[string]bool
		token string
		want  int
	}{
		{"the owner deletes", allModulesOn, "user-deleter", http.StatusNoContent},
		{"users:write is not users:delete", allModulesOn, "users-writer", http.StatusForbidden},
		{"a teacher is refused", allModulesOn, "maestro", http.StatusForbidden},
		{"module_iam off hides it", map[string]bool{"module_students": true}, "user-deleter", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			res := doGateway(t, gw, http.MethodDelete, path, tc.token, "")
			assert.Equal(t, tc.want, res.StatusCode)
		})
	}
}
