package grpcadapter_test

import (
	"context"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/services/iam/internal/adapters/bcrypt"
	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/adapters/jwt"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// T2 — DeleteUser at the wire: the caller comes from the access token, never
// from a request field, and the refusals map to the codes the gateway forwards.

const selfID = "0b8a1c0e-0000-4000-8000-000000000001"

// deleterHandler seeds the caller with the given permissions and logs it in.
func deleterHandler(t *testing.T, perms ...string) (*grpcadapter.IAMHandler, string) {
	t.Helper()
	h, _, access := deleterHandlerWithUsers(t, perms...)
	return h, access
}

// deleterHandlerWithUsers is deleterHandler that also returns the fake
// repository, for the tests that seed more accounts or assert on writes.
func deleterHandlerWithUsers(t *testing.T, perms ...string) (*grpcadapter.IAMHandler, *fakeUsers, string) {
	t.Helper()
	hasher := bcrypt.NewHasher(4)
	hash, _ := hasher.Hash("test-owner-pw")
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Rol", perms)
	user, _ := domain.NewUser(selfID, "t1", email, "Ana", domain.StatusActive, false, []domain.Role{role})
	users := &fakeUsers{user: user, hash: hash}
	roles := &fakeRoles{known: "r1"}
	tokens := jwt.NewHMAC([]byte(strings.Repeat("gl-key-", 6)), "garcia-lorca", time.Hour, time.Now)
	auth := usecase.NewAuthService(users, roles, hasher, tokens, tokens, nil, nil, "t1", 15*time.Minute, time.Now)
	admin := usecase.NewAdminService(users, roles, hasher, nil, nil, nil, "", "t1", func() string { return "id" }, nil)
	h := grpcadapter.NewIAMHandler(auth, admin, usecase.NewHealthService("iam"))
	return h, users, loginAccessToken(t, h)
}

func TestDeleteUserCodes(t *testing.T) {
	cases := []struct {
		name   string
		perms  []string
		token  func(real string) string
		target string
		want   connect.Code
	}{
		{"no valid token", []string{"users:delete"}, func(string) string { return "garbage" }, "0b8a1c0e-0000-4000-8000-000000000002", connect.CodeUnauthenticated},
		{"caller lacks users:delete", []string{"users:write"}, func(r string) string { return r }, "0b8a1c0e-0000-4000-8000-000000000002", connect.CodePermissionDenied},
		{"caller deletes themself", []string{"users:delete"}, func(r string) string { return r }, "0b8a1c0e-0000-4000-8000-000000000001", connect.CodePermissionDenied},
		// Another spelling of the caller's own uuid is still the caller.
		{"caller deletes themself in capitals", []string{"users:delete"}, func(r string) string { return r }, "0B8A1C0E-0000-4000-8000-000000000001", connect.CodePermissionDenied},
		{"unknown target", []string{"users:delete"}, func(r string) string { return r }, "0b8a1c0e-0000-4000-8000-000000000002", connect.CodeNotFound},
		// Validated at the boundary: a malformed id names no account.
		{"malformed target id", []string{"users:delete"}, func(r string) string { return r }, "not-a-uuid", connect.CodeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, access := deleterHandler(t, tc.perms...)
			_, err := h.DeleteUser(context.Background(), connect.NewRequest(&iamv1.DeleteUserRequest{
				AccessToken: tc.token(access), UserId: tc.target,
			}))
			assertCode(t, err, tc.want)
		})
	}
}

func TestDeletedStatusOnTheWire(t *testing.T) {
	email, _ := domain.NewEmail("gone@garcialorca.mx")
	gone, _ := domain.NewUser("u9", "t1", email, "Ex", domain.StatusDeleted, false, nil)
	users := &fakeUsers{user: gone}
	h := handlerOver(t, users, &fakeRoles{})

	res, err := h.GetUser(context.Background(), connect.NewRequest(&iamv1.GetUserRequest{Id: "u9"}))
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if res.Msg.GetUser().GetStatus() != iamv1.UserStatus_USER_STATUS_DELETED {
		t.Errorf("status = %v, want DELETED", res.Msg.GetUser().GetStatus())
	}

	// SetUserStatus is not a road to `deleted`: DeleteUser is, with its guards.
	_, err = h.SetUserStatus(context.Background(), connect.NewRequest(&iamv1.SetUserStatusRequest{
		AccessToken: actorToken(t, users, adminActorID), UserId: "u9", Status: iamv1.UserStatus_USER_STATUS_DELETED,
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}
