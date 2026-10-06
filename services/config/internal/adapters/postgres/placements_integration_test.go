//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// placementSchool is one tenant's repository, placement use case and a fresh
// cycle of its own (every test makes its own, so they never share rows).
type placementSchool struct {
	repo    *pgadapter.Repository
	svc     *usecase.PlacementService
	cycleID string
}

func newPlacementSchool(t *testing.T, tenantID string) placementSchool {
	t.Helper()
	repo := pgadapter.NewRepository(appDB(t), tenantID)
	cycleID := uuid.NewString()
	cycle, err := domain.NewCycle(cycleID, tenantID, "Import "+cycleID, domain.CycleStatusActive, startsOn, endsOn)
	if err != nil {
		t.Fatalf("NewCycle: %v", err)
	}
	if err := repo.CreateCycle(context.Background(), cycle); err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}
	return placementSchool{repo: repo, svc: usecase.NewPlacementService(repo, tenantID, uuid.NewString), cycleID: cycleID}
}

func (s placementSchool) ensure(t *testing.T, dryRun bool, reqs ...usecase.PlacementRequest) []usecase.Placement {
	t.Helper()
	got, err := s.svc.EnsurePlacements(context.Background(), s.cycleID, reqs, dryRun)
	if err != nil {
		t.Fatalf("EnsurePlacements: %v", err)
	}
	if len(got) != len(reqs) {
		t.Fatalf("got %d answers for %d placements", len(got), len(reqs))
	}
	return got
}

// countAs counts rows as tenantID sees them (RLS applies).
func countAs(t *testing.T, tenantID, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	err := appDB(t).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantID).Error; err != nil {
			return err
		}
		return tx.Table(table).Where(where, args...).Count(&n).Error
	})
	if err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func levelsNamed(t *testing.T, tenantID, name string) int64 {
	return countAs(t, tenantID, "levels", "config_fold_name(name) = config_fold_name(?)", name)
}

// One call creates a level once, its grades by ordinal and its groups, and a
// second identical call finds every row instead of creating it again.
func TestEnsurePlacementsCreatesOnceAndThenFinds(t *testing.T) {
	s := newPlacementSchool(t, tenantA)
	reqs := []usecase.PlacementRequest{
		{NewLevelName: "Prepa Import Uno", GradeOrdinal: 1, GroupName: "a", Capacity: 30},
		{NewLevelName: "Prepa Import Uno", GradeOrdinal: 1, GroupName: "a", Capacity: 30},
		{NewLevelName: " PREPA  IMPORT UNO", GradeOrdinal: 2, GroupName: "B", Capacity: 25},
	}
	first := s.ensure(t, false, reqs...)
	if first[0] != first[1] {
		t.Errorf("identical placements must answer alike:\n%+v\n%+v", first[0], first[1])
	}
	if !first[0].LevelCreated || !first[0].GradeCreated || !first[0].GroupCreated || first[0].GroupName != "A" || first[0].GradeName != "1°" {
		t.Errorf("first placement: %+v", first[0])
	}
	if first[2].LevelID != first[0].LevelID || first[2].GradeName != "2°" || first[2].Capacity != 25 {
		t.Errorf("third placement must share the level: %+v", first[2])
	}
	if n := levelsNamed(t, tenantA, "Prepa Import Uno"); n != 1 {
		t.Errorf("levels = %d, want 1", n)
	}
	if n := countAs(t, tenantA, "grade_levels", "level_id = ?", first[0].LevelID); n != 2 {
		t.Errorf("grades = %d, want 2", n)
	}
	if n := countAs(t, tenantA, "groups", "cycle_id = ?", s.cycleID); n != 2 {
		t.Errorf("groups = %d, want 2", n)
	}
	level, err := s.repo.GetLevel(context.Background(), first[0].LevelID)
	if err != nil {
		t.Fatalf("GetLevel: %v", err)
	}
	if level.Granularity() != domain.GranularityCycle || level.SEPCode() != "" || level.Name() != "Prepa Import Uno" {
		t.Errorf("created level: %+v", level)
	}

	again := s.ensure(t, false, reqs...)
	for i, p := range again {
		if p.LevelCreated || p.GradeCreated || p.GroupCreated {
			t.Errorf("second call, placement %d created something: %+v", i, p)
		}
		if p.GroupID != first[i].GroupID || p.GradeLevelID != first[i].GradeLevelID || p.LevelID != first[i].LevelID {
			t.Errorf("second call, placement %d resolved elsewhere: %+v vs %+v", i, p, first[i])
		}
	}
}

