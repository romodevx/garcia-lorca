//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Migration 0019 (ADR 0033): the BBVA reference, the import batch, and the
// enrollment's origin.

func TestBankReferenceIsTenantIsolatedAndUniquePerTenant(t *testing.T) {
	ctx := context.Background()
	a1 := seedStudent(t, tenantA, "aaaaaaaa-0019-0000-0000-000000000001", "BAAA050312MDFRNN01")
	a2 := seedStudent(t, tenantA, "aaaaaaaa-0019-0000-0000-000000000002", "BAAB050312MDFRNN02")
	b1 := seedStudent(t, tenantB, "bbbbbbbb-0019-0000-0000-000000000001", "BABA050312MDFRNN03")
	repoA, repoB := appRepo(t, tenantA), appRepo(t, tenantB)

	if err := repoA.ExecAsTenant(ctx, "UPDATE students SET bank_reference = '0019001' WHERE id = '"+a1+"'"); err != nil {
		t.Fatalf("set A's reference: %v", err)
	}
	// Tenant B cannot see A's reference.
	if n, err := repoB.CountAsTenant(ctx, "SELECT count(*) FROM students WHERE bank_reference = '0019001'"); err != nil || n != 0 {
		t.Fatalf("tenant B sees %d rows with A's reference (err %v); RLS breach", n, err)
	}
	if n, err := repoA.CountAsTenant(ctx, "SELECT count(*) FROM students WHERE bank_reference = '0019001'"); err != nil || n != 1 {
		t.Fatalf("tenant A sees %d of its own rows (err %v); want 1", n, err)
	}
	// Uniqueness is per tenant: B may hold the same reference...
	if err := repoB.ExecAsTenant(ctx, "UPDATE students SET bank_reference = '0019001' WHERE id = '"+b1+"'"); err != nil {
		t.Fatalf("same reference in another tenant must be allowed: %v", err)
	}
	// ...but a second pupil of A may not, and the violation names the index.
	err := repoA.ExecAsTenant(ctx, "UPDATE students SET bank_reference = '0019001' WHERE id = '"+a2+"'")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "uq_students_bank_reference" {
		t.Fatalf("duplicate reference in one tenant: want uq_students_bank_reference, got %v", err)
	}
	// Pupils without a reference never collide (partial index), and the
	// batch column starts empty.
	if n, err := repoA.CountAsTenant(ctx,
		"SELECT count(*) FROM students WHERE id = ? AND bank_reference IS NULL AND import_batch_id IS NULL", a2); err != nil || n != 1 {
		t.Fatalf("a pupil without reference/batch: %d rows (err %v); want 1", n, err)
	}
}

func TestEnrollmentOriginDefaultsToManualAndIsChecked(t *testing.T) {
	ctx := context.Background()
	sid := seedStudent(t, tenantA, "aaaaaaaa-0019-0000-0000-000000000010", "OAAA050312MDFRNN04")
	repo := appRepo(t, tenantA)
	if err := createRoomy(ctx, repo, enrollment(t, "e0190000-0000-0000-0000-000000000001", tenantA, sid, groupOne)); err != nil {
		t.Fatalf("create enrollment: %v", err)
	}
	if n, err := repo.CountAsTenant(ctx, "SELECT count(*) FROM enrollments WHERE student_id = ? AND origin = 'manual'", sid); err != nil || n != 1 {
		t.Fatalf("origin default: %d manual rows (err %v); want 1", n, err)
	}
	err := repo.ExecAsTenant(ctx, "UPDATE enrollments SET origin = 'other' WHERE student_id = '"+sid+"'")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("origin outside (manual, import): want a CHECK violation, got %v", err)
	}
}
