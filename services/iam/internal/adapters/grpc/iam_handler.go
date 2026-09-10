package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/iam/v1/iamv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// Compile-time proof the adapter implements the generated Connect handler.
var _ iamv1connect.IamServiceHandler = (*IAMHandler)(nil)

// IAMHandler adapts the iam use cases to the generated garcialorca.iam.v1
// Connect service. It only translates: proto messages ↔ domain/usecase calls,
// and domain error Kinds ↔ Connect codes. All logic lives in usecase/domain.
type IAMHandler struct {
	auth   *usecase.AuthService
	admin  *usecase.AdminService
	health *usecase.HealthService
}

// NewIAMHandler wires the handler to the auth + admin + health use cases.
func NewIAMHandler(auth *usecase.AuthService, admin *usecase.AdminService, health *usecase.HealthService) *IAMHandler {
	return &IAMHandler{auth: auth, admin: admin, health: health}
}

func (h *IAMHandler) Health(ctx context.Context, _ *connect.Request[iamv1.HealthRequest]) (*connect.Response[iamv1.HealthResponse], error) {
	status, err := h.health.Ping(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.HealthResponse{Status: status.Status}), nil
}

func (h *IAMHandler) Login(ctx context.Context, req *connect.Request[iamv1.LoginRequest]) (*connect.Response[iamv1.LoginResponse], error) {
	res, err := h.auth.AuthenticateAs(ctx, req.Msg.GetEmail(), req.Msg.GetPassword(),
		sessionKindOf(req.Msg.GetSessionKind()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.LoginResponse{
		AccessToken:        res.AccessToken,
		RefreshToken:       res.RefreshToken,
		ExpiresIn:          res.ExpiresIn,
		User:               toProtoUser(res.User),
		MustChangePassword: res.MustChangePassword,
	}), nil
}

// LoginWithIDToken exchanges a provider ID token for OUR session pair.
//
// The session KIND comes from the request, which is set by the ROUTE the call
// arrived on at the edge, never by anything the person signing in can type —
// the same rule Login follows. Otherwise a staff sign-in could ask for the
// day-long family session.
func (h *IAMHandler) LoginWithIDToken(ctx context.Context, req *connect.Request[iamv1.LoginWithIDTokenRequest]) (*connect.Response[iamv1.LoginWithIDTokenResponse], error) {
	res, err := h.auth.AuthenticateWithIDToken(ctx, req.Msg.GetIdToken(),
		sessionKindOf(req.Msg.GetSessionKind()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.LoginWithIDTokenResponse{
		AccessToken:        res.AccessToken,
		RefreshToken:       res.RefreshToken,
		ExpiresIn:          res.ExpiresIn,
		User:               toProtoUser(res.User),
		MustChangePassword: res.MustChangePassword,
	}), nil
}

func (h *IAMHandler) RefreshToken(ctx context.Context, req *connect.Request[iamv1.RefreshTokenRequest]) (*connect.Response[iamv1.RefreshTokenResponse], error) {
	res, err := h.auth.RefreshToken(ctx, req.Msg.GetRefreshToken())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.RefreshTokenResponse{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		ExpiresIn:    res.ExpiresIn,
	}), nil
}

func (h *IAMHandler) ValidateToken(ctx context.Context, req *connect.Request[iamv1.ValidateTokenRequest]) (*connect.Response[iamv1.ValidateTokenResponse], error) {
	claims, err := h.auth.ValidateToken(ctx, req.Msg.GetAccessToken())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.ValidateTokenResponse{Claims: toProtoClaims(claims)}), nil
}

func (h *IAMHandler) CheckPermission(ctx context.Context, req *connect.Request[iamv1.CheckPermissionRequest]) (*connect.Response[iamv1.CheckPermissionResponse], error) {
	allowed, err := h.auth.CheckPermission(ctx, req.Msg.GetAccessToken(), req.Msg.GetPermission())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.CheckPermissionResponse{Allowed: allowed}), nil
}

func (h *IAMHandler) AssignRole(ctx context.Context, req *connect.Request[iamv1.AssignRoleRequest]) (*connect.Response[iamv1.AssignRoleResponse], error) {
	if err := h.auth.AssignRole(ctx, req.Msg.GetUserId(), req.Msg.GetRoleId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.AssignRoleResponse{}), nil
}

func (h *IAMHandler) RevokeRole(ctx context.Context, req *connect.Request[iamv1.RevokeRoleRequest]) (*connect.Response[iamv1.RevokeRoleResponse], error) {
	if err := h.auth.RevokeRole(ctx, req.Msg.GetUserId(), req.Msg.GetRoleId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.RevokeRoleResponse{}), nil
}

func (h *IAMHandler) ChangePassword(ctx context.Context, req *connect.Request[iamv1.ChangePasswordRequest]) (*connect.Response[iamv1.ChangePasswordResponse], error) {
	if err := h.auth.ChangePassword(ctx, req.Msg.GetAccessToken(), req.Msg.GetCurrentPassword(), req.Msg.GetNewPassword()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.ChangePasswordResponse{}), nil
}

