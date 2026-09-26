package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Review #9: every in-place change and every clear carries the status it
// replaced; a new exception carries none.
func TestEventsCarryThePreviousStatus(t *testing.T) {
	repo := newFakeRepo()
	pub := &spyPublisher{}
	svc := newService(t, repo, pub, "rec-1", "rec-2", "rec-3", "rec-4")
	ctx := context.Background()
	type seen struct{ Status, Previous string }
	saveAna := func(status string) []seen {
		t.Helper()
		pub.marked, pub.absences = nil, nil
		marks := []service.Mark{}
		if status != "" {
			marks = append(marks, service.Mark{EnrollmentID: enrollmentAna, Status: status})
		}
		_, _, err := save(svc, ctx, groupID, schoolDay, marks)
		require.NoError(t, err)
		out := make([]seen, 0, len(pub.marked))
		for _, e := range pub.marked {
			out = append(out, seen{e.Status, e.PreviousStatus})
		}
		return out
	}

	assert.Equal(t, []seen{{"absent", ""}}, saveAna("absent"))
	assert.Equal(t, "", pub.absences[0].PreviousStatus)
	assert.Equal(t, []seen{{"late", "absent"}}, saveAna("late"))
	assert.Equal(t, []seen{{"excused", "late"}}, saveAna("excused"))
	assert.Equal(t, []seen{{"absent", "excused"}}, saveAna("absent"))
	require.Len(t, pub.absences, 1)
	assert.Equal(t, "excused", pub.absences[0].PreviousStatus)
	assert.Equal(t, []seen{{"present", "absent"}}, saveAna(""))
}
