package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func mustAssignment(t *testing.T, teacherID string) domain.TeachingAssignment {
	t.Helper()
	a, err := domain.NewTeachingAssignment("ta1", "t1", "c1", "gr1", "su1", teacherID)
	require.NoError(t, err)
	return a
}

func TestTeachingAssignmentSlot(t *testing.T) {
	a := mustAssignment(t, "te1")
	assert.Equal(t, domain.TeachingSlot{GroupID: "gr1", SubjectID: "su1"}, a.Slot())
}

func TestTeachingAssignmentHeldBy(t *testing.T) {
	a := mustAssignment(t, "te1")
	tests := []struct {
		name      string
		teacherID string
		want      bool
	}{
		{"same teacher", "te1", true},
		{"another teacher", "te2", false},
		{"empty teacher", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, a.HeldBy(tt.teacherID))
		})
	}
}

func TestTeachingAssignmentReassignTo(t *testing.T) {
	tests := []struct {
		name      string
		teacherID string
		wantKind  glerrors.Kind
	}{
		{"another teacher", "te2", glerrors.KindUnknown},
		{"blank teacher is invalid", "  ", glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := mustAssignment(t, "te1")
			got, err := a.ReassignTo(tt.teacherID)
			if tt.wantKind != glerrors.KindUnknown {
				assert.Equal(t, tt.wantKind, glerrors.KindOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.teacherID, got.TeacherID())
			// Everything but the teacher is the same slot, same row.
			assert.Equal(t, a.ID(), got.ID())
			assert.Equal(t, a.CycleID(), got.CycleID())
			assert.Equal(t, a.Slot(), got.Slot())
			assert.Equal(t, "te1", a.TeacherID(), "receiver must not be mutated")
		})
	}
}

func TestTeacherFullName(t *testing.T) {
	te, err := domain.NewTeacher("te1", "t1", "Gabriela", "Rojas", "gabriela@example.com", "")
	require.NoError(t, err)
	assert.Equal(t, "Gabriela Rojas", te.FullName())
}
