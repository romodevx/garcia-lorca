//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// loadRig extends teachingAssignmentFixture with what a teaching load needs: a
// second subject in the same grade level and a second teacher who already
// holds (group, subject2).
type loadRig struct {
	cycleID, groupID, subject1, subject2 string
	teacher1, teacher2                   string
	heldID                               string // teacher2's assignment on (group, subject2)
}

func seedLoadRig(t *testing.T, tenantID, prefix string) loadRig {
	t.Helper()
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantID)
	var rig loadRig
	rig.cycleID, rig.groupID, rig.subject1, rig.teacher1 = teachingAssignmentFixture(t, tenantID, prefix)
	rig.subject2 = prefix + "0000-0000-0000-0000-000000000014"
	rig.teacher2 = prefix + "0000-0000-0000-0000-000000000016"
	rig.heldID = prefix + "0000-0000-0000-0000-000000000017"

	grp, err := repo.GetGroup(ctx, rig.groupID)
	require.NoError(t, err)
	subj, err := domain.NewSubject(rig.subject2, tenantID, grp.GradeLevelID(), "Subject2-"+prefix)
	require.NoError(t, err)
	require.NoError(t, repo.CreateSubject(ctx, subj))
	te, err := domain.NewTeacher(rig.teacher2, tenantID, "Ada", "Lovelace", "ada-"+prefix+"@example.com", "")
	require.NoError(t, err)
	require.NoError(t, repo.CreateTeacher(ctx, te))
	held, err := domain.NewTeachingAssignment(rig.heldID, tenantID, rig.cycleID, rig.groupID, rig.subject2, rig.teacher2)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTeachingAssignment(ctx, held))
	return rig
}

func (r loadRig) slot1() domain.TeachingSlot {
	return domain.TeachingSlot{GroupID: r.groupID, SubjectID: r.subject1}
}

func (r loadRig) slot2() domain.TeachingSlot {
	return domain.TeachingSlot{GroupID: r.groupID, SubjectID: r.subject2}
}

// TestApplyTeachingLoadSnapshotsAndWritesInOneTransaction proves the adapter
// hands the plan exactly the referenced rows (and the occupied slots with their
// holders), then inserts and reassigns what the plan returns.
func TestApplyTeachingLoadSnapshotsAndWritesInOneTransaction(t *testing.T) {
	ctx := context.Background()
	rig := seedLoadRig(t, tenantA, "c1c1")
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	const missingGroup = "c1c1ffff-0000-0000-0000-000000000099"
	const newID = "c1c17000-0000-0000-0000-000000000001"
	slots := []domain.TeachingSlot{rig.slot1(), rig.slot2(), {GroupID: missingGroup, SubjectID: rig.subject1}}

	err := repo.ApplyTeachingLoad(ctx, rig.teacher1, slots, func(snap ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error) {
		assert.True(t, snap.TeacherFound)
		assert.Equal(t, rig.teacher1, snap.Teacher.ID())
		assert.Contains(t, snap.Groups, rig.groupID)
		assert.NotContains(t, snap.Groups, missingGroup)
		assert.Len(t, snap.Subjects, 2)
		require.Len(t, snap.Occupied, 1, "only (group, subject2) is occupied")
		held := snap.Occupied[rig.slot2()]
		assert.Equal(t, rig.heldID, held.ID())
		assert.Equal(t, "Ada Lovelace", snap.Holders[rig.teacher2].FullName())

		created, err := domain.NewTeachingAssignment(newID, tenantA, rig.cycleID, rig.groupID, rig.subject1, rig.teacher1)
		require.NoError(t, err)
		moved, err := held.ReassignTo(rig.teacher1)
		require.NoError(t, err)
		return ports.TeachingLoadWrite{Create: []domain.TeachingAssignment{created}, Reassign: []domain.TeachingAssignment{moved}}, nil
	})
	require.NoError(t, err)

	got, err := repo.ListTeachingAssignments(ctx, "", rig.groupID, rig.teacher1)
	require.NoError(t, err)
	assert.Len(t, got, 2, "one created + one reassigned now belong to teacher1")
	moved, err := repo.GetTeachingAssignment(ctx, rig.heldID)
	require.NoError(t, err)
	assert.Equal(t, rig.teacher1, moved.TeacherID())
}

