package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
)

// User is the gateway's view of an iam identity. It is the edge struct handlers
// depend on, never the generated proto type. Status is a canonical lowercase
// name ("active"/"suspended"); RoleIDs carries the user's assigned roles.
type User struct {
	ID                 string
	Email              string
	DisplayName        string
	Status             string
	MustChangePassword bool
	RoleIDs            []string
}

// Role is the gateway's view of an iam role: a named bundle of "resource:action"
// permission strings.
type Role struct {
	ID          string
	Name        string
	Permissions []string
}

// InviteUser provisions an identity with NO password and has the provider mail
// its owner the link that sets the first one. Preferred over CreateUser: nobody
// but the account's owner ever knows the password, and a mistyped address never
// becomes a working account.
//
// An address that already has an account is a conflict, not a resend — see
// ResendInvitation for that.
func (c *AuthClient) InviteUser(ctx context.Context, email, displayName string, roleIDs []string) (User, Invitation, error) {
	res, err := c.rpc.InviteUser(ctx, connect.NewRequest(&iamv1.InviteUserRequest{
		Email:       email,
		DisplayName: displayName,
		RoleIds:     roleIDs,
	}))
	if err != nil {
		return User{}, Invitation{}, err
	}
	return toUser(res.Msg.GetUser()), toInvitation(res.Msg.GetInvitation()), nil
}

// InviteGuardian invites a family contact into the guardian role. It takes no
// role: iam chooses it, and this is the only road into it (InviteUser refuses
// the guardian role, ISSUE-CC-115).
func (c *AuthClient) InviteGuardian(ctx context.Context, email, displayName string) (User, Invitation, error) {
	res, err := c.rpc.InviteGuardian(ctx, connect.NewRequest(&iamv1.InviteGuardianRequest{
		Email:       email,
		DisplayName: displayName,
	}))
	if err != nil {
		return User{}, Invitation{}, err
	}
	return toUser(res.Msg.GetUser()), toInvitation(res.Msg.GetInvitation()), nil
}

// Invitation is what came of issuing the link: whether it was delivered, why
// not, and — when it was not — the link itself, so the caller has something to
// hand over.
type Invitation struct {
	URL       string
	ExpiresAt time.Time
	Sent      bool
	SendError string
	Via       string
}

func toInvitation(p *iamv1.Invitation) Invitation {
	if p == nil {
		return Invitation{}
	}
	return Invitation{
		URL:       p.GetUrl(),
		ExpiresAt: p.GetExpiresAt().AsTime(),
		Sent:      p.GetSent(),
		SendError: p.GetSendError(),
		Via:       p.GetVia(),
	}
}

// ResendInvitation mints a fresh link for somebody who has not walked through
// the door yet. iam refuses on an account that is already active.
func (c *AuthClient) ResendInvitation(ctx context.Context, userID string) (Invitation, error) {
	res, err := c.rpc.ResendInvitation(ctx, connect.NewRequest(&iamv1.ResendInvitationRequest{UserId: userID}))
	if err != nil {
		return Invitation{}, err
	}
	return toInvitation(res.Msg.GetInvitation()), nil
}

// AcceptInvitation redeems a token and sets the password its owner chose.
func (c *AuthClient) AcceptInvitation(ctx context.Context, token, password string) error {
	_, err := c.rpc.AcceptInvitation(ctx, connect.NewRequest(&iamv1.AcceptInvitationRequest{
		Token: token, Password: password,
	}))
	return err
}

// RequestPasswordReset asks iam to mail a reset link. It reports nothing about
// whether the address exists, and neither does this wrapper.
func (c *AuthClient) RequestPasswordReset(ctx context.Context, email string) error {
	_, err := c.rpc.RequestPasswordReset(ctx, connect.NewRequest(&iamv1.RequestPasswordResetRequest{Email: email}))
	return err
}

// CreateUser provisions a new identity with a temporary password (the user must
// rotate it at first login) and assigns the given roles. Connect error codes
// propagate (AlreadyExists on a duplicate email, InvalidArgument on a bad field)
// so the handler can map them.
func (c *AuthClient) CreateUser(ctx context.Context, email, displayName, tempPassword string, roleIDs []string) (User, error) {
	res, err := c.rpc.CreateUser(ctx, connect.NewRequest(&iamv1.CreateUserRequest{
		Email:        email,
		DisplayName:  displayName,
		TempPassword: tempPassword,
		RoleIds:      roleIDs,
	}))
	if err != nil {
		return User{}, err
	}
	return toUser(res.Msg.GetUser()), nil
}

// GetUser fetches a single identity by id. NotFound propagates so callers (e.g.
// the teacher-link validation) can map a missing user to a 404.
func (c *AuthClient) GetUser(ctx context.Context, id string) (User, error) {
	res, err := c.rpc.GetUser(ctx, connect.NewRequest(&iamv1.GetUserRequest{Id: id}))
	if err != nil {
		return User{}, err
	}
	return toUser(res.Msg.GetUser()), nil
}

