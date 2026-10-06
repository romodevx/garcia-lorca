//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

const levelFoldMigration = "0019_level_folded_name_unique.sql"

func newCycleLevel(t *testing.T, id, tenantID, name string) domain.Level {
	t.Helper()
	l, err := domain.NewLevel(id, tenantID, name, "", domain.GranularityCycle)
	if err != nil {
		t.Fatalf("NewLevel(%q): %v", name, err)
	}
	return l
}

// The SQL fold is the one the Go FoldName mirrors: case, the Spanish accents
// (either case, ñ included) and runs of whitespace are ignored, ends trimmed.
func TestConfigFoldNameFolds(t *testing.T) {
	db := appDB(t)
	cases := map[string]string{
		"Preparatoria":            "preparatoria",
		"  PREPARATORIA  ":        "preparatoria",
		"Educación  Básica":       "educacion basica",
		"ÁÉÍÓÚÜÑ áéíóúüñ":         "aeiouun aeiouun",
		"Primaria\t\n Bilingüe  ": "primaria bilingue",
		"":                        "",
	}
	for in, want := range cases {
		if got := scalar[string](t, db, "SELECT config_fold_name(?)", in); got != want {
			t.Errorf("config_fold_name(%q) = %q, want %q", in, got, want)
		}
	}
}

// "PREPARATORIA" after "Preparatoria" is the same level: the folded name is
// unique per school, so an import can never create it twice.
func TestALevelNameIsUniqueOnceFolded(t *testing.T) {
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	ctx := context.Background()
	if err := repo.CreateLevel(ctx, newCycleLevel(t, "f4000000-0000-0000-0000-000000000001", tenantA, "Preparatoria Fold")); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	spellings := map[string]string{
		"f4000000-0000-0000-0000-000000000011": "PREPARATORIA FOLD",
		"f4000000-0000-0000-0000-000000000012": "  preparatoria   fold ",
		"f4000000-0000-0000-0000-000000000013": "Préparatoria Fold",
	}
	for id, spelling := range spellings {
		err := repo.CreateLevel(ctx, newCycleLevel(t, id, tenantA, spelling))
		if glerrors.KindOf(err) != glerrors.KindConflict {
			t.Errorf("%q must clash with %q, got %v", spelling, "Preparatoria Fold", err)
		}
	}
}

// The uniqueness is per school: tenant B may hold the very same name, and
// neither tenant sees the other's level.
func TestLevelFoldedNameUniquenessIsPerTenant(t *testing.T) {
	ctx := context.Background()
	repoA := pgadapter.NewRepository(appDB(t), tenantA)
	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	const idA, idB = "f4000000-0000-0000-0000-000000000021", "f4000000-0000-0000-0000-000000000022"
	if err := repoA.CreateLevel(ctx, newCycleLevel(t, idA, tenantA, "Bachillerato Tenant")); err != nil {
		t.Fatalf("tenant A CreateLevel: %v", err)
	}
	if err := repoB.CreateLevel(ctx, newCycleLevel(t, idB, tenantB, "BACHILLERATO TENANT")); err != nil {
		t.Fatalf("tenant B must be free to use the same name: %v", err)
	}
	if _, err := repoB.GetLevel(ctx, idA); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B saw tenant A's level (RLS breach): %v", err)
	}
	if n := writeAsTenant(t, tenantB, "UPDATE levels SET name = 'hijacked' WHERE id = ?", idA); n != 0 {
		t.Errorf("tenant B UPDATE affected %d rows (RLS breach)", n)
	}
}

// Existing levels whose names fold alike stop the migration, named by id, and
// nothing changes — which one is the real level is the school's call.
func TestMigration0019StopsOnLevelsThatFoldAlike(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_level_fold", "0019")
	for _, stmt := range []string{
		"INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES ('f5000000-0000-0000-0000-000000000001', '" + schoolTenant + "', 'Preparatoria', 'CYCLE')",
		"INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES ('f5000000-0000-0000-0000-000000000002', '" + schoolTenant + "', 'PREPARATORIA ', 'CYCLE')",
		"INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES ('f5000000-0000-0000-0000-000000000003', '" + schoolTenant + "', 'Primaria', 'CYCLE')",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	err := tryMigration(t, db, levelFoldMigration)
	if err == nil {
		t.Fatal("levels that fold alike must stop the migration")
	}
	for _, id := range []string{"f5000000-0000-0000-0000-000000000001", "f5000000-0000-0000-0000-000000000002"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("the error must name %s: %v", id, err)
		}
	}
	if strings.Contains(err.Error(), "f5000000-0000-0000-0000-000000000003") {
		t.Errorf("a level with no clash must not be named: %v", err)
	}
	if n := scalar[int64](t, db, "SELECT count(*) FROM levels"); n != 3 {
		t.Errorf("nothing may be deleted, have %d", n)
	}
	if n := scalar[int64](t, db, "SELECT count(*) FROM pg_proc WHERE proname = 'config_fold_name'"); n != 0 {
		t.Errorf("a stopped migration must leave nothing behind, found the fold function")
	}
}
