package dbdsn

import (
	"errors"
	"strings"
	"testing"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveReturnsDatabaseURLVerbatim(t *testing.T) {
	const want = "postgres://user:pass@db:5432/garcia?sslmode=disable"
	// Split vars are present too; DATABASE_URL must still win untouched.
	got, err := resolve(envFrom(map[string]string{
		"DATABASE_URL":  want,
		"DB_SOCKET_DIR": "/cloudsql/p:r:i",
		"DB_USER":       "app",
		"DB_PASSWORD":   "secret",
		"DB_NAME":       "garcia",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("DATABASE_URL not returned verbatim: got %q want %q", got, want)
	}
}

func TestResolveAssemblesSocketDSN(t *testing.T) {
	got, err := resolve(envFrom(map[string]string{
		"DB_SOCKET_DIR": "/cloudsql/proj:us-central1:main",
		"DB_USER":       "app_user",
		"DB_PASSWORD":   "s3cr3t",
		"DB_NAME":       "garcia_lorca",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "host='/cloudsql/proj:us-central1:main' user='app_user' password='s3cr3t' dbname='garcia_lorca' sslmode=disable"
	if got != want {
		t.Fatalf("assembled DSN mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestResolveQuotesSpecialCharacters(t *testing.T) {
	got, err := resolve(envFrom(map[string]string{
		"DB_SOCKET_DIR": "/cloudsql/p:r:i",
		"DB_USER":       "app",
		"DB_PASSWORD":   `pa ss'w\d`,
		"DB_NAME":       "garcia",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Backslash and single quote must be escaped inside the quoted value so the
	// libpq keyword/value parser keeps the password intact.
	if !strings.Contains(got, `password='pa ss\'w\\d'`) {
		t.Fatalf("special characters not escaped: %q", got)
	}
}

func TestResolveErrorsWhenSplitVarsMissing(t *testing.T) {
	_, err := resolve(envFrom(map[string]string{
		// DATABASE_URL absent and DB_USER / DB_PASSWORD missing.
		"DB_SOCKET_DIR": "/cloudsql/p:r:i",
		"DB_NAME":       "garcia",
	}))
	if err == nil {
		t.Fatal("expected error when required split vars are missing")
	}
	if !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("error should wrap ErrMissingConfig: %v", err)
	}
	if !strings.Contains(err.Error(), "DB_USER") || !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Fatalf("error should name the missing vars: %v", err)
	}
}

func TestResolveErrorsWhenEnvironmentEmpty(t *testing.T) {
	_, err := resolve(envFrom(map[string]string{}))
	if !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("expected ErrMissingConfig on empty env, got %v", err)
	}
}
