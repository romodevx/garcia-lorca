package main

import (
	"errors"
	"io"
	"log/slog"
	"testing"
)

// discardLogger keeps the table-driven cases quiet.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestJWTSecret pins the boot contract for the token signing key.
//
// Regression for the deploy blocker found by /autoplan (F3): jwtSecret used to
// fall back to a RANDOM key whenever IAM_JWT_SECRET was unset, warning "dev
// only" and continuing. That is harmless under docker-compose, where iam never
// restarts — but on Cloud Run with min_instances=0 the service sleeps after
// ~15 minutes of idle and wakes holding a NEW key, silently invalidating every
// token it had already issued. The user gets thrown back to the login screen
// mid-session, with no error that explains why, and two concurrent instances
// sign with different keys.
//
// So: outside a local environment, a missing secret must stop the process at
// boot. A revision that never goes ready is a loud, obvious failure; a service
// that quietly forgets its sessions is not.
func TestJWTSecret(t *testing.T) {
	tests := []struct {
		name      string
		appEnv    string
		secret    string
		wantErr   error
		wantValue string
	}{
		{
			name:   "explicit secret is used verbatim in any environment",
			appEnv: "prod",
			// Deliberately unmistakable as a fixture. A leetspeak value here read
			// as a "Generic High Entropy Secret" to the repository secret scanner
			// and failed the PR — a false positive, but one that trains people to
			// wave the scanner through, which is how a real leak gets merged.
			secret:    "fixture-not-a-real-key",
			wantValue: "fixture-not-a-real-key",
		},
		{
			name:   "unset APP_ENV is local: ephemeral key allowed",
			appEnv: "",
			secret: "",
		},
		{
			name:   "APP_ENV=local: ephemeral key allowed",
			appEnv: "local",
			secret: "",
		},
		{
			name:    "APP_ENV=dev without a secret must NOT boot",
			appEnv:  "dev",
			secret:  "",
			wantErr: errMissingJWTSecret,
		},
		{
			name:    "APP_ENV=staging without a secret must NOT boot",
			appEnv:  "staging",
			secret:  "",
			wantErr: errMissingJWTSecret,
		},
		{
			name:    "APP_ENV=prod without a secret must NOT boot",
			appEnv:  "prod",
			secret:  "",
			wantErr: errMissingJWTSecret,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tt.appEnv)
			t.Setenv("IAM_JWT_SECRET", tt.secret)

			got, err := jwtSecret(discardLogger())

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("jwtSecret() error = %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("jwtSecret() returned a key alongside an error: %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("jwtSecret() unexpected error: %v", err)
			}
			if tt.wantValue != "" && string(got) != tt.wantValue {
				t.Errorf("jwtSecret() = %q, want %q", got, tt.wantValue)
			}
			if len(got) == 0 {
				t.Error("jwtSecret() returned an empty key")
			}
		})
	}
}

// TestJWTSecretEphemeralKeysDiffer documents WHY the ephemeral fallback is only
// safe locally: two calls produce two different keys, which is exactly what
// breaks sessions when a scaled-to-zero instance restarts.
func TestJWTSecretEphemeralKeysDiffer(t *testing.T) {
	t.Setenv("APP_ENV", "local")
	t.Setenv("IAM_JWT_SECRET", "")

	first, err := jwtSecret(discardLogger())
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := jwtSecret(discardLogger())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(first) == string(second) {
		t.Fatal("two ephemeral keys were identical; the test can no longer prove restart-instability")
	}
}