// A dry run reads the catalog and writes nothing.
func TestEnsurePlacementsDryRunWritesNothing(t *testing.T) {
	s := newPlacementSchool(t, tenantA)
	got := s.ensure(t, true, usecase.PlacementRequest{NewLevelName: "Dry Run Nivel", GradeOrdinal: 1, GroupName: "A", Capacity: 30})
	want := usecase.Placement{LevelName: "Dry Run Nivel", LevelCreated: true, GradeName: "1°", GradeCreated: true, GroupName: "A", GroupCreated: true, Capacity: 30}
	if got[0] != want {
		t.Errorf("dry run\n got: %+v\nwant: %+v", got[0], want)
	}
	if n := levelsNamed(t, tenantA, "Dry Run Nivel"); n != 0 {
		t.Errorf("a dry run created %d levels", n)
	}
	if n := countAs(t, tenantA, "groups", "cycle_id = ?", s.cycleID); n != 0 {
		t.Errorf("a dry run created %d groups", n)
	}
}

// A spreadsheet's "SECUNDARIA" is the school's existing "Secundaria", and an
// existing group keeps its capacity.
func TestEnsurePlacementsMatchesTheFoldedName(t *testing.T) {
	s := newPlacementSchool(t, tenantA)
	ctx := context.Background()
	levelID, gradeID, groupID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if err := s.repo.CreateLevel(ctx, newCycleLevel(t, levelID, tenantA, "Secundaria Técnica Match")); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	grade, _ := domain.NewGradeLevel(gradeID, tenantA, levelID, "Primero", 1)
	if err := s.repo.CreateGradeLevel(ctx, grade); err != nil {
		t.Fatalf("CreateGradeLevel: %v", err)
	}
	group, _ := domain.NewGroup(groupID, tenantA, s.cycleID, gradeID, "A", 18)
	if err := s.repo.CreateGroup(ctx, group); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	got := s.ensure(t, false, usecase.PlacementRequest{NewLevelName: "SECUNDARIA  TECNICA MATCH", GradeOrdinal: 1, GroupName: " a", Capacity: 40})
	want := usecase.Placement{
		LevelID: levelID, LevelName: "Secundaria Técnica Match", GradeLevelID: gradeID, GradeName: "Primero",
		GroupID: groupID, GroupName: "A", Capacity: 18,
	}
	if got[0] != want {
		t.Errorf("\n got: %+v\nwant: %+v", got[0], want)
	}
}

// A "<n>°" already used at another ordinal cannot be created at this one: the
// placement is invalid and nothing of it is written.
func TestEnsurePlacementsRefusesAGradeNameHeldElsewhere(t *testing.T) {
	s := newPlacementSchool(t, tenantA)
	ctx := context.Background()
	levelID := uuid.NewString()
	if err := s.repo.CreateLevel(ctx, newCycleLevel(t, levelID, tenantA, "Grade Clash Nivel")); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	misnamed, _ := domain.NewGradeLevel(uuid.NewString(), tenantA, levelID, "2°", 3)
	if err := s.repo.CreateGradeLevel(ctx, misnamed); err != nil {
		t.Fatalf("CreateGradeLevel: %v", err)
	}
	got := s.ensure(t, false, usecase.PlacementRequest{LevelID: levelID, GradeOrdinal: 2, GroupName: "A", Capacity: 30})
	if got[0].Error != usecase.PlacementInvalid {
		t.Errorf("got %+v, want invalid_placement", got[0])
	}
	if n := countAs(t, tenantA, "grade_levels", "level_id = ?", levelID); n != 1 {
		t.Errorf("grades = %d, want the one that was there", n)
	}
}

