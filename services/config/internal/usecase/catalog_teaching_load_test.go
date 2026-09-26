package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// --- fake: the teaching-load seam -------------------------------------------
//
// ApplyTeachingLoad mirrors the adapter: it snapshots the referenced rows, hands
// them to the use case's plan, and applies the plan's writes only when the plan
// succeeds (the adapter's transaction rolls back on a plan error).

func (f *fakeCatalog) ApplyTeachingLoad(_ context.Context, teacherID string, slots []domain.TeachingSlot, plan func(ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error)) error {
	snap := ports.TeachingLoadSnapshot{
		Groups:   map[string]domain.Group{},
		Subjects: map[string]domain.Subject{},
		Occupied: map[domain.TeachingSlot]domain.TeachingAssignment{},
		Holders:  map[string]domain.Teacher{},
	}
	snap.Teacher, snap.TeacherFound = f.teachers[teacherID]
	wanted := map[domain.TeachingSlot]bool{}
	for _, s := range slots {
		wanted[s] = true
		if g, ok := f.groups[s.GroupID]; ok {
			snap.Groups[s.GroupID] = g
		}
		if su, ok := f.subjects[s.SubjectID]; ok {
			snap.Subjects[s.SubjectID] = su
		}
	}
	for _, a := range f.assignments {
		if wanted[a.Slot()] {
			snap.Occupied[a.Slot()] = a
			snap.Holders[a.TeacherID()] = f.teachers[a.TeacherID()]
		}
	}
	w, err := plan(snap)
	if err != nil {
		return err
	}
	for _, a := range w.Create {
		if err := f.CreateTeachingAssignment(context.Background(), a); err != nil {
			return err
		}
	}
	for _, a := range w.Reassign {
		f.assignments[a.ID()] = a
	}
	return nil
}

func (f *fakeCatalog) DeleteTeachingAssignment(_ context.Context, id string) error {
	a, ok := f.assignments[id]
	if !ok {
		return glerrors.ErrNotFound
	}
	delete(f.assignments, id)
	delete(f.unique, "assignment|"+a.CycleID()+"|"+a.GroupID()+"|"+a.SubjectID())
	return nil
}

func (f *fakeCatalog) UpdateTeachingAssignmentTeacher(_ context.Context, id, teacherID string) error {
	a, ok := f.assignments[id]
	if !ok {
		return glerrors.ErrNotFound
	}
	moved, err := a.ReassignTo(teacherID)
	if err != nil {
		return err
	}
	f.assignments[id] = moved
	return nil
}

// --- harness ------------------------------------------------------------------

// loadFixture is one grade level with two groups and three subjects, plus two
// teachers: enough to fill, skip, collide and reassign.
type loadFixture struct {
	groupA, groupB   domain.Group
	math, spanish    domain.Subject
	otherGradeSubj   domain.Subject
	ada, grace       domain.Teacher
	cycleID          string
	otherGradeGroupA domain.Group
}

func seedLoadFixture(t *testing.T, svc *usecase.CatalogService) loadFixture {
	t.Helper()
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	gl := seedGradeLevel(t, svc, l.ID())
	gl2, err := svc.CreateGradeLevel(ctx(), l.ID(), "2° Primaria", 2)
	require.NoError(t, err)

	var fx loadFixture
	fx.cycleID = c.ID()
	fx.groupA, err = svc.CreateGroup(ctx(), c.ID(), gl.ID(), "A", 30)
	require.NoError(t, err)
	fx.groupB, err = svc.CreateGroup(ctx(), c.ID(), gl.ID(), "B", 30)
	require.NoError(t, err)
	fx.otherGradeGroupA, err = svc.CreateGroup(ctx(), c.ID(), gl2.ID(), "A", 30)
	require.NoError(t, err)
	fx.math, err = svc.CreateSubject(ctx(), gl.ID(), "Matemáticas")
	require.NoError(t, err)
	fx.spanish, err = svc.CreateSubject(ctx(), gl.ID(), "Español")
	require.NoError(t, err)
	fx.otherGradeSubj, err = svc.CreateSubject(ctx(), gl2.ID(), "Historia")
	require.NoError(t, err)
	fx.ada = seedTeacher(t, svc)
	fx.grace, err = svc.CreateTeacher(ctx(), "Grace", "Hopper", "grace@example.com", "")
	require.NoError(t, err)
	return fx
}

