package authctx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/garcia-lorca/pkg/authctx"
)

// recordedExec captures one Exec call so the test can assert on the SQL the
// helper issued without a real database.
type recordedExec struct {
	sql  string
	args []any
}

// fakeTx records the statements executed against it.
type fakeTx struct {
	calls   []recordedExec
	execErr error
}

func (f *fakeTx) Exec(_ context.Context, sql string, args ...any) error {
	f.calls = append(f.calls, recordedExec{sql: sql, args: args})
	return f.execErr
}

// fakeRunner runs fn against a single fakeTx and reports whether it committed
// (fn returned nil) or rolled back.
type fakeRunner struct {
	tx           *fakeTx
	committed    bool
	rolledBack   bool
	beginErr     error
	transactions int
}

func (r *fakeRunner) Transaction(ctx context.Context, fn func(tx authctx.Tx) error) error {
	r.transactions++
	if r.beginErr != nil {
		return r.beginErr
	}
	if err := fn(r.tx); err != nil {
		r.rolledBack = true
		return err
	}
	r.committed = true
	return nil
}

func TestWithUserContext_BindsRLSUserThenRunsCallback(t *testing.T) {
	tx := &fakeTx{}
	runner := &fakeRunner{tx: tx}
	callbackRan := false

	err := authctx.WithUserContext(context.Background(), runner, "user-123", func(got authctx.Tx) error {
		callbackRan = true
		if got != tx {
			t.Fatalf("callback received a different Tx than the runner provided")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !callbackRan {
		t.Fatal("callback did not run")
	}
	if !runner.committed {
		t.Fatal("expected transaction to commit")
	}

	// The very first statement must bind the RLS identity.
	if len(tx.calls) != 1 {
		t.Fatalf("expected exactly 1 setup statement, got %d", len(tx.calls))
	}
	call := tx.calls[0]
	if call.sql != "SELECT set_config(?, ?, true)" {
		t.Fatalf("unexpected RLS bind SQL: %q", call.sql)
	}
	if len(call.args) != 2 || call.args[0] != authctx.SessionVar() || call.args[1] != "user-123" {
		t.Fatalf("RLS bind args = %v, want [%q user-123]", call.args, authctx.SessionVar())
	}
}

func TestWithUserContext_EmptyUserIDFailsClosed(t *testing.T) {
	runner := &fakeRunner{tx: &fakeTx{}}
	err := authctx.WithUserContext(context.Background(), runner, "", func(authctx.Tx) error {
		t.Fatal("callback must not run without an RLS identity")
		return nil
	})
	if err == nil {
		t.Fatal("expected error for empty userID")
	}
	if runner.transactions != 0 {
		t.Fatal("must not open a transaction without an identity")
	}
}

func TestWithUserContext_NilRunner(t *testing.T) {
	if err := authctx.WithUserContext(context.Background(), nil, "u", func(authctx.Tx) error { return nil }); err == nil {
		t.Fatal("expected error for nil runner")
	}
}

func TestWithUserContext_BindFailureRollsBack(t *testing.T) {
	wantErr := errors.New("connection reset")
	tx := &fakeTx{execErr: wantErr}
	runner := &fakeRunner{tx: tx}

	err := authctx.WithUserContext(context.Background(), runner, "user-1", func(authctx.Tx) error {
		t.Fatal("callback must not run when RLS bind fails")
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped bind error, got %v", err)
	}
	if !runner.rolledBack {
		t.Fatal("expected rollback when bind fails")
	}
}

func TestPrincipalRoundTrip(t *testing.T) {
	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{UserID: "u1", Roles: []string{"admin"}})
	got, ok := authctx.PrincipalFrom(ctx)
	if !ok || got.UserID != "u1" {
		t.Fatalf("PrincipalFrom() = %+v, %v", got, ok)
	}

	if _, ok := authctx.PrincipalFrom(context.Background()); ok {
		t.Fatal("expected no principal on a bare context")
	}
}
