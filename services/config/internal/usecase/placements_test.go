package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// --- fake PlacementStore -----------------------------------------------------
//
// It models the adapter's contract: lookups by folded level name, by grade
// ordinal and by normalized group name; Insert* as find-or-create; and the
// transaction — an error from fn puts every slice back.

type insertCounts struct{ levels, grades, groups int }

type fakePlacements struct {
	cycles  map[string]bool
	levels  []domain.Level
	grades  []domain.GradeLevel
	groups  []domain.Group
	inserts insertCounts
	txCalls int
	// concurrent is a level another import commits just before our insert.
	concurrent *domain.Level
	// cycleErr makes CycleExists fail, lookupErr every level lookup (an
	// infrastructure fault).
	cycleErr  error
	lookupErr error
}

func (f *fakePlacements) InPlacementTx(_ context.Context, fn func(ports.PlacementTx) error) error {
	f.txCalls++
	levels, grades, groups, inserts := slices.Clone(f.levels), slices.Clone(f.grades), slices.Clone(f.groups), f.inserts
	if err := fn(f); err != nil {
		f.levels, f.grades, f.groups, f.inserts = levels, grades, groups, inserts
		return err
	}
	return nil
}

func (f *fakePlacements) CycleExists(id string) (bool, error) {
	return f.cycles[id], f.cycleErr
}

func (f *fakePlacements) LockLevel(id string) (domain.Level, bool, error) {
	if f.lookupErr != nil {
		return domain.Level{}, false, f.lookupErr
	}
	for _, l := range f.levels {
		if l.ID() == id {
			return l, true, nil
		}
	}
	return domain.Level{}, false, nil
}

func (f *fakePlacements) LockLevelByName(name string) (domain.Level, bool, error) {
	for _, l := range f.levels {
		if domain.FoldName(l.Name()) == domain.FoldName(name) {
			return l, true, nil
		}
	}
	return domain.Level{}, false, nil
}

func (f *fakePlacements) InsertLevel(l domain.Level) (domain.Level, bool, error) {
	if f.concurrent != nil {
		f.levels = append(f.levels, *f.concurrent)
		f.concurrent = nil
	}
	if found, ok, _ := f.LockLevelByName(l.Name()); ok {
		return found, false, nil
	}
	f.levels = append(f.levels, l)
	f.inserts.levels++
	return l, true, nil
}

func (f *fakePlacements) GradeByOrdinal(levelID string, ordinal int) (domain.GradeLevel, bool, error) {
	for _, g := range f.grades {
		if g.LevelID() == levelID && g.Ordinal() == ordinal {
			return g, true, nil
		}
	}
	return domain.GradeLevel{}, false, nil
}

