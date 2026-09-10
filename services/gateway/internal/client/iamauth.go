package client

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/iam/v1/iamv1connect"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// AuthClient is the gateway's typed client for iam's auth surface, over the
// generated Connect client. It is the ONLY place the edge talks to iam for
// authentication; handlers/middleware depend on its small methods, not on the
// generated types.
type AuthClient struct {
	rpc iamv1connect.IamServiceClient
}

// LoginResult is the gateway's view of a successful login.
type LoginResult struct {
	AccessToken        string
	RefreshToken       string
	ExpiresIn          int64
	MustChangePassword bool
}

// NewAuthClient builds an AuthClient targeting the iam base URL (e.g.
// "http://iam:8080") over httpClient.
func NewAuthClient(baseURL string, httpClient connect.HTTPClient) *AuthClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &AuthClient{rpc: iamv1connect.NewIamServiceClient(httpClient, baseURL)}
}

// Login exchanges credentials for tokens via iam. Connect error codes propagate
// (e.g. Unauthenticated for bad credentials) so the handler can map them.
// SessionKind is which surface is logging in. It decides how long the session
// may sit unused: the school's panel closes after fifteen idle minutes, a
// parent's phone lasts a day.
type SessionKind string

const (
	// SessionStaff is the school's panel.
	SessionStaff SessionKind = "staff"
	// SessionFamily is a parent's phone.
	SessionFamily SessionKind = "family"
)

// Login authenticates for the school's panel. It is the safe default: a caller
// that does not say which surface it is gets the shorter session.
func (c *AuthClient) Login(ctx context.Context, email, password string) (LoginResult, error) {
	return c.LoginAs(ctx, email, password, SessionStaff)
}

// LoginAs authenticates for a named surface. The kind comes from the ROUTE the
// request arrived on — never from anything a user can type.
func (c *AuthClient) LoginAs(ctx context.Context, email, password string, kind SessionKind) (LoginResult, error) {
	res, err := c.rpc.Login(ctx, connect.NewRequest(&iamv1.LoginRequest{
		Email: email, Password: password, SessionKind: protoSessionKind(kind),
	}))
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		AccessToken:        res.Msg.GetAccessToken(),
		RefreshToken:       res.Msg.GetRefreshToken(),
		ExpiresIn:          res.Msg.GetExpiresIn(),
		MustChangePassword: res.Msg.GetMustChangePassword(),
	}, nil
}

// LoginWithIDToken exchanges a provider ID token for OUR session pair. The kind
// comes from the ROUTE the request arrived on, exactly as it does for a password
// login — never from anything the caller can set.
func (c *AuthClient) LoginWithIDToken(ctx context.Context, idToken string, kind SessionKind) (LoginResult, error) {
	res, err := c.rpc.LoginWithIDToken(ctx, connect.NewRequest(&iamv1.LoginWithIDTokenRequest{
		IdToken: idToken, SessionKind: protoSessionKind(kind),
	}))
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		AccessToken:        res.Msg.GetAccessToken(),
		RefreshToken:       res.Msg.GetRefreshToken(),
		ExpiresIn:          res.Msg.GetExpiresIn(),
		MustChangePassword: res.Msg.GetMustChangePassword(),
	}, nil
}

// Refresh exchanges a refresh token for a fresh, rotated token pair via iam.
func (c *AuthClient) Refresh(ctx context.Context, refreshToken string) (LoginResult, error) {
	res, err := c.rpc.RefreshToken(ctx, connect.NewRequest(&iamv1.RefreshTokenRequest{RefreshToken: refreshToken}))
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		AccessToken:  res.Msg.GetAccessToken(),
		RefreshToken: res.Msg.GetRefreshToken(),
		ExpiresIn:    res.Msg.GetExpiresIn(),
	}, nil
}

// ChangePassword rotates the caller's password via iam. iam derives the subject
// from accessToken (never a caller-supplied id), so the gateway forwards the
// caller's own access token, not a user id.
func (c *AuthClient) ChangePassword(ctx context.Context, accessToken, currentPassword, newPassword string) error {
	_, err := c.rpc.ChangePassword(ctx, connect.NewRequest(&iamv1.ChangePasswordRequest{
		AccessToken:     accessToken,
		CurrentPassword: currentPassword,
		NewPassword:     newPassword,
	}))
	return err
}

// ValidateToken verifies an access token with iam and returns the principal.
// It implements middleware.Authenticator.
func (c *AuthClient) ValidateToken(ctx context.Context, accessToken string) (middleware.Principal, error) {
	res, err := c.rpc.ValidateToken(ctx, connect.NewRequest(&iamv1.ValidateTokenRequest{AccessToken: accessToken}))
	if err != nil {
		return middleware.Principal{}, err
	}
	claims := res.Msg.GetClaims()
	return middleware.Principal{
		UserID:             claims.GetSubject(),
		TenantID:           claims.GetTenantId(),
		Email:              claims.GetEmail(),
		Permissions:        claims.GetPermissions(),
		MustChangePassword: claims.GetMustChangePassword(),
	}, nil
}

// compile-time proof AuthClient satisfies the middleware's Authenticator.
var _ middleware.Authenticator = (*AuthClient)(nil)

// protoSessionKind maps the edge's name for a surface to the wire enum.
func protoSessionKind(kind SessionKind) iamv1.SessionKind {
	if kind == SessionFamily {
		return iamv1.SessionKind_SESSION_KIND_FAMILY
	}
	return iamv1.SessionKind_SESSION_KIND_STAFF
}
