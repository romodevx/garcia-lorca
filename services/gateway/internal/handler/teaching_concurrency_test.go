package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// rendezvousGiveUp bounds how long one read waits for the other. It only
// elapses when the two reads are NOT in flight together — the bug under test.
const rendezvousGiveUp = 2 * time.Second

// rendezvousConfig makes the assignment read and the cycle read each wait for
// the other to have started. Run one after the other, the first gives up and
// fails; run concurrently, both proceed at once.
type rendezvousConfig struct {
	*fakeTeachingConfig
	cyclesStarted      chan struct{}
	assignmentsStarted chan struct{}
}

func newRendezvousConfig(base *fakeTeachingConfig) *rendezvousConfig {
	return &rendezvousConfig{
		fakeTeachingConfig: base,
		cyclesStarted:      make(chan struct{}),
		assignmentsStarted: make(chan struct{}),
	}
}

func awaitPeer(started chan<- struct{}, peer <-chan struct{}) error {
	close(started)
	select {
	case <-peer:
		return nil
	case <-time.After(rendezvousGiveUp):
		return errors.New("the other read never started: the two reads ran one after the other")
	}
}

func (c *rendezvousConfig) ListCycles(ctx context.Context) ([]client.CycleResult, error) {
	if err := awaitPeer(c.cyclesStarted, c.assignmentsStarted); err != nil {
		return nil, err
	}
	return c.fakeTeachingConfig.ListCycles(ctx)
}

func (c *rendezvousConfig) ListTeachingAssignments(ctx context.Context, cycleID, groupID, teacherID string) ([]client.TeachingAssignment, error) {
	if err := awaitPeer(c.assignmentsStarted, c.cyclesStarted); err != nil {
		return nil, err
	}
	return c.fakeTeachingConfig.ListTeachingAssignments(ctx, cycleID, groupID, teacherID)
}

func oneAssignmentTeacher() *fakeTeachingConfig {
	return &fakeTeachingConfig{
		teachers: []client.Teacher{{ID: "tch-1", UserID: "usr-t"}},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-1", TeacherID: "tch-1"}},
		},
		groups:   []client.GroupResult{{ID: "grp-1", Name: "3° A"}},
		subjects: []client.SubjectResult{{ID: "sub-1", Name: "Matemáticas"}},
	}
}

// The caller's assignments and the school's cycles are independent reads: the
// teaching surface issues them together, not one after the other.
func TestTeachingReadsAssignmentsAndCyclesConcurrently(t *testing.T) {
	cfg := newRendezvousConfig(oneAssignmentTeacher())
	rec := httptest.NewRecorder()
	handler.NewTeachingHandler(cfg, &fakeTeachingGrades{}, &fakeTeachingStudents{}).
		Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "asg-1")
}

// cancelWatchConfig fails the cycle read and reports whether the assignment
// read, still in flight, saw its context cancelled.
type cancelWatchConfig struct {
	*fakeTeachingConfig
	sawCancel chan bool
}

func (c *cancelWatchConfig) ListCycles(context.Context) ([]client.CycleResult, error) {
	return nil, errors.New("config down")
}

func (c *cancelWatchConfig) ListTeachingAssignments(ctx context.Context, _, _, _ string) ([]client.TeachingAssignment, error) {
	select {
	case <-ctx.Done():
		c.sawCancel <- true
		return nil, ctx.Err()
	case <-time.After(rendezvousGiveUp):
		c.sawCancel <- false
		return nil, nil
	}
}

// One read failing cancels the other: no work outlives a request that is
// already lost, and the failure — never a widened scope — is what answers.
func TestTeachingCycleFailureCancelsTheAssignmentRead(t *testing.T) {
	cfg := &cancelWatchConfig{fakeTeachingConfig: oneAssignmentTeacher(), sawCancel: make(chan bool, 1)}
	rec := httptest.NewRecorder()
	handler.NewTeachingHandler(cfg, &fakeTeachingGrades{}, &fakeTeachingStudents{}).
		Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	assert.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError)
	assert.True(t, <-cfg.sawCancel, "the in-flight assignment read is cancelled")
}