// --- admin surface (user/role CRUD) ----------------------------------------

// InviteUser provisions a principal with no password and mails them the link
// that sets the first one. Prefer it over CreateUser: nobody else ever knows
// the person's password, and a mistyped address fails visibly instead of
// becoming a working account.
func (h *IAMHandler) InviteUser(ctx context.Context, req *connect.Request[iamv1.InviteUserRequest]) (*connect.Response[iamv1.InviteUserResponse], error) {
	out, err := h.admin.InviteUser(ctx, req.Msg.GetEmail(), req.Msg.GetDisplayName(), req.Msg.GetRoleIds())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.InviteUserResponse{
		User:       toProtoUser(out.User),
		Invitation: toProtoInvitation(out),
	}), nil
}

// ResendInvitation mints a fresh link for somebody who has not walked through
// the door yet, and refuses on an account that is already active.
func (h *IAMHandler) ResendInvitation(ctx context.Context, req *connect.Request[iamv1.ResendInvitationRequest]) (*connect.Response[iamv1.ResendInvitationResponse], error) {
	out, err := h.admin.ResendInvitation(ctx, req.Msg.GetUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.ResendInvitationResponse{Invitation: toProtoInvitation(out)}), nil
}

// AcceptInvitation redeems a token. Unauthenticated by design: the token IS the
// credential, which is why it is single-use, short-lived and 256 bits wide.
func (h *IAMHandler) AcceptInvitation(ctx context.Context, req *connect.Request[iamv1.AcceptInvitationRequest]) (*connect.Response[iamv1.AcceptInvitationResponse], error) {
	if err := h.admin.AcceptInvitation(ctx, req.Msg.GetToken(), req.Msg.GetPassword()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.AcceptInvitationResponse{}), nil
}

// RequestPasswordReset always succeeds, whatever happened. Any difference
// between "that address has an account" and "it does not" would turn this
// unauthenticated endpoint into a directory read.
func (h *IAMHandler) RequestPasswordReset(ctx context.Context, req *connect.Request[iamv1.RequestPasswordResetRequest]) (*connect.Response[iamv1.RequestPasswordResetResponse], error) {
	if err := h.admin.RequestPasswordReset(ctx, req.Msg.GetEmail()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.RequestPasswordResetResponse{}), nil
}

// toProtoInvitation carries the delivery outcome to the caller. The URL rides
// along so an administrator can hand it over when no mail went out.
func toProtoInvitation(inv usecase.Invitation) *iamv1.Invitation {
	return &iamv1.Invitation{
		Url:       inv.URL,
		ExpiresAt: timestamppb.New(inv.ExpiresAt),
		Sent:      inv.Sent,
		SendError: inv.SendError,
		Via:       inv.Via,
	}
}

func (h *IAMHandler) CreateUser(ctx context.Context, req *connect.Request[iamv1.CreateUserRequest]) (*connect.Response[iamv1.CreateUserResponse], error) {
	user, err := h.admin.CreateUser(ctx, req.Msg.GetEmail(), req.Msg.GetDisplayName(), req.Msg.GetTempPassword(), req.Msg.GetRoleIds())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.CreateUserResponse{User: toProtoUser(user)}), nil
}

func (h *IAMHandler) GetUser(ctx context.Context, req *connect.Request[iamv1.GetUserRequest]) (*connect.Response[iamv1.GetUserResponse], error) {
	user, err := h.admin.GetUser(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.GetUserResponse{User: toProtoUser(user)}), nil
}

func (h *IAMHandler) ListUsers(ctx context.Context, _ *connect.Request[iamv1.ListUsersRequest]) (*connect.Response[iamv1.ListUsersResponse], error) {
	users, err := h.admin.ListUsers(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*iamv1.User, 0, len(users))
	for _, u := range users {
		out = append(out, toProtoUser(u))
	}
	return connect.NewResponse(&iamv1.ListUsersResponse{Users: out}), nil
}

func (h *IAMHandler) SetUserStatus(ctx context.Context, req *connect.Request[iamv1.SetUserStatusRequest]) (*connect.Response[iamv1.SetUserStatusResponse], error) {
	status, err := fromProtoStatus(req.Msg.GetStatus())
	if err != nil {
		return nil, connectErr(err)
	}
	if err := h.admin.SetUserStatus(ctx, req.Msg.GetUserId(), status); err != nil {
		return nil, connectErr(err)
	}
	user, err := h.admin.GetUser(ctx, req.Msg.GetUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.SetUserStatusResponse{User: toProtoUser(user)}), nil
}

func (h *IAMHandler) CreateRole(ctx context.Context, req *connect.Request[iamv1.CreateRoleRequest]) (*connect.Response[iamv1.CreateRoleResponse], error) {
	role, err := h.admin.CreateRole(ctx, req.Msg.GetName(), req.Msg.GetPermissions())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.CreateRoleResponse{Role: toProtoRole(role)}), nil
}

