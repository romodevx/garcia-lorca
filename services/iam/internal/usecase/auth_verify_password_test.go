package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// providerPasswords stands in for Identity Platform's password check.
type providerPasswords struct {
	email    string
	password string
	calls    int
}

func (p *providerPasswords) CheckPassword(_ context.Context, email, password string) error {
	p.calls++
	if email == p.email && password == p.password {
		return nil
	}
	return glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
}

// anyIDTokens marks the service as provider-mode; its Verify is never reached
// by VerifyPassword.
type anyIDTokens struct{}

func (anyIDTokens) Verify(context.Context, string) (ports.VerifiedIdentity, error) {
	return ports.VerifiedIdentity{}, glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
}

func newReauth(users *cpUsers, provider ports.IDTokenVerifier, passwords ports.ProviderPasswordChecker) *usecase.AuthService {
	svc := usecase.NewAuthService(users, nil, cpHasher{}, nil, cpValidator{}, provider, nil, cpTenantID, 15*time.Minute, time.Now)
	return svc.WithProviderPasswords(passwords)
}

// VerifyPassword re-checks a password the SAME way sign-in checks it: the local
// hash where iam owns credentials, the provider where the provider does.
func TestVerifyPassword(t *testing.T) {
	suspended := func(t *testing.T) domain.User {
		email, _ := domain.NewEmail("owner@example.com")
		u, _ := domain.NewUser(cpSubject, cpTenantID, email, "Owner", domain.StatusSuspended, false, nil)
		return u
	}
	cases := []struct {
		name          string
		token         string
		password      string
		provider      bool
		checker       bool
		user          func(*testing.T) domain.User
		wantKind      glerrors.Kind
		wantProvCalls int
	}{
		{"password mode: right password", cpValidToken, cpCurrentPass, false, false, newCPUser, glerrors.KindUnknown, 0},
		{"password mode: wrong password", cpValidToken, "wrong", false, false, newCPUser, glerrors.KindUnauthorized, 0},
		{"password mode: empty password", cpValidToken, "", false, false, newCPUser, glerrors.KindUnauthorized, 0},
		{"bad token", "bogus", cpCurrentPass, false, false, newCPUser, glerrors.KindUnauthorized, 0},
		{"suspended account", cpValidToken, cpCurrentPass, false, false, suspended, glerrors.KindForbidden, 0},
		{"provider mode: right password at the provider", cpValidToken, "provider-pw", true, true, newCPUser, glerrors.KindUnknown, 1},
		{"provider mode: the stale local hash is NOT accepted", cpValidToken, cpCurrentPass, true, true, newCPUser, glerrors.KindUnauthorized, 1},
		{"provider mode: wrong password", cpValidToken, "wrong", true, true, newCPUser, glerrors.KindUnauthorized, 1},
		{"provider mode without a key fails closed as a server fault", cpValidToken, "provider-pw", true, false, newCPUser, glerrors.KindInternal, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			users := &cpUsers{user: tc.user(t), hash: "H:" + cpCurrentPass}
			prov := &providerPasswords{email: "owner@example.com", password: "provider-pw"}
			var idTokens ports.IDTokenVerifier
			if tc.provider {
				idTokens = anyIDTokens{}
			}
			var checker ports.ProviderPasswordChecker
			if tc.checker {
				checker = prov
			}
			err := newReauth(users, idTokens, checker).VerifyPassword(context.Background(), tc.token, tc.password)
			if tc.wantKind == glerrors.KindUnknown {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tc.wantKind, glerrors.KindOf(err), "err = %v", err)
			}
			assert.Equal(t, tc.wantProvCalls, prov.calls)
		})
	}
}