func (f *fakePlacements) GradeNameTaken(levelID, name string) (bool, error) {
	for _, g := range f.grades {
		if g.LevelID() == levelID && strings.EqualFold(g.Name(), name) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakePlacements) InsertGrade(g domain.GradeLevel) (domain.GradeLevel, bool, error) {
	if found, ok, _ := f.GradeByOrdinal(g.LevelID(), g.Ordinal()); ok {
		return found, false, nil
	}
	if taken, _ := f.GradeNameTaken(g.LevelID(), g.Name()); taken {
		return domain.GradeLevel{}, false, ports.ErrGradeNameTaken
	}
	f.grades = append(f.grades, g)
	f.inserts.grades++
	return g, true, nil
}

func (f *fakePlacements) GroupByName(cycleID, gradeLevelID, name string) (domain.Group, bool, error) {
	for _, g := range f.groups {
		if g.CycleID() == cycleID && g.GradeLevelID() == gradeLevelID && strings.ToUpper(strings.TrimSpace(g.Name())) == name {
			return g, true, nil
		}
	}
	return domain.Group{}, false, nil
}

func (f *fakePlacements) InsertGroup(g domain.Group) (domain.Group, bool, error) {
	if found, ok, _ := f.GroupByName(g.CycleID(), g.GradeLevelID(), g.Name()); ok {
		return found, false, nil
	}
	f.groups = append(f.groups, g)
	f.inserts.groups++
	return g, true, nil
}

// --- fixtures ----------------------------------------------------------------

const placementCycle = "cyc-1"

func mustLevel(t *testing.T, id, name string, deleting bool) domain.Level {
	t.Helper()
	l, err := domain.NewLevel(id, tenant, name, "", domain.GranularityCycle)
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	return l.WithDeleting(deleting)
}

func mustGrade(t *testing.T, id, levelID, name string, ordinal int) domain.GradeLevel {
	t.Helper()
	g, err := domain.NewGradeLevel(id, tenant, levelID, name, ordinal)
	if err != nil {
		t.Fatalf("NewGradeLevel: %v", err)
	}
	return g
}

func mustGroup(t *testing.T, id, gradeID, name string, capacity int) domain.Group {
	t.Helper()
	g, err := domain.NewGroup(id, tenant, placementCycle, gradeID, name, capacity)
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	return g
}

// newPlacementFixture is a school with Primaria (1° and, at ordinal 3, a grade
// misnamed "2°"; group 1°A of 25 seats), Preparatoria, and Kinder mid-deletion.
func newPlacementFixture(t *testing.T) *fakePlacements {
	t.Helper()
	return &fakePlacements{
		cycles: map[string]bool{placementCycle: true},
		levels: []domain.Level{
			mustLevel(t, "lvl-prim", "Primaria", false),
			mustLevel(t, "lvl-prepa", "Preparatoria", false),
			mustLevel(t, "lvl-del", "Kinder", true),
		},
		grades: []domain.GradeLevel{
			mustGrade(t, "gr-prim-1", "lvl-prim", "1°", 1),
			mustGrade(t, "gr-prim-3", "lvl-prim", "2°", 3),
		},
		groups: []domain.Group{mustGroup(t, "grp-1A", "gr-prim-1", "A", 25)},
	}
}

func newPlacementService(store ports.PlacementStore) *usecase.PlacementService {
	n := 0
	return usecase.NewPlacementService(store, tenant, func() string {
		n++
		return fmt.Sprintf("id-%d", n)
	})
}

func invalid(code string) usecase.Placement { return usecase.Placement{Error: code} }

// --- tests -------------------------------------------------------------------

func TestEnsurePlacements(t *testing.T) {
	maternal := usecase.PlacementRequest{NewLevelName: "Maternal", GradeOrdinal: 1, GroupName: "A", Capacity: 20}
	newMaternal := usecase.Placement{
		LevelID: "id-1", LevelName: "Maternal", LevelCreated: true,
		GradeLevelID: "id-2", GradeName: "1°", GradeCreated: true,
		GroupID: "id-3", GroupName: "A", GroupCreated: true, Capacity: 20,
	}
	tests := []struct {
		name        string
		reqs        []usecase.PlacementRequest
		dryRun      bool
		concurrent  string // a level of this name is committed by someone else first
		want        []usecase.Placement
		wantInserts insertCounts
	}{
		{
			name: "everything exists: matched, nothing created, capacity untouched",
			reqs: []usecase.PlacementRequest{{LevelID: "lvl-prim", GradeOrdinal: 1, GroupName: " a ", Capacity: 40}},
			want: []usecase.Placement{{
				LevelID: "lvl-prim", LevelName: "Primaria", GradeLevelID: "gr-prim-1", GradeName: "1°",
				GroupID: "grp-1A", GroupName: "A", Capacity: 25,
			}},
		},
		{
			name: "an existing group needs no capacity",
			reqs: []usecase.PlacementRequest{{LevelID: "lvl-prim", GradeOrdinal: 1, GroupName: "A"}},
			want: []usecase.Placement{{
				LevelID: "lvl-prim", LevelName: "Primaria", GradeLevelID: "gr-prim-1", GradeName: "1°",
				GroupID: "grp-1A", GroupName: "A", Capacity: 25,
			}},
		},
		{
			name: "unknown or malformed level id",
			reqs: []usecase.PlacementRequest{
				{LevelID: "lvl-none", GradeOrdinal: 1, GroupName: "A", Capacity: 30},
				{LevelID: "not a uuid", GradeOrdinal: 1, GroupName: "A", Capacity: 30},
			},
			want: []usecase.Placement{invalid(usecase.PlacementLevelNotFound), invalid(usecase.PlacementLevelNotFound)},
		},
		{
			name: "a level being deleted, by id or by name",
			reqs: []usecase.PlacementRequest{
				{LevelID: "lvl-del", GradeOrdinal: 1, GroupName: "A", Capacity: 30},
				{NewLevelName: "KÍNDER", GradeOrdinal: 1, GroupName: "A", Capacity: 30},
			},
			want: []usecase.Placement{invalid(usecase.PlacementLevelBeingDeleted), invalid(usecase.PlacementLevelBeingDeleted)},
		},
		{
			name: "a new level name that folds to an existing one reuses it",
			reqs: []usecase.PlacementRequest{{NewLevelName: "  PREPARATORIA ", GradeOrdinal: 1, GroupName: "b", Capacity: 30}},
			want: []usecase.Placement{{
				LevelID: "lvl-prepa", LevelName: "Preparatoria",
				GradeLevelID: "id-1", GradeName: "1°", GradeCreated: true,
				GroupID: "id-2", GroupName: "B", GroupCreated: true, Capacity: 30,
			}},
			wantInserts: insertCounts{grades: 1, groups: 1},
		},
		{
			name: "level_id wins over new_level_name",
			reqs: []usecase.PlacementRequest{{LevelID: "lvl-prim", NewLevelName: "Otro", GradeOrdinal: 1, GroupName: "A"}},
			want: []usecase.Placement{{
				LevelID: "lvl-prim", LevelName: "Primaria", GradeLevelID: "gr-prim-1", GradeName: "1°",
				GroupID: "grp-1A", GroupName: "A", Capacity: 25,
			}},
		},
		{
			name: "an unknown level is created with its grade and group",
			reqs: []usecase.PlacementRequest{{NewLevelName: "  Secundaria   Técnica ", GradeOrdinal: 2, GroupName: "a", Capacity: 35}},
			want: []usecase.Placement{{
				LevelID: "id-1", LevelName: "Secundaria Técnica", LevelCreated: true,
				GradeLevelID: "id-2", GradeName: "2°", GradeCreated: true,
				GroupID: "id-3", GroupName: "A", GroupCreated: true, Capacity: 35,
			}},
			wantInserts: insertCounts{levels: 1, grades: 1, groups: 1},
		},
		{
			name: "a grade name held at another ordinal is invalid, nothing created",
			reqs: []usecase.PlacementRequest{{LevelID: "lvl-prim", GradeOrdinal: 2, GroupName: "A", Capacity: 30}},
			want: []usecase.Placement{invalid(usecase.PlacementInvalid)},
		},
		{
			name: "a group to create needs a capacity; the level is not created either",
			reqs: []usecase.PlacementRequest{
				{LevelID: "lvl-prim", GradeOrdinal: 1, GroupName: "C", Capacity: 0},
				{NewLevelName: "Maternal", GradeOrdinal: 1, GroupName: "A", Capacity: -1},
			},
			want: []usecase.Placement{invalid(usecase.PlacementInvalid), invalid(usecase.PlacementInvalid)},
		},
		{
			name: "malformed placements",
			reqs: []usecase.PlacementRequest{
				{LevelID: "lvl-prim", GradeOrdinal: 0, GroupName: "A", Capacity: 30},
				{LevelID: "lvl-prim", GradeOrdinal: 1, GroupName: "   ", Capacity: 30},
				{NewLevelName: " \t ", GradeOrdinal: 1, GroupName: "A", Capacity: 30},
			},
			want: []usecase.Placement{invalid(usecase.PlacementInvalid), invalid(usecase.PlacementInvalid), invalid(usecase.PlacementInvalid)},
		},
		{
			name:        "identical placements: one answer, created once",
			reqs:        []usecase.PlacementRequest{maternal, maternal},
			want:        []usecase.Placement{newMaternal, newMaternal},
			wantInserts: insertCounts{levels: 1, grades: 1, groups: 1},
		},
		{
			name: "placements sharing a new level create it once and all say so",
			reqs: []usecase.PlacementRequest{maternal, {NewLevelName: "MATERNAL", GradeOrdinal: 2, GroupName: "A", Capacity: 20}},
			want: []usecase.Placement{newMaternal, {
				LevelID: "id-1", LevelName: "Maternal", LevelCreated: true,
				GradeLevelID: "id-4", GradeName: "2°", GradeCreated: true,
				GroupID: "id-5", GroupName: "A", GroupCreated: true, Capacity: 20,
			}},
			wantInserts: insertCounts{levels: 1, grades: 2, groups: 2},
		},
		{
			name:       "a level another import committed first is reused, not duplicated",
			reqs:       []usecase.PlacementRequest{maternal},
			concurrent: "MATERNAL",
			want: []usecase.Placement{{
				LevelID: "lvl-other", LevelName: "MATERNAL",
				GradeLevelID: "id-2", GradeName: "1°", GradeCreated: true,
				GroupID: "id-3", GroupName: "A", GroupCreated: true, Capacity: 20,
			}},
			wantInserts: insertCounts{grades: 1, groups: 1},
		},
		{
			name:   "dry run: what would be created, without ids, nothing written",
			reqs:   []usecase.PlacementRequest{maternal, maternal, {NewLevelName: "Preparatoria", GradeOrdinal: 1, GroupName: "A", Capacity: 30}},
			dryRun: true,
			want: []usecase.Placement{
				{LevelName: "Maternal", LevelCreated: true, GradeName: "1°", GradeCreated: true, GroupName: "A", GroupCreated: true, Capacity: 20},
				{LevelName: "Maternal", LevelCreated: true, GradeName: "1°", GradeCreated: true, GroupName: "A", GroupCreated: true, Capacity: 20},
				{LevelID: "lvl-prepa", LevelName: "Preparatoria", GradeName: "1°", GradeCreated: true, GroupName: "A", GroupCreated: true, Capacity: 30},
			},
		},
		{
			name:   "dry run reports existing ids and the same errors as a commit",
			reqs:   []usecase.PlacementRequest{{LevelID: "lvl-prim", GradeOrdinal: 1, GroupName: "A"}, {LevelID: "lvl-prim", GradeOrdinal: 2, GroupName: "A", Capacity: 9}},
			dryRun: true,
			want: []usecase.Placement{
				{LevelID: "lvl-prim", LevelName: "Primaria", GradeLevelID: "gr-prim-1", GradeName: "1°", GroupID: "grp-1A", GroupName: "A", Capacity: 25},
				invalid(usecase.PlacementInvalid),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newPlacementFixture(t)
			if tt.concurrent != "" {
				other := mustLevel(t, "lvl-other", tt.concurrent, false)
				store.concurrent = &other
			}
			got, err := newPlacementService(store).EnsurePlacements(context.Background(), placementCycle, tt.reqs, tt.dryRun)
			if err != nil {
				t.Fatalf("EnsurePlacements: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("placements\n got: %+v\nwant: %+v", got, tt.want)
			}
			if store.inserts != tt.wantInserts {
				t.Errorf("inserts = %+v, want %+v", store.inserts, tt.wantInserts)
			}
			if store.txCalls != 1 {
				t.Errorf("the whole call must be ONE transaction, ran %d", store.txCalls)
			}
		})
	}
}

// A created level gets what CreateLevel would give it: per-cycle enrollment,
// no SEP code, and the attendance and grading defaults for its name.
func TestEnsurePlacementsCreatesALevelWithCreateLevelDefaults(t *testing.T) {
	store := newPlacementFixture(t)
	_, err := newPlacementService(store).EnsurePlacements(context.Background(), placementCycle,
		[]usecase.PlacementRequest{{NewLevelName: "Secundaria", GradeOrdinal: 1, GroupName: "A", Capacity: 30}}, false)
	if err != nil {
		t.Fatalf("EnsurePlacements: %v", err)
	}
	created, ok, _ := store.LockLevelByName("secundaria")
	if !ok {
		t.Fatal("the level was not stored")
	}
	want := mustLevel(t, created.ID(), "Secundaria", false)
	if !reflect.DeepEqual(created, want) {
		t.Errorf("created level\n got: %+v\nwant: %+v", created, want)
	}
	if created.AttendanceMode() != domain.AttendancePerClass {
		t.Errorf("Secundaria must start per class, got %v", created.AttendanceMode())
	}
}

func TestEnsurePlacementsRefusesTheCall(t *testing.T) {
	tooMany := make([]usecase.PlacementRequest, usecase.MaxPlacements+1)
	for i := range tooMany {
		tooMany[i] = usecase.PlacementRequest{LevelID: "lvl-prim", GradeOrdinal: 1, GroupName: "A"}
	}
	tests := []struct {
		name      string
		cycleID   string
		reqs      []usecase.PlacementRequest
		cycleErr  error
		lookupErr error
		wantKind  glerrors.Kind
		wantTx    int
	}{
		{name: "more than the maximum", cycleID: placementCycle, reqs: tooMany, wantKind: glerrors.KindInvalid},
		{name: "unknown cycle", cycleID: "cyc-none", reqs: tooMany[:1], wantKind: glerrors.KindInvalid, wantTx: 1},
		{name: "store fault", cycleID: placementCycle, reqs: tooMany[:1], cycleErr: glerrors.New(glerrors.KindInternal, "db down"), wantKind: glerrors.KindInternal, wantTx: 1},
		{name: "lookup fault", cycleID: placementCycle, reqs: tooMany[:1], lookupErr: glerrors.New(glerrors.KindInternal, "db down"), wantKind: glerrors.KindInternal, wantTx: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newPlacementFixture(t)
			store.cycleErr, store.lookupErr = tt.cycleErr, tt.lookupErr
			got, err := newPlacementService(store).EnsurePlacements(context.Background(), tt.cycleID, tt.reqs, false)
			if glerrors.KindOf(err) != tt.wantKind {
				t.Fatalf("kind = %v (%v), want %v", glerrors.KindOf(err), err, tt.wantKind)
			}
			if got != nil {
				t.Errorf("a refused call answers nothing, got %+v", got)
			}
			if store.txCalls != tt.wantTx {
				t.Errorf("transactions = %d, want %d", store.txCalls, tt.wantTx)
			}
		})
	}
}

// Exactly the maximum is allowed.
func TestEnsurePlacementsAcceptsTheMaximum(t *testing.T) {
	reqs := make([]usecase.PlacementRequest, usecase.MaxPlacements)
	for i := range reqs {
		reqs[i] = usecase.PlacementRequest{LevelID: "lvl-prim", GradeOrdinal: 1, GroupName: "A"}
	}
	got, err := newPlacementService(newPlacementFixture(t)).EnsurePlacements(context.Background(), placementCycle, reqs, false)
	if err != nil || len(got) != usecase.MaxPlacements {
		t.Fatalf("got %d placements, err %v", len(got), err)
	}
}

// A fault mid-call rolls the whole commit back: nothing half-created stays.
func TestEnsurePlacementsRollsBackOnAFault(t *testing.T) {
	store := &faultyInsertGroup{fakePlacements: newPlacementFixture(t)}
	_, err := newPlacementService(store).EnsurePlacements(context.Background(), placementCycle,
		[]usecase.PlacementRequest{{NewLevelName: "Maternal", GradeOrdinal: 1, GroupName: "A", Capacity: 20}}, false)
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("kind = %v, want KindInternal", glerrors.KindOf(err))
	}
	if store.inserts != (insertCounts{}) || len(store.levels) != 3 {
		t.Errorf("a failed call must leave nothing behind: %+v, %d levels", store.inserts, len(store.levels))
	}
}

