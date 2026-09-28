package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/grades/internal/ports"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// Review finding 4: while a level's deletion runs (config: status DELETING),
// every monthly write that resolves it is refused with LEVEL_BEING_DELETED —
// a grade written between grades' archive step and config's would otherwise
// survive, pointing at a group that is gone. Reads keep working.
func TestMonthlyWritesRefuseALevelBeingDeleted(t *testing.T) {
	writes := []struct {
		name string
		call func(f monthlyFixture) error
	}{
		{"upsert", func(f monthlyFixture) error {
			_, err := f.svc.Upsert(context.Background(), upsertInput("2026-09", enrollmentID, "9"))
			return err
		}},
		{"clear", func(f monthlyFixture) error {
			return f.svc.Clear(context.Background(), usecase.ScopeSchool, assignmentID, "2026-09", enrollmentID)
		}},
	}
	for _, w := range writes {
		t.Run(w.name, func(t *testing.T) {
			f := newMonthlyFixture(t)
			lvl := f.catalog.levelByGroup[groupID]
			lvl.Deleting = true
			f.catalog.levelByGroup[groupID] = lvl

			err := w.call(f)
			assert.Equal(t, levelarchive.CodeLevelBeingDeleted, levelarchive.CodeOf(err), "err = %v", err)
			assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
			assert.Empty(t, f.repo.rows, "nothing is stored")
		})
	}

	f := newMonthlyFixture(t)
	lvl := f.catalog.levelByGroup[groupID]
	lvl.Deleting = true
	f.catalog.levelByGroup[groupID] = lvl
	_, err := f.svc.Sheet(context.Background(), usecase.ScopeSchool, assignmentID, "2026-09")
	require.NoError(t, err, "the grid still reads")
}

// The legacy term capture resolves the level through the same lookup and is
// refused the same way; its roster still reads.
func TestLegacyCaptureRefusesALevelBeingDeleted(t *testing.T) {
	asg := baseAssignments()
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	repo := newFakeEntries()
	catalog := legacyCatalog()
	catalog.levelByGroup[groupID] = ports.LevelRef{ID: levelPrimaria, GradingEnabled: true, Deleting: true}
	svc := usecase.NewGradesService(repo, asg, catalog, enr, testTenant, func() string { return "ge-1" }, fixedLegacyNow)

	_, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 8)
	assert.Equal(t, levelarchive.CodeLevelBeingDeleted, levelarchive.CodeOf(err), "err = %v", err)
	assert.Empty(t, repo.rows)
	_, err = svc.Roster(context.Background(), assignmentID, termID)
	assert.NoError(t, err, "the roster still reads")
}

func fixedLegacyNow() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }
