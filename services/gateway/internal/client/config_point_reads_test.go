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

// pointStub answers the single-row catalog reads the attendance surfaces use
// to resolve one group's grade and level without listing the whole catalog.
type pointStub struct {
	configv1connect.UnimplementedConfigServiceHandler
}

func (pointStub) GetGroup(_ context.Context, req *connect.Request[configv1.GetGroupRequest]) (*connect.Response[configv1.GetGroupResponse], error) {
	if req.Msg.GetId() != "grp-1" {
		return nil, connect.NewError(connect.CodeNotFound, errString("group not found"))
	}
	return connect.NewResponse(&configv1.GetGroupResponse{Group: &configv1.Group{Id: "grp-1", CycleId: "cyc-1", GradeLevelId: "gl-1", Name: "A"}}), nil
}

func (pointStub) GetGradeLevel(_ context.Context, req *connect.Request[configv1.GetGradeLevelRequest]) (*connect.Response[configv1.GetGradeLevelResponse], error) {
	return connect.NewResponse(&configv1.GetGradeLevelResponse{GradeLevel: &configv1.GradeLevel{Id: req.Msg.GetId(), LevelId: "lvl-1", Name: "1° SECUNDARIA"}}), nil
}

func (pointStub) GetLevel(_ context.Context, req *connect.Request[configv1.GetLevelRequest]) (*connect.Response[configv1.GetLevelResponse], error) {
	status := configv1.LevelStatus_LEVEL_STATUS_ACTIVE
	if req.Msg.GetId() == "lvl-deleting" {
		status = configv1.LevelStatus_LEVEL_STATUS_DELETING
	}
	return connect.NewResponse(&configv1.GetLevelResponse{Level: &configv1.Level{
		Id: req.Msg.GetId(), Name: "Secundaria", AttendanceMode: configv1.AttendanceMode_ATTENDANCE_MODE_PER_CLASS,
		Status: status,
	}}), nil
}

func newPointStub(t *testing.T) *client.ConfigClient {
	t.Helper()
	mux := http.NewServeMux()
	path, h := configv1connect.NewConfigServiceHandler(pointStub{})
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return client.NewConfigClient(srv.URL, srv.Client())
}

func TestConfigClientPointReads(t *testing.T) {
	c := newPointStub(t)
	ctx := context.Background()

	g, err := c.GetGroup(ctx, "grp-1")
	require.NoError(t, err)
	assert.Equal(t, client.GroupResult{ID: "grp-1", CycleID: "cyc-1", GradeLevelID: "gl-1", Name: "A"}, g)

	gl, err := c.GetGradeLevel(ctx, "gl-1")
	require.NoError(t, err)
	assert.Equal(t, client.GradeLevelResult{ID: "gl-1", LevelID: "lvl-1", Name: "1° SECUNDARIA"}, gl)

	l, err := c.GetLevel(ctx, "lvl-1")
	require.NoError(t, err)
	assert.Equal(t, "Secundaria", l.Name)
	assert.Equal(t, client.AttendanceModePerClass, l.AttendanceMode)
	assert.False(t, l.Deleting)

	l, err = c.GetLevel(ctx, "lvl-deleting")
	require.NoError(t, err)
	assert.True(t, l.Deleting, "config's DELETING status reaches the edge (ADR 0021)")

	_, err = c.GetGroup(ctx, "ghost")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "a missing group keeps its NotFound code")
}