// TestApplyTeachingLoadRollsBackEverything covers both abort paths: the plan
// refusing, and a write failing halfway (an insert on an occupied slot). In
// neither case may an earlier write survive.
func TestApplyTeachingLoadRollsBackEverything(t *testing.T) {
	ctx := context.Background()
	rig := seedLoadRig(t, tenantA, "c2c2")
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	first, err := domain.NewTeachingAssignment("c2c27000-0000-0000-0000-000000000001", tenantA, rig.cycleID, rig.groupID, rig.subject1, rig.teacher1)
	require.NoError(t, err)
	collides, err := domain.NewTeachingAssignment("c2c27000-0000-0000-0000-000000000002", tenantA, rig.cycleID, rig.groupID, rig.subject2, rig.teacher1)
	require.NoError(t, err)
	errRefused := errors.New("refused")

	tests := []struct {
		name     string
		plan     func(ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error)
		wantErr  error
		wantKind glerrors.Kind
	}{
		{
			name: "plan error",
			plan: func(ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error) {
				return ports.TeachingLoadWrite{Create: []domain.TeachingAssignment{first}}, errRefused
			},
			wantErr: errRefused,
		},
		{
			name: "second insert hits the one-teacher-per-slot UNIQUE",
			plan: func(ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error) {
				return ports.TeachingLoadWrite{Create: []domain.TeachingAssignment{first, collides}}, nil
			},
			wantKind: glerrors.KindConflict,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.ApplyTeachingLoad(ctx, rig.teacher1, []domain.TeachingSlot{rig.slot1(), rig.slot2()}, tt.plan)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.Equal(t, tt.wantKind, glerrors.KindOf(err))
			}
			_, err = repo.GetTeachingAssignment(ctx, first.ID())
			assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "the first insert must have been rolled back")
		})
	}
}

func TestApplyTeachingLoadUnknownTeacherIsNotFoundInSnapshot(t *testing.T) {
	ctx := context.Background()
	rig := seedLoadRig(t, tenantA, "c3c3")
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	err := repo.ApplyTeachingLoad(ctx, "c3c3ffff-0000-0000-0000-000000000099", []domain.TeachingSlot{rig.slot1()},
		func(snap ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error) {
			assert.False(t, snap.TeacherFound)
			return ports.TeachingLoadWrite{}, nil
		})
	require.NoError(t, err)
}

func TestDeleteTeachingAssignment(t *testing.T) {
	ctx := context.Background()
	rig := seedLoadRig(t, tenantA, "c4c4")
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	require.NoError(t, repo.DeleteTeachingAssignment(ctx, rig.heldID))
	_, err := repo.GetTeachingAssignment(ctx, rig.heldID)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(repo.DeleteTeachingAssignment(ctx, rig.heldID)))
}

func TestUpdateTeachingAssignmentTeacher(t *testing.T) {
	ctx := context.Background()
	rig := seedLoadRig(t, tenantA, "c5c5")
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	require.NoError(t, repo.UpdateTeachingAssignmentTeacher(ctx, rig.heldID, rig.teacher1))
	got, err := repo.GetTeachingAssignment(ctx, rig.heldID)
	require.NoError(t, err)
	assert.Equal(t, rig.teacher1, got.TeacherID())

	err = repo.UpdateTeachingAssignmentTeacher(ctx, "c5c5ffff-0000-0000-0000-000000000099", rig.teacher1)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "unknown assignment")
	err = repo.UpdateTeachingAssignmentTeacher(ctx, rig.heldID, "c5c5ffff-0000-0000-0000-000000000098")
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "unknown teacher trips the FK")
}

// TestRLSIsolatesTeachingLoadWrites proves tenant B can neither see tenant A's
// rows through the load snapshot nor delete or reassign tenant A's assignment.
func TestRLSIsolatesTeachingLoadWrites(t *testing.T) {
	ctx := context.Background()
	rig := seedLoadRig(t, tenantA, "c6c6")
	repoA := pgadapter.NewRepository(appDB(t), tenantA)
	repoB := pgadapter.NewRepository(appDB(t), tenantB)

	err := repoB.ApplyTeachingLoad(ctx, rig.teacher1, []domain.TeachingSlot{rig.slot1(), rig.slot2()},
		func(snap ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error) {
			assert.False(t, snap.TeacherFound, "tenant B saw tenant A's teacher (RLS breach)")
			assert.Empty(t, snap.Groups, "tenant B saw tenant A's group (RLS breach)")
			assert.Empty(t, snap.Subjects, "tenant B saw tenant A's subjects (RLS breach)")
			assert.Empty(t, snap.Occupied, "tenant B saw tenant A's assignment (RLS breach)")
			return ports.TeachingLoadWrite{}, nil
		})
	require.NoError(t, err)

	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(repoB.DeleteTeachingAssignment(ctx, rig.heldID)))
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(repoB.UpdateTeachingAssignmentTeacher(ctx, rig.heldID, rig.teacher1)))

	got, err := repoA.GetTeachingAssignment(ctx, rig.heldID)
	require.NoError(t, err, "tenant A's assignment survives")
	assert.Equal(t, rig.teacher2, got.TeacherID(), "and still belongs to its teacher")
}