func slot(g domain.Group, s domain.Subject) domain.TeachingSlot {
	return domain.TeachingSlot{GroupID: g.ID(), SubjectID: s.ID()}
}

func slotsOf(as []domain.TeachingAssignment) []domain.TeachingSlot {
	out := make([]domain.TeachingSlot, 0, len(as))
	for _, a := range as {
		out = append(out, a.Slot())
	}
	return out
}

// --- AssignTeachingLoad -------------------------------------------------------

func TestAssignTeachingLoadCreatesEveryFreeSlotInRequestOrder(t *testing.T) {
	svc, repo := newService(t)
	fx := seedLoadFixture(t, svc)
	items := []domain.TeachingSlot{slot(fx.groupB, fx.spanish), slot(fx.groupA, fx.math), slot(fx.groupA, fx.spanish)}

	res, err := svc.AssignTeachingLoad(ctx(), usecase.TeachingLoadRequest{TeacherID: fx.ada.ID(), Items: items})

	require.NoError(t, err)
	assert.Equal(t, items, slotsOf(res.Created))
	assert.Empty(t, res.Reassigned)
	assert.Empty(t, res.Skipped)
	assert.Empty(t, res.Conflicts)
	for _, a := range res.Created {
		assert.Equal(t, fx.ada.ID(), a.TeacherID())
		assert.Equal(t, fx.cycleID, a.CycleID(), "cycle derived from the group")
		assert.Equal(t, tenant, a.TenantID())
	}
	assert.Len(t, repo.assignments, 3)
}

func TestAssignTeachingLoadSkipsSlotsTheTeacherAlreadyHolds(t *testing.T) {
	svc, repo := newService(t)
	fx := seedLoadFixture(t, svc)
	_, err := svc.CreateTeachingAssignment(ctx(), fx.groupA.ID(), fx.math.ID(), fx.ada.ID())
	require.NoError(t, err)

	res, err := svc.AssignTeachingLoad(ctx(), usecase.TeachingLoadRequest{
		TeacherID: fx.ada.ID(),
		Items:     []domain.TeachingSlot{slot(fx.groupA, fx.math), slot(fx.groupA, fx.spanish)},
	})

	require.NoError(t, err)
	assert.Equal(t, []domain.TeachingSlot{slot(fx.groupA, fx.math)}, res.Skipped)
	assert.Equal(t, []domain.TeachingSlot{slot(fx.groupA, fx.spanish)}, slotsOf(res.Created))
	assert.Len(t, repo.assignments, 2)
}

func TestAssignTeachingLoadReportsConflictsWithoutTouchingThem(t *testing.T) {
	svc, repo := newService(t)
	fx := seedLoadFixture(t, svc)
	held, err := svc.CreateTeachingAssignment(ctx(), fx.groupA.ID(), fx.math.ID(), fx.grace.ID())
	require.NoError(t, err)

	res, err := svc.AssignTeachingLoad(ctx(), usecase.TeachingLoadRequest{
		TeacherID: fx.ada.ID(),
		Items:     []domain.TeachingSlot{slot(fx.groupA, fx.math), slot(fx.groupB, fx.math)},
	})

	require.NoError(t, err)
	assert.Equal(t, []usecase.TeachingLoadConflict{{
		Slot:         slot(fx.groupA, fx.math),
		AssignmentID: held.ID(),
		TeacherID:    fx.grace.ID(),
		TeacherName:  "Grace Hopper",
	}}, res.Conflicts)
	assert.Equal(t, []domain.TeachingSlot{slot(fx.groupB, fx.math)}, slotsOf(res.Created), "the free slot is still created")
	assert.Empty(t, res.Reassigned)
	assert.Equal(t, fx.grace.ID(), repo.assignments[held.ID()].TeacherID(), "the occupied slot is untouched")
}

