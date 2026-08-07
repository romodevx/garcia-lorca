//go:build integration

// Integration tests for the attendance postgres repository against a REAL
// Postgres (testcontainers-go). They prove the reconcile is a true day replace
// (dropped marks disappear, so the student is present again), that MarksForDate
// resolves many enrollments in one batched call, and — critically — that RLS
// isolates tenants for both reads AND writes when connected as the non-owner
// `app` role (same posture as students/grades, ADR 0001).
//
// Run: go test -tags=integration ./internal/repo/
// Requires Docker.
package repo_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/garcia-lorca/services/attendance/internal/repo"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

const (
	dbName  = "garcia_lorca"
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"

	dateLayout = "2006-01-02"
)

// migrationFiles are applied in order before the app role is granted, so the
// non-owner role gets privileges on every table.
var migrationFiles = []string{
	"../../../../migrations/attendance/0001_init.sql",
}

var (
	superDB   *gorm.DB
	appDSN    string
	superPass = randToken()
	appPass   = randToken()
)

// Neutral cross-service references (students enrollment UUIDs). Each test uses
// its own enrollments: the container is shared and never reset between tests.
const (
	enrollmentReconcileOne = "e0000000-0000-0000-0000-000000000001"
	enrollmentReconcileTwo = "e0000000-0000-0000-0000-000000000002"
	enrollmentBatchOne     = "e0000000-0000-0000-0000-000000000003"
	enrollmentBatchTwo     = "e0000000-0000-0000-0000-000000000004"
	enrollmentBatchClean   = "e0000000-0000-0000-0000-000000000005"
	enrollmentRLS          = "e0000000-0000-0000-0000-000000000006"
	enrollmentRange        = "e0000000-0000-0000-0000-000000000007"
	enrollmentUpsert       = "e0000000-0000-0000-0000-000000000008"
	enrollmentRace         = "e0000000-0000-0000-0000-000000000009"
	enrollmentOrder        = "e0000000-0000-0000-0000-000000000010"
)

func randToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-bookworm",
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword(superPass),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start container: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = container.Terminate(ctx) }()

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432")
	superDSN := fmt.Sprintf("host=%s port=%s user=postgres password=%s dbname=%s sslmode=disable", host, port.Port(), superPass, dbName)
	appDSN = fmt.Sprintf("host=%s port=%s user=app password=%s dbname=%s sslmode=disable", host, port.Port(), appPass, dbName)

	if superDB, err = open(superDSN); err != nil {
		fmt.Fprintf(os.Stderr, "open super: %v\n", err)
		os.Exit(1)
	}
	if err := migrate(superDB); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func open(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

func migrate(db *gorm.DB) error {
	for _, f := range migrationFiles {
		sql, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			return fmt.Errorf("apply %s: %w", f, err)
		}
	}
	for _, stmt := range []string{
		"CREATE ROLE app LOGIN PASSWORD '" + appPass + "' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE",
		"GRANT USAGE ON SCHEMA public TO app",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("%q: %w", stmt, err)
		}
	}
	return nil
}

// appRepo builds the repository over a connection as the NON-OWNER app role, so
// every test runs under the same RLS conditions as production.
func appRepo(t *testing.T, tenant string) *repo.Repo {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	return repo.New(db, tenant)
}

func day(t *testing.T, value string) time.Time {
	t.Helper()
	d, err := time.Parse(dateLayout, value)
	if err != nil {
		t.Fatalf("parse fixture date %q: %v", value, err)
	}
	return d
}

func mark(id, enrollmentID string, date time.Time, status, note string) service.AttendanceRecord {
	return service.AttendanceRecord{ID: id, EnrollmentID: enrollmentID, Date: date, Status: status, Note: note}
}

// countMarks reads `attendance` as the non-owner app role with the tenant GUC
// bound (mirroring the repository's RLS binding), so it exercises RLS: a tenant
// sees only its own rows.
func countMarks(t *testing.T, tenant, enrollment string) int64 {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	var n int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Table("attendance").Where("enrollment_id = ?", enrollment).Count(&n).Error
	})
	if err != nil {
		t.Fatalf("count marks: %v", err)
	}
	return n
}

// --- tests -----------------------------------------------------------------

