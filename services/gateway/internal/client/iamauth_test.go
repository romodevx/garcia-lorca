package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/iam/v1/iamv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// stubIAM implements the generated iam handler with canned auth behavior, so the
// AuthClient is tested over a REAL Connect round-trip (wire + error codes)
// without iam's internals.
type stubIAM struct {
	iamv1connect.UnimplementedIamServiceHandler
}

func (stubIAM) Login(_ context.Context, req *connect.Request[iamv1.LoginRequest]) (*connect.Response[iamv1.LoginResponse], error) {
	if req.Msg.GetPassword() != "secret" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errString("invalid credentials"))
	}
	return connect.NewResponse(&iamv1.LoginResponse{AccessToken: "acc", RefreshToken: "ref", ExpiresIn: 900}), nil
}

func (stubIAM) RefreshToken(_ context.Context, req *connect.Request[iamv1.RefreshTokenRequest]) (*connect.Response[iamv1.RefreshTokenResponse], error) {
	if req.Msg.GetRefreshToken() != "good-ref" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errString("invalid refresh"))
	}
	return connect.NewResponse(&iamv1.RefreshTokenResponse{AccessToken: "acc2", RefreshToken: "ref2", ExpiresIn: 900}), nil
}

func (stubIAM) ValidateToken(_ context.Context, req *connect.Request[iamv1.ValidateTokenRequest]) (*connect.Response[iamv1.ValidateTokenResponse], error) {
	if req.Msg.GetAccessToken() != "valid" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errString("invalid token"))
	}
	return connect.NewResponse(&iamv1.ValidateTokenResponse{Claims: &iamv1.Claims{
		Subject: "u1", TenantId: "t1", Email: "ana@garcialorca.mx", Permissions: []string{"students:read"},
	}}), nil
}

type errString string

func (e errString) Error() string { return string(e) }

func newStubServer(t *testing.T) (*client.AuthClient, func()) {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := iamv1connect.NewIamServiceHandler(stubIAM{})
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	return client.NewAuthClient(srv.URL, srv.Client()), srv.Close
}

func TestAuthClientLogin(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	res, err := c.Login(context.Background(), "ana@garcialorca.mx", "secret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.AccessToken != "acc" || res.RefreshToken != "ref" || res.ExpiresIn != 900 {
		t.Errorf("result mismatch: %+v", res)
	}
}

func TestAuthClientLoginBadCredentials(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	_, err := c.Login(context.Background(), "ana@garcialorca.mx", "wrong")
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
	}
}

func TestAuthClientRefresh(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	res, err := c.Refresh(context.Background(), "good-ref")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if res.AccessToken != "acc2" || res.RefreshToken != "ref2" {
		t.Errorf("rotated pair mismatch: %+v", res)
	}
	if _, err := c.Refresh(context.Background(), "stale"); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("stale refresh: want Unauthenticated, got %v", connect.CodeOf(err))
	}
}

func TestAuthClientValidateToken(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	p, err := c.ValidateToken(context.Background(), "valid")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if p.UserID != "u1" || p.TenantID != "t1" || len(p.Permissions) != 1 {
		t.Errorf("principal mismatch: %+v", p)
	}
}

func TestAuthClientValidateTokenInvalid(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	if _, err := c.ValidateToken(context.Background(), "garbage"); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
	}
}
