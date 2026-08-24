// Package dbdsn resolves the PostgreSQL DSN a service opens its GORM pool with.
//
// Local development (docker-compose) injects a ready-made DATABASE_URL, which is
// used verbatim. Cloud Run injects only the split pieces of the Cloud SQL
// unix-socket connection (DB_SOCKET_DIR, DB_USER, DB_PASSWORD, DB_NAME) and
// never DATABASE_URL, so Resolve assembles the keyword/value DSN pgx
// understands from those. Extracted here because all seven data services had a
// verbatim os.Getenv("DATABASE_URL") copy that crashed on Cloud Run.
package dbdsn

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrMissingConfig is returned when neither DATABASE_URL nor the full set of
// split connection variables is present in the environment.
var ErrMissingConfig = errors.New("dbdsn: neither DATABASE_URL nor the split DB_* variables are set")

const (
	envDatabaseURL = "DATABASE_URL"
	envSocketDir   = "DB_SOCKET_DIR"
	envUser        = "DB_USER"
	envPassword    = "DB_PASSWORD"
	envName        = "DB_NAME"
)

// Resolve returns the DSN to open the Postgres pool with, reading the process
// environment. See resolve for the resolution rules.
func Resolve() (string, error) {
	return resolve(os.Getenv)
}

// resolve returns DATABASE_URL verbatim when set (the docker-compose path);
// otherwise it assembles the Cloud SQL unix-socket DSN from the split
// variables. A missing required variable yields an error naming it, so a
// misconfigured revision fails fast at boot instead of dialing localhost.
func resolve(getenv func(string) string) (string, error) {
	if url := getenv(envDatabaseURL); url != "" {
		return url, nil
	}

	socketDir := getenv(envSocketDir)
	user := getenv(envUser)
	password := getenv(envPassword)
	name := getenv(envName)

	var missing []string
	for _, v := range []struct{ key, val string }{
		{envSocketDir, socketDir},
		{envUser, user},
		{envPassword, password},
		{envName, name},
	} {
		if v.val == "" {
			missing = append(missing, v.key)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("%w: missing %s", ErrMissingConfig, strings.Join(missing, ", "))
	}

	// Cloud SQL unix-socket DSN: the socket directory is the host. Each value is
	// single-quoted so a password containing spaces or symbols parses intact.
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s sslmode=disable",
		quote(socketDir), quote(user), quote(password), quote(name),
	), nil
}

// quote wraps a libpq keyword/value in single quotes, escaping backslashes and
// single quotes so values with spaces or special characters parse correctly.
func quote(v string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
}