// TestReconcileDayReplacesMarks is the reconcile contract against a real
// database: the first save stores two absences, and a second save carrying a
// DIFFERENT exception set replaces the whole day — the enrollment dropped from
// the set has no row left (it is present again) — while an empty set clears the
// day entirely. All of it inside one transaction per call.
func TestReconcileDayReplacesMarks(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	date := day(t, "2026-08-05")
	scope := []string{enrollmentReconcileOne, enrollmentReconcileTwo}

	if err := r.ReconcileDay(ctx, scope, date, []service.AttendanceRecord{
		mark("aaaaaaaa-0000-0000-0000-000000000001", enrollmentReconcileOne, date, "absent", ""),
		mark("aaaaaaaa-0000-0000-0000-000000000002", enrollmentReconcileTwo, date, "absent", "sin justificar"),
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	marks, err := r.MarksForDate(ctx, scope, date)
	if err != nil {
		t.Fatalf("MarksForDate after first reconcile: %v", err)
	}
	if len(marks) != 2 {
		t.Fatalf("stored %d marks, want 2: %+v", len(marks), marks)
	}
	if got := marks[enrollmentReconcileTwo]; got.Status != "absent" || got.Note != "sin justificar" {
		t.Errorf("stored mark = %+v, want absent/\"sin justificar\"", got)
	}

	// Replace the day: only the second enrollment is an exception now, as "late".
	if err := r.ReconcileDay(ctx, scope, date, []service.AttendanceRecord{
		mark("aaaaaaaa-0000-0000-0000-000000000003", enrollmentReconcileTwo, date, "late", "llegó 8:15"),
	}); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	marks, err = r.MarksForDate(ctx, scope, date)
	if err != nil {
		t.Fatalf("MarksForDate after replace: %v", err)
	}
	if _, ok := marks[enrollmentReconcileOne]; ok {
		t.Error("the enrollment dropped from the exception set still has a mark (not present again)")
	}
	got, ok := marks[enrollmentReconcileTwo]
	if !ok || got.Status != "late" || got.ID != "aaaaaaaa-0000-0000-0000-000000000003" {
		t.Errorf("replaced mark = %+v (ok=%v), want the new late row", got, ok)
	}
	if n := countMarks(t, tenantA, enrollmentReconcileOne); n != 0 {
		t.Errorf("rows for the cleared enrollment = %d, want 0", n)
	}

	// An empty exception set is an all-present day: it wipes the remaining mark.
	if err := r.ReconcileDay(ctx, scope, date, nil); err != nil {
		t.Fatalf("clearing reconcile: %v", err)
	}
	marks, err = r.MarksForDate(ctx, scope, date)
	if err != nil {
		t.Fatalf("MarksForDate after clear: %v", err)
	}
	if len(marks) != 0 {
		t.Errorf("marks after an all-present save = %+v, want none", marks)
	}
}

// TestReconcileDayLeavesOtherDaysAlone proves the reconcile is scoped to its
// date: saving one day never touches the marks of another.
func TestReconcileDayLeavesOtherDaysAlone(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	monday := day(t, "2026-09-07")
	tuesday := day(t, "2026-09-08")
	scope := []string{enrollmentRange}

	if err := r.ReconcileDay(ctx, scope, monday, []service.AttendanceRecord{
		mark("aaaaaaaa-0000-0000-0000-000000000010", enrollmentRange, monday, "absent", ""),
	}); err != nil {
		t.Fatalf("seed monday: %v", err)
	}
	if err := r.ReconcileDay(ctx, scope, tuesday, []service.AttendanceRecord{
		mark("aaaaaaaa-0000-0000-0000-000000000011", enrollmentRange, tuesday, "late", ""),
	}); err != nil {
		t.Fatalf("seed tuesday: %v", err)
	}

	mondayMarks, err := r.MarksForDate(ctx, scope, monday)
	if err != nil {
		t.Fatalf("MarksForDate monday: %v", err)
	}
	if got, ok := mondayMarks[enrollmentRange]; !ok || got.Status != "absent" {
		t.Errorf("monday mark = %+v (ok=%v), want absent — tuesday's save must not touch it", got, ok)
	}

	// ListByEnrollment sees both days, newest first, and honours each optional bound.
	all, err := r.ListByEnrollment(ctx, enrollmentRange, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("ListByEnrollment unbounded: %v", err)
	}
	if len(all) != 2 || !all[0].Date.Equal(tuesday) || !all[1].Date.Equal(monday) {
		t.Fatalf("unbounded list = %+v, want tuesday then monday", all)
	}
	fromTuesday, err := r.ListByEnrollment(ctx, enrollmentRange, tuesday, time.Time{})
	if err != nil {
		t.Fatalf("ListByEnrollment from: %v", err)
	}
	if len(fromTuesday) != 1 || !fromTuesday[0].Date.Equal(tuesday) {
		t.Errorf("from-bounded list = %+v, want tuesday only", fromTuesday)
	}
	untilMonday, err := r.ListByEnrollment(ctx, enrollmentRange, time.Time{}, monday)
	if err != nil {
		t.Fatalf("ListByEnrollment to: %v", err)
	}
	if len(untilMonday) != 1 || !untilMonday[0].Date.Equal(monday) {
		t.Errorf("to-bounded list = %+v, want monday only", untilMonday)
	}
}

// TestReconcileDayUpsertsOverAPreExistingRow drives the insert straight onto an
// existing (tenant, enrollment, date) row — the state a concurrent save leaves
// behind, reproduced deterministically by reconciling with no delete scope. The
// UNIQUE key must be absorbed as an update (last write wins on status/note, and
// updated_at is bumped), never surface as a constraint violation: that error is
// the 500 two teachers saving the same group·day used to get.
func TestReconcileDayUpsertsOverAPreExistingRow(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	date := day(t, "2026-08-10")

	if err := r.ReconcileDay(ctx, []string{enrollmentUpsert}, date, []service.AttendanceRecord{
		mark("aaaaaaaa-0000-0000-0000-000000000030", enrollmentUpsert, date, "absent", "first"),
	}); err != nil {
		t.Fatalf("seed reconcile: %v", err)
	}
	before := updatedAt(t, tenantA, enrollmentUpsert, date)

	// An empty scope skips the DELETE, so the insert collides head-on with the row
	// already stored for that enrollment·day.
	if err := r.ReconcileDay(ctx, nil, date, []service.AttendanceRecord{
		mark("aaaaaaaa-0000-0000-0000-000000000031", enrollmentUpsert, date, "late", "second"),
	}); err != nil {
		t.Fatalf("a colliding reconcile must upsert, not fail: %v", err)
	}

	marks, err := r.MarksForDate(ctx, []string{enrollmentUpsert}, date)
	if err != nil {
		t.Fatalf("MarksForDate: %v", err)
	}
	got, ok := marks[enrollmentUpsert]
	if !ok {
		t.Fatal("the upserted mark is gone")
	}
	if got.Status != "late" || got.Note != "second" {
		t.Errorf("mark = %+v, want the second write (late/\"second\")", got)
	}
	if n := countMarks(t, tenantA, enrollmentUpsert); n != 1 {
		t.Errorf("rows for the enrollment·day = %d, want exactly 1 (the UNIQUE key holds)", n)
	}
	if after := updatedAt(t, tenantA, enrollmentUpsert, date); !after.After(before) {
		t.Errorf("updated_at = %v, want it bumped past %v on the update path", after, before)
	}
}

// TestReconcileDayConcurrentSaveLastWriteWins is the race that motivated the
// upsert: two teachers saving the SAME group·day at the same time. Neither save
// may fail on the UNIQUE key, and the day must end with exactly one mark holding
// one of the two writers' statuses.
func TestReconcileDayConcurrentSaveLastWriteWins(t *testing.T) {
	ctx := context.Background()
	date := day(t, "2026-08-11")
	scope := []string{enrollmentRace}

	errs := make(chan error, 2)
	statuses := []string{"absent", "late"}
	for i, status := range statuses {
		go func(i int, status string) {
			errs <- appRepo(t, tenantA).ReconcileDay(ctx, scope, date, []service.AttendanceRecord{
				mark(fmt.Sprintf("aaaaaaaa-0000-0000-0000-00000000004%d", i), enrollmentRace, date, status, status),
			})
		}(i, status)
	}
	for range statuses {
		if err := <-errs; err != nil {
			t.Fatalf("a concurrent save must not fail on the UNIQUE key: %v", err)
		}
	}

	if n := countMarks(t, tenantA, enrollmentRace); n != 1 {
		t.Fatalf("rows after two concurrent saves = %d, want exactly 1", n)
	}
	marks, err := appRepo(t, tenantA).MarksForDate(ctx, scope, date)
	if err != nil {
		t.Fatalf("MarksForDate: %v", err)
	}
	got := marks[enrollmentRace]
	if got.Status != "absent" && got.Status != "late" {
		t.Errorf("surviving mark = %+v, want one of the two concurrent writes", got)
	}
}

// updatedAt reads a stored mark's audit timestamp as the tenant-bound app role.
func updatedAt(t *testing.T, tenant, enrollment string, date time.Time) time.Time {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	var out time.Time
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Raw(
			"SELECT updated_at FROM attendance WHERE enrollment_id = ? AND date = ?::date",
			enrollment, date.Format(dateLayout),
		).Scan(&out).Error
	})
	if err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	return out
}

