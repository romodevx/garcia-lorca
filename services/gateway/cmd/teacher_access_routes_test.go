package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
)

// POST /api/v1/teachers/{id}/access: ModuleGuard(teachers) → RequireAuth →
// rotation → RequireAllPermissions(teachers:write, users:write) → handler
// (caller-subset guard on the maestro role).

func (stubConfig) GetTeacher(_ context.Context, req *connect.Request[configv1.GetTeacherRequest]) (*connect.Response[configv1.GetTeacherResponse], error) {
	if req.Msg.GetId() != "tch-1" {
		return nil, connect.NewError(connect.CodeNotFound, errStr("teacher not found"))
	}
	return connect.NewResponse(&configv1.GetTeacherResponse{Teacher: &configv1.Teacher{
		Id: "tch-1", FirstName: "Juan", LastName: "Pérez", Email: "juan@garcialorca.mx",
	}}), nil
}

func (stubIAM) InviteUser(_ context.Context, req *connect.Request[iamv1.InviteUserRequest]) (*connect.Response[iamv1.InviteUserResponse], error) {
	return connect.NewResponse(&iamv1.InviteUserResponse{
		User: &iamv1.User{
			Id: "usr-new", Email: req.Msg.GetEmail(), DisplayName: req.Msg.GetDisplayName(),
			Status: iamv1.UserStatus_USER_STATUS_INVITED, RoleIds: req.Msg.GetRoleIds(),
		},
		Invitation: &iamv1.Invitation{Url: "http://localhost:3002/invitacion/tok", Sent: false},
	}), nil
}

const teacherAccessPath = "/api/v1/teachers/tch-1/access"

func TestTeacherAccessRouteSecurityChain(t *testing.T) {
	on := map[string]bool{"module_teachers": true}
	tests := []struct {
		name  string
		flags map[string]bool
		path  string
		token string
		want  int
	}{
		{"module off is 404", map[string]bool{}, teacherAccessPath, "teacher-access-admin", http.StatusNotFound},
		{"no token is 401", on, teacherAccessPath, "", http.StatusUnauthorized},
		{"without users:write is 403", on, teacherAccessPath, "teacher-access-no-users", http.StatusForbidden},
		{"without teachers:write is 403", on, teacherAccessPath, "teacher-access-no-teachers", http.StatusForbidden},
		{"without the maestro permissions is 403 (subset guard)", on, teacherAccessPath, "teacher-access-no-subset", http.StatusForbidden},
		{"a maestro is 403", on, teacherAccessPath, "maestro", http.StatusForbidden},
		{"an unknown teacher is 404", on, "/api/v1/teachers/tch-nope/access", "teacher-access-admin", http.StatusNotFound},
		{"with both permissions is 200", on, teacherAccessPath, "teacher-access-admin", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := doGateway(t, newTestGateway(t, tc.flags), http.MethodPost, tc.path, tc.token, "")
			assert.Equal(t, tc.want, res.StatusCode)
		})
	}
}

func TestTeacherAccessRouteReturnsTheLinkedTeacherAndTheAccount(t *testing.T) {
	res := doGateway(t, newTestGateway(t, map[string]bool{"module_teachers": true}), http.MethodPost, teacherAccessPath, "teacher-access-admin", "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var out struct {
		Teacher struct {
			ID     string `json:"id"`
			UserID string `json:"userId"`
		} `json:"teacher"`
		UserID     string `json:"userId"`
		Email      string `json:"email"`
		Status     string `json:"status"`
		Invitation struct {
			URL  string `json:"url"`
			Sent bool   `json:"sent"`
		} `json:"invitation"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&out))
	assert.Equal(t, "tch-1", out.Teacher.ID)
	assert.Equal(t, "usr-new", out.Teacher.UserID)
	assert.Equal(t, "usr-new", out.UserID)
	assert.Equal(t, "juan@garcialorca.mx", out.Email)
	assert.Equal(t, "invited", out.Status)
	assert.Equal(t, "http://localhost:3002/invitacion/tok", out.Invitation.URL)
}