func (h *IAMHandler) GetRole(ctx context.Context, req *connect.Request[iamv1.GetRoleRequest]) (*connect.Response[iamv1.GetRoleResponse], error) {
	role, err := h.admin.GetRole(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.GetRoleResponse{Role: toProtoRole(role)}), nil
}

func (h *IAMHandler) ListRoles(ctx context.Context, _ *connect.Request[iamv1.ListRolesRequest]) (*connect.Response[iamv1.ListRolesResponse], error) {
	roles, err := h.admin.ListRoles(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*iamv1.Role, 0, len(roles))
	for _, r := range roles {
		out = append(out, toProtoRole(r))
	}
	return connect.NewResponse(&iamv1.ListRolesResponse{Roles: out}), nil
}

func (h *IAMHandler) SetRolePermissions(ctx context.Context, req *connect.Request[iamv1.SetRolePermissionsRequest]) (*connect.Response[iamv1.SetRolePermissionsResponse], error) {
	if err := h.admin.SetRolePermissions(ctx, req.Msg.GetRoleId(), req.Msg.GetPermissions()); err != nil {
		return nil, connectErr(err)
	}
	role, err := h.admin.GetRole(ctx, req.Msg.GetRoleId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.SetRolePermissionsResponse{Role: toProtoRole(role)}), nil
}

// --- mapping ---------------------------------------------------------------

func toProtoUser(u domain.User) *iamv1.User {
	return &iamv1.User{
		Id:                 u.ID(),
		TenantId:           u.TenantID(),
		Email:              u.Email().String(),
		DisplayName:        u.DisplayName(),
		Status:             toProtoStatus(u.Status()),
		MustChangePassword: u.MustChangePassword(),
		RoleIds:            u.RoleIDs(),
	}
}

func toProtoRole(r domain.Role) *iamv1.Role {
	perms := r.Permissions()
	raw := make([]string, 0, len(perms))
	for _, p := range perms {
		raw = append(raw, p.String())
	}
	return &iamv1.Role{
		Id:          r.ID(),
		Name:        r.Name(),
		Permissions: raw,
	}
}

func toProtoClaims(c domain.Claims) *iamv1.Claims {
	return &iamv1.Claims{
		Subject:            c.Subject,
		TenantId:           c.TenantID,
		Email:              c.Email,
		Permissions:        c.Permissions,
		MustChangePassword: c.MustChangePassword,
		ExpiresAt:          timestamppb.New(c.ExpiresAt),
	}
}

func toProtoStatus(s domain.Status) iamv1.UserStatus {
	switch s {
	case domain.StatusActive:
		return iamv1.UserStatus_USER_STATUS_ACTIVE
	case domain.StatusSuspended:
		return iamv1.UserStatus_USER_STATUS_SUSPENDED
	case domain.StatusInvited:
		return iamv1.UserStatus_USER_STATUS_INVITED
	default:
		return iamv1.UserStatus_USER_STATUS_UNSPECIFIED
	}
}

// fromProtoStatus maps a wire UserStatus to the domain enum, rejecting
// UNSPECIFIED (and any unknown value) as KindInvalid so the boundary never
// forwards an unset status into a lifecycle change.
func fromProtoStatus(s iamv1.UserStatus) (domain.Status, error) {
	switch s {
	case iamv1.UserStatus_USER_STATUS_ACTIVE:
		return domain.StatusActive, nil
	case iamv1.UserStatus_USER_STATUS_SUSPENDED:
		return domain.StatusSuspended, nil
	// INVITED is deliberately absent, and its absence is a guard rather than an
	// oversight. This function feeds SetUserStatus, so accepting it would let
	// anyone holding users:write push a working account into a state that
	// cannot log in and has no way out through the UI. Only accepting an
	// invitation, or issuing one, may write `invited`.
	default:
		return domain.StatusUnspecified, glerrors.New(glerrors.KindInvalid, "user status is required")
	}
}

// connectErr maps a domain error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks to the client);
// the other Kinds carry already-generic domain messages.
// Every code goes through glerrors.SafeMessage, not just the internal one. The
// kind decides the STATUS; it does not decide whether the wrapped cause is the
// caller's business, and it never is. Error() concatenates the cause, so handing
// the error straight to connect.NewError shipped Postgres constraint names and
// SQLSTATEs — and on some constraints the conflicting value — to whoever called.
func connectErr(err error) *connect.Error {
	safe := errors.New(glerrors.SafeMessage(err))

	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, safe)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, safe)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, safe)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, safe)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, safe)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}

// sessionKindOf reads the surface off the wire. UNSPECIFIED — which is what an
// older client sends — is a STAFF session: the shorter life, because that is the
// safe direction to be wrong in.
func sessionKindOf(kind iamv1.SessionKind) domain.SessionKind {
	if kind == iamv1.SessionKind_SESSION_KIND_FAMILY {
		return domain.SessionFamily
	}
	return domain.SessionStaff
}
