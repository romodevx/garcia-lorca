//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// Tenant G owns the timetable rows of this file; tenant H is the neighbour
// RLS must hide them from.
const (
	tenantG = "77777777-7777-7777-7777-777777777777"
	tenantH = "88888888-8888-8888-8888-888888888888"
)

const (
	scCycle   = "c7000000-0000-0000-0000-000000000001"
	scLevel   = "c7000000-0000-0000-0000-000000000002"
	scGrade   = "c7000000-0000-0000-0000-000000000003"
	scGroupA  = "c7000000-0000-0000-0000-000000000004"
	scGroupB  = "c7000000-0000-0000-0000-000000000005"
	scMath    = "c7000000-0000-0000-0000-000000000006"
	scMusic   = "c7000000-0000-0000-0000-000000000007"
	scAna     = "c7000000-0000-0000-0000-000000000008"
	scLuis    = "c7000000-0000-0000-0000-000000000009"
	scMathA   = "c7000000-0000-0000-0000-00000000000a" // Ana, math, A
	scMathB   = "c7000000-0000-0000-0000-00000000000b" // Ana, math, B
	scMusicA  = "c7000000-0000-0000-0000-00000000000c" // Luis, music, A
	scMusicB  = "c7000000-0000-0000-0000-00000000000d" // Luis, music, B
	scOutside = "c7000000-0000-0000-0000-00000000000e"
)

// seedSchedule builds tenant G's school as the superuser, wiping it first.
func seedSchedule(t *testing.T) {
	t.Helper()
	db := superDB(t)
	for _, stmt := range []string{
		"DELETE FROM schedule_events WHERE tenant_id = @t",
		"DELETE FROM schedule_overrides WHERE tenant_id = @t",
		"DELETE FROM schedule_blocks WHERE tenant_id = @t",
		"DELETE FROM schedule_settings WHERE tenant_id = @t",
		"DELETE FROM teaching_assignments WHERE tenant_id = @t",
		"DELETE FROM groups WHERE tenant_id = @t",
		"DELETE FROM subjects WHERE tenant_id = @t",
		"DELETE FROM grade_levels WHERE tenant_id = @t",
		"DELETE FROM teachers WHERE tenant_id = @t",
		"DELETE FROM levels WHERE tenant_id = @t",
		"DELETE FROM cycles WHERE tenant_id = @t",
	} {
		require.NoError(t, db.Exec(stmt, map[string]any{"t": tenantG}).Error, stmt)
	}
	for _, stmt := range []string{
		`INSERT INTO cycles (id, tenant_id, name, status, starts_on, ends_on) VALUES ('` + scCycle + `', @t, 'G 2026-2027', 'active', '2026-08-24', '2027-07-09')`,
		`INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES ('` + scLevel + `', @t, 'Primaria G', 'CYCLE')`,
		`INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES ('` + scGrade + `', @t, '` + scLevel + `', '3.º', 3)`,
		`INSERT INTO groups (id, tenant_id, cycle_id, grade_level_id, name) VALUES ('` + scGroupA + `', @t, '` + scCycle + `', '` + scGrade + `', 'A'), ('` + scGroupB + `', @t, '` + scCycle + `', '` + scGrade + `', 'B')`,
		`INSERT INTO subjects (id, tenant_id, grade_level_id, name) VALUES ('` + scMath + `', @t, '` + scGrade + `', 'Matemáticas'), ('` + scMusic + `', @t, '` + scGrade + `', 'Música')`,
		`INSERT INTO teachers (id, tenant_id, first_name, last_name, email) VALUES ('` + scAna + `', @t, 'Ana', 'Ruiz', 'ana@g.mx'), ('` + scLuis + `', @t, 'Luis', 'Pérez', 'luis@g.mx')`,
		`INSERT INTO teaching_assignments (id, tenant_id, cycle_id, group_id, subject_id, teacher_id) VALUES
			('` + scMathA + `', @t, '` + scCycle + `', '` + scGroupA + `', '` + scMath + `', '` + scAna + `'),
			('` + scMathB + `', @t, '` + scCycle + `', '` + scGroupB + `', '` + scMath + `', '` + scAna + `'),
			('` + scMusicA + `', @t, '` + scCycle + `', '` + scGroupA + `', '` + scMusic + `', '` + scLuis + `'),
			('` + scMusicB + `', @t, '` + scCycle + `', '` + scGroupB + `', '` + scMusic + `', '` + scLuis + `')`,
	} {
		require.NoError(t, db.Exec(stmt, map[string]any{"t": tenantG}).Error, stmt)
	}
}

