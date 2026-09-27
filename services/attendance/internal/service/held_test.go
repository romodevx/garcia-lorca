package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// The counts describe what THIS list stored, not what was submitted: a mark for
// a pupil whose day another group's list holds is skipped by the repository,
// so it is neither counted nor published, and its enrollment id comes back in
// HeldElsewhere for the taker.
func TestSaveDailyAttendanceCountsWhatWasStoredNotWhatWasSubmitted(t *testing.T) {
	tests := []struct {
		name        string
		held        []string
		marks       []service.Mark
		wantAbsent  int
		wantMarked  int
		wantHeld    []string
		wantEvents  int
		wantStoreds []string
	}{
		{
			name:        "nothing held: every submitted mark is stored and counted",
			marks:       []service.Mark{{EnrollmentID: enrollmentAna, Status: "absent"}, {EnrollmentID: enrollmentBeto, Status: "late"}},
			wantAbsent:  1,
			wantMarked:  2,
			wantEvents:  2,
			wantStoreds: []string{enrollmentAna, enrollmentBeto},
		},
		{
			name:        "a held absence is skipped, not counted, and reported",
			held:        []string{enrollmentAna},
			marks:       []service.Mark{{EnrollmentID: enrollmentAna, Status: "absent"}, {EnrollmentID: enrollmentBeto, Status: "late"}},
			wantAbsent:  0,
			wantMarked:  1,
			wantHeld:    []string{enrollmentAna},
			wantEvents:  1,
			wantStoreds: []string{enrollmentBeto},
		},
		{
			name:       "every mark held: nothing stored, both reported in ascending order",
			held:       []string{enrollmentCruz, enrollmentAna},
			marks:      []service.Mark{{EnrollmentID: enrollmentCruz, Status: "absent"}, {EnrollmentID: enrollmentAna, Status: "absent"}},
			wantAbsent: 0,
			wantMarked: 0,
			wantHeld:   []string{enrollmentAna, enrollmentCruz},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.held = map[string]bool{}
			for _, id := range tt.held {
				repo.held[dayKey(id, day(t, schoolDay))] = true
			}
			pub := &spyPublisher{}
			svc := newService(t, repo, pub)

			res, err := svc.SaveDailyAttendance(context.Background(), service.SaveRequest{
				GroupID: groupID, Date: schoolDay, Marks: tt.marks, TakenBy: testTaker.UserID, TakenByName: testTaker.Name,
			})

			require.NoError(t, err)
			assert.Equal(t, tt.wantAbsent, res.AbsentCount)
			assert.Equal(t, tt.wantMarked, res.MarkedCount)
			assert.Equal(t, tt.wantHeld, res.HeldElsewhere)
			assert.Len(t, pub.absences, countAbsences(tt.marks, tt.held), "only stored absences are announced")
			assert.Len(t, pub.marked, tt.wantEvents)
			for _, id := range tt.wantStoreds {
				assert.Contains(t, repo.rows, dayKey(id, day(t, schoolDay)))
			}
		})
	}
}

// countAbsences is how many submitted absences were NOT held elsewhere.
func countAbsences(marks []service.Mark, held []string) int {
	skip := make(map[string]bool, len(held))
	for _, id := range held {
		skip[id] = true
	}
	n := 0
	for _, m := range marks {
		if m.Status == "absent" && !skip[m.EnrollmentID] {
			n++
		}
	}
	return n
}