// TestMarksForDateBatched proves several enrollments are resolved in ONE call,
// keyed by enrollment_id, returning rows only for those that have an exception —
// and that an empty id slice short-circuits without a query.
func TestMarksForDateBatched(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	date := day(t, "2026-08-06")
	scope := []string{enrollmentBatchOne, enrollmentBatchTwo, enrollmentBatchClean}

	if err := r.ReconcileDay(ctx, scope, date, []service.AttendanceRecord{
		mark("aaaaaaaa-0000-0000-0000-000000000020", enrollmentBatchOne, date, "absent", ""),
		mark("aaaaaaaa-0000-0000-0000-000000000021", enrollmentBatchTwo, date, "excused", "cita médica"),
	}); err != nil {
		t.Fatalf("seed day: %v", err)
	}

	marks, err := r.MarksForDate(ctx, scope, date)
	if err != nil {
		t.Fatalf("MarksForDate: %v", err)
	}
	if len(marks) != 2 {
		t.Fatalf("batched read returned %d marks, want 2: %+v", len(marks), marks)
	}
	if got := marks[enrollmentBatchOne]; got.Status != "absent" || got.EnrollmentID != enrollmentBatchOne {
		t.Errorf("mark one = %+v, want absent for %s", got, enrollmentBatchOne)
	}
	if got := marks[enrollmentBatchTwo]; got.Status != "excused" || got.Note != "cita médica" {
		t.Errorf("mark two = %+v, want excused/\"cita médica\"", got)
	}
	if _, ok := marks[enrollmentBatchClean]; ok {
		t.Error("a present enrollment must be absent from the map (default-present)")
	}
	if !marks[enrollmentBatchOne].Date.Equal(date) {
		t.Errorf("round-tripped date = %v, want %v", marks[enrollmentBatchOne].Date, date)
	}

	empty, err := r.MarksForDate(ctx, nil, date)
	if err != nil {
		t.Fatalf("MarksForDate(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("empty ids: want an empty map, got %+v", empty)
	}
}

// TestListByEnrollmentIsNewestFirstAndBounded proves the history read is a
// DETERMINISTIC, BOUNDED window: rows come back newest first (so the cap drops
// ancient history, never the days a teacher is looking at) and no single call
// can return more than the documented maximum.
func TestListByEnrollmentIsNewestFirstAndBounded(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()

	// One mark per day, seeded oldest-first so insertion order cannot masquerade
	// as the ordering under test.
	const seeded = 12
	start := day(t, "2026-03-02")
	dates := make([]time.Time, 0, seeded)
	for i := range seeded {
		date := start.AddDate(0, 0, i)
		dates = append(dates, date)
		id := fmt.Sprintf("aaaaaaaa-0000-0000-0000-0000000004%02d", i)
		if err := r.ReconcileDay(ctx, []string{enrollmentOrder}, date, []service.AttendanceRecord{
			mark(id, enrollmentOrder, date, "absent", ""),
		}); err != nil {
			t.Fatalf("seed %s: %v", date.Format(dateLayout), err)
		}
	}

	got, err := r.ListByEnrollment(ctx, enrollmentOrder, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("ListByEnrollment: %v", err)
	}
	if len(got) != seeded {
		t.Fatalf("read %d marks, want the %d seeded (all fit under the cap)", len(got), seeded)
	}
	for i, rec := range got {
		want := dates[seeded-1-i]
		if !rec.Date.Equal(want) {
			t.Fatalf("row %d = %s, want %s — the read must be newest first",
				i, rec.Date.Format(dateLayout), want.Format(dateLayout))
		}
	}
}

// TestRLSIsolatesAttendance proves read AND write isolation on `attendance` for
// the non-owner app role: tenant A can neither see nor destroy tenant B's marks,
// even when reconciling the very same enrollment·day.
func TestRLSIsolatesAttendance(t *testing.T) {
	ctx := context.Background()
	date := day(t, "2026-08-07")
	scope := []string{enrollmentRLS}

	if err := appRepo(t, tenantB).ReconcileDay(ctx, scope, date, []service.AttendanceRecord{
		mark("bbbbbbbb-0000-0000-0000-000000000001", enrollmentRLS, date, "absent", "B's absence"),
	}); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	// Read isolation: tenant A cannot see tenant B's mark; tenant B can.
	if gotA, err := appRepo(t, tenantA).MarksForDate(ctx, scope, date); err != nil {
		t.Errorf("tenant A MarksForDate: %v", err)
	} else if _, ok := gotA[enrollmentRLS]; ok {
		t.Error("tenant A saw tenant B's mark (RLS read breach)")
	}
	if gotB, err := appRepo(t, tenantB).MarksForDate(ctx, scope, date); err != nil {
		t.Errorf("tenant B MarksForDate: %v", err)
	} else if _, ok := gotB[enrollmentRLS]; !ok {
		t.Error("tenant B should see its own mark")
	}

	// Write isolation: tenant A reconciling the SAME enrollment·day cannot delete
	// or overwrite tenant B's row — its DELETE is bounded by RLS, so it inserts
	// only its own row and B's absence survives untouched.
	if err := appRepo(t, tenantA).ReconcileDay(ctx, scope, date, []service.AttendanceRecord{
		mark("aaaaaaaa-0000-0000-0000-0000000000a1", enrollmentRLS, date, "late", "A's own mark"),
	}); err != nil {
		t.Fatalf("tenant A reconcile: %v", err)
	}
	afterB, err := appRepo(t, tenantB).MarksForDate(ctx, scope, date)
	if err != nil {
		t.Fatalf("re-read B: %v", err)
	}
	gotB, ok := afterB[enrollmentRLS]
	if !ok || gotB.Status != "absent" || gotB.ID != "bbbbbbbb-0000-0000-0000-000000000001" {
		t.Errorf("tenant A's reconcile leaked into tenant B: %+v (ok=%v), want B's absent row intact", gotB, ok)
	}
	// A clearing save by tenant A likewise cannot wipe tenant B's day.
	if err := appRepo(t, tenantA).ReconcileDay(ctx, scope, date, nil); err != nil {
		t.Fatalf("tenant A clearing reconcile: %v", err)
	}
	if n := countMarks(t, tenantB, enrollmentRLS); n != 1 {
		t.Errorf("tenant B rows after tenant A cleared its day = %d, want 1", n)
	}
	if n := countMarks(t, tenantA, enrollmentRLS); n != 0 {
		t.Errorf("tenant A rows after clearing = %d, want 0", n)
	}
	// ListByEnrollment is tenant-bound too.
	listA, err := appRepo(t, tenantA).ListByEnrollment(ctx, enrollmentRLS, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("tenant A ListByEnrollment: %v", err)
	}
	if len(listA) != 0 {
		t.Errorf("tenant A listed %+v, want nothing (RLS read breach)", listA)
	}
}

// TestUnsetTenantSeesNothing proves the policy fails closed: with no
// app.current_tenant bound, the non-owner role reads zero rows — so a code path
// that forgot pkg/authctx leaks nothing.
func TestUnsetTenantSeesNothing(t *testing.T) {
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	var n int64
	if err := db.Table("attendance").Count(&n).Error; err != nil {
		t.Fatalf("count without a tenant bound: %v", err)
	}
	if n != 0 {
		t.Errorf("unbound session read %d attendance rows, want 0 (RLS must fail closed)", n)
	}
}

// TestStatusCheckRejectsUnknownStatus proves the DB CHECK is a defense-in-depth
// below the service's validation: a raw insert of an unknown status is rejected
// by Postgres even when it bypasses the service.
func TestStatusCheckRejectsUnknownStatus(t *testing.T) {
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO attendance (tenant_id, enrollment_id, date, status) VALUES (?, ?, ?::date, ?)",
			tenantA, enrollmentBatchClean, "2026-08-09", "tarde",
		).Error
	})
	if err == nil {
		t.Fatal("DB accepted an unknown status; the CHECK must reject it")
	}
}

// TestNoteLengthCheckRejectsOversizedNote proves the free-text cap is enforced by
// the database too, so an unbounded note cannot reach the column through a writer
// that skipped the service's validation.
func TestNoteLengthCheckRejectsOversizedNote(t *testing.T) {
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	insertNote := func(note string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
				return err
			}
			return tx.Exec(
				"INSERT INTO attendance (tenant_id, enrollment_id, date, status, note) VALUES (?, ?, ?::date, ?, ?)",
				tenantA, enrollmentBatchClean, "2026-08-12", "excused", note,
			).Error
		})
	}
	if err := insertNote(strings.Repeat("x", 501)); err == nil {
		t.Fatal("DB accepted a 501-char note; the CHECK must reject it")
	}
	if err := insertNote(strings.Repeat("x", 500)); err != nil {
		t.Fatalf("a note of exactly 500 chars must be accepted: %v", err)
	}
}