func scBlock(t *testing.T, id, assignment, group, subject, teacher string, weekday, from, to int) domain.ScheduleBlock {
	t.Helper()
	ct, err := domain.NewClassTime(from, to)
	require.NoError(t, err)
	b, err := domain.NewScheduleBlock(id, tenantG, scCycle, assignment, group, subject, teacher, weekday, ct)
	require.NoError(t, err)
	return b
}

func insertBlock(ctx context.Context, repo *pgadapter.Repository, b domain.ScheduleBlock) error {
	return repo.RunScheduleTx(ctx, scCycle, func(tx ports.ScheduleTx) error { return tx.InsertBlock(b) })
}

func newID(n int) string { return fmt.Sprintf("c7000000-0000-0000-0001-%012d", n) }

func TestScheduleBlocksRoundTripAndTheDatabaseRefusesDoubleBooking(t *testing.T) {
	seedSchedule(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantG)

	mathA := scBlock(t, newID(1), scMathA, scGroupA, scMath, scAna, 1, 480, 530)
	require.NoError(t, insertBlock(ctx, repo, mathA))
	// back to back: [08:00,08:50) and [08:50,09:40) do not collide
	require.NoError(t, insertBlock(ctx, repo, scBlock(t, newID(2), scMusicA, scGroupA, scMusic, scLuis, 1, 530, 580)))

	got, err := repo.ListScheduleBlocks(ctx, scCycle)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, mathA, got[0])

	// Ana in group B at 08:30 the same Monday — the EXCLUDE refuses it even
	// without the use case's checks.
	err = insertBlock(ctx, repo, scBlock(t, newID(3), scMathB, scGroupB, scMath, scAna, 1, 510, 560))
	assert.Equal(t, domain.CodeScheduleChanged, domain.RefusalCodeOf(err))
	// Group A with two classes at 08:40.
	err = insertBlock(ctx, repo, scBlock(t, newID(4), scMusicA, scGroupA, scMusic, scLuis, 1, 520, 560))
	assert.Equal(t, domain.CodeScheduleChanged, domain.RefusalCodeOf(err))
	// A class whose (teacher, group, subject) is not its assignment's.
	err = insertBlock(ctx, repo, scBlock(t, newID(5), scMathA, scGroupA, scMath, scLuis, 2, 480, 530))
	assert.Equal(t, domain.CodeScheduleAssignmentInvalid, domain.RefusalCodeOf(err))

	// Tenant H sees none of it.
	other := pgadapter.NewRepository(appDB(t), tenantH)
	hidden, err := other.ListScheduleBlocks(ctx, scCycle)
	require.NoError(t, err)
	assert.Empty(t, hidden)
	_, err = other.GetScheduleBlock(ctx, mathA.ID())
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
	err = pgadapter.NewRepository(appDB(t), tenantH).RunScheduleTx(ctx, scCycle, func(tx ports.ScheduleTx) error {
		return tx.InsertBlock(scBlock(t, newID(6), scMathB, scGroupB, scMath, scAna, 3, 480, 530))
	})
	require.Error(t, err, "a neighbour cannot write into tenant G (RLS WITH CHECK / FK hidden)")
}

