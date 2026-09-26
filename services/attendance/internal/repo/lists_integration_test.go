//go:build integration

package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/garcia-lorca/services/attendance/internal/repo"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// T3 fixtures: their own enrollments, groups and subjects, since the container
// is shared across tests and never reset.
const (
	enrollmentPerClass = "e0000000-0000-0000-0000-000000000101"
	enrollmentSession  = "e0000000-0000-0000-0000-000000000102"
	enrollmentNulls    = "e0000000-0000-0000-0000-000000000103"

	groupPerClass = "90000000-0000-0000-0000-000000000101"
	groupSession  = "90000000-0000-0000-0000-000000000102"
	groupRLS      = "90000000-0000-0000-0000-000000000103"
	groupInPlace  = "90000000-0000-0000-0000-000000000104"

	subjectMath    = "50000000-0000-0000-0000-00000000000a"
	subjectSpanish = "50000000-0000-0000-0000-00000000000b"
)

func saveList(t *testing.T, r *repo.Repo, key service.ListKey, scope []string, taker service.Taker, records ...service.AttendanceRecord) service.ListChanges {
	t.Helper()
	changes, err := r.ReconcileList(context.Background(), key, scope, taker,
		func(current map[string]service.AttendanceRecord) (service.ListChanges, error) {
			return service.DiffList(current, records), nil
		})
	require.NoError(t, err)
	return changes
}

func subjectMark(id, enrollmentID string, date time.Time, subjectID, status string) service.AttendanceRecord {
	return service.AttendanceRecord{ID: id, EnrollmentID: enrollmentID, Date: date, SubjectID: subjectID, Status: status}
}

// Each subject's list, and the daily list, are separate keys for the same
// student·day: saving one never touches another, and each reads back with its
// subject.
func TestPerClassListsAreIndependentKeys(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	date := day(t, "2026-09-14")
	scope := []string{enrollmentPerClass}
	math := service.ListKey{GroupID: groupPerClass, Date: date, SubjectID: subjectMath}
	spanish := service.ListKey{GroupID: groupPerClass, Date: date, SubjectID: subjectSpanish}
	daily := service.ListKey{GroupID: groupPerClass, Date: date}

	saveList(t, r, math, scope, testTaker, subjectMark("a0000000-0000-0000-0000-000000000101", enrollmentPerClass, date, subjectMath, "absent"))
	saveList(t, r, spanish, scope, testTaker, subjectMark("a0000000-0000-0000-0000-000000000102", enrollmentPerClass, date, subjectSpanish, "late"))
	saveList(t, r, daily, scope, testTaker, subjectMark("a0000000-0000-0000-0000-000000000103", enrollmentPerClass, date, "", "excused"))

	for key, want := range map[service.ListKey]string{math: "absent", spanish: "late", daily: "excused"} {
		marks, err := r.MarksForList(ctx, scope, key)
		require.NoError(t, err)
		got, ok := marks[enrollmentPerClass]
		require.True(t, ok, "list %q lost its mark", key.SubjectID)
		assert.Equal(t, want, got.Status, "list %q", key.SubjectID)
		assert.Equal(t, key.SubjectID, got.SubjectID)
	}

	// Clearing the math list leaves the other two alone.
	saveList(t, r, math, scope, testTaker)
	marks, err := r.MarksForList(ctx, scope, math)
	require.NoError(t, err)
	assert.Empty(t, marks)
	assert.Equal(t, int64(2), countMarks(t, tenantA, enrollmentPerClass))

	history, err := r.ListByEnrollment(ctx, enrollmentPerClass, date, date)
	require.NoError(t, err)
	subjects := map[string]bool{}
	for _, rec := range history {
		subjects[rec.SubjectID] = true
	}
	assert.Equal(t, map[string]bool{"": true, subjectSpanish: true}, subjects, "history carries each mark's subject")
}

// NULLS NOT DISTINCT: two daily-list rows for one student·day collide even when
// written raw, so the daily list still holds one mark per student per day.
func TestDailyListKeyTreatsNullSubjectsAsEqual(t *testing.T) {
	insert := func(id string) error {
		return superDB.Exec(
			"INSERT INTO attendance (id, tenant_id, enrollment_id, date, status) VALUES (?, ?, ?, '2026-09-15', 'absent')",
			id, tenantA, enrollmentNulls).Error
	}
	require.NoError(t, insert("a0000000-0000-0000-0000-000000000201"))
	assert.Error(t, insert("a0000000-0000-0000-0000-000000000202"), "a second daily row must violate the key")

	var n int64
	require.NoError(t, superDB.Raw(
		"SELECT count(*) FROM pg_constraint WHERE conname = 'attendance_tenant_id_enrollment_id_date_key'").Scan(&n).Error)
	assert.Zero(t, n, "the old (tenant, enrollment, date) key must be gone")
}

