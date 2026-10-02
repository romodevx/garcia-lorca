//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// otherGradeSubject adds a second grade to the fixture's level and a subject of
// it, returning (gradeID, subjectID). The subject is teachable only in groups
// of THAT grade.
func otherGradeSubject(t *testing.T, tenantID, prefix string) (string, string) {
	t.Helper()
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantID)
	levelID := prefix + "0000-0000-0000-0000-000000000002"
	gradeID := prefix + "0000-0000-0000-0000-000000000013"
	subjectID := prefix + "0000-0000-0000-0000-000000000014"
	grade, err := domain.NewGradeLevel(gradeID, tenantID, levelID, "Grade-2-"+prefix, 2)
	if err != nil {
		t.Fatalf("NewGradeLevel: %v", err)
	}
	if err := repo.CreateGradeLevel(ctx, grade); err != nil {
		t.Fatalf("CreateGradeLevel: %v", err)
	}
	subject, err := domain.NewSubject(subjectID, tenantID, gradeID, "Subject-2-"+prefix)
	if err != nil {
		t.Fatalf("NewSubject: %v", err)
	}
	if err := repo.CreateSubject(ctx, subject); err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	return gradeID, subjectID
}

// The database itself refuses an assignment whose subject belongs to another
// grade than its group — the use case checks it first, but a write that skips
// the use case (a script, a restore, a future code path) must not slip through.
func TestDBRefusesAnAssignmentWhoseSubjectIsOfAnotherGrade(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	cycleID, groupID, _, teacherID := teachingAssignmentFixture(t, tenantA, "d1d1")
	_, foreignSubject := otherGradeSubject(t, tenantA, "d1d1")

	a, err := domain.NewTeachingAssignment("d1d17000-0000-0000-0000-000000000001", tenantA, cycleID, groupID, foreignSubject, teacherID)
	if err != nil {
		t.Fatalf("NewTeachingAssignment: %v", err)
	}
	err = repo.CreateTeachingAssignment(ctx, a)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("a subject of another grade must be KindInvalid, got %v", err)
	}
	if !strings.Contains(glerrors.SafeMessage(err), "grade") {
		t.Errorf("the refusal must name the grade mismatch: %q", glerrors.SafeMessage(err))
	}
}

// Moving an existing assignment to another grade's subject is refused too.
func TestDBRefusesMovingAnAssignmentToAnotherGradesSubject(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	cycleID, groupID, subjectID, teacherID := teachingAssignmentFixture(t, tenantA, "d2d2")
	_, foreignSubject := otherGradeSubject(t, tenantA, "d2d2")
	const assignID = "d2d27000-0000-0000-0000-000000000001"
	a, _ := domain.NewTeachingAssignment(assignID, tenantA, cycleID, groupID, subjectID, teacherID)
	if err := repo.CreateTeachingAssignment(ctx, a); err != nil {
		t.Fatalf("CreateTeachingAssignment: %v", err)
	}

	err := appDB(t).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE teaching_assignments SET subject_id = ? WHERE id = ?", foreignSubject, assignID).Error
	})
	if err == nil || !strings.Contains(err.Error(), "23514") {
		t.Fatalf("moving the assignment to another grade's subject must be refused, got %v", err)
	}
}

// A subject or group that an assignment points at cannot change grade under it.
func TestDBRefusesRegradingAnAssignedSubjectOrGroup(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	cycleID, groupID, subjectID, teacherID := teachingAssignmentFixture(t, tenantA, "d3d3")
	otherGrade, _ := otherGradeSubject(t, tenantA, "d3d3")
	a, _ := domain.NewTeachingAssignment("d3d37000-0000-0000-0000-000000000001", tenantA, cycleID, groupID, subjectID, teacherID)
	if err := repo.CreateTeachingAssignment(ctx, a); err != nil {
		t.Fatalf("CreateTeachingAssignment: %v", err)
	}

	for _, stmt := range []string{
		"UPDATE subjects SET grade_level_id = ? WHERE id = ?",
		"UPDATE groups SET grade_level_id = ? WHERE id = ?",
	} {
		target := subjectID
		if strings.Contains(stmt, "groups") {
			target = groupID
		}
		err := appDB(t).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
				return err
			}
			return tx.Exec(stmt, otherGrade, target).Error
		})
		if err == nil || !strings.Contains(err.Error(), "23514") {
			t.Errorf("%s must be refused while assigned, got %v", stmt, err)
		}
	}

	// Rewriting a row with its SAME grade (an ORM saving every column) is fine.
	if n := writeAsTenant(t, tenantA, "UPDATE subjects SET grade_level_id = grade_level_id, name = name WHERE id = ?", subjectID); n != 1 {
		t.Errorf("a no-op rewrite must pass, affected %d", n)
	}
}

