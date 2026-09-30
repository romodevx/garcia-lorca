//go:build integration

// Integration tests for T1 (review 2026-09-30): the read the billing run uses to
// recognise a line it already issued under another reference.
//
// Run: go test -tags=integration ./internal/adapters/postgres/
// Requires Docker.
package postgres_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// issuedFixture stores, for one school, the lines the run reads and the ones it
// must not: a legacy and a concept inscription, a fee share, a tuition month, a
// hand-made charge, a cancelled line and a line of another cycle.
func issuedFixture(t *testing.T, tenant, hex string) (cycle, student string) {
	t.Helper()
	ctx := context.Background()
	repo := appRepo(t, tenant)
	id := func(tag string) string { return "a9000000-0000-0000-0000-0000000" + hex + tag }

	concept := newConcept(t, id("c00"), tenant, "Cargos T1 "+hex, 1000)
	if _, err := repo.CreateConcept(ctx, concept); err != nil {
		t.Fatalf("create concept: %v", err)
	}
	cycle, other, student := id("ce0"), id("ce1"), id("a01")
	due := ymd(2026, time.September, 10)
	charge := func(tag, cyc, ref string) domain.Charge {
		c := newCharge(t, id(tag), tenant, student, cyc, concept.ID(), 1000, 0, due)
		if ref == "" {
			return c
		}
		return referenced(t, c, ref)
	}
	rows := []domain.Charge{
		charge("f01", cycle, "enrollment:"+cycle+":"+student),
		charge("f02", cycle, "enrollment:"+cycle+":"+student+":apf"),
		charge("f03", cycle, "fee:"+cycle+":"+student+":graduacin:2026-09"),
		charge("f04", cycle, "tuition:"+cycle+":"+student+":2026-09"),
		charge("f05", other, "enrollment:"+other+":"+student),
		charge("f07", cycle, "enrollment:"+cycle+":"+student+":cancelada"),
	}
	if _, err := repo.CreateChargesBulk(ctx, rows); err != nil {
		t.Fatalf("create charges: %v", err)
	}
	// A cancelled line owes nothing: it must not stop the right one being issued.
	if _, err := repo.CancelChargeIfUnpaid(ctx, id("f07")); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// A charge typed by hand carries no ref; it is not the run's to recognise.
	if _, err := repo.CreateCharge(ctx, charge("f06", cycle, "")); err != nil {
		t.Fatalf("create hand-made charge: %v", err)
	}
	return cycle, student
}

func TestListIssuedLinesReadsOnlyTheRunsNamedLinesOfTheCycle(t *testing.T) {
	ctx := context.Background()
	cycle, student := issuedFixture(t, tenantA, "d1")
	repo := appRepo(t, tenantA)

	for name, studentID := range map[string]string{"whole cycle": "", "one student": student} {
		t.Run(name, func(t *testing.T) {
			got, err := repo.ListIssuedLines(ctx, cycle, studentID)
			if err != nil {
				t.Fatalf("ListIssuedLines: %v", err)
			}
			refs := make([]string, 0, len(got))
			for _, l := range got {
				refs = append(refs, l.ExternalRef)
				if l.StudentID != student || l.Description == "" {
					t.Errorf("line %q came back without its student or description: %+v", l.ExternalRef, l)
				}
			}
			sort.Strings(refs)
			want := []string{
				"enrollment:" + cycle + ":" + student,
				"enrollment:" + cycle + ":" + student + ":apf",
				"fee:" + cycle + ":" + student + ":graduacin:2026-09",
			}
			if len(refs) != len(want) {
				t.Fatalf("refs = %v, want %v", refs, want)
			}
			for i := range want {
				if refs[i] != want[i] {
					t.Errorf("refs[%d] = %q, want %q", i, refs[i], want[i])
				}
			}
		})
	}
}

// RLS: one school's ledger is invisible to another's run.
func TestListIssuedLinesCannotReadAnotherSchool(t *testing.T) {
	cycle, _ := issuedFixture(t, tenantB, "d2")

	got, err := appRepo(t, tenantA).ListIssuedLines(context.Background(), cycle, "")
	if err != nil {
		t.Fatalf("ListIssuedLines: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("tenant A read %d of tenant B's lines", len(got))
	}
}