func TestReassigningATeacherMovesTheClassesOrRefuses(t *testing.T) {
	seedSchedule(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantG)
	require.NoError(t, insertBlock(ctx, repo, scBlock(t, newID(1), scMathA, scGroupA, scMath, scAna, 1, 480, 530)))
	require.NoError(t, insertBlock(ctx, repo, scBlock(t, newID(2), scMusicB, scGroupB, scMusic, scLuis, 1, 480, 530)))
	require.NoError(t, insertBlock(ctx, repo, scBlock(t, newID(3), scMathB, scGroupB, scMath, scAna, 2, 480, 530)))

	// Luis gives music to B on Mondays at 08:00: handing him math A (same
	// time) would double-book him.
	err := repo.ReassignTeachingAssignmentTx(ctx, scMathA, scLuis, func(domain.TeachingAssignment, bool) (bool, error) { return true, nil })
	require.Error(t, err)
	assert.Equal(t, domain.CodeScheduleTeacherBusy, domain.RefusalCodeOf(err))

	// Math B (Tuesday) can go to Luis: its class follows the assignment.
	require.NoError(t, repo.ReassignTeachingAssignmentTx(ctx, scMathB, scLuis, func(domain.TeachingAssignment, bool) (bool, error) { return true, nil }))
	b, err := repo.GetScheduleBlock(ctx, newID(3))
	require.NoError(t, err)
	assert.Equal(t, scLuis, b.TeacherID())

	// Freeing the slot removes its classes.
	require.NoError(t, repo.DeleteTeachingAssignment(ctx, scMathB))
	_, err = repo.GetScheduleBlock(ctx, newID(3))
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestScheduleOverridesEventsAndGrid(t *testing.T) {
	seedSchedule(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantG)
	mathA := scBlock(t, newID(1), scMathA, scGroupA, scMath, scAna, 1, 480, 530)
	musicA := scBlock(t, newID(2), scMusicA, scGroupA, scMusic, scLuis, 1, 530, 580)
	require.NoError(t, insertBlock(ctx, repo, mathA))
	require.NoError(t, insertBlock(ctx, repo, musicA))

	mon := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	by := domain.Actor{UserID: "u-1", Name: "Directora"}
	sw, err := domain.NewScheduleOverride(newID(10), tenantG, scCycle, mathA.ID(), mon, domain.OverrideSwap, "", musicA.ID(), "Música primero", by, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, repo.RunScheduleTx(ctx, scCycle, func(tx ports.ScheduleTx) error {
		if err := tx.InsertOverride(sw); err != nil {
			return err
		}
		return tx.AppendEvent(domain.ScheduleEvent{ID: newID(20), TenantID: tenantG, CycleID: scCycle, At: time.Now().UTC(), By: by,
			Action: domain.ActionOverrideCreated, BlockID: mathA.ID(), OverrideID: sw.ID(), Date: mon, Reason: "Música primero", Summary: "Intercambio"})
	}))

	got, err := repo.ListScheduleOverrides(ctx, scCycle, mon, mon)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, sw.SwapBlockID(), got[0].SwapBlockID())
	assert.Equal(t, mon, got[0].Date())
	assert.Equal(t, by, got[0].By())
	none, err := repo.ListScheduleOverrides(ctx, scCycle, mon.AddDate(0, 0, 1), mon.AddDate(0, 0, 30))
	require.NoError(t, err)
	assert.Empty(t, none)

	// The swap's other class cannot take a second change that date.
	dup, err := domain.NewScheduleOverride(newID(11), tenantG, scCycle, newID(1), mon, domain.OverrideCancel, "", "", "x", by, time.Now().UTC())
	require.NoError(t, err)
	err = repo.RunScheduleTx(ctx, scCycle, func(tx ports.ScheduleTx) error { return tx.InsertOverride(dup) })
	assert.Equal(t, domain.CodeScheduleChanged, domain.RefusalCodeOf(err))

	ev, err := repo.ListScheduleEvents(ctx, scCycle, 10)
	require.NoError(t, err)
	require.Len(t, ev, 1)
	assert.Equal(t, mon, ev[0].Date)
	assert.Equal(t, domain.ActionOverrideCreated, ev[0].Action)

	// Removing a class removes its changes.
	require.NoError(t, repo.RunScheduleTx(ctx, scCycle, func(tx ports.ScheduleTx) error { return tx.DeleteBlock(musicA.ID()) }))
	got, err = repo.ListScheduleOverrides(ctx, scCycle, mon, mon)
	require.NoError(t, err)
	assert.Empty(t, got)

	// The grid upserts.
	g, err := domain.NewScheduleGrid(scLevel, 420, 840)
	require.NoError(t, err)
	require.NoError(t, repo.SaveScheduleGrid(ctx, g))
	g2, err := domain.NewScheduleGrid(scLevel, 450, 900)
	require.NoError(t, err)
	require.NoError(t, repo.SaveScheduleGrid(ctx, g2))
	cat, err := repo.LoadScheduleCatalog(ctx, scCycle)
	require.NoError(t, err)
	require.Len(t, cat.Grids, 1)
	assert.Equal(t, 450, cat.Grids[0].Start())
	assert.Len(t, cat.Groups, 2)
	assert.Len(t, cat.Subjects, 2)
	assert.Len(t, cat.Assignments, 4)
	assert.Len(t, cat.Teachers, 2)
}

