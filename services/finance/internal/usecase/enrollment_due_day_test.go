package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/ports"
)

// ISSUE-T14-022: a pupil enrolled on Saturday 17 Oct 2026 had their inscription
// written by the StudentEnrolled consumer due that Saturday, while the run dates
// it on the next working day. The next run then "moved" it to Monday 19, and a
// withdrawal in between treated it as not yet due. One rule dates it on both
// paths: the next working day, closed days included.
func TestAnInscriptionIssuedOnASaturdayFallsDueOnMondayOnBothPaths(t *testing.T) {
	f := billingFixture(t)
	ctx := context.Background()
	saturday := time.Date(2026, time.October, 17, 0, 0, 0, 0, time.UTC)
	monday := time.Date(2026, time.October, 19, 0, 0, 0, 0, time.UTC)
	// Noon UTC: EnrolledAt is an instant read in Morelia (ADR 0035).
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{{
		ID: "enr-sat", StudentID: "stu-sat", GroupID: testGroup, CycleID: testCycle,
		Status: "active", EnrolledAt: saturday.Add(12 * time.Hour),
	}}

	if _, err := f.svc.EnsureEnrollmentCharge(ctx, testCycle, "stu-sat"); err != nil {
		t.Fatalf("EnsureEnrollmentCharge: %v", err)
	}
	list, err := allCharges(ctx, f.svc, "stu-sat", testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	found := false
	for _, c := range list {
		if c.Charge.Description() != "Inscripción" {
			continue
		}
		found = true
		if !c.Charge.DueDate().Equal(monday) {
			t.Errorf("the enrolment dated the inscription %s, want Monday %s",
				c.Charge.DueDate().Format(time.DateOnly), monday.Format(time.DateOnly))
		}
	}
	if !found {
		t.Fatal("the enrolment issued no inscription")
	}

	got := runOn(t, f, "2026-10-20")
	if got.EnrollmentMoved != 0 {
		t.Errorf("the run moved %d inscriptions the enrolment had just written", got.EnrollmentMoved)
	}
}
