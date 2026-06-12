// Package authctx carries the authenticated principal through context and
// provides WithUserContext: the single, security-critical primitive that binds
// a database transaction to a user so PostgreSQL Row Level Security policies
// apply to every statement inside it.
//
// Every read or write against an RLS-protected table MUST go through
// WithUserContext. The helper opens a transaction and issues
//
//	SET LOCAL app.current_user = '<userID>'
//
// so that policies referencing current_setting('app.current_user') see the
// caller's identity. SET LOCAL scopes the setting to the surrounding
// transaction, so it is automatically reset on commit or rollback — there is no
// session-leak risk across pooled connections.
//
// The helper is defined against the small TxRunner / Tx interfaces below rather
// than against *gorm.DB directly, so it lives in domain-adjacent code without
// importing GORM and can be unit-tested with a fake. The GORM adapter in each
// service supplies the concrete implementation.
package authctx

import (
	"context"
	"fmt"
)

// sessionVar is the PostgreSQL session variable read by RLS policies. It is
// intentionally the only place this string is written, so policies and the
// application can never drift.
const sessionVar = "app.current_user"

// Tx is the minimal transaction surface WithUserContext needs. The concrete
// implementation (e.g. a *gorm.DB bound to a transaction) lives in the
// infrastructure layer.
type Tx interface {
	// Exec runs a statement with positional placeholders ($-style or ?-style as
	// the driver expects). It is used to issue the SET LOCAL command.
	Exec(ctx context.Context, sql string, args ...any) error
}

// TxRunner opens a transaction and invokes fn with it. The implementation must
// commit when fn returns nil and roll back when fn returns an error.
type TxRunner interface {
	Transaction(ctx context.Context, fn func(tx Tx) error) error
}

// WithUserContext runs fn inside a transaction that has been bound to userID for
// Row Level Security. It is the mandatory entry point for any data access on an
// RLS-protected table.
//
// The user identity is bound via "SET LOCAL app.current_user = ?" using a bound
// parameter (never string concatenation) so the value cannot break out of the
// statement. An empty userID is rejected: an unset RLS variable would silently
// expose or hide rows depending on policy phrasing, so we fail closed.
func WithUserContext(ctx context.Context, runner TxRunner, userID string, fn func(tx Tx) error) error {
	if runner == nil {
		return fmt.Errorf("authctx: nil TxRunner")
	}
	if userID == "" {
		return fmt.Errorf("authctx: empty userID; refusing to run without an RLS identity")
	}
	if fn == nil {
		return fmt.Errorf("authctx: nil callback")
	}

	return runner.Transaction(ctx, func(tx Tx) error {
		// SET LOCAL with a bound parameter keeps the identity inside the
		// transaction and safe from injection. set_config(..., true) is the
		// parameterizable equivalent of "SET LOCAL".
		if err := tx.Exec(ctx, "SELECT set_config(?, ?, true)", sessionVar, userID); err != nil {
			return fmt.Errorf("authctx: binding RLS user: %w", err)
		}
		return fn(tx)
	})
}

// SessionVar exposes the RLS session variable name so migrations and tests can
// reference the exact key the application uses.
func SessionVar() string { return sessionVar }