// The use case over the real database: the cycle lock serialises concurrent
// writers, so two directors placing Ana at the same time get one class.
func TestScheduleServiceConcurrentWritersCannotDoubleBook(t *testing.T) {
	seedSchedule(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantG)
	loc, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	before := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	svc := usecase.NewScheduleService(repo, tenantG, uuid.NewString, func() time.Time { return before }, loc)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, assignment := range []string{scMathA, scMathB} {
		wg.Add(1)
		go func(i int, assignment string) {
			defer wg.Done()
			_, errs[i] = svc.CreateBlock(ctx, usecase.BlockInput{AssignmentID: assignment, Weekday: 3, Start: 600, End: 650}, domain.Actor{UserID: "u", Name: "D"})
		}(i, assignment)
	}
	wg.Wait()
	failures := 0
	for _, err := range errs {
		if err != nil {
			failures++
			assert.Equal(t, domain.CodeScheduleTeacherBusy, domain.RefusalCodeOf(err))
		}
	}
	assert.Equal(t, 1, failures, "exactly one of the two lands")

	// A substitute through the service, evaluated on the real effective day.
	music, err := svc.CreateBlock(ctx, usecase.BlockInput{AssignmentID: scMusicA, Weekday: 1, Start: 530, End: 580}, domain.Actor{UserID: "u", Name: "D"})
	require.NoError(t, err)
	during := usecase.NewScheduleService(repo, tenantG, uuid.NewString, func() time.Time { return time.Date(2026, time.October, 1, 18, 0, 0, 0, time.UTC) }, loc)
	o, err := during.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC),
		Kind: domain.OverrideSubstitute, TeacherID: scAna, Reason: "Llegó tarde"}, domain.Actor{UserID: "u-dir", Name: "Directora"})
	require.NoError(t, err)
	ev, err := repo.ListScheduleEvents(ctx, scCycle, 5)
	require.NoError(t, err)
	require.NotEmpty(t, ev)
	assert.Equal(t, o.ID(), ev[0].OverrideID)
	assert.Contains(t, ev[0].Summary, "Ana Ruiz en lugar de Luis Pérez")

	require.NoError(t, during.DeleteOverride(ctx, o.ID(), "Ya llegó", domain.Actor{UserID: "u-dir", Name: "Directora"}))
	day, err := during.Day(ctx, scCycle, time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC), "")
	require.NoError(t, err)
	require.Len(t, day.Classes, 1)
	assert.Equal(t, scLuis, day.Classes[0].TeacherID)
}

var _ = gorm.ErrRecordNotFound
