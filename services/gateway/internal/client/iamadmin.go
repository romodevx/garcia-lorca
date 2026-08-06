package client

import (
	"context"

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
// unspecified wire enum, which iam rejects as invalid.
func (c *AuthClient) SetUserStatus(ctx context.Context, userID, status string) (User, error) {
	res, err := c.rpc.SetUserStatus(ctx, connect.NewRequest(&iamv1.SetUserStatusRequest{
		UserId: userID,
		Status: userStatusFromName(status),
	}))
	if err != nil {
		return User{}, err
	}
	return toUser(res.Msg.GetUser()), nil
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
}

func userStatusName(s iamv1.UserStatus) string { return userStatusNames[s] }

func userStatusFromName(name string) iamv1.UserStatus {
	for status, n := range userStatusNames {
		if n == name {
			return status
		}
	}
	return iamv1.UserStatus_USER_STATUS_UNSPECIFIED
}
