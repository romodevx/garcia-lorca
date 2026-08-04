package grpcadapter

import (
	"context"

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
	health *usecase.HealthService
}

// NewIAMHandler wires the handler to the auth + health use cases.
func NewIAMHandler(auth *usecase.AuthService, health *usecase.HealthService) *IAMHandler {
	return &IAMHandler{auth: auth, health: health}
}

func (h *IAMHandler) Health(ctx context.Context, _ *connect.Request[iamv1.HealthRequest]) (*connect.Response[iamv1.HealthResponse], error) {
	status, err := h.health.Ping(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.HealthResponse{Status: status.Status}), nil
}

func (h *IAMHandler) Login(ctx context.Context, req *connect.Request[iamv1.LoginRequest]) (*connect.Response[iamv1.LoginResponse], error) {
	res, err := h.auth.Authenticate(ctx, req.Msg.GetEmail(), req.Msg.GetPassword())
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

// --- mapping ---------------------------------------------------------------

func toProtoUser(u domain.User) *iamv1.User {
	return &iamv1.User{
		Id:                 u.ID(),
		TenantId:           u.TenantID(),
		Email:              u.Email().String(),
		DisplayName:        u.DisplayName(),
		Status:             toProtoStatus(u.Status()),
		MustChangePassword: u.MustChangePassword(),
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
	default:
		return iamv1.UserStatus_USER_STATUS_UNSPECIFIED
	}
}

// connectErr maps a domain error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks to the client);
// the other Kinds carry already-generic domain messages.
func connectErr(err error) *connect.Error {
	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, err)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, err)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, err)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, err)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, err)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}