// ListUsers lists every identity in the tenant.
func (c *AuthClient) ListUsers(ctx context.Context) ([]User, error) {
	res, err := c.rpc.ListUsers(ctx, connect.NewRequest(&iamv1.ListUsersRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(res.Msg.GetUsers()))
	for _, u := range res.Msg.GetUsers() {
		out = append(out, toUser(u))
	}
	return out, nil
}

// SetUserStatus suspends or reactivates an identity. status is the canonical
// lowercase name ("active"/"suspended"); an unknown value maps to the
// unspecified wire enum, which iam rejects as invalid. accessToken is the
// CALLER's own: iam's self-suspension guard reads the verified subject from it.
func (c *AuthClient) SetUserStatus(ctx context.Context, accessToken, userID, status string) (User, error) {
	res, err := c.rpc.SetUserStatus(ctx, connect.NewRequest(&iamv1.SetUserStatusRequest{
		AccessToken: accessToken,
		UserId:      userID,
		Status:      userStatusFromName(status),
	}))
	if err != nil {
		return User{}, err
	}
	return toUser(res.Msg.GetUser()), nil
}

// DeleteUser soft-deletes an account (T2). accessToken is the CALLER's own:
// iam derives who is asking from it — the self-deletion guard and the
// users:delete check read the verified subject, never a request field.
func (c *AuthClient) DeleteUser(ctx context.Context, accessToken, userID string) error {
	_, err := c.rpc.DeleteUser(ctx, connect.NewRequest(&iamv1.DeleteUserRequest{
		AccessToken: accessToken,
		UserId:      userID,
	}))
	return err
}

// AssignRole grants a role to a user. Used by the handler to add a role while
// editing an existing user's role set (paired with RevokeRole).
func (c *AuthClient) AssignRole(ctx context.Context, userID, roleID string) error {
	_, err := c.rpc.AssignRole(ctx, connect.NewRequest(&iamv1.AssignRoleRequest{
		UserId: userID,
		RoleId: roleID,
	}))
	return err
}

// RevokeRole removes a role from a user. Used by the handler to drop a role while
// editing an existing user's role set (paired with AssignRole).
func (c *AuthClient) RevokeRole(ctx context.Context, userID, roleID string) error {
	_, err := c.rpc.RevokeRole(ctx, connect.NewRequest(&iamv1.RevokeRoleRequest{
		UserId: userID,
		RoleId: roleID,
	}))
	return err
}

// CreateRole defines a new role from a permission catalog. AlreadyExists
// propagates on a duplicate name; InvalidArgument on a malformed permission.
func (c *AuthClient) CreateRole(ctx context.Context, name string, perms []string) (Role, error) {
	res, err := c.rpc.CreateRole(ctx, connect.NewRequest(&iamv1.CreateRoleRequest{
		Name:        name,
		Permissions: perms,
	}))
	if err != nil {
		return Role{}, err
	}
	return toRole(res.Msg.GetRole()), nil
}

// GetRole fetches a single role by id.
func (c *AuthClient) GetRole(ctx context.Context, id string) (Role, error) {
	res, err := c.rpc.GetRole(ctx, connect.NewRequest(&iamv1.GetRoleRequest{Id: id}))
	if err != nil {
		return Role{}, err
	}
	return toRole(res.Msg.GetRole()), nil
}

// ListRoles lists every role in the tenant.
func (c *AuthClient) ListRoles(ctx context.Context) ([]Role, error) {
	res, err := c.rpc.ListRoles(ctx, connect.NewRequest(&iamv1.ListRolesRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]Role, 0, len(res.Msg.GetRoles()))
	for _, r := range res.Msg.GetRoles() {
		out = append(out, toRole(r))
	}
	return out, nil
}

// SetRolePermissions replaces a role's permission set. InvalidArgument
// propagates on a malformed permission string.
func (c *AuthClient) SetRolePermissions(ctx context.Context, roleID string, perms []string) (Role, error) {
	res, err := c.rpc.SetRolePermissions(ctx, connect.NewRequest(&iamv1.SetRolePermissionsRequest{
		RoleId:      roleID,
		Permissions: perms,
	}))
	if err != nil {
		return Role{}, err
	}
	return toRole(res.Msg.GetRole()), nil
}

// --- proto ↔ edge mappers -------------------------------------------------

func toUser(u *iamv1.User) User {
	return User{
		ID:                 u.GetId(),
		Email:              u.GetEmail(),
		DisplayName:        u.GetDisplayName(),
		Status:             userStatusName(u.GetStatus()),
		MustChangePassword: u.GetMustChangePassword(),
		RoleIDs:            u.GetRoleIds(),
	}
}

func toRole(r *iamv1.Role) Role {
	return Role{
		ID:          r.GetId(),
		Name:        r.GetName(),
		Permissions: r.GetPermissions(),
	}
}

// --- enum ↔ string --------------------------------------------------------

var userStatusNames = map[iamv1.UserStatus]string{
	iamv1.UserStatus_USER_STATUS_ACTIVE:    "active",
	iamv1.UserStatus_USER_STATUS_SUSPENDED: "suspended",
	iamv1.UserStatus_USER_STATUS_INVITED:   "invited",
	iamv1.UserStatus_USER_STATUS_DELETED:   "deleted",
}

// userStatusName never returns the empty string, and that is the whole reason
// this is a function and not a bare map read.
//
// It used to be `return userStatusNames[s]`, so a status with no entry shipped
// as {"status":""} — no error, no log, nothing. The web then compared it to
// "active", found it different, and drew every one of those users as
// "Suspendido". A silent wrong answer is worse than a loud unknown one: the
// screen would have looked plausible and been lying.
func userStatusName(s iamv1.UserStatus) string {
	if name, ok := userStatusNames[s]; ok {
		return name
	}
	return "unknown"
}

func userStatusFromName(name string) iamv1.UserStatus {
	for status, n := range userStatusNames {
		if n == name {
			return status
		}
	}
	return iamv1.UserStatus_USER_STATUS_UNSPECIFIED
}
