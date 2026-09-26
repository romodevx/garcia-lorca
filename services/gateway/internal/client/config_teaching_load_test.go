package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// loadStub records what reached the config service and answers canned
// teaching-load responses, so the client's mapping is proven in BOTH directions
// over a real Connect round-trip.
type loadStub struct {
	configv1connect.UnimplementedConfigServiceHandler
	gotLoad     *configv1.AssignTeachingLoadRequest
	gotDelete   string
	gotReassign *configv1.ReassignTeachingAssignmentRequest
}

func (s *loadStub) AssignTeachingLoad(_ context.Context, req *connect.Request[configv1.AssignTeachingLoadRequest]) (*connect.Response[configv1.AssignTeachingLoadResponse], error) {
	s.gotLoad = req.Msg
	return connect.NewResponse(&configv1.AssignTeachingLoadResponse{
		Created:    []*configv1.TeachingAssignment{{Id: "asg-new", CycleId: "cyc-1", GroupId: "grp-1", SubjectId: "sub-1", TeacherId: "tch-1"}},
		Reassigned: []*configv1.TeachingAssignment{{Id: "asg-old", CycleId: "cyc-1", GroupId: "grp-2", SubjectId: "sub-2", TeacherId: "tch-1"}},
		Skipped:    []*configv1.TeachingLoadItem{{GroupId: "grp-3", SubjectId: "sub-3"}},
		Conflicts: []*configv1.TeachingLoadConflict{{
			GroupId: "grp-4", SubjectId: "sub-4", AssignmentId: "asg-x", TeacherId: "tch-2", TeacherName: "Grace Hopper",
		}},
	}), nil
}

func (s *loadStub) DeleteTeachingAssignment(_ context.Context, req *connect.Request[configv1.DeleteTeachingAssignmentRequest]) (*connect.Response[configv1.DeleteTeachingAssignmentResponse], error) {
	if req.Msg.GetId() == "ghost" {
		return nil, connect.NewError(connect.CodeNotFound, errString("teaching assignment not found"))
	}
	s.gotDelete = req.Msg.GetId()
	return connect.NewResponse(&configv1.DeleteTeachingAssignmentResponse{}), nil
}

func (s *loadStub) ReassignTeachingAssignment(_ context.Context, req *connect.Request[configv1.ReassignTeachingAssignmentRequest]) (*connect.Response[configv1.ReassignTeachingAssignmentResponse], error) {
	s.gotReassign = req.Msg
	return connect.NewResponse(&configv1.ReassignTeachingAssignmentResponse{TeachingAssignment: &configv1.TeachingAssignment{
		Id: req.Msg.GetId(), CycleId: "cyc-1", GroupId: "grp-1", SubjectId: "sub-1", TeacherId: req.Msg.GetTeacherId(),
	}}), nil
}

func newLoadStub(t *testing.T) (*client.ConfigClient, *loadStub) {
	t.Helper()
	stub := &loadStub{}
	mux := http.NewServeMux()
	path, h := configv1connect.NewConfigServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return client.NewConfigClient(srv.URL, srv.Client()), stub
}

func TestConfigClientAssignTeachingLoadRoundTrip(t *testing.T) {
	c, stub := newLoadStub(t)

	res, err := c.AssignTeachingLoad(context.Background(), client.TeachingLoadInput{
		TeacherID:        "tch-1",
		Items:            []client.TeachingLoadItem{{GroupID: "grp-1", SubjectID: "sub-1"}, {GroupID: "grp-2", SubjectID: "sub-2"}},
		ReplaceConflicts: true,
	})

	require.NoError(t, err)
	// outbound
	require.NotNil(t, stub.gotLoad)
	assert.Equal(t, "tch-1", stub.gotLoad.GetTeacherId())
	assert.True(t, stub.gotLoad.GetReplaceConflicts())
	require.Len(t, stub.gotLoad.GetItems(), 2)
	assert.Equal(t, "grp-2", stub.gotLoad.GetItems()[1].GetGroupId())
	assert.Equal(t, "sub-2", stub.gotLoad.GetItems()[1].GetSubjectId())
	// inbound
	assert.Equal(t, []client.TeachingAssignment{{ID: "asg-new", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-1", TeacherID: "tch-1"}}, res.Created)
	assert.Equal(t, []client.TeachingAssignment{{ID: "asg-old", CycleID: "cyc-1", GroupID: "grp-2", SubjectID: "sub-2", TeacherID: "tch-1"}}, res.Reassigned)
	assert.Equal(t, []client.TeachingLoadItem{{GroupID: "grp-3", SubjectID: "sub-3"}}, res.Skipped)
	assert.Equal(t, []client.TeachingLoadConflict{{
		GroupID: "grp-4", SubjectID: "sub-4", AssignmentID: "asg-x", TeacherID: "tch-2", TeacherName: "Grace Hopper",
	}}, res.Conflicts)
}

func TestConfigClientDeleteTeachingAssignment(t *testing.T) {
	c, stub := newLoadStub(t)

	require.NoError(t, c.DeleteTeachingAssignment(context.Background(), "asg-1"))
	assert.Equal(t, "asg-1", stub.gotDelete)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(c.DeleteTeachingAssignment(context.Background(), "ghost")))
}

func TestConfigClientReassignTeachingAssignment(t *testing.T) {
	c, stub := newLoadStub(t)

	got, err := c.ReassignTeachingAssignment(context.Background(), "asg-1", "tch-2")

	require.NoError(t, err)
	assert.Equal(t, "asg-1", stub.gotReassign.GetId())
	assert.Equal(t, "tch-2", stub.gotReassign.GetTeacherId())
	assert.Equal(t, client.TeachingAssignment{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-1", TeacherID: "tch-2"}, got)
}
