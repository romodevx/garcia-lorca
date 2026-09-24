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

// The same grade cannot be created twice in one level — by name as well as by
// order, and whatever the capitalization: "1° Preescolar" and "1° PREESCOLAR"
// are one grade (task 2 keeps this integrity check while the SEP rules go).
func TestAGradeNameIsUniqueWithinItsLevel(t *testing.T) {
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	ctx := context.Background()
	const levelID = "f2000000-0000-0000-0000-000000000001"
	level, err := domain.NewLevel(levelID, tenantA, "Preescolar (grados únicos)", "", domain.GranularityCycle)
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	if err := repo.CreateLevel(ctx, level); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	first, _ := domain.NewGradeLevel("f2000000-0000-0000-0000-000000000011", tenantA, levelID, "1° Preescolar", 1)
	if err := repo.CreateGradeLevel(ctx, first); err != nil {
		t.Fatalf("CreateGradeLevel: %v", err)
	}

	again, _ := domain.NewGradeLevel("f2000000-0000-0000-0000-000000000012", tenantA, levelID, "1° PREESCOLAR", 2)
	err = repo.CreateGradeLevel(ctx, again)
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("a repeated grade name must be KindConflict, got %v", err)
	}
	if !strings.Contains(glerrors.SafeMessage(err), "name") {
		t.Errorf("the conflict must say it is the NAME that repeats: %q", glerrors.SafeMessage(err))
	}
}

// Existing duplicates stop migration 0006, named by id, with nothing changed.
func TestMigration0006StopsOnDuplicateGradeNames(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_grade_dup", "0006")
	for _, stmt := range []string{
		"INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES ('f3000000-0000-0000-0000-000000000001', '" + schoolTenant + "', 'Primaria', 'CYCLE')",
		"INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES ('f3000000-0000-0000-0000-000000000011', '" + schoolTenant + "', 'f3000000-0000-0000-0000-000000000001', '1° A', 1)",
		"INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES ('f3000000-0000-0000-0000-000000000012', '" + schoolTenant + "', 'f3000000-0000-0000-0000-000000000001', '1° a', 2)",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	err := tryMigration(t, db, "0006_grade_name_unique.sql")
	if err == nil {
		t.Fatal("duplicate grade names must stop the migration")
	}
	for _, id := range []string{"f3000000-0000-0000-0000-000000000011", "f3000000-0000-0000-0000-000000000012"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("the error must name %s: %v", id, err)
		}
	}
	if n := scalar[int64](t, db, "SELECT count(*) FROM grade_levels"); n != 2 {
		t.Errorf("nothing may be deleted, have %d", n)
	}
}
