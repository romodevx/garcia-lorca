package grpcadapter_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// ISSUE-CC-115 at the wire: the guardian role is refused on InviteUser and
// reachable only through InviteGuardian, which picks the role itself.

func TestInviteUserRefusesTheGuardianRole(t *testing.T) {
	users := &fakeUsers{}
	h := handlerWithMailer(t, users, &fakeRoles{known: domain.GuardianRoleID}, &recordingProvisioner{}, &recordingMailer{})

	_, err := h.InviteUser(context.Background(), connect.NewRequest(&iamv1.InviteUserRequest{
		Email: "mama@garcialorca.mx", DisplayName: "Mamá", RoleIds: []string{domain.GuardianRoleID},
	}))

	assertCode(t, err, connect.CodeInvalidArgument)
	if users.created != nil {
		t.Errorf("a refused invitation created %+v", users.created)
	}
}

func TestInviteGuardianAssignsTheGuardianRole(t *testing.T) {
	users := &fakeUsers{}
	h := handlerWithMailer(t, users, &fakeRoles{known: domain.GuardianRoleID}, &recordingProvisioner{}, &recordingMailer{})

	res, err := h.InviteGuardian(context.Background(), connect.NewRequest(&iamv1.InviteGuardianRequest{
		Email: "mama@garcialorca.mx", DisplayName: "Mamá",
	}))
	if err != nil {
		t.Fatalf("InviteGuardian: %v", err)
	}

	id := res.Msg.GetUser().GetId()
	if roles := users.rolesSet[id]; len(roles) != 1 || roles[0] != domain.GuardianRoleID {
		t.Errorf("roles = %v, want exactly the guardian role", roles)
	}
	if res.Msg.GetUser().GetStatus() != iamv1.UserStatus_USER_STATUS_INVITED {
		t.Errorf("status = %v, want INVITED", res.Msg.GetUser().GetStatus())
	}
	if res.Msg.GetInvitation().GetExpiresAt() == nil {
		t.Error("no invitation came back")
	}
}
