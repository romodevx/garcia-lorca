package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// --- carga académica routes: the same chain as /api/v1/teaching-assignments —
// ModuleGuard(teachers) → RequireAuth → RequirePermission(teachers:write).

func (stubConfig) AssignTeachingLoad(_ context.Context, req *connect.Request[configv1.AssignTeachingLoadRequest]) (*connect.Response[configv1.AssignTeachingLoadResponse], error) {
	created := make([]*configv1.TeachingAssignment, 0, len(req.Msg.GetItems()))
	for _, it := range req.Msg.GetItems() {
		created = append(created, &configv1.TeachingAssignment{
			Id: "asg-new", CycleId: "cyc-1", GroupId: it.GetGroupId(), SubjectId: it.GetSubjectId(), TeacherId: req.Msg.GetTeacherId(),
		})
	}
	return connect.NewResponse(&configv1.AssignTeachingLoadResponse{Created: created}), nil
}

func (stubConfig) DeleteTeachingAssignment(_ context.Context, _ *connect.Request[configv1.DeleteTeachingAssignmentRequest]) (*connect.Response[configv1.DeleteTeachingAssignmentResponse], error) {
	return connect.NewResponse(&configv1.DeleteTeachingAssignmentResponse{}), nil
}

func (stubConfig) ReassignTeachingAssignment(_ context.Context, req *connect.Request[configv1.ReassignTeachingAssignmentRequest]) (*connect.Response[configv1.ReassignTeachingAssignmentResponse], error) {
	return connect.NewResponse(&configv1.ReassignTeachingAssignmentResponse{TeachingAssignment: &configv1.TeachingAssignment{
		Id: req.Msg.GetId(), CycleId: "cyc-1", GroupId: "grp-1", SubjectId: "sub-1", TeacherId: req.Msg.GetTeacherId(),
	}}), nil
}

const (
	loadAssignmentID = "00000000-0000-4000-8000-000000000030"
	validBulkBody    = `{"teacherId":"00000000-0000-4000-8000-000000000001","items":[{"groupId":"00000000-0000-4000-8000-000000000010","subjectId":"00000000-0000-4000-8000-000000000020"}]}`
	validPatchBody   = `{"teacherId":"00000000-0000-4000-8000-000000000002"}`
)

func TestTeachingLoadRoutesSecurityChain(t *testing.T) {
	routes := []struct {
		name   string
		method string
		path   string
		body   string
		ok     int
	}{
		{"bulk", http.MethodPost, "/api/v1/teaching-assignments/bulk", validBulkBody, http.StatusOK},
		{"delete", http.MethodDelete, "/api/v1/teaching-assignments/" + loadAssignmentID, "", http.StatusNoContent},
		{"reassign", http.MethodPatch, "/api/v1/teaching-assignments/" + loadAssignmentID, validPatchBody, http.StatusOK},
	}
	on := map[string]bool{"module_teachers": true}
	for _, rt := range routes {
		t.Run(rt.name+"/module off is 404", func(t *testing.T) {
			res := doGateway(t, newTestGateway(t, map[string]bool{}), rt.method, rt.path, "teachers-writer", rt.body)
			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})
		t.Run(rt.name+"/no token is 401", func(t *testing.T) {
			res := doGateway(t, newTestGateway(t, on), rt.method, rt.path, "", rt.body)
			assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})
		t.Run(rt.name+"/read-only permission is 403", func(t *testing.T) {
			res := doGateway(t, newTestGateway(t, on), rt.method, rt.path, "teachers-reader", rt.body)
			assert.Equal(t, http.StatusForbidden, res.StatusCode)
		})
		t.Run(rt.name+"/config:write does not open it", func(t *testing.T) {
			gw := newTestGateway(t, map[string]bool{"module_teachers": true, "module_config": true})
			res := doGateway(t, gw, rt.method, rt.path, "config-writer", rt.body)
			assert.Equal(t, http.StatusForbidden, res.StatusCode)
		})
		t.Run(rt.name+"/teachers:write succeeds", func(t *testing.T) {
			res := doGateway(t, newTestGateway(t, on), rt.method, rt.path, "teachers-writer", rt.body)
			assert.Equal(t, rt.ok, res.StatusCode)
		})
	}
}

// The web once linked a teacher's login through /config/teachers/{id}/user, a
// route the gateway never registered: the button 404'd in silence. The link
// lives at /teachers/{id}/user only.
func TestTeacherLinkStaleConfigPathIsNotARoute(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true, "module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teachers/tch-1/user", "teachers-writer", `{"userId":"usr-1"}`)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}