// A level mid-deletion is answered per placement, by id and by name, and the
// call goes on with the others.
func TestEnsurePlacementsSkipsALevelBeingDeleted(t *testing.T) {
	s := newPlacementSchool(t, tenantA)
	levelID := uuid.NewString()
	if err := s.repo.CreateLevel(context.Background(), newCycleLevel(t, levelID, tenantA, "Deleting Placement Nivel")); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	if n := writeAsTenant(t, tenantA, "UPDATE levels SET status = 'deleting' WHERE id = ?", levelID); n != 1 {
		t.Fatalf("marking the level deleting affected %d rows", n)
	}
	got := s.ensure(t, false,
		usecase.PlacementRequest{LevelID: levelID, GradeOrdinal: 1, GroupName: "A", Capacity: 30},
		usecase.PlacementRequest{NewLevelName: "DELETING PLACEMENT NIVEL", GradeOrdinal: 1, GroupName: "A", Capacity: 30},
		usecase.PlacementRequest{NewLevelName: "Survivor Placement Nivel", GradeOrdinal: 1, GroupName: "A", Capacity: 30},
	)
	for i := range 2 {
		if got[i].Error != usecase.PlacementLevelBeingDeleted {
			t.Errorf("placement %d: %+v, want level_being_deleted", i, got[i])
		}
	}
	if got[2].Error != "" || !got[2].GroupCreated {
		t.Errorf("the other placement must go through: %+v", got[2])
	}
	if n := countAs(t, tenantA, "grade_levels", "level_id = ?", levelID); n != 0 {
		t.Errorf("a grade was written under a level being deleted")
	}
}