// A saved list records its session; an all-present save counts as taken; a
// correction by someone else keeps the first taker and bumps updated_at.
func TestReconcileListRecordsTheSession(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	date := day(t, "2026-09-16")
	key := service.ListKey{GroupID: groupSession, Date: date}
	scope := []string{enrollmentSession}

	_, found, err := r.SessionFor(ctx, key)
	require.NoError(t, err)
	assert.False(t, found, "an untaken list has no session")

	saveList(t, r, key, scope, testTaker) // everybody present
	first, found, err := r.SessionFor(ctx, key)
	require.NoError(t, err)
	require.True(t, found, "an all-present list that was saved is taken")
	assert.Equal(t, "Gabriela Rojas", first.TakenByName)
	assert.False(t, first.TakenAt.IsZero())

	director := service.Taker{UserID: "80000000-0000-0000-0000-000000000002", Name: "Dirección"}
	saveList(t, r, key, scope, director, mark("a0000000-0000-0000-0000-000000000301", enrollmentSession, date, "absent", ""))
	second, found, err := r.SessionFor(ctx, key)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "Gabriela Rojas", second.TakenByName, "the first taker is kept")
	assert.True(t, second.TakenAt.Equal(first.TakenAt), "taken_at is the first save")
	assert.True(t, second.UpdatedAt.After(first.UpdatedAt), "a correction bumps updated_at")

	// The per-class list of the same group·day is a different session.
	_, found, err = r.SessionFor(ctx, service.ListKey{GroupID: groupSession, Date: date, SubjectID: subjectMath})
	require.NoError(t, err)
	assert.False(t, found)
}

// A correction updates the stored row in place: its id survives, and the
// reconcile reports exactly what it changed.
func TestReconcileListUpdatesInPlace(t *testing.T) {
	r := appRepo(t, tenantA)
	date := day(t, "2026-09-17")
	key := service.ListKey{GroupID: groupInPlace, Date: date}
	scope := []string{enrollmentSession}

	saveList(t, r, key, scope, testTaker, mark("a0000000-0000-0000-0000-000000000401", enrollmentSession, date, "absent", ""))
	changes := saveList(t, r, key, scope, testTaker, mark("a0000000-0000-0000-0000-000000000402", enrollmentSession, date, "excused", "receta"))
	require.Len(t, changes.Updates, 1)
	assert.True(t, changes.Updates[0].StatusChanged)

	marks, err := r.MarksForList(context.Background(), scope, key)
	require.NoError(t, err)
	got := marks[enrollmentSession]
	assert.Equal(t, "a0000000-0000-0000-0000-000000000401", got.ID, "the stored id survives a correction")
	assert.Equal(t, "excused", got.Status)
	assert.Equal(t, "receta", got.Note)
	assert.Equal(t, int64(1), countSessionRows(t, tenantA, groupInPlace))
}

// RLS on attendance_sessions: another tenant neither sees nor overwrites a
// session, and an unbound session reads nothing.
func TestRLSIsolatesSessions(t *testing.T) {
	ctx := context.Background()
	date := day(t, "2026-09-18")
	key := service.ListKey{GroupID: groupRLS, Date: date}

	saveList(t, appRepo(t, tenantB), key, nil, service.Taker{UserID: testTaker.UserID, Name: "B"})

	_, found, err := appRepo(t, tenantA).SessionFor(ctx, key)
	require.NoError(t, err)
	assert.False(t, found, "tenant A saw tenant B's session (RLS read breach)")

	saveList(t, appRepo(t, tenantA), key, nil, service.Taker{UserID: testTaker.UserID, Name: "A"})
	gotB, found, err := appRepo(t, tenantB).SessionFor(ctx, key)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "B", gotB.TakenByName, "tenant A's save leaked into tenant B's session")
	assert.Equal(t, int64(1), countSessionRows(t, tenantA, groupRLS))
	assert.Equal(t, int64(1), countSessionRows(t, tenantB, groupRLS))

	db, err := open(appDSN)
	require.NoError(t, err)
	var n int64
	require.NoError(t, db.Table("attendance_sessions").Count(&n).Error)
	assert.Zero(t, n, "an unbound session must read no sessions (RLS fails closed)")

	// WITH CHECK: a tenant cannot write a session row for another tenant.
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO attendance_sessions (tenant_id, group_id, date, taken_by, taken_by_name) VALUES (?, ?, '2026-09-19', ?, 'x')",
			tenantB, groupRLS, testTaker.UserID).Error
	})
	assert.Error(t, err, "RLS WITH CHECK must reject a cross-tenant session row")
}

func countSessionRows(t *testing.T, tenant, group string) int64 {
	t.Helper()
	db, err := open(appDSN)
	require.NoError(t, err)
	var n int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Table("attendance_sessions").Where("group_id = ?", group).Count(&n).Error
	})
	require.NoError(t, err)
	return n
}
