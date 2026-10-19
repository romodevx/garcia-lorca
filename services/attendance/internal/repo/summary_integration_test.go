//go:build integration

package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// T12 fixtures: their own group and enrollments (the container is shared).
const (
	groupReport      = "90000000-0000-0000-0000-000000001201"
	groupReportOther = "90000000-0000-0000-0000-000000001202"

	enrollmentReportAna  = "e0000000-0000-0000-0000-000000001201"
	enrollmentReportBeto = "e0000000-0000-0000-0000-000000001202"
	enrollmentReportCruz = "e0000000-0000-0000-0000-000000001203"
)

// The report reads the group's lists in the range (daily and per-class, one
// entry each) and the pupils' exceptions on them: a legacy mark (no group)
// comes along, a mark another group's list holds does not, nor anything
// outside the range or of another tenant.
func TestGroupHistoryReadsTheGroupsListsAndMarks(t *testing.T) {
	ctx := context.Background()
	a := appRepo(t, tenantA)
	scope := []string{enrollmentReportAna, enrollmentReportBeto, enrollmentReportCruz}
	list := func(group, date, subject string) service.ListKey {
		return service.ListKey{GroupID: group, Date: day(t, date), SubjectID: subject}
	}
	saveList(t, a, list(groupReport, "2026-09-01", ""), scope, testTaker,
		subjectMark("a0000000-0000-0000-0000-000000001201", enrollmentReportAna, day(t, "2026-09-01"), "", "absent"),
		subjectMark("a0000000-0000-0000-0000-000000001202", enrollmentReportBeto, day(t, "2026-09-01"), "", "late"))
	saveList(t, a, list(groupReport, "2026-09-02", ""), scope, testTaker,
		subjectMark("a0000000-0000-0000-0000-000000001203", enrollmentReportAna, day(t, "2026-09-02"), "", "late"))
	saveList(t, a, list(groupReport, "2026-09-03", subjectMath), scope, testTaker,
		subjectMark("a0000000-0000-0000-0000-000000001204", enrollmentReportAna, day(t, "2026-09-03"), subjectMath, "excused"))
	saveList(t, a, list(groupReport, "2026-10-01", ""), scope, testTaker, // outside the range
		subjectMark("a0000000-0000-0000-0000-000000001205", enrollmentReportAna, day(t, "2026-10-01"), "", "absent"))
	saveList(t, a, list(groupReportOther, "2026-09-02", ""), []string{enrollmentReportCruz}, testTaker, // held by another group
		subjectMark("a0000000-0000-0000-0000-000000001206", enrollmentReportCruz, day(t, "2026-09-02"), "", "absent"))
	require.NoError(t, superDB.Exec( // a legacy mark: stored before lists carried their group
		"INSERT INTO attendance (id, tenant_id, enrollment_id, date, status) VALUES (?, ?, ?, '2026-09-04', 'absent')",
		"a0000000-0000-0000-0000-000000001207", tenantA, enrollmentReportBeto).Error)
	b := appRepo(t, tenantB)
	saveList(t, b, list(groupReport, "2026-09-05", ""), scope, testTaker,
		subjectMark("a0000000-0000-0000-0000-000000001208", enrollmentReportAna, day(t, "2026-09-05"), "", "absent"))

	got, err := a.GroupHistory(ctx, groupReport, scope, day(t, "2026-09-01"), day(t, "2026-09-30"))
	require.NoError(t, err)

	assert.Equal(t, []time.Time{day(t, "2026-09-01"), day(t, "2026-09-02"), day(t, "2026-09-03")}, utcDays(got.Lists))
	assert.Equal(t, []service.DayMark{
		{EnrollmentID: enrollmentReportAna, Date: day(t, "2026-09-01"), Status: "absent"},
		{EnrollmentID: enrollmentReportAna, Date: day(t, "2026-09-02"), Status: "late"},
		{EnrollmentID: enrollmentReportAna, Date: day(t, "2026-09-03"), Status: "excused"},
		{EnrollmentID: enrollmentReportBeto, Date: day(t, "2026-09-01"), Status: "late"},
		{EnrollmentID: enrollmentReportBeto, Date: day(t, "2026-09-04"), Status: "absent"},
	}, got.Marks)
	assert.Equal(t, []service.DayMark{
		{EnrollmentID: enrollmentReportCruz, Date: day(t, "2026-09-02"), Status: "absent"},
	}, got.Elsewhere, "another group's mark comes back apart: that day is that group's (C2-011)")

	gotB, err := b.GroupHistory(ctx, groupReport, scope, day(t, "2026-09-01"), day(t, "2026-09-30"))
	require.NoError(t, err)
	assert.Len(t, gotB.Lists, 1, "tenant B reads only its own list")
	assert.Equal(t, []service.DayMark{{EnrollmentID: enrollmentReportAna, Date: day(t, "2026-09-05"), Status: "absent"}}, gotB.Marks)
}

func TestGroupHistoryWithoutPupilsReadsOnlyLists(t *testing.T) {
	got, err := appRepo(t, tenantA).GroupHistory(context.Background(), groupReportOther, nil, day(t, "2026-09-01"), day(t, "2026-09-30"))
	require.NoError(t, err)
	assert.Len(t, got.Lists, 1)
	assert.Empty(t, got.Marks)
}

func utcDays(ts []time.Time) []time.Time {
	out := make([]time.Time, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.UTC())
	}
	return out
}
