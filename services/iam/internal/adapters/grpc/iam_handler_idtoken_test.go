package grpcadapter_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/adapters/bcrypt"
	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/adapters/jwt"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

/*
The RPC that makes the provider path reachable.

Until this handler existed the use case was written, tested and unreachable:
nothing in the wire contract could call it, so the whole thing was inert at
runtime — which is exactly what a code review caught. These tests are the seam
that closes that gap, at the layer where it was missing.
*/

type stubIDTokens struct{}

func (stubIDTokens) Verify(_ context.Context, idToken string) (ports.VerifiedIdentity, error) {
	if idToken != "google:ana" {
		return ports.VerifiedIdentity{}, glerrors.New(glerrors.KindUnauthorized, "invalid id token")
	}
	return ports.VerifiedIdentity{Email: "ana@garcialorca.mx", Subject: "google-uid-1"}, nil
}

// idTokenHandler builds the handler with a provider wired, which newHandler
// deliberately does not do (it passes nil, the shape every other test needs).
func idTokenHandler(t *testing.T) *grpcadapter.IAMHandler {
	t.Helper()
	hasher := bcrypt.NewHasher(4)
	hash, _ := hasher.Hash("test-owner-pw")
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	user, _ := domain.NewUser("u1", "t1", email, "Ana", domain.StatusActive, false, []domain.Role{role})

	users := &fakeUsers{user: user, hash: hash}
	tokens := jwt.NewHMAC([]byte(strings.Repeat("gl-key-", 6)), "garcia-lorca", time.Hour, time.Now)
	roles := &fakeRoles{known: "r1"}
	auth := usecase.NewAuthService(users, roles, hasher, tokens, tokens, stubIDTokens{}, nil, "t1", 15*time.Minute, time.Now)
	admin := usecase.NewAdminService(users, roles, hasher, nil, nil, nil, "", "t1", func() string { return "generated-id" }, nil)
	return grpcadapter.NewIAMHandler(auth, admin, usecase.NewHealthService("iam"))
}

func TestLoginWithIDTokenIssuesOurSession(t *testing.T) {
	h := idTokenHandler(t)

	res, err := h.LoginWithIDToken(context.Background(), connect.NewRequest(&iamv1.LoginWithIDTokenRequest{
		IdToken:     "google:ana",
		SessionKind: iamv1.SessionKind_SESSION_KIND_STAFF,
	}))
	if err != nil {
		t.Fatalf("LoginWithIDToken: %v", err)
	}
	// OUR pair, not the provider's token handed back. If this ever echoed the ID
	// token, the session would live and die on Google's clock instead of ours.
	if res.Msg.GetAccessToken() == "" || res.Msg.GetRefreshToken() == "" {
		t.Fatal("the response carries no session; the exchange must mint our own pair")
	}
	if res.Msg.GetUser().GetEmail() != "ana@garcialorca.mx" {
		t.Errorf("user email = %q", res.Msg.GetUser().GetEmail())
	}
}

func TestLoginWithIDTokenRejectsBadToken(t *testing.T) {
	h := idTokenHandler(t)

	_, err := h.LoginWithIDToken(context.Background(), connect.NewRequest(&iamv1.LoginWithIDTokenRequest{
		IdToken:     "google:forged",
		SessionKind: iamv1.SessionKind_SESSION_KIND_STAFF,
	}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", connect.CodeOf(err))
	}
}
