package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/config/internal/domain"
)

// Two concurrent loads that insert overlapping slots take the UNIQUE index's
// locks in insert order. Inserting in (group_id, subject_id) order — whatever
// order the request listed them in — makes that order the same for everyone,
// so the two wait on each other instead of deadlocking. The caller's slice
// (the response's request order) is left untouched.
func TestInLockOrderSortsByGroupThenSubject(t *testing.T) {
	mk := func(id, group, subject string) domain.TeachingAssignment {
		a, err := domain.NewTeachingAssignment(id, "t", "c", group, subject, "teacher")
		require.NoError(t, err)
		return a
	}
	in := []domain.TeachingAssignment{
		mk("1", "g2", "s1"),
		mk("2", "g1", "s2"),
		mk("3", "g2", "s0"),
		mk("4", "g1", "s1"),
	}

	got := inLockOrder(in)

	ids := make([]string, 0, len(got))
	for _, a := range got {
		ids = append(ids, a.ID())
	}
	assert.Equal(t, []string{"4", "2", "3", "1"}, ids)
	assert.Equal(t, "1", in[0].ID(), "the input keeps request order")
}

// byIDs answers an empty id list with an empty (non-nil) map and never touches
// the database: a load with no holders must not issue an `IN ()` query.
func TestByIDsWithNoIDsSkipsTheQuery(t *testing.T) {
	got, err := byIDs[groupModel, domain.Group](nil, nil, toDomainGroup, "groups")

	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}
