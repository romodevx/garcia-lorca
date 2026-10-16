//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	if err := repoA.Create(ctx, first, testNumberYear); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if err := repoA.Create(ctx, second, testNumberYear); err != nil {
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
	if err := appRepo(t, tenantB).Create(ctx, other, testNumberYear); err != nil {
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
	if err := repo.Create(ctx, applicant(t, "aaaaaaaa-0000-0000-0000-0000000013d1", tenant, "MATE050312MDFRNN05", ""), testNumberYear); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := superDB.Exec(`UPDATE student_number_counters SET next_seq = 1 WHERE tenant_id = ?`, tenant).Error; err != nil {
		t.Fatalf("rolling the counter back: %v", err)
	}
	err := repo.Create(ctx, applicant(t, "aaaaaaaa-0000-0000-0000-0000000013d2", tenant, "MATF050312MDFRNN06", ""), testNumberYear)
	if kind := glerrors.KindOf(err); kind != glerrors.KindInternal {
		t.Errorf("a matrícula clash answered %v (%v); want an internal fault", kind, err)
	}
}

// Review of task 13: past 99999 pupils in a year the matrícula has no digits
// left; the refusal says so instead of "creating student".
func TestAYearWithoutMatriculasLeftSaysSo(t *testing.T) {
	const tenant = "44444444-4444-4444-4444-444444444444"
	ctx := context.Background()
	repo := appRepo(t, tenant)
	if err := repo.Create(ctx, applicant(t, "aaaaaaaa-0000-0000-0000-0000000013f1", tenant, "MATG050312MDFRNN08", ""), testNumberYear); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := superDB.Exec(`UPDATE student_number_counters SET next_seq = 100000 WHERE tenant_id = ?`, tenant).Error; err != nil {
		t.Fatalf("exhausting the counter: %v", err)
	}
	err := repo.Create(ctx, applicant(t, "aaaaaaaa-0000-0000-0000-0000000013f2", tenant, "MATH050312MDFRNN09", ""), testNumberYear)
	if err == nil || !strings.Contains(err.Error(), "matrícula") {
		t.Errorf("an exhausted year: %v, want the matrícula named in the refusal", err)
	}
}

// testNumberYear is the matrícula year the tests hand the repository.
const testNumberYear = 2026

// T5b review: the matrícula is numbered in the year the use case hands in (the
// school's calendar year), not the database's UTC year — a pupil registered at
// 19:00 on 31 December in Morelia is registered on a day already next year in
// UTC. 2031 is far from the real clock, so only the parameter can produce it.
func TestAMatriculaIsNumberedInTheYearGiven(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	pupil := applicant(t, "aaaaaaaa-0000-0000-0000-0000000015b1", tenantA, "MATI050312MDFRNN01", "")
	if err := repo.Create(ctx, pupil, 2031); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(pupil.EnrollmentNumber(), "2031-") {
		t.Errorf("matrícula = %q, want 2031-…", pupil.EnrollmentNumber())
	}

	cmd := importCommand(t, tenantA, "aaaaaaaa-0000-0000-0000-0000000015b2", "e1a00000-0000-0000-0000-0000000015b2", "MATJ050312MDFRNN02", "")
	cmd.NumberYear = 2031
	got, err := repo.ImportStudent(ctx, cmd)
	if err != nil {
		t.Fatalf("ImportStudent: %v", err)
	}
	if !strings.HasPrefix(got.EnrollmentNumber, "2031-") {
		t.Errorf("imported matrícula = %q, want 2031-…", got.EnrollmentNumber)
	}
}
