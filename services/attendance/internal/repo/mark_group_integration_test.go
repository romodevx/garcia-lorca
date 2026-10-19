//go:build integration

package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Review #4 fixtures (the container is shared and never reset).
const (
	groupBeforeTransfer = "90000000-0000-0000-0000-000000000401"
	groupAfterTransfer  = "90000000-0000-0000-0000-000000000402"
	enrollmentMoved     = "e0000000-0000-0000-0000-000000000401"
	enrollmentLegacy    = "e0000000-0000-0000-0000-000000000402"
)

func markGroup(t *testing.T, id string) *string {
	t.Helper()
	var group *string
	require.NoError(t, superDB.Raw("SELECT group_id::text FROM attendance WHERE id = ?", id).Scan(&group).Error)
	return group
}

// A mark belongs to the list that stored it. The pupil is absent in A on
// Monday, moves to B on Wednesday, and B's teacher edits Monday: B neither sees
// nor clears A's absence (no correction is published), and a mark B submits
// for that pupil·day is not stored over it.
func TestAnotherGroupsListNeverTouchesAStoredMark(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	monday := day(t, "2026-09-21")
	scope := []string{enrollmentMoved}
	listA := service.ListKey{GroupID: groupBeforeTransfer, Date: monday}
	listB := service.ListKey{GroupID: groupAfterTransfer, Date: monday}

	stored := saveList(t, r, listA, scope, testTaker, subjectMark("a0000000-0000-0000-0000-000000001401", enrollmentMoved, monday, "", "absent"))
	require.Len(t, stored.Inserts, 1)
	require.NotNil(t, markGroup(t, "a0000000-0000-0000-0000-000000001401"))
	assert.Equal(t, groupBeforeTransfer, *markGroup(t, "a0000000-0000-0000-0000-000000001401"), "a new mark stores its list's group")

	readB, err := r.MarksForList(ctx, scope, listB)
	require.NoError(t, err)
	assert.Empty(t, readB.Own, "B's list does not show A's mark")
	assert.Equal(t, map[string]bool{enrollmentMoved: true}, readB.HeldElsewhere,
		"B learns the pupil·day is A's, so its roster leaves the pupil out (C2-011)")

	cleared := saveList(t, r, listB, scope, testTaker)
	assert.Empty(t, cleared.Deletes, "B's all-present save must not clear A's absence (no correction push)")
	assert.Empty(t, cleared.Updates)

	overwritten := saveList(t, r, listB, scope, testTaker, subjectMark("a0000000-0000-0000-0000-000000001402", enrollmentMoved, monday, "", "late"))
	assert.Empty(t, overwritten.Inserts, "a pupil·day held by A is not re-marked from B")
	assert.Empty(t, overwritten.Updates)
	assert.Equal(t, []string{enrollmentMoved}, overwritten.HeldElsewhere, "the skipped mark is reported, so the save's counts exclude it")

	marksA, err := ownMarks(r.MarksForList(ctx, scope, listA))
	require.NoError(t, err)
	require.Contains(t, marksA, enrollmentMoved)
	assert.Equal(t, "absent", marksA[enrollmentMoved].Status, "A's absence is untouched")
	assert.Equal(t, int64(1), marksA[enrollmentMoved].Revision)
}

// A mark stored before 0004 has no group: any list of its day may read and
// correct it, and the correction stamps the correcting list's group.
func TestALegacyMarkBelongsToEveryList(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	const legacyID = "a0000000-0000-0000-0000-000000001403"
	require.NoError(t, superDB.Exec(
		"INSERT INTO attendance (id, tenant_id, enrollment_id, date, status) VALUES (?, ?, ?, '2026-09-22', 'absent')",
		legacyID, tenantA, enrollmentLegacy).Error)
	list := service.ListKey{GroupID: groupAfterTransfer, Date: day(t, "2026-09-22")}
	scope := []string{enrollmentLegacy}

	marks, err := ownMarks(r.MarksForList(ctx, scope, list))
	require.NoError(t, err)
	require.Contains(t, marks, enrollmentLegacy)

	changes := saveList(t, r, list, scope, testTaker, subjectMark("a0000000-0000-0000-0000-000000001404", enrollmentLegacy, day(t, "2026-09-22"), "", "late"))
	require.Len(t, changes.Updates, 1)
	assert.Equal(t, legacyID, changes.Updates[0].Record.ID)
	require.NotNil(t, markGroup(t, legacyID))
	assert.Equal(t, groupAfterTransfer, *markGroup(t, legacyID))
}