func TestAssignTeachingLoadReplacesConflictsWhenAsked(t *testing.T) {
	svc, repo := newService(t)
	fx := seedLoadFixture(t, svc)
	held, err := svc.CreateTeachingAssignment(ctx(), fx.groupA.ID(), fx.math.ID(), fx.grace.ID())
	require.NoError(t, err)

	res, err := svc.AssignTeachingLoad(ctx(), usecase.TeachingLoadRequest{
		TeacherID:        fx.ada.ID(),
		Items:            []domain.TeachingSlot{slot(fx.groupA, fx.math)},
		ReplaceConflicts: true,
	})

	require.NoError(t, err)
	require.Len(t, res.Reassigned, 1)
	assert.Equal(t, held.ID(), res.Reassigned[0].ID(), "reassignment keeps the assignment id")
	assert.Equal(t, fx.ada.ID(), res.Reassigned[0].TeacherID())
	assert.Empty(t, res.Conflicts)
	assert.Empty(t, res.Created)
	assert.Equal(t, fx.ada.ID(), repo.assignments[held.ID()].TeacherID())
}

func TestAssignTeachingLoadRejectsTheWholeRequestOnAnyBadItem(t *testing.T) {
	type build func(fx loadFixture) usecase.TeachingLoadRequest
	req := func(teacherID string, items ...domain.TeachingSlot) usecase.TeachingLoadRequest {
		return usecase.TeachingLoadRequest{TeacherID: teacherID, Items: items}
	}
	tests := []struct {
		name    string
		build   build
		wantMsg string
	}{
		{"blank teacher", func(fx loadFixture) usecase.TeachingLoadRequest {
			return req(" ", slot(fx.groupA, fx.math))
		}, "teacher"},
		{"no items", func(fx loadFixture) usecase.TeachingLoadRequest {
			return req(fx.ada.ID())
		}, "items"},
		{"too many items", func(fx loadFixture) usecase.TeachingLoadRequest {
			items := make([]domain.TeachingSlot, usecase.MaxTeachingLoadItems+1)
			return req(fx.ada.ID(), items...)
		}, "items"},
		{"blank group id", func(fx loadFixture) usecase.TeachingLoadRequest {
			return req(fx.ada.ID(), slot(fx.groupA, fx.math), domain.TeachingSlot{SubjectID: fx.math.ID()})
		}, "items[1]"},
		{"duplicated item", func(fx loadFixture) usecase.TeachingLoadRequest {
			return req(fx.ada.ID(), slot(fx.groupA, fx.math), slot(fx.groupA, fx.spanish), slot(fx.groupA, fx.math))
		}, "items[2]"},
		{"unknown teacher", func(fx loadFixture) usecase.TeachingLoadRequest {
			return req("no-such-teacher", slot(fx.groupA, fx.math))
		}, "teacher not found"},
		{"unknown group", func(fx loadFixture) usecase.TeachingLoadRequest {
			return req(fx.ada.ID(), slot(fx.groupA, fx.math), domain.TeachingSlot{GroupID: "no-such-group", SubjectID: fx.math.ID()})
		}, "items[1]: group not found"},
		{"unknown subject", func(fx loadFixture) usecase.TeachingLoadRequest {
			return req(fx.ada.ID(), slot(fx.groupA, fx.math), domain.TeachingSlot{GroupID: fx.groupA.ID(), SubjectID: "no-such-subject"})
		}, "items[1]: subject not found"},
		{"subject of another grade level", func(fx loadFixture) usecase.TeachingLoadRequest {
			return req(fx.ada.ID(), slot(fx.groupA, fx.math), slot(fx.groupA, fx.otherGradeSubj))
		}, "items[1]: subject does not belong to the group's grade level"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newService(t)
			fx := seedLoadFixture(t, svc)

			_, err := svc.AssignTeachingLoad(ctx(), tt.build(fx))

			require.Error(t, err)
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.True(t, strings.Contains(err.Error(), tt.wantMsg), "error %q should name %q", err.Error(), tt.wantMsg)
			assert.Empty(t, repo.assignments, "nothing is written when any item is bad")
		})
	}
}

