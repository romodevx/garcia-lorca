//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// These tests' own cycles, so other suites' enrollments never answer.
const (
	moveCycle      = "c0000000-0000-0000-0000-0000000001b0"
	moveOtherCycle = "c0000000-0000-0000-0000-0000000001b1"
)

var (
	moveCycleStart = time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC) // 00:00 in Morelia
	moveImportDay  = time.Date(2026, 10, 6, 15, 30, 0, 0, time.UTC)
	moveWithdrawn  = time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
)

// importedAt stores one imported pupil whose enrollment in cycle starts at `at`.
func importedAt(t *testing.T, tenant, id, enrollmentID, curp, cycle string, at time.Time) {
	t.Helper()
	cmd := importCommand(t, tenant, id, enrollmentID, curp, "")
	e, err := domain.NewEnrollment(enrollmentID, tenant, id, cycle, importGroup, "", domain.EnrollmentStatusActive, at)
	require.NoError(t, err)
	cmd.Enrollment = e.WithOrigin(domain.EnrollmentOriginImport)
	_, err = appRepo(t, tenant).ImportStudent(context.Background(), cmd)
	require.NoError(t, err)
}

func enrolledAtOf(t *testing.T, enrollmentID string) time.Time {
	t.Helper()
	var at time.Time
	require.NoError(t, superDB.Raw(`SELECT enrolled_at FROM enrollments WHERE id = ?`, enrollmentID).Scan(&at).Error)
	return at
}

// Owner decision 2026-10-17: an earlier import dated on its own day moves to
// the cycle's first day — and only that: a manual enrollment, another cycle, an
// import already at the start, and a withdrawn one's end date are untouched.
func TestMoveImportedToCycleStartMovesOnlyLaterImportsOfTheCycle(t *testing.T) {
	ctx := context.Background()
	const (
		late      = "aaaaaaaa-1b00-0000-0000-000000000001"
		atStart   = "aaaaaaaa-1b00-0000-0000-000000000002"
		other     = "aaaaaaaa-1b00-0000-0000-000000000003"
		manual    = "aaaaaaaa-1b00-0000-0000-000000000004"
		withdrawn = "aaaaaaaa-1b00-0000-0000-000000000005"
		lateEnr   = "e1b00000-0000-0000-0000-000000000001"
		startEnr  = "e1b00000-0000-0000-0000-000000000002"
		otherEnr  = "e1b00000-0000-0000-0000-000000000003"
		manualEnr = "e1b00000-0000-0000-0000-000000000004"
		wdEnr     = "e1b00000-0000-0000-0000-000000000005"
	)
	importedAt(t, tenantA, late, lateEnr, "IAMA100312MDFRNN01", moveCycle, moveImportDay)
	importedAt(t, tenantA, atStart, startEnr, "IAMB100312MDFRNN02", moveCycle, moveCycleStart)
	importedAt(t, tenantA, other, otherEnr, "IAMC100312MDFRNN03", moveOtherCycle, moveImportDay)
	importedAt(t, tenantA, withdrawn, wdEnr, "IAMD100312MDFRNN04", moveCycle, moveImportDay)
	wdEndedAt := time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC)
	require.NoError(t, superDB.Exec(`UPDATE enrollments SET status = 'withdrawn', withdrawn_on = ?, updated_at = ? WHERE id = ?`,
		moveWithdrawn, wdEndedAt, wdEnr).Error)
	seedStudent(t, tenantA, manual, "IAME100312MDFRNN05")
	repo := appRepo(t, tenantA)
	manualE, err := domain.NewEnrollment(manualEnr, tenantA, manual, moveCycle, importGroup, "", domain.EnrollmentStatusActive, moveImportDay)
	require.NoError(t, err)
	require.NoError(t, repo.CreateEnrollment(ctx, manualE, roomFor(manualE)))

	move := ports.CycleStartMove{CycleID: moveCycle, StartsAt: moveCycleStart, StudentIDs: []string{late, atStart, other, manual, withdrawn}}

	dry := move
	dry.DryRun = true
	wouldMove, err := repo.MoveImportedToCycleStart(ctx, dry)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{late, withdrawn}, wouldMove)
	assert.True(t, enrolledAtOf(t, lateEnr).Equal(moveImportDay), "a dry run wrote")

	moved, err := repo.MoveImportedToCycleStart(ctx, move)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{late, withdrawn}, moved)

	assert.True(t, enrolledAtOf(t, lateEnr).Equal(moveCycleStart), "the late import was not moved")
	assert.True(t, enrolledAtOf(t, wdEnr).Equal(moveCycleStart), "the withdrawn import was not moved")
	assert.True(t, enrolledAtOf(t, startEnr).Equal(moveCycleStart))
	assert.True(t, enrolledAtOf(t, otherEnr).Equal(moveImportDay), "another cycle's import moved")
	assert.True(t, enrolledAtOf(t, manualEnr).Equal(moveImportDay), "a manual enrollment moved")
	var ended time.Time
	require.NoError(t, superDB.Raw(`SELECT withdrawn_on FROM enrollments WHERE id = ?`, wdEnr).Scan(&ended).Error)
	assert.True(t, ended.Equal(moveWithdrawn), "the withdrawn import's end date changed: %v", ended)
	// A non-active enrollment's END INSTANT is its updated_at for a transfer
	// (enrollmentToDomain): the move never rewrites updated_at on one.
	var updated time.Time
	require.NoError(t, superDB.Raw(`SELECT updated_at FROM enrollments WHERE id = ?`, wdEnr).Scan(&updated).Error)
	assert.True(t, updated.Equal(wdEndedAt), "the withdrawn import's updated_at moved: %v, want %v", updated, wdEndedAt)

	again, err := repo.MoveImportedToCycleStart(ctx, move)
	require.NoError(t, err)
	assert.Empty(t, again, "a second run moved somebody")
}

