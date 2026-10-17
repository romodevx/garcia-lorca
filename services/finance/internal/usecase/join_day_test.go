package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/ports"
)

// T9 (ISSUE-009). Students stores a chosen entry date as 00:00 in Morelia, and
// a default one as the instant of the alta. Finance reads the DAY the pupil
// joined in the school's zone (ADR 0035), never the UTC date of that instant:
// 00:00 Morelia is 06:00 UTC (same day), but an alta at 19:00 on 31 October is
// 01:00 UTC on 1 November, and the pupil still joined in October.
func joinerFixture(t *testing.T, joined time.Time) *fixture {
	t.Helper()
	f := billingFixture(t)
	f.cycles.byID[testCycle] = ports.CycleRef{ID: testCycle, StartsOn: aug(2026, time.August, 24)}
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{{
		ID: "enr-join", StudentID: "stu-join", GroupID: testGroup, CycleID: testCycle,
		Status: "active", EnrolledAt: joined,
	}}
	return f
}

func moreliaMidnight(y int, m time.Month, d int) time.Time {
	// .UTC(): what the students RPC hands over (Timestamp.AsTime is UTC).
	return time.Date(y, m, d, 0, 0, 0, 0, schoolZone).UTC()
}

func TestAJoinerOwesTuitionFromTheirSchoolDayMonth(t *testing.T) {
	cases := []struct {
		name       string
		joined     time.Time
		runOn      string
		wantMonths int
		wantFirst  string // first month that must be billed
		notBilled  []string
	}{
		{"entry on the cycle's first day owes September on", moreliaMidnight(2026, time.August, 24), "2026-10-16", 10, "septiembre", []string{"agosto"}},
		{"entry on 15 October owes October on", moreliaMidnight(2026, time.October, 15), "2026-10-16", 9, "octubre", []string{"septiembre"}},
		{"an alta at 19:00 on 31 October joined in October", time.Date(2026, time.October, 31, 19, 0, 0, 0, schoolZone).UTC(), "2026-11-02", 9, "octubre", []string{"septiembre"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := joinerFixture(t, tc.joined)

			got := runOn(t, f, tc.runOn)

			if got.TuitionCreated != tc.wantMonths {
				t.Errorf("tuition = %d, want %d", got.TuitionCreated, tc.wantMonths)
			}
			issued := issuedTo(t, f, "stu-join")
			if !anyContains(issued, tc.wantFirst) {
				t.Errorf("no %s charge in %v", tc.wantFirst, keys(issued))
			}
			for _, month := range tc.notBilled {
				if anyContains(issued, month) {
					t.Errorf("%s was billed to a pupil who joined on %s", month, tc.joined)
				}
			}
		})
	}
}

// The month they join is due no earlier than their own day — the DATE, at
// midnight like every stored finance date, not 06:00 UTC of it.
func TestAJoinersFirstMonthFallsDueOnTheirEntryDay(t *testing.T) {
	f := joinerFixture(t, moreliaMidnight(2026, time.October, 15))

	runOn(t, f, "2026-10-16")

	list, err := allCharges(context.Background(), f.svc, "stu-join", testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	for _, c := range list {
		if strings.Contains(c.Charge.Description(), "octubre") {
			if want := aug(2026, time.October, 15); !c.Charge.DueDate().Equal(want) {
				t.Errorf("October due %s, want %s", c.Charge.DueDate(), want)
			}
			return
		}
	}
	t.Fatal("no October charge")
}

func anyContains(m map[string]int64, sub string) bool {
	for k := range m {
		if strings.Contains(k, sub) {
			return true
		}
	}
	return false
}

func keys(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