type faultyInsertGroup struct{ *fakePlacements }

func (f *faultyInsertGroup) InPlacementTx(ctx context.Context, fn func(ports.PlacementTx) error) error {
	return f.fakePlacements.InPlacementTx(ctx, func(ports.PlacementTx) error { return fn(f) })
}

func (f *faultyInsertGroup) InsertGroup(domain.Group) (domain.Group, bool, error) {
	return domain.Group{}, false, glerrors.Wrap(glerrors.KindInternal, "inserting group", errors.New("connection reset"))
}

// What a concurrent writer leaves between our lookup and our insert is decided
// per placement, like the lookup would have: a level adopted mid-deletion is
// level_being_deleted, a grade name taken meanwhile is invalid_placement — and
// the placements after it still go through.
func TestEnsurePlacementsJudgesWhatAConcurrentWriterLeft(t *testing.T) {
	store := newPlacementFixture(t)
	deleting := mustLevel(t, "lvl-other", "Maternal", true)
	store.concurrent = &deleting
	race := &gradeNameRace{fakePlacements: store, level: "lvl-prepa"}
	got, err := newPlacementService(race).EnsurePlacements(context.Background(), placementCycle, []usecase.PlacementRequest{
		{NewLevelName: "Maternal", GradeOrdinal: 1, GroupName: "A", Capacity: 20},
		{LevelID: "lvl-prepa", GradeOrdinal: 4, GroupName: "A", Capacity: 20},
		{LevelID: "lvl-prim", GradeOrdinal: 1, GroupName: "A"},
	}, false)
	if err != nil {
		t.Fatalf("EnsurePlacements: %v", err)
	}
	want := []string{usecase.PlacementLevelBeingDeleted, usecase.PlacementInvalid, ""}
	for i, p := range got {
		if p.Error != want[i] {
			t.Errorf("placement %d: error %q, want %q", i, p.Error, want[i])
		}
	}
	if got[2].GroupID != "grp-1A" {
		t.Errorf("the last placement must still resolve: %+v", got[2])
	}
}

// gradeNameRace commits "<n>°" at another ordinal of level just before our
// grade insert.
type gradeNameRace struct {
	*fakePlacements
	level string
}

func (f *gradeNameRace) InPlacementTx(ctx context.Context, fn func(ports.PlacementTx) error) error {
	return f.fakePlacements.InPlacementTx(ctx, func(ports.PlacementTx) error { return fn(f) })
}

func (f *gradeNameRace) InsertGrade(g domain.GradeLevel) (domain.GradeLevel, bool, error) {
	if g.LevelID() == f.level {
		other, _ := domain.NewGradeLevel("gr-race", tenant, g.LevelID(), g.Name(), g.Ordinal()+10)
		f.grades = append(f.grades, other)
	}
	return f.fakePlacements.InsertGrade(g)
}
