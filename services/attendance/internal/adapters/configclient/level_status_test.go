package configclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
)

// stubConfig serves the group → grade level → level chain.
type stubConfig struct {
	configv1connect.UnimplementedConfigServiceHandler
	status configv1.LevelStatus
}

func (s *stubConfig) GetGroup(_ context.Context, r *connect.Request[configv1.GetGroupRequest]) (*connect.Response[configv1.GetGroupResponse], error) {
	if r.Msg.GetId() != "grp-1" {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&configv1.GetGroupResponse{Group: &configv1.Group{Id: "grp-1", GradeLevelId: "gl-1"}}), nil
}

func (s *stubConfig) GetGradeLevel(context.Context, *connect.Request[configv1.GetGradeLevelRequest]) (*connect.Response[configv1.GetGradeLevelResponse], error) {
	return connect.NewResponse(&configv1.GetGradeLevelResponse{GradeLevel: &configv1.GradeLevel{Id: "gl-1", LevelId: "lvl-1"}}), nil
}

func (s *stubConfig) GetLevel(context.Context, *connect.Request[configv1.GetLevelRequest]) (*connect.Response[configv1.GetLevelResponse], error) {
	return connect.NewResponse(&configv1.GetLevelResponse{Level: &configv1.Level{Id: "lvl-1", Status: s.status}}), nil
}

// Review finding 4: the write path resolves the group's level and refuses one
// whose deletion runs; an unknown group stays NotFound.
func TestWritableByIDRefusesALevelBeingDeleted(t *testing.T) {
	tests := []struct {
		name     string
		group    string
		status   configv1.LevelStatus
		wantCode string
		wantKind glerrors.Kind
		wantNil  bool
	}{
		{name: "active level", group: "grp-1", status: configv1.LevelStatus_LEVEL_STATUS_ACTIVE, wantNil: true},
		{name: "older config without status", group: "grp-1", status: configv1.LevelStatus_LEVEL_STATUS_UNSPECIFIED, wantNil: true},
		{name: "level being deleted", group: "grp-1", status: configv1.LevelStatus_LEVEL_STATUS_DELETING,
			wantCode: levelarchive.CodeLevelBeingDeleted, wantKind: glerrors.KindConflict},
		{name: "unknown group", group: "ghost", wantKind: glerrors.KindNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, h := configv1connect.NewConfigServiceHandler(&stubConfig{status: tt.status})
			mux := http.NewServeMux()
			mux.Handle(path, h)
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			err := New(srv.URL, srv.Client()).WritableByID(context.Background(), tt.group)
			if tt.wantNil {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if glerrors.KindOf(err) != tt.wantKind {
				t.Fatalf("KindOf(%v) = %v, want %v", err, glerrors.KindOf(err), tt.wantKind)
			}
			if levelarchive.CodeOf(err) != tt.wantCode {
				t.Fatalf("CodeOf = %q, want %q", levelarchive.CodeOf(err), tt.wantCode)
			}
		})
	}
}
