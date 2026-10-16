package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// --- catalog corrections (T8): the same chain as the rest of the catalog —
// ModuleGuard(config) → RequireAuth → RequirePermission(config:write).

func (stubConfig) ReorderLevels(_ context.Context, req *connect.Request[configv1.ReorderLevelsRequest]) (*connect.Response[configv1.ReorderLevelsResponse], error) {
	out := make([]*configv1.Level, 0, len(req.Msg.GetLevelIds()))
	for i, id := range req.Msg.GetLevelIds() {
		out = append(out, &configv1.Level{Id: id, Name: "Primaria", SortOrder: int32(i + 1)})
	}
	return connect.NewResponse(&configv1.ReorderLevelsResponse{Levels: out}), nil
}

func (stubConfig) RenameGradeLevel(_ context.Context, req *connect.Request[configv1.RenameGradeLevelRequest]) (*connect.Response[configv1.RenameGradeLevelResponse], error) {
	return connect.NewResponse(&configv1.RenameGradeLevelResponse{GradeLevel: &configv1.GradeLevel{Id: req.Msg.GetId(), Name: req.Msg.GetName(), Ordinal: 1}}), nil
}

func (stubConfig) DeleteGradeLevel(context.Context, *connect.Request[configv1.DeleteGradeLevelRequest]) (*connect.Response[configv1.DeleteGradeLevelResponse], error) {
	return connect.NewResponse(&configv1.DeleteGradeLevelResponse{}), nil
}

func (stubConfig) RenameGroup(_ context.Context, req *connect.Request[configv1.RenameGroupRequest]) (*connect.Response[configv1.RenameGroupResponse], error) {
	return connect.NewResponse(&configv1.RenameGroupResponse{Group: &configv1.Group{Id: req.Msg.GetId(), Name: req.Msg.GetName()}}), nil
}

func (stubConfig) GroupUsage(context.Context, *connect.Request[configv1.GroupUsageRequest]) (*connect.Response[configv1.GroupUsageResponse], error) {
	return connect.NewResponse(&configv1.GroupUsageResponse{}), nil
}

func (stubConfig) DeleteGroup(context.Context, *connect.Request[configv1.DeleteGroupRequest]) (*connect.Response[configv1.DeleteGroupResponse], error) {
	return connect.NewResponse(&configv1.DeleteGroupResponse{}), nil
}

func (stubStudents) GroupUsage(context.Context, *connect.Request[studentsv1.GroupUsageRequest]) (*connect.Response[studentsv1.GroupUsageResponse], error) {
	return connect.NewResponse(&studentsv1.GroupUsageResponse{}), nil
}

func (stubStudents) RefreshGroupName(context.Context, *connect.Request[studentsv1.RefreshGroupNameRequest]) (*connect.Response[studentsv1.RefreshGroupNameResponse], error) {
	return connect.NewResponse(&studentsv1.RefreshGroupNameResponse{}), nil
}

func (stubAttendance) GroupUsage(context.Context, *connect.Request[attendancev1.GroupUsageRequest]) (*connect.Response[attendancev1.GroupUsageResponse], error) {
	return connect.NewResponse(&attendancev1.GroupUsageResponse{}), nil
}

func (stubGrades) GroupUsage(context.Context, *connect.Request[gradesv1.GroupUsageRequest]) (*connect.Response[gradesv1.GroupUsageResponse], error) {
	return connect.NewResponse(&gradesv1.GroupUsageResponse{}), nil
}

const (
	correctionLevelA = "00000000-0000-4000-8000-0000000000a1"
	correctionLevelB = "00000000-0000-4000-8000-0000000000a2"
	correctionGrade  = "00000000-0000-4000-8000-0000000000a3"
	correctionGroup  = "00000000-0000-4000-8000-0000000000a4"
)

func TestCatalogCorrectionRoutesSecurityChain(t *testing.T) {
	routes := []struct {
		name, method, path, body string
		ok                       int
	}{
		{"reorder levels", http.MethodPut, "/api/v1/config/levels/order",
			`{"levelIds":["` + correctionLevelB + `","` + correctionLevelA + `"]}`, http.StatusOK},
		{"rename grade", http.MethodPatch, "/api/v1/config/grade-levels/" + correctionGrade, `{"name":"3°"}`, http.StatusOK},
		{"delete grade", http.MethodDelete, "/api/v1/config/grade-levels/" + correctionGrade, "", http.StatusNoContent},
		{"rename group", http.MethodPatch, "/api/v1/config/groups/" + correctionGroup, `{"name":"B"}`, http.StatusOK},
		{"delete group", http.MethodDelete, "/api/v1/config/groups/" + correctionGroup, "", http.StatusNoContent},
	}
	on := map[string]bool{"module_config": true}
	for _, rt := range routes {
		t.Run(rt.name+"/module off is 404", func(t *testing.T) {
			res := doGateway(t, newTestGateway(t, map[string]bool{}), rt.method, rt.path, "config-writer", rt.body)
			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})
		t.Run(rt.name+"/no token is 401", func(t *testing.T) {
			res := doGateway(t, newTestGateway(t, on), rt.method, rt.path, "", rt.body)
			assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})
		t.Run(rt.name+"/config:read is 403", func(t *testing.T) {
			res := doGateway(t, newTestGateway(t, on), rt.method, rt.path, "config-reader", rt.body)
			assert.Equal(t, http.StatusForbidden, res.StatusCode)
		})
		t.Run(rt.name+"/config:write succeeds", func(t *testing.T) {
			res := doGateway(t, newTestGateway(t, on), rt.method, rt.path, "config-writer", rt.body)
			assert.Equal(t, rt.ok, res.StatusCode)
		})
	}
}
