package grpcadapter_test

import (
	"context"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/adapters/jwt"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// ISSUE-CC-117 at the wire: SetUserStatus reads its caller from the access
// token, like DeleteUser, so the self-suspension guard cannot be talked out of
// it by a request field.

const adminActorID = "0b8a1c0e-0000-4000-8000-0000000000ad"

// actorToken registers an active account with id as one more user the fake can
// see, and mints a real access token for it with the handlers' signing key.
func actorToken(t *testing.T, users *fakeUsers, id string) string {
	t.Helper()
	email, _ := domain.NewEmail("admin-" + strings.ToLower(id[len(id)-2:]) + "@garcialorca.mx")
	role, _ := domain.NewRole("r-admin", "admin", []string{"users:write"})
	actor, err := domain.NewUser(id, "t1", email, "Admin", domain.StatusActive, false, []domain.Role{role})
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if users.actors == nil {
		users.actors = map[string]domain.User{}
	}
	users.actors[id] = actor
	tokens := jwt.NewHMAC([]byte(strings.Repeat("gl-key-", 6)), "garcia-lorca", time.Hour, time.Now)
	access, err := tokens.IssueAccessToken(context.Background(), domain.NewAccessClaims(actor, time.Now(), 15*time.Minute))
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	return access
}

func TestSetUserStatusCodes(t *testing.T) {
	cases := []struct {
		name   string
		token  func(self, admin string) string
		target string
		status iamv1.UserStatus
		forced error
		want   connect.Code
	}{
		{"no valid token", func(string, string) string { return "garbage" }, selfID, iamv1.UserStatus_USER_STATUS_SUSPENDED, nil, connect.CodeUnauthenticated},
		{"no token at all", func(string, string) string { return "" }, selfID, iamv1.UserStatus_USER_STATUS_ACTIVE, nil, connect.CodeUnauthenticated},
		{"caller suspends themself", func(self, _ string) string { return self }, selfID, iamv1.UserStatus_USER_STATUS_SUSPENDED, nil, connect.CodePermissionDenied},
		{"caller suspends themself in capitals", func(self, _ string) string { return self }, strings.ToUpper(selfID), iamv1.UserStatus_USER_STATUS_SUSPENDED, nil, connect.CodePermissionDenied},
		{"the last active owner", func(_, admin string) string { return admin }, selfID, iamv1.UserStatus_USER_STATUS_SUSPENDED, domain.ErrLastOwner, connect.CodeAlreadyExists},
		{"malformed target id", func(_, admin string) string { return admin }, "not-a-uuid", iamv1.UserStatus_USER_STATUS_SUSPENDED, nil, connect.CodeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, users, self := statusHandler(t)
			admin := actorToken(t, users, adminActorID)
			users.statusErr = tc.forced

			_, err := h.SetUserStatus(context.Background(), connect.NewRequest(&iamv1.SetUserStatusRequest{
				AccessToken: tc.token(self, admin), UserId: tc.target, Status: tc.status,
			}))

			assertCode(t, err, tc.want)
			if len(users.statusSet) != 0 {
				t.Errorf("a refused change wrote %v", users.statusSet)
			}
		})
	}
}

func TestSetUserStatusSuspendsSomebodyElse(t *testing.T) {
	h, users, _ := statusHandler(t)
	admin := actorToken(t, users, adminActorID)

	res, err := h.SetUserStatus(context.Background(), connect.NewRequest(&iamv1.SetUserStatusRequest{
		AccessToken: admin, UserId: strings.ToUpper(selfID), Status: iamv1.UserStatus_USER_STATUS_SUSPENDED,
	}))
	if err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if got := users.statusSet[selfID]; got != domain.StatusSuspended {
		t.Errorf("status = %v, want the canonical id suspended (got %v)", got, users.statusSet)
	}
	if res.Msg.GetUser().GetStatus() != iamv1.UserStatus_USER_STATUS_SUSPENDED {
		t.Errorf("response user = %+v, want SUSPENDED", res.Msg.GetUser())
	}
}

// ISSUE-CC-116 at the wire: the token of an account suspended after it was
// minted stops validating at once.
func TestValidateTokenRefusesAnAccountSuspendedAfterLogin(t *testing.T) {
	h, users, self := statusHandler(t)
	admin := actorToken(t, users, adminActorID)
	if _, err := h.SetUserStatus(context.Background(), connect.NewRequest(&iamv1.SetUserStatusRequest{
		AccessToken: admin, UserId: selfID, Status: iamv1.UserStatus_USER_STATUS_SUSPENDED,
	})); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}

	_, err := h.ValidateToken(context.Background(), connect.NewRequest(&iamv1.ValidateTokenRequest{AccessToken: self}))

	assertCode(t, err, connect.CodeUnauthenticated)
}

// statusHandler is deleterHandler that also hands back the fake repository.
func statusHandler(t *testing.T) (*grpcadapter.IAMHandler, *fakeUsers, string) {
	t.Helper()
	return deleterHandlerWithUsers(t, "users:write")
}