// Migration 0014 stops, naming the rows, if an assignment already breaks the rule.
func TestMigration0014StopsOnAnExistingMismatch(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_subject_grade", "0014")
	for _, stmt := range []string{
		"INSERT INTO cycles (id, tenant_id, name, status, starts_on, ends_on) VALUES ('c4000000-0000-0000-0000-000000000001', '" + schoolTenant + "', '2026-2027', 'active', '2026-08-01', '2027-07-15')",
		"INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES ('c4000000-0000-0000-0000-000000000002', '" + schoolTenant + "', 'Primaria', 'CYCLE')",
		"INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES ('c4000000-0000-0000-0000-000000000011', '" + schoolTenant + "', 'c4000000-0000-0000-0000-000000000002', '1°', 1)",
		"INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES ('c4000000-0000-0000-0000-000000000012', '" + schoolTenant + "', 'c4000000-0000-0000-0000-000000000002', '2°', 2)",
		"INSERT INTO subjects (id, tenant_id, grade_level_id, name) VALUES ('c4000000-0000-0000-0000-000000000021', '" + schoolTenant + "', 'c4000000-0000-0000-0000-000000000012', 'Español')",
		"INSERT INTO groups (id, tenant_id, cycle_id, grade_level_id, name) VALUES ('c4000000-0000-0000-0000-000000000031', '" + schoolTenant + "', 'c4000000-0000-0000-0000-000000000001', 'c4000000-0000-0000-0000-000000000011', 'A')",
		"INSERT INTO teachers (id, tenant_id, first_name, last_name, email) VALUES ('c4000000-0000-0000-0000-000000000041', '" + schoolTenant + "', 'x', 'y', 'x@example.com')",
		"INSERT INTO teaching_assignments (id, tenant_id, cycle_id, group_id, subject_id, teacher_id) VALUES ('c4000000-0000-0000-0000-000000000051', '" + schoolTenant + "', 'c4000000-0000-0000-0000-000000000001', 'c4000000-0000-0000-0000-000000000031', 'c4000000-0000-0000-0000-000000000021', 'c4000000-0000-0000-0000-000000000041')",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	err := tryMigration(t, db, "0014_subject_matches_group_grade.sql")
	if err == nil {
		t.Fatal("an existing mismatch must stop the migration")
	}
	if !strings.Contains(err.Error(), "c4000000-0000-0000-0000-000000000051") {
		t.Errorf("the error must name the assignment: %v", err)
	}
	if n := scalar[int64](t, db, "SELECT count(*) FROM teaching_assignments"); n != 1 {
		t.Errorf("nothing may be deleted, have %d", n)
	}
}

// Renaming keeps the subject's id (grades, attendance and assignments point at
// it) and its grade; a name already used in the grade is KindConflict.
func TestRenameSubject(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	_, _, subjectID, _ := teachingAssignmentFixture(t, tenantA, "d5d5")
	sibling, _ := domain.NewSubject("d5d50000-0000-0000-0000-000000000099", tenantA, "d5d50000-0000-0000-0000-000000000003", "Inglés")
	if err := repo.CreateSubject(ctx, sibling); err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}

	got, err := repo.RenameSubject(ctx, subjectID, "Matemáticas I")
	if err != nil {
		t.Fatalf("RenameSubject: %v", err)
	}
	if got.ID() != subjectID || got.Name() != "Matemáticas I" || got.GradeLevelID() != "d5d50000-0000-0000-0000-000000000003" {
		t.Errorf("renamed subject = %+v", got)
	}
	if read, _ := repo.GetSubject(ctx, subjectID); read.Name() != "Matemáticas I" {
		t.Errorf("the new name must persist, read %q", read.Name())
	}

	if _, err := repo.RenameSubject(ctx, subjectID, "Inglés"); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("a name already in the grade must be KindConflict, got %v", err)
	}
	if _, err := repo.RenameSubject(ctx, "d5d50000-0000-0000-0000-0000000000ff", "X"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("an unknown subject must be KindNotFound, got %v", err)
	}
	if _, err := repo.RenameSubject(ctx, "not-a-uuid", "X"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("a malformed id must be KindNotFound, got %v", err)
	}
}

