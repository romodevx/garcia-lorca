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
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
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
func (c *AuthClient) Login(ctx context.Context, email, password string) (LoginResult, error) {
	res, err := c.rpc.Login(ctx, connect.NewRequest(&iamv1.LoginRequest{Email: email, Password: password}))
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		AccessToken:  res.Msg.GetAccessToken(),
		RefreshToken: res.Msg.GetRefreshToken(),
		ExpiresIn:    res.Msg.GetExpiresIn(),
	}, nil
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
		UserID:      claims.GetSubject(),
		TenantID:    claims.GetTenantId(),
		Email:       claims.GetEmail(),
		Permissions: claims.GetPermissions(),
	}, nil
}

// compile-time proof AuthClient satisfies the middleware's Authenticator.
var _ middleware.Authenticator = (*AuthClient)(nil)
