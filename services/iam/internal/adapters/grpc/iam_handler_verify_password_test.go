package grpcadapter_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
)

// VerifyPassword is the re-authentication of a destructive action (ADR 0021).
func TestVerifyPasswordWire(t *testing.T) {
	cases := []struct {
		name     string
		token    func(t *testing.T) string
		password string
		want     connect.Code
	}{
		{"right password", nil, "test-owner-pw", 0},
		{"wrong password", nil, "wrong", connect.CodeUnauthenticated},
		{"garbage token", func(*testing.T) string { return "garbage" }, "test-owner-pw", connect.CodeUnauthenticated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newHandlerWithUsers(t, false)
			token := loginAccessToken(t, h)
			if tc.token != nil {
				token = tc.token(t)
			}
			_, err := h.VerifyPassword(context.Background(), connect.NewRequest(&iamv1.VerifyPasswordRequest{
				AccessToken: token, Password: tc.password,
			}))
			if tc.want == 0 {
				if err != nil {
					t.Fatalf("VerifyPassword: %v", err)
				}
				return
			}
			assertCode(t, err, tc.want)
		})
	}
}