// A subject a teacher is assigned to cannot be deleted; a free one can.
func TestDeleteSubject(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	cycleID, groupID, subjectID, teacherID := teachingAssignmentFixture(t, tenantA, "d6d6")
	a, _ := domain.NewTeachingAssignment("d6d67000-0000-0000-0000-000000000001", tenantA, cycleID, groupID, subjectID, teacherID)
	if err := repo.CreateTeachingAssignment(ctx, a); err != nil {
		t.Fatalf("CreateTeachingAssignment: %v", err)
	}

	if err := repo.DeleteSubject(ctx, subjectID); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("an assigned subject must be KindConflict, got %v", err)
	}
	if _, err := repo.GetSubject(ctx, subjectID); err != nil {
		t.Fatalf("the refused delete must leave the subject: %v", err)
	}

	free, _ := domain.NewSubject("d6d60000-0000-0000-0000-000000000099", tenantA, "d6d60000-0000-0000-0000-000000000003", "Artes")
	if err := repo.CreateSubject(ctx, free); err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	if err := repo.DeleteSubject(ctx, free.ID()); err != nil {
		t.Fatalf("DeleteSubject(free): %v", err)
	}
	if _, err := repo.GetSubject(ctx, free.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the deleted subject must be gone, got %v", err)
	}
	if err := repo.DeleteSubject(ctx, free.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("deleting twice must be KindNotFound, got %v", err)
	}
}

// RLS: tenant B can neither rename nor delete tenant A's subject.
func TestRenameAndDeleteSubjectAreTenantScoped(t *testing.T) {
	ctx := context.Background()
	_, _, subjectID, _ := teachingAssignmentFixture(t, tenantA, "d7d7")
	other := pgadapter.NewRepository(appDB(t), tenantB)
	if _, err := other.RenameSubject(ctx, subjectID, "Hack"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("another tenant's rename must be KindNotFound, got %v", err)
	}
	if err := other.DeleteSubject(ctx, subjectID); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("another tenant's delete must be KindNotFound, got %v", err)
	}
}

// A subject that was ever in a teacher's load cannot be deleted, even after the
// slot was freed: grades and attendance in other services keep its id (0015).
func TestDeleteSubjectOnceAssignedIsRefusedForever(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	cycleID, groupID, subjectID, teacherID := teachingAssignmentFixture(t, tenantA, "d8d8")
	const assignID = "d8d87000-0000-0000-0000-000000000001"
	a, _ := domain.NewTeachingAssignment(assignID, tenantA, cycleID, groupID, subjectID, teacherID)
	if err := repo.CreateTeachingAssignment(ctx, a); err != nil {
		t.Fatalf("CreateTeachingAssignment: %v", err)
	}
	if err := repo.DeleteTeachingAssignment(ctx, assignID); err != nil {
		t.Fatalf("DeleteTeachingAssignment: %v", err)
	}

	if err := repo.DeleteSubject(ctx, subjectID); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("a subject once assigned must be KindConflict, got %v", err)
	}
	if _, err := repo.GetSubject(ctx, subjectID); err != nil {
		t.Fatalf("the refused delete must keep the subject: %v", err)
	}
}

// One name per grade whatever the capitalization: "Matemáticas" and
// "MATEMÁTICAS" are the same subject (0015, like grade names in 0006).
func TestSubjectNameIsUniqueInItsGradeIgnoringCase(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	_, _, _, _ = teachingAssignmentFixture(t, tenantA, "d9d9")
	const gradeID = "d9d90000-0000-0000-0000-000000000003"
	first, _ := domain.NewSubject("d9d90000-0000-0000-0000-000000000091", tenantA, gradeID, "Matemáticas")
	if err := repo.CreateSubject(ctx, first); err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	again, _ := domain.NewSubject("d9d90000-0000-0000-0000-000000000092", tenantA, gradeID, "MATEMÁTICAS")
	if err := repo.CreateSubject(ctx, again); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("the same name in another case must be KindConflict, got %v", err)
	}
	other, _ := domain.NewSubject("d9d90000-0000-0000-0000-000000000093", tenantA, gradeID, "Artes")
	if err := repo.CreateSubject(ctx, other); err != nil {
		t.Fatalf("CreateSubject(Artes): %v", err)
	}
	if _, err := repo.RenameSubject(ctx, other.ID(), "matemáticas"); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("renaming onto another case of a sibling must be KindConflict, got %v", err)
	}
}

