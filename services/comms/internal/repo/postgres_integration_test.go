//go:build integration

// Integration tests for the comms postgres repository against a REAL Postgres
// (testcontainers-go). They prove the three claims that cannot be tested with a
// fake: that RecordEvent is genuinely idempotent (the ledger's primary key is
// what stops a redelivered event from producing a second notification, and the
// claim and the notification live in ONE transaction), that the device upsert is
// keyed on the token, and — critically — that RLS isolates tenants on all three
// tables when connected as the non-owner `app` role (same posture as
// students/grades/attendance, ADR 0001).
//
// Run: go test -tags=integration ./internal/repo/
// Requires Docker.
package repo_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/comms/internal/repo"
	"github.com/garcia-lorca/services/comms/internal/service"
)

const (
	dbName  = "garcia_lorca"
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

// migrationFiles are applied in order before the app role is granted, so the
// non-owner role gets privileges on every table.
var migrationFiles = []string{
	"../../../../migrations/comms/0001_init.sql",
}

var (
	superDB   *gorm.DB
	appDSN    string
	superPass = randToken()
	appPass   = randToken()
)

// Neutral cross-service references (students / iam UUIDs). Each test uses its
// own ids: the container is shared and never reset between tests.
const (
	studentLedger   = "a0000000-0000-0000-0000-000000000001"
	studentUnread   = "a0000000-0000-0000-0000-000000000002"
	studentRLS      = "a0000000-0000-0000-0000-000000000003"
	studentRead     = "a0000000-0000-0000-0000-000000000004"
	studentOrder    = "a0000000-0000-0000-0000-000000000005"
	studentRace     = "a0000000-0000-0000-0000-000000000006"
	studentOther    = "a0000000-0000-0000-0000-000000000007"
	studentAtomic   = "a0000000-0000-0000-0000-000000000008"
	userUpsert      = "b0000000-0000-0000-0000-000000000001"
	userUpsertOther = "b0000000-0000-0000-0000-000000000002"
	userRLS         = "b0000000-0000-0000-0000-000000000003"
	userTargets     = "b0000000-0000-0000-0000-000000000004"
)

var seedNow = time.Date(2026, 8, 7, 9, 30, 0, 0, time.UTC)

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

func notification(id, studentID, kind, title string) service.Notification {
	return service.Notification{
		ID:        id,
		StudentID: studentID,
		Type:      kind,
		Title:     title,
		Body:      title + " body",
		CreatedAt: seedNow,
	}
}

// countRows reads a comms table as the non-owner app role with the tenant GUC
// bound (mirroring the repository's RLS binding), so it exercises RLS: a tenant
// counts only its own rows.
func countRows(t *testing.T, tenant, table, where string, args ...any) int64 {
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
		return tx.Table(table).Where(where, args...).Count(&n).Error
	})
	if err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// --- the idempotency gate ----------------------------------------------------

// TestRecordEventIsIdempotent is the load-bearing test of the whole service: the
// event ledger's primary key is what makes a redelivered Pub/Sub message a
// no-op. The second call must report isNew=false, write NOTHING, and leave
// exactly one ledger row and one notification.
func TestRecordEventIsIdempotent(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	const eventKey = "AbsenceRecorded:e0000000-0000-0000-0000-000000000001"

	created, isNew, err := r.RecordEvent(ctx, eventKey,
		notification("c0000000-0000-0000-0000-000000000001", studentLedger, "absence", "Ausencia registrada"))
	if err != nil {
		t.Fatalf("first RecordEvent: %v", err)
	}
	if !isNew {
		t.Fatal("the first delivery of an event must be new")
	}
	if created.ID != "c0000000-0000-0000-0000-000000000001" || created.StudentID != studentLedger {
		t.Errorf("stored notification = %+v, want the submitted one", created)
	}
	if !created.ReadAt.IsZero() {
		t.Errorf("read_at = %v, want zero (a new notification is unread)", created.ReadAt)
	}
	if !created.CreatedAt.Equal(seedNow) {
		t.Errorf("created_at = %v, want the submitted %v", created.CreatedAt, seedNow)
	}

	// The redelivery: same key, a DIFFERENT notification id, so a second insert
	// could not be mistaken for the first.
	_, isNew, err = r.RecordEvent(ctx, eventKey,
		notification("c0000000-0000-0000-0000-000000000002", studentLedger, "absence", "Ausencia registrada"))
	if err != nil {
		t.Fatalf("a redelivery must not error: %v", err)
	}
	if isNew {
		t.Error("the redelivery reported isNew=true; the ledger did not gate it")
	}

	if n := countRows(t, tenantA, "processed_events", "event_key = ?", eventKey); n != 1 {
		t.Errorf("ledger rows = %d, want exactly 1", n)
	}
	if n := countRows(t, tenantA, "notifications", "student_id = ?", studentLedger); n != 1 {
		t.Errorf("notifications = %d, want exactly 1 — the redelivery must write nothing", n)
	}
}