// Finance needs the day the import brought a pupil in, now that enrolled_at is
// the cycle start: an import enrollment reads it from created_at — which the
// move to the cycle start never changes — and a manual one carries none.
func TestAnImportEnrollmentCarriesWhenItWasImported(t *testing.T) {
	ctx := context.Background()
	const id, enr = "aaaaaaaa-1b00-0000-0000-000000000020", "e1b00000-0000-0000-0000-000000000020"
	before := time.Now().Add(-time.Minute)
	importedAt(t, tenantA, id, enr, "IAMG100312MDFRNN07", moveCycle, moveImportDay)
	repo := appRepo(t, tenantA)
	_, err := repo.MoveImportedToCycleStart(ctx, ports.CycleStartMove{CycleID: moveCycle, StartsAt: moveCycleStart, StudentIDs: []string{id}})
	require.NoError(t, err)

	rows, err := repo.ListEnrollments(ctx, id, moveCycle, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].EnrolledAt().Equal(moveCycleStart))
	assert.True(t, rows[0].ImportedAt().After(before), "imported_at = %v; want the insert's instant", rows[0].ImportedAt())

	const manual, manualEnr = "aaaaaaaa-1b00-0000-0000-000000000021", "e1b00000-0000-0000-0000-000000000021"
	seedStudent(t, tenantA, manual, "IAMH100312MDFRNN08")
	e, err := domain.NewEnrollment(manualEnr, tenantA, manual, moveCycle, importGroup, "", domain.EnrollmentStatusActive, moveImportDay)
	require.NoError(t, err)
	require.NoError(t, repo.CreateEnrollment(ctx, e, roomFor(e)))
	got, err := repo.ListEnrollments(ctx, manual, moveCycle, "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, got[0].ImportedAt().IsZero(), "a manual enrollment carries an import instant")
}

// RLS: one school cannot move another's enrollments.
func TestMoveImportedToCycleStartIsTenantScoped(t *testing.T) {
	ctx := context.Background()
	const id, enr = "aaaaaaaa-1b00-0000-0000-000000000010", "e1b00000-0000-0000-0000-000000000010"
	importedAt(t, tenantA, id, enr, "IAMF100312MDFRNN06", moveCycle, moveImportDay)

	moved, err := appRepo(t, tenantB).MoveImportedToCycleStart(ctx,
		ports.CycleStartMove{CycleID: moveCycle, StartsAt: moveCycleStart, StudentIDs: []string{id}})

	require.NoError(t, err)
	assert.Empty(t, moved)
	assert.True(t, enrolledAtOf(t, enr).Equal(moveImportDay), "tenant B moved tenant A's enrollment (RLS breach)")
}

func TestMoveImportedToCycleStartWithNobodyAsksNothing(t *testing.T) {
	moved, err := appRepo(t, tenantA).MoveImportedToCycleStart(context.Background(),
		ports.CycleStartMove{CycleID: moveCycle, StartsAt: moveCycleStart})
	require.NoError(t, err)
	assert.Empty(t, moved)
}