// Migration 0015 stops, naming the rows, if a grade already repeats a subject
// name in another case.
func TestMigration0015StopsOnCaseDuplicates(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_subject_case", "0015")
	for _, stmt := range []string{
		"INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES ('c5000000-0000-0000-0000-000000000002', '" + schoolTenant + "', 'Primaria', 'CYCLE')",
		"INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES ('c5000000-0000-0000-0000-000000000011', '" + schoolTenant + "', 'c5000000-0000-0000-0000-000000000002', '1°', 1)",
		"INSERT INTO subjects (id, tenant_id, grade_level_id, name) VALUES ('c5000000-0000-0000-0000-000000000021', '" + schoolTenant + "', 'c5000000-0000-0000-0000-000000000011', 'Español')",
		"INSERT INTO subjects (id, tenant_id, grade_level_id, name) VALUES ('c5000000-0000-0000-0000-000000000022', '" + schoolTenant + "', 'c5000000-0000-0000-0000-000000000011', 'ESPAÑOL')",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	err := tryMigration(t, db, "0015_subject_identity.sql")
	if err == nil {
		t.Fatal("case duplicates must stop the migration")
	}
	if !strings.Contains(err.Error(), "c5000000-0000-0000-0000-000000000022") {
		t.Errorf("the error must name the rows: %v", err)
	}
}

// 0015 backfills: every subject that predates it counts as used — a slot freed
// earlier leaves grades behind that config cannot see.
func TestMigration0015MarksEveryExistingSubjectUsed(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_subject_used", "0015")
	for _, stmt := range []string{
		"INSERT INTO cycles (id, tenant_id, name, status, starts_on, ends_on) VALUES ('c6000000-0000-0000-0000-000000000001', '" + schoolTenant + "', '2026-2027', 'active', '2026-08-01', '2027-07-15')",
		"INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES ('c6000000-0000-0000-0000-000000000002', '" + schoolTenant + "', 'Primaria', 'CYCLE')",
		"INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES ('c6000000-0000-0000-0000-000000000011', '" + schoolTenant + "', 'c6000000-0000-0000-0000-000000000002', '1°', 1)",
		"INSERT INTO subjects (id, tenant_id, grade_level_id, name) VALUES ('c6000000-0000-0000-0000-000000000021', '" + schoolTenant + "', 'c6000000-0000-0000-0000-000000000011', 'Español')",
		"INSERT INTO subjects (id, tenant_id, grade_level_id, name) VALUES ('c6000000-0000-0000-0000-000000000022', '" + schoolTenant + "', 'c6000000-0000-0000-0000-000000000011', 'Artes')",
		"INSERT INTO groups (id, tenant_id, cycle_id, grade_level_id, name) VALUES ('c6000000-0000-0000-0000-000000000031', '" + schoolTenant + "', 'c6000000-0000-0000-0000-000000000001', 'c6000000-0000-0000-0000-000000000011', 'A')",
		"INSERT INTO teachers (id, tenant_id, first_name, last_name, email) VALUES ('c6000000-0000-0000-0000-000000000041', '" + schoolTenant + "', 'x', 'y', 'x@example.com')",
		"INSERT INTO teaching_assignments (id, tenant_id, cycle_id, group_id, subject_id, teacher_id) VALUES ('c6000000-0000-0000-0000-000000000051', '" + schoolTenant + "', 'c6000000-0000-0000-0000-000000000001', 'c6000000-0000-0000-0000-000000000031', 'c6000000-0000-0000-0000-000000000021', 'c6000000-0000-0000-0000-000000000041')",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := tryMigration(t, db, "0015_subject_identity.sql"); err != nil {
		t.Fatalf("0015: %v", err)
	}
	if n := scalar[int64](t, db, "SELECT count(*) FROM subjects WHERE first_assigned_at IS NULL"); n != 0 {
		t.Errorf("every pre-existing subject must be marked used, %d are not", n)
	}
}
