//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Task 13: every pupil gets a matrícula when registered — <year>-<five
// digits>, one run per school and year, allocated in the insert's own
// transaction.
func TestAPupilGetsAMatricula(t *testing.T) {
	ctx := context.Background()
	repoA := appRepo(t, tenantA)
	first := applicant(t, "aaaaaaaa-0000-0000-0000-0000000013a1", tenantA, "MATA050312MDFRNN01", "")
	second := applicant(t, "aaaaaaaa-0000-0000-0000-0000000013a2", tenantA, "MATB050312MDFRNN02", "")
	if err := repoA.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if err := repoA.Create(ctx, second); err != nil {
		t.Fatalf("Create second: %v", err)
	}
	m1, m2 := first.EnrollmentNumber(), second.EnrollmentNumber()
	if !regexp.MustCompile(`^[0-9]{4}-[0-9]{5}$`).MatchString(m1) {
		t.Fatalf("matrícula %q is not AAAA-NNNNN", m1)
	}
	if seqOfNumber(t, m2) != seqOfNumber(t, m1)+1 || m1[:5] != m2[:5] {
		t.Errorf("matrículas %q then %q; want the next number of the same year", m1, m2)
	}
	stored, err := repoA.FindByID(ctx, first.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if stored.EnrollmentNumber() != m1 {
		t.Errorf("read back %q, want %q", stored.EnrollmentNumber(), m1)
	}

	other := applicant(t, "aaaaaaaa-0000-0000-0000-0000000013b1", tenantB, "MATC050312MDFRNN03", "")
	if err := appRepo(t, tenantB).Create(ctx, other); err != nil {
		t.Fatalf("Create in B: %v", err)
	}
	if got := seqOfNumber(t, other.EnrollmentNumber()); got != 1 {
		t.Errorf("tenant B's first matrícula = %q, want number 1 of its own run", fmt.Sprint(other.EnrollmentNumber()))
	}
}

func seqOfNumber(t *testing.T, number string) int {
	t.Helper()
	n, err := strconv.Atoi(number[len(number)-5:])
	if err != nil {
		t.Fatalf("matrícula %q: %v", number, err)
	}
	return n
}

// During a deploy the previous revision writes pupils without the column: the
// table gives them their matrícula all the same.
func TestAPupilWrittenWithoutANumberGetsOne(t *testing.T) {
	const id = "b0000000-0000-0000-0000-0000000013c1"
	if err := superDB.Exec(
		"INSERT INTO students (id, tenant_id, curp, first_name, last_name, level_name) VALUES (?, ?, 'MATD050312MDFRNN04', 'Old', 'Revision', 'Primaria')",
		id, tenantA).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := appRepo(t, tenantA).FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !regexp.MustCompile(`^[0-9]{4}-[0-9]{5}$`).MatchString(got.EnrollmentNumber()) {
		t.Errorf("matrícula = %q, want one given by the table", got.EnrollmentNumber())
	}
}

// A counter behind its numbers is a fault, never "that CURP already exists".
func TestAMatriculaClashIsNotADuplicateCURP(t *testing.T) {
	const tenant = "33333333-3333-3333-3333-333333333333"
	ctx := context.Background()
	repo := appRepo(t, tenant)
	if err := repo.Create(ctx, applicant(t, "aaaaaaaa-0000-0000-0000-0000000013d1", tenant, "MATE050312MDFRNN05", "")); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := superDB.Exec(`UPDATE student_number_counters SET next_seq = 1 WHERE tenant_id = ?`, tenant).Error; err != nil {
		t.Fatalf("rolling the counter back: %v", err)
	}
	err := repo.Create(ctx, applicant(t, "aaaaaaaa-0000-0000-0000-0000000013d2", tenant, "MATF050312MDFRNN06", ""))
	if kind := glerrors.KindOf(err); kind != glerrors.KindInternal {
		t.Errorf("a matrícula clash answered %v (%v); want an internal fault", kind, err)
	}
}