// The import share-locks the levels it resolves: a level deletion cannot
// begin (UPDATE levels SET status) until the import's transaction ends.
func TestEnsurePlacementsHoldsTheLevelAgainstADeletion(t *testing.T) {
	s := newPlacementSchool(t, tenantA)
	levelID := uuid.NewString()
	if err := s.repo.CreateLevel(context.Background(), newCycleLevel(t, levelID, tenantA, "Locked Placement Nivel")); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	err := s.repo.InPlacementTx(context.Background(), func(tx ports.PlacementTx) error {
		if _, found, err := tx.LockLevel(levelID); err != nil || !found {
			t.Fatalf("LockLevel: found=%v err=%v", found, err)
		}
		begin := appDB(t).Transaction(func(other *gorm.DB) error {
			for _, stmt := range []string{"SET LOCAL lock_timeout = '300ms'", "SELECT set_config('app.current_tenant', '" + tenantA + "', true)"} {
				if err := other.Exec(stmt).Error; err != nil {
					return err
				}
			}
			return other.Exec("UPDATE levels SET status = 'deleting' WHERE id = ?", levelID).Error
		})
		var pgErr *pgconn.PgError
		if !errors.As(begin, &pgErr) || pgErr.Code != "55P03" {
			t.Errorf("a deletion must wait for the import (lock_not_available), got %v", begin)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("InPlacementTx: %v", err)
	}
}

// Two imports of the same spreadsheet at once never duplicate a row: one
// creates, the other finds — and both answer the same ids.
func TestEnsurePlacementsConcurrentCallsNeverDuplicate(t *testing.T) {
	const rounds = 10 // several races, each on fresh names, to catch an interleaving
	for round := range rounds {
		s := newPlacementSchool(t, tenantA)
		other := usecase.NewPlacementService(pgadapter.NewRepository(appDB(t), tenantA), tenantA, uuid.NewString)
		name := "Concurrent Nivel " + s.cycleID[:8]
		reqs := []usecase.PlacementRequest{
			{NewLevelName: name, GradeOrdinal: 1, GroupName: "A", Capacity: 30},
			{NewLevelName: name, GradeOrdinal: 1, GroupName: "B", Capacity: 30},
			{NewLevelName: name, GradeOrdinal: 2, GroupName: "A", Capacity: 30},
		}
		answers := raceEnsure(t, s.cycleID, reqs, s.svc, other)
		if n := levelsNamed(t, tenantA, name); n != 1 {
			t.Errorf("round %d: levels = %d, want 1", round, n)
		}
		if n := countAs(t, tenantA, "grade_levels", "level_id = ?", answers[0][0].LevelID); n != 2 {
			t.Errorf("round %d: grades = %d, want 2", round, n)
		}
		if n := countAs(t, tenantA, "groups", "cycle_id = ?", s.cycleID); n != 3 {
			t.Errorf("round %d: groups = %d, want 3", round, n)
		}
		for i := range reqs {
			a, b := answers[0][i], answers[1][i]
			if a.LevelID != b.LevelID || a.GradeLevelID != b.GradeLevelID || a.GroupID != b.GroupID {
				t.Errorf("round %d: placement %d resolved differently:\n%+v\n%+v", round, i, a, b)
			}
		}
		if answers[0][0].LevelCreated == answers[1][0].LevelCreated {
			t.Errorf("round %d: exactly one call must report creating the level", round)
		}
	}
}

// raceEnsure releases both calls at once and returns their answers.
func raceEnsure(t *testing.T, cycleID string, reqs []usecase.PlacementRequest, svcs ...*usecase.PlacementService) [][]usecase.Placement {
	t.Helper()
	var (
		wg      sync.WaitGroup
		start   = make(chan struct{})
		answers = make([][]usecase.Placement, len(svcs))
		errs    = make([]error, len(svcs))
	)
	for i, svc := range svcs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			answers[i], errs[i] = svc.EnsurePlacements(context.Background(), cycleID, reqs, false)
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	return answers
}

// The find-or-create inserts yield to existing rows instead of failing: the
// level to its folded name (ON CONFLICT on the 0019 expression index), the
// grade to its ordinal or — reported — to its name, the group to its name.
func TestPlacementInsertsYieldToExistingRows(t *testing.T) {
	s := newPlacementSchool(t, tenantA)
	ctx := context.Background()
	levelID, gradeID, groupID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if err := s.repo.CreateLevel(ctx, newCycleLevel(t, levelID, tenantA, "Yield Nivel")); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	grade, _ := domain.NewGradeLevel(gradeID, tenantA, levelID, "1°", 1)
	if err := s.repo.CreateGradeLevel(ctx, grade); err != nil {
		t.Fatalf("CreateGradeLevel: %v", err)
	}
	group, _ := domain.NewGroup(groupID, tenantA, s.cycleID, gradeID, "A", 12)
	if err := s.repo.CreateGroup(ctx, group); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	err := s.repo.InPlacementTx(ctx, func(tx ports.PlacementTx) error {
		l, created, err := tx.InsertLevel(newCycleLevel(t, uuid.NewString(), tenantA, "YIELD  NIVEL"))
		if err != nil || created || l.ID() != levelID {
			t.Errorf("InsertLevel: id=%s created=%v err=%v", l.ID(), created, err)
		}
		sameOrdinal, _ := domain.NewGradeLevel(uuid.NewString(), tenantA, levelID, "Primero", 1)
		g, created, err := tx.InsertGrade(sameOrdinal)
		if err != nil || created || g.ID() != gradeID {
			t.Errorf("InsertGrade by ordinal: id=%s created=%v err=%v", g.ID(), created, err)
		}
		sameName, _ := domain.NewGradeLevel(uuid.NewString(), tenantA, levelID, "1°", 7)
		if _, _, err := tx.InsertGrade(sameName); !errors.Is(err, ports.ErrGradeNameTaken) {
			t.Errorf("InsertGrade by name: err=%v, want ErrGradeNameTaken", err)
		}
		again, _ := domain.NewGroup(uuid.NewString(), tenantA, s.cycleID, gradeID, "A", 40)
		gr, created, err := tx.InsertGroup(again)
		if err != nil || created || gr.ID() != groupID || gr.Capacity() != 12 {
			t.Errorf("InsertGroup: %+v created=%v err=%v", gr, created, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("InPlacementTx: %v", err)
	}
}

// Each school folds its own names: tenant B's import creates its own level of
// the very same name and never sees tenant A's.
func TestEnsurePlacementsIsPerTenant(t *testing.T) {
	a := newPlacementSchool(t, tenantA)
	b := newPlacementSchool(t, tenantB)
	req := usecase.PlacementRequest{NewLevelName: "Tenant Placement Nivel", GradeOrdinal: 1, GroupName: "A", Capacity: 30}
	gotA := a.ensure(t, false, req)
	gotB := b.ensure(t, false, req)
	if !gotA[0].LevelCreated || !gotB[0].LevelCreated || gotA[0].LevelID == gotB[0].LevelID {
		t.Errorf("each school must get its own level:\n%+v\n%+v", gotA[0], gotB[0])
	}
	if n := countAs(t, tenantB, "levels", "id = ?", gotA[0].LevelID); n != 0 {
		t.Errorf("tenant B sees tenant A's level (RLS breach)")
	}
	byID, err := b.svc.EnsurePlacements(context.Background(), b.cycleID,
		[]usecase.PlacementRequest{{LevelID: gotA[0].LevelID, GradeOrdinal: 1, GroupName: "A", Capacity: 30}}, true)
	if err != nil || byID[0].Error != usecase.PlacementLevelNotFound {
		t.Errorf("tenant B naming tenant A's level must be level_not_found: %+v, %v", byID, err)
	}
}