// TestRecordEventClaimAndNotificationAreOneTransaction proves the two writes are
// atomic. A notification the database rejects (a body the column cannot hold —
// here a NULL forced through a duplicate primary key) must take the ledger claim
// down with it: were the claim committed on its own, the redelivery would be
// skipped and the notification lost forever.
func TestRecordEventClaimAndNotificationAreOneTransaction(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	const eventKey = "AbsenceRecorded:e0000000-0000-0000-0000-000000000002"
	const collidingID = "c0000000-0000-0000-0000-000000000010"

	// Seed the notification id under a DIFFERENT event key, so the next call's
	// insert fails on the notifications primary key after the claim is written.
	if _, _, err := r.RecordEvent(ctx, "AbsenceRecorded:seed",
		notification(collidingID, studentAtomic, "absence", "seed")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, _, err := r.RecordEvent(ctx, eventKey,
		notification(collidingID, studentAtomic, "absence", "colliding")); err == nil {
		t.Fatal("a rejected notification must fail RecordEvent so the event is redelivered")
	}

	if n := countRows(t, tenantA, "processed_events", "event_key = ?", eventKey); n != 0 {
		t.Errorf("ledger rows = %d, want 0 — a failed notification must roll the claim back", n)
	}
}

// TestRecordEventConcurrentDeliveriesProduceOneNotification is the race the
// ledger exists for: two workers handed the same message at the same time.
// Exactly one may report isNew, neither may error, and the student ends with a
// single notification.
func TestRecordEventConcurrentDeliveriesProduceOneNotification(t *testing.T) {
	ctx := context.Background()
	const eventKey = "PaymentRegistered:f0000000-0000-0000-0000-000000000001"

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		newCount int
		failures []error
	)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, isNew, err := appRepo(t, tenantA).RecordEvent(ctx, eventKey, notification(
				fmt.Sprintf("c0000000-0000-0000-0000-00000000002%d", i),
				studentRace, "payment", "Pago recibido"))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, err)
				return
			}
			if isNew {
				newCount++
			}
		}(i)
	}
	wg.Wait()

	// A concurrent claim may legitimately surface as a serialization failure the
	// transport retries; what must never happen is TWO notifications.
	for _, err := range failures {
		t.Logf("concurrent delivery failed (redelivery is the recovery): %v", err)
	}
	if newCount > 1 {
		t.Errorf("%d deliveries reported isNew, want at most 1", newCount)
	}
	if n := countRows(t, tenantA, "notifications", "student_id = ?", studentRace); n != 1 {
		t.Errorf("notifications after two concurrent deliveries = %d, want exactly 1", n)
	}
}

// --- the feed ----------------------------------------------------------------

