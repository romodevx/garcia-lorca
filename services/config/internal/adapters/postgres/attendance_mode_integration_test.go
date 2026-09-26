//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func seedLevel0008(t *testing.T, db *gorm.DB, id, name string) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES (?, ?, ?, 'CYCLE')",
		id, schoolTenant, name).Error; err != nil {
		t.Fatalf("seed level %q: %v", name, err)
	}
}

// Migration 0008 gives every existing level an attendance mode: per class for
// the SEP levels taught by one teacher per subject, matched regardless of case,
// accents and surrounding spaces; daily for everything else.
func TestMigration0008BackfillsAttendanceModeByLevelName(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_att_mode", "0008")
	levels := map[string]struct {
		name string
		want string
	}{
		"aaaa0008-0000-0000-0000-000000000001": {"Preescolar", "daily"},
		"aaaa0008-0000-0000-0000-000000000002": {"Primaria", "daily"},
		"aaaa0008-0000-0000-0000-000000000003": {"Secundaria", "per_class"},
		"aaaa0008-0000-0000-0000-000000000004": {" PREPARATORIA ", "per_class"},
		"aaaa0008-0000-0000-0000-000000000005": {"Bachillerato", "per_class"},
		"aaaa0008-0000-0000-0000-000000000006": {"SECUNDÁRIA", "per_class"},
		"aaaa0008-0000-0000-0000-000000000007": {"Secundaria Técnica", "daily"},
	}
	for id, l := range levels {
		seedLevel0008(t, db, id, l.name)
	}

	applyMigration(t, db, "0008_level_attendance_mode.sql")

	for id, l := range levels {
		if got := scalar[string](t, db, "SELECT attendance_mode FROM levels WHERE id = ?", id); got != l.want {
			t.Errorf("level %q attendance_mode = %q, want %q", l.name, got, l.want)
		}
	}
	if err := db.Exec("UPDATE levels SET attendance_mode = 'weekly' WHERE id = 'aaaa0008-0000-0000-0000-000000000001'").Error; err == nil {
		t.Error("the CHECK must reject an unknown attendance mode")
	}
}

// A created level persists its default mode, a changed mode reads back, and the
// change is scoped: a second update over the same row keeps working (the path a
// school walks after the first save) and an unknown id is NotFound.
func TestLevelAttendanceModeRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	sec, err := domain.NewLevel("eeee0008-0000-0000-0000-000000000001", tenantA, "Secundaria T3", "", domain.GranularityCycle)
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	prim, err := domain.NewLevel("eeee0008-0000-0000-0000-000000000002", tenantA, "Primaria T3", "", domain.GranularityCycle)
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	perClass, err := prim.WithAttendanceMode(domain.AttendancePerClass)
	if err != nil {
		t.Fatalf("WithAttendanceMode: %v", err)
	}
	for _, l := range []domain.Level{sec, perClass} {
		if err := repo.CreateLevel(ctx, l); err != nil {
			t.Fatalf("CreateLevel(%s): %v", l.Name(), err)
		}
	}
	if got, _ := repo.GetLevel(ctx, perClass.ID()); got.AttendanceMode() != domain.AttendancePerClass {
		t.Errorf("created mode must persist: got %v", got.AttendanceMode())
	}

	for _, mode := range []domain.AttendanceMode{domain.AttendanceDaily, domain.AttendancePerClass, domain.AttendanceDaily} {
		if err := repo.UpdateLevelAttendanceMode(ctx, sec.ID(), mode); err != nil {
			t.Fatalf("UpdateLevelAttendanceMode(%v): %v", mode, err)
		}
		got, err := repo.GetLevel(ctx, sec.ID())
		if err != nil {
			t.Fatalf("GetLevel: %v", err)
		}
		if got.AttendanceMode() != mode {
			t.Errorf("mode = %v, want %v", got.AttendanceMode(), mode)
		}
	}
	if got, _ := repo.GetLevel(ctx, perClass.ID()); got.AttendanceMode() != domain.AttendancePerClass {
		t.Errorf("updating one level must not touch another: got %v", got.AttendanceMode())
	}

	err = repo.UpdateLevelAttendanceMode(ctx, "eeee0008-0000-0000-0000-00000000ffff", domain.AttendanceDaily)
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown level: want KindNotFound, got %v", err)
	}
	// RLS: another tenant cannot change this tenant's level.
	other := pgadapter.NewRepository(appDB(t), tenantB)
	err = other.UpdateLevelAttendanceMode(ctx, sec.ID(), domain.AttendancePerClass)
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("foreign tenant update: want KindNotFound, got %v", err)
	}
	if got, _ := repo.GetLevel(ctx, sec.ID()); got.AttendanceMode() != domain.AttendanceDaily {
		t.Errorf("a foreign tenant changed the mode: %v", got.AttendanceMode())
	}
}

// The gateway resolves a pass list's group → grade → level with these point
// reads, straight from a request's groupId. A malformed id names no row: it is
// NotFound (→ 404 at the edge), never a uuid cast error surfacing as Internal.
func TestPointReadsTreatAMalformedIDAsNotFound(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	const malformed = "not-a-uuid"

	if _, err := repo.GetGroup(ctx, malformed); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetGroup(malformed): want KindNotFound, got %v", err)
	}
	if _, err := repo.GetGradeLevel(ctx, malformed); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetGradeLevel(malformed): want KindNotFound, got %v", err)
	}
	if _, err := repo.GetLevel(ctx, malformed); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetLevel(malformed): want KindNotFound, got %v", err)
	}
}