// --- DeleteTeachingAssignment -------------------------------------------------

func TestDeleteTeachingAssignment(t *testing.T) {
	svc, repo := newService(t)
	fx := seedLoadFixture(t, svc)
	a, err := svc.CreateTeachingAssignment(ctx(), fx.groupA.ID(), fx.math.ID(), fx.ada.ID())
	require.NoError(t, err)

	require.NoError(t, svc.DeleteTeachingAssignment(ctx(), a.ID()))
	assert.Empty(t, repo.assignments)

	err = svc.DeleteTeachingAssignment(ctx(), a.ID())
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "deleting twice is NotFound")

	_, err = svc.CreateTeachingAssignment(ctx(), fx.groupA.ID(), fx.math.ID(), fx.grace.ID())
	assert.NoError(t, err, "the freed slot can be given to someone else")
}

// --- ReassignTeachingAssignment -----------------------------------------------

func TestReassignTeachingAssignment(t *testing.T) {
	tests := []struct {
		name      string
		id        func(a domain.TeachingAssignment) string
		teacherID func(fx loadFixture) string
		wantKind  glerrors.Kind
		wantHeld  func(fx loadFixture) string
	}{
		{
			name:      "hands the slot to another teacher",
			id:        func(a domain.TeachingAssignment) string { return a.ID() },
			teacherID: func(fx loadFixture) string { return fx.grace.ID() },
			wantHeld:  func(fx loadFixture) string { return fx.grace.ID() },
		},
		{
			name:      "same teacher is a no-op",
			id:        func(a domain.TeachingAssignment) string { return a.ID() },
			teacherID: func(fx loadFixture) string { return fx.ada.ID() },
			wantHeld:  func(fx loadFixture) string { return fx.ada.ID() },
		},
		{
			name:      "unknown assignment is NotFound",
			id:        func(domain.TeachingAssignment) string { return "no-such-assignment" },
			teacherID: func(fx loadFixture) string { return fx.grace.ID() },
			wantKind:  glerrors.KindNotFound,
			wantHeld:  func(fx loadFixture) string { return fx.ada.ID() },
		},
		{
			name:      "unknown teacher is Invalid",
			id:        func(a domain.TeachingAssignment) string { return a.ID() },
			teacherID: func(loadFixture) string { return "no-such-teacher" },
			wantKind:  glerrors.KindInvalid,
			wantHeld:  func(fx loadFixture) string { return fx.ada.ID() },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newService(t)
			fx := seedLoadFixture(t, svc)
			a, err := svc.CreateTeachingAssignment(ctx(), fx.groupA.ID(), fx.math.ID(), fx.ada.ID())
			require.NoError(t, err)

			got, err := svc.ReassignTeachingAssignment(ctx(), tt.id(a), tt.teacherID(fx))

			if tt.wantKind != glerrors.KindUnknown {
				assert.Equal(t, tt.wantKind, glerrors.KindOf(err))
			} else {
				require.NoError(t, err)
				assert.Equal(t, a.ID(), got.ID())
				assert.Equal(t, tt.wantHeld(fx), got.TeacherID())
			}
			assert.Equal(t, tt.wantHeld(fx), repo.assignments[a.ID()].TeacherID())
		})
	}
}