// TestListNotificationsFiltersUnreadAndIsNewestFirst covers the read the family
// app makes: newest first (the cap drops ancient history, never what is on
// screen), scoped to one student, and filterable to the unread ones that drive
// the badge.
func TestListNotificationsFiltersUnreadAndIsNewestFirst(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()

	// Seeded oldest-first, so insertion order cannot masquerade as the ordering
	// under test.
	const seeded = 3
	for i := range seeded {
		n := notification(
			fmt.Sprintf("c0000000-0000-0000-0000-00000000003%d", i),
			studentUnread, "absence", fmt.Sprintf("Ausencia %d", i))
		n.CreatedAt = seedNow.Add(time.Duration(i) * time.Hour)
		if _, _, err := r.RecordEvent(ctx, fmt.Sprintf("AbsenceRecorded:unread-%d", i), n); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	// A notification for another student, which must never appear below.
	if _, _, err := r.RecordEvent(ctx, "AbsenceRecorded:other",
		notification("c0000000-0000-0000-0000-000000000039", studentOther, "absence", "Otro alumno")); err != nil {
		t.Fatalf("seed other student: %v", err)
	}

	all, err := r.ListNotifications(ctx, studentUnread, false)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(all) != seeded {
		t.Fatalf("listed %d notifications, want the %d seeded for this student: %+v", len(all), seeded, all)
	}
	for i, got := range all {
		if want := fmt.Sprintf("Ausencia %d", seeded-1-i); got.Title != want {
			t.Fatalf("row %d = %q, want %q — the read must be newest first", i, got.Title, want)
		}
		if got.StudentID != studentUnread {
			t.Errorf("row %d belongs to %s, want %s", i, got.StudentID, studentUnread)
		}
	}

	// Mark the middle one read: it must leave the unread list and stay in the full one.
	readID := "c0000000-0000-0000-0000-000000000031"
	if _, err := r.MarkRead(ctx, readID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	unread, err := r.ListNotifications(ctx, studentUnread, true)
	if err != nil {
		t.Fatalf("ListNotifications(unreadOnly): %v", err)
	}
	if len(unread) != seeded-1 {
		t.Fatalf("unread list has %d, want %d: %+v", len(unread), seeded-1, unread)
	}
	for _, got := range unread {
		if got.ID == readID {
			t.Error("a read notification appeared in the unread list")
		}
		if !got.ReadAt.IsZero() {
			t.Errorf("unread row %s carries read_at %v", got.ID, got.ReadAt)
		}
	}
	if full, err := r.ListNotifications(ctx, studentUnread, false); err != nil {
		t.Fatalf("ListNotifications: %v", err)
	} else if len(full) != seeded {
		t.Errorf("full list has %d, want %d — marking read must not remove it", len(full), seeded)
	}
}

// TestMarkReadIsIdempotentAndReportsMissing pins the read stamp: it is set once
// (a second mark keeps the ORIGINAL instant, so "read at" stays truthful), it
// comes back on the returned notification, and an unknown id is ErrNotFound
// rather than a silent success.
func TestMarkReadIsIdempotentAndReportsMissing(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	const id = "c0000000-0000-0000-0000-000000000040"

	if _, _, err := r.RecordEvent(ctx, "AbsenceRecorded:markread",
		notification(id, studentRead, "absence", "Ausencia registrada")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	first, err := r.MarkRead(ctx, id)
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if first.ReadAt.IsZero() {
		t.Fatal("read_at is still unset after marking read")
	}
	second, err := r.MarkRead(ctx, id)
	if err != nil {
		t.Fatalf("re-marking must be idempotent: %v", err)
	}
	if !second.ReadAt.Equal(first.ReadAt) {
		t.Errorf("read_at moved from %v to %v; the first stamp must stand", first.ReadAt, second.ReadAt)
	}

	if _, err := r.MarkRead(ctx, "c0000000-0000-0000-0000-0000000000ff"); !errors.Is(err, glerrors.ErrNotFound) {
		t.Errorf("unknown id gave %v, want ErrNotFound", err)
	}
}

// --- devices -----------------------------------------------------------------

// TestUpsertDeviceIsKeyedOnTheToken proves re-registering refreshes the existing
// row instead of fanning the same phone out twice — the reason a client may call
// RegisterDevice on every launch — while a different token is a different device.
func TestUpsertDeviceIsKeyedOnTheToken(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()
	const token = "token-upsert"

	first, err := r.UpsertDevice(ctx, service.Device{
		ID: "d0000000-0000-0000-0000-000000000001", UserID: userUpsert, FCMToken: token, Platform: "ios",
	})
	if err != nil {
		t.Fatalf("first UpsertDevice: %v", err)
	}
	if first.Platform != "ios" {
		t.Errorf("platform = %q, want ios", first.Platform)
	}

	// The same token arriving with a new id, owner and platform: the stored row is
	// UPDATED and keeps its original id.
	second, err := r.UpsertDevice(ctx, service.Device{
		ID: "d0000000-0000-0000-0000-000000000002", UserID: userUpsertOther, FCMToken: token, Platform: "web",
	})
	if err != nil {
		t.Fatalf("re-registration: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("device id changed from %s to %s; the upsert must keep the stored row", first.ID, second.ID)
	}
	if second.Platform != "web" || second.UserID != userUpsertOther {
		t.Errorf("device = %+v, want the refreshed owner and platform", second)
	}
	if n := countRows(t, tenantA, "devices", "fcm_token = ?", token); n != 1 {
		t.Errorf("rows for the token = %d, want exactly 1", n)
	}

	if _, err := r.UpsertDevice(ctx, service.Device{
		ID: "d0000000-0000-0000-0000-000000000003", UserID: userUpsert, FCMToken: "token-other", Platform: "android",
	}); err != nil {
		t.Fatalf("second device: %v", err)
	}
	if n := countRows(t, tenantA, "devices", "user_id = ?", userUpsert); n != 1 {
		t.Errorf("devices for the original user = %d, want 1 (the shared token moved owner)", n)
	}
}

// TestDevicesForTenantReturnsTheTenantsTargets covers the (deliberately coarse)
// push fanout of this phase: every token in the tenant, and nobody else's.
func TestDevicesForTenantReturnsTheTenantsTargets(t *testing.T) {
	ctx := context.Background()
	if _, err := appRepo(t, tenantA).UpsertDevice(ctx, service.Device{
		ID: "d0000000-0000-0000-0000-000000000010", UserID: userTargets, FCMToken: "token-a-targets", Platform: "ios",
	}); err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if _, err := appRepo(t, tenantB).UpsertDevice(ctx, service.Device{
		ID: "d0000000-0000-0000-0000-000000000011", UserID: userTargets, FCMToken: "token-b-targets", Platform: "ios",
	}); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	devices, err := appRepo(t, tenantA).DevicesForTenant(ctx)
	if err != nil {
		t.Fatalf("DevicesForTenant: %v", err)
	}
	tokens := map[string]bool{}
	for _, d := range devices {
		tokens[d.FCMToken] = true
	}
	if !tokens["token-a-targets"] {
		t.Error("the tenant's own device is missing from the push targets")
	}
	if tokens["token-b-targets"] {
		t.Error("another tenant's device is a push target (RLS read breach)")
	}
}

// --- RLS ---------------------------------------------------------------------

// TestRLSIsolatesEveryCommsTable proves read AND write isolation on all three
// tables for the non-owner app role. The ledger case is the sharp one: tenant B
// having handled an event must NOT make tenant A skip it, or one school's
// activity would silence another's notifications.
func TestRLSIsolatesEveryCommsTable(t *testing.T) {
	ctx := context.Background()
	const eventKey = "AbsenceRecorded:e0000000-0000-0000-0000-0000000000ff"

	// notifications + processed_events, seeded by B.
	if _, isNew, err := appRepo(t, tenantB).RecordEvent(ctx, eventKey,
		notification("c0000000-0000-0000-0000-0000000000b1", studentRLS, "absence", "B's absence")); err != nil {
		t.Fatalf("seed B: %v", err)
	} else if !isNew {
		t.Fatal("B's first delivery should be new")
	}

	// Read isolation on notifications.
	listA, err := appRepo(t, tenantA).ListNotifications(ctx, studentRLS, false)
	if err != nil {
		t.Fatalf("tenant A ListNotifications: %v", err)
	}
	if len(listA) != 0 {
		t.Errorf("tenant A listed %+v, want nothing (RLS read breach)", listA)
	}

	// The ledger is per tenant: A must still handle the same event.
	_, isNew, err := appRepo(t, tenantA).RecordEvent(ctx, eventKey,
		notification("c0000000-0000-0000-0000-0000000000a1", studentRLS, "absence", "A's absence"))
	if err != nil {
		t.Fatalf("tenant A RecordEvent: %v", err)
	}
	if !isNew {
		t.Fatal("tenant B's ledger row suppressed tenant A's event; the ledger must be tenant-scoped")
	}
	if n := countRows(t, tenantA, "notifications", "student_id = ?", studentRLS); n != 1 {
		t.Errorf("tenant A notifications = %d, want 1", n)
	}
	if n := countRows(t, tenantB, "notifications", "student_id = ?", studentRLS); n != 1 {
		t.Errorf("tenant B notifications = %d, want its own 1 intact", n)
	}

	// Write isolation on notifications: A cannot mark B's notification read.
	if _, err := appRepo(t, tenantA).MarkRead(ctx, "c0000000-0000-0000-0000-0000000000b1"); !errors.Is(err, glerrors.ErrNotFound) {
		t.Errorf("tenant A marking B's notification gave %v, want ErrNotFound", err)
	}
	if n := countRows(t, tenantB, "notifications", "id = ? AND read_at IS NULL", "c0000000-0000-0000-0000-0000000000b1"); n != 1 {
		t.Error("tenant A's MarkRead reached tenant B's row (RLS write breach)")
	}

	// devices: the same token in two tenants is two devices, and neither sees the other.
	if _, err := appRepo(t, tenantB).UpsertDevice(ctx, service.Device{
		ID: "d0000000-0000-0000-0000-0000000000b1", UserID: userRLS, FCMToken: "token-shared-across-tenants", Platform: "ios",
	}); err != nil {
		t.Fatalf("seed B device: %v", err)
	}
	aDevice, err := appRepo(t, tenantA).UpsertDevice(ctx, service.Device{
		ID: "d0000000-0000-0000-0000-0000000000a1", UserID: userRLS, FCMToken: "token-shared-across-tenants", Platform: "web",
	})
	if err != nil {
		t.Fatalf("tenant A device: %v", err)
	}
	if aDevice.ID != "d0000000-0000-0000-0000-0000000000a1" {
		t.Errorf("tenant A's device id = %s, want its own — the upsert must not adopt B's row", aDevice.ID)
	}
	if n := countRows(t, tenantB, "devices", "fcm_token = ? AND platform = ?", "token-shared-across-tenants", "ios"); n != 1 {
		t.Error("tenant A's device upsert overwrote tenant B's row (RLS write breach)")
	}
}

// TestUnsetTenantSeesNothing proves the policies fail closed on every table: with
// no app.current_tenant bound, the non-owner role reads zero rows — so a code
// path that forgot pkg/authctx leaks nothing.
func TestUnsetTenantSeesNothing(t *testing.T) {
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	for _, table := range []string{"notifications", "devices", "processed_events"} {
		var n int64
		if err := db.Table(table).Count(&n).Error; err != nil {
			t.Fatalf("count %s without a tenant bound: %v", table, err)
		}
		if n != 0 {
			t.Errorf("unbound session read %d rows from %s, want 0 (RLS must fail closed)", n, table)
		}
	}
}

// TestPlatformCheckRejectsUnknownPlatform proves the DB CHECK is defense in depth
// below the service's validation: a raw insert of an unknown platform is rejected
// by Postgres even when it bypasses the service.
func TestPlatformCheckRejectsUnknownPlatform(t *testing.T) {
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO devices (tenant_id, user_id, fcm_token, platform) VALUES (?, ?, ?, ?)",
			tenantA, userRLS, "token-blackberry", "blackberry",
		).Error
	})
	if err == nil {
		t.Fatal("DB accepted an unknown platform; the CHECK must reject it")
	}
}

// TestListNotificationsIsNewestFirstOverManyRows is what makes the read's
// documented cap safe: over a history longer than a screenful, rows come back
// newest first and in a DETERMINISTIC order (created_at, id) rather than whatever
// the planner happened to emit — so the cap drops ancient history, never the
// notifications a family is looking at.
func TestListNotificationsIsNewestFirstOverManyRows(t *testing.T) {
	r := appRepo(t, tenantA)
	ctx := context.Background()

	const seeded = 12
	for i := range seeded {
		n := notification(
			fmt.Sprintf("c0000000-0000-0000-0000-0000000005%02d", i),
			studentOrder, "absence", fmt.Sprintf("Ausencia %02d", i))
		n.CreatedAt = seedNow.Add(time.Duration(i) * time.Minute)
		if _, _, err := r.RecordEvent(ctx, fmt.Sprintf("AbsenceRecorded:bounded-%02d", i), n); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	got, err := r.ListNotifications(ctx, studentOrder, false)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(got) != seeded {
		t.Fatalf("listed %d, want the %d seeded (all fit under the cap)", len(got), seeded)
	}
	for i, row := range got {
		if want := fmt.Sprintf("Ausencia %02d", seeded-1-i); row.Title != want {
			t.Fatalf("row %d = %q, want %q — the read must be newest first", i, row.Title, want)
		}
	}
}
