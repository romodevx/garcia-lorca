package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestNewTeachingAssignmentValidates(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		tenantID  string
		cycleID   string
		groupID   string
		subjectID string
		teacherID string
		wantKind  glerrors.Kind
	}{
		{"empty id", "", "t1", "c1", "gr1", "su1", "te1", glerrors.KindInvalid},
		{"empty tenant", "ta1", "", "c1", "gr1", "su1", "te1", glerrors.KindInvalid},
		{"empty cycle id", "ta1", "t1", "", "gr1", "su1", "te1", glerrors.KindInvalid},
		{"empty group id", "ta1", "t1", "c1", "", "su1", "te1", glerrors.KindInvalid},
		{"empty subject id", "ta1", "t1", "c1", "gr1", "", "te1", glerrors.KindInvalid},
		{"empty teacher id", "ta1", "t1", "c1", "gr1", "su1", "", glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewTeachingAssignment(tt.id, tt.tenantID, tt.cycleID, tt.groupID, tt.subjectID, tt.teacherID)
			if glerrors.KindOf(err) != tt.wantKind {
				t.Fatalf("want %v, got %v", tt.wantKind, glerrors.KindOf(err))
			}
		})
	}
}

func TestNewTeachingAssignmentValid(t *testing.T) {
	ta, err := domain.NewTeachingAssignment("ta1", "t1", "c1", "gr1", "su1", "te1")
	if err != nil {
		t.Fatalf("valid assignment: %v", err)
	}
	if ta.ID() != "ta1" || ta.TenantID() != "t1" || ta.CycleID() != "c1" {
		t.Errorf("identity accessors: %q %q %q", ta.ID(), ta.TenantID(), ta.CycleID())
	}
	if ta.GroupID() != "gr1" || ta.SubjectID() != "su1" || ta.TeacherID() != "te1" {
		t.Errorf("reference accessors: %q %q %q", ta.GroupID(), ta.SubjectID(), ta.TeacherID())
	}
}
