package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// ADR 0021 through the real mux: module_config guards it, levels:delete (the
// owner's alone) gates it, and the whole preview → delete → trash → restore
// path runs against stub services.

const routeLevel = "aaaaaaaa-0000-0000-0000-0000000000a1"

func (stubIAM) VerifyPassword(_ context.Context, req *connect.Request[iamv1.VerifyPasswordRequest]) (*connect.Response[iamv1.VerifyPasswordResponse], error) {
	if req.Msg.GetPassword() != "director-pw" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errStr("invalid credentials"))
	}
	return connect.NewResponse(&iamv1.VerifyPasswordResponse{}), nil
}

func routeDeletion() *configv1.LevelDeletion {
	at := time.Now().UTC()
	return &configv1.LevelDeletion{
		BatchId: "bbbbbbbb-0000-0000-0000-0000000000b1", LevelId: routeLevel, LevelName: "Maternal",
		Counts: []*configv1.LevelArchiveCount{{Kind: "groups", Count: 1}}, DeletedByName: "Directora",
		DeletedAt: timestamppb.New(at), RestorableUntil: timestamppb.New(at.Add(7 * 24 * time.Hour)), Status: "deleted",
	}
}

func (stubConfig) PreviewLevelDeletion(_ context.Context, req *connect.Request[configv1.PreviewLevelDeletionRequest]) (*connect.Response[configv1.PreviewLevelDeletionResponse], error) {
	return connect.NewResponse(&configv1.PreviewLevelDeletionResponse{
		LevelId: req.Msg.GetLevelId(), LevelName: "Maternal", GroupIds: []string{"grp-1"},
		Counts: []*configv1.LevelArchiveCount{{Kind: "levels", Count: 1}, {Kind: "groups", Count: 1}},
	}), nil
}

func (stubConfig) BeginLevelDeletion(_ context.Context, req *connect.Request[configv1.BeginLevelDeletionRequest]) (*connect.Response[configv1.BeginLevelDeletionResponse], error) {
	d := routeDeletion()
	d.Status = "deleting"
	return connect.NewResponse(&configv1.BeginLevelDeletionResponse{Deletion: d}), nil
}

func (stubConfig) SweepLevelDeletions(context.Context, *connect.Request[configv1.SweepLevelDeletionsRequest]) (*connect.Response[configv1.SweepLevelDeletionsResponse], error) {
	return connect.NewResponse(&configv1.SweepLevelDeletionsResponse{}), nil
}

func (stubConfig) ArchiveLevel(context.Context, *connect.Request[configv1.ArchiveLevelRequest]) (*connect.Response[configv1.ArchiveLevelResponse], error) {
	return connect.NewResponse(&configv1.ArchiveLevelResponse{Deletion: routeDeletion()}), nil
}

func (stubConfig) RestoreLevelBatch(context.Context, *connect.Request[configv1.RestoreLevelBatchRequest]) (*connect.Response[configv1.RestoreLevelBatchResponse], error) {
	return connect.NewResponse(&configv1.RestoreLevelBatchResponse{Restored: []*configv1.LevelArchiveCount{{Kind: "groups", Count: 1}}}), nil
}

func (stubConfig) MarkLevelDeletionRestored(context.Context, *connect.Request[configv1.MarkLevelDeletionRestoredRequest]) (*connect.Response[configv1.MarkLevelDeletionRestoredResponse], error) {
	d := routeDeletion()
	d.Status = "restored"
	return connect.NewResponse(&configv1.MarkLevelDeletionRestoredResponse{Deletion: d}), nil
}

func (stubConfig) BeginLevelRestore(context.Context, *connect.Request[configv1.BeginLevelRestoreRequest]) (*connect.Response[configv1.BeginLevelRestoreResponse], error) {
	d := routeDeletion()
	d.RestoringFrom, d.Status = d.Status, "restoring"
	return connect.NewResponse(&configv1.BeginLevelRestoreResponse{Deletion: d}), nil
}

func (stubConfig) ListLevelDeletions(context.Context, *connect.Request[configv1.ListLevelDeletionsRequest]) (*connect.Response[configv1.ListLevelDeletionsResponse], error) {
	return connect.NewResponse(&configv1.ListLevelDeletionsResponse{Deletions: []*configv1.LevelDeletion{routeDeletion()}}), nil
}

func (stubConfig) GetLevelDeletion(context.Context, *connect.Request[configv1.GetLevelDeletionRequest]) (*connect.Response[configv1.GetLevelDeletionResponse], error) {
	return connect.NewResponse(&configv1.GetLevelDeletionResponse{Deletion: routeDeletion()}), nil
}

func (stubStudents) PreviewLevelDeletion(context.Context, *connect.Request[studentsv1.PreviewLevelDeletionRequest]) (*connect.Response[studentsv1.PreviewLevelDeletionResponse], error) {
	return connect.NewResponse(&studentsv1.PreviewLevelDeletionResponse{
		Counts: []*studentsv1.LevelArchiveCount{{Kind: "students", Count: 1}}, StudentIds: []string{"stu-1"},
	}), nil
}

func (stubStudents) ArchiveLevel(context.Context, *connect.Request[studentsv1.ArchiveLevelRequest]) (*connect.Response[studentsv1.ArchiveLevelResponse], error) {
	return connect.NewResponse(&studentsv1.ArchiveLevelResponse{}), nil
}

func (stubStudents) RestoreLevelBatch(context.Context, *connect.Request[studentsv1.RestoreLevelBatchRequest]) (*connect.Response[studentsv1.RestoreLevelBatchResponse], error) {
	return connect.NewResponse(&studentsv1.RestoreLevelBatchResponse{}), nil
}

func (stubGrades) PreviewLevelDeletion(context.Context, *connect.Request[gradesv1.PreviewLevelDeletionRequest]) (*connect.Response[gradesv1.PreviewLevelDeletionResponse], error) {
	return connect.NewResponse(&gradesv1.PreviewLevelDeletionResponse{}), nil
}

func (stubGrades) ArchiveLevel(context.Context, *connect.Request[gradesv1.ArchiveLevelRequest]) (*connect.Response[gradesv1.ArchiveLevelResponse], error) {
	return connect.NewResponse(&gradesv1.ArchiveLevelResponse{}), nil
}

func (stubGrades) RestoreLevelBatch(context.Context, *connect.Request[gradesv1.RestoreLevelBatchRequest]) (*connect.Response[gradesv1.RestoreLevelBatchResponse], error) {
	return connect.NewResponse(&gradesv1.RestoreLevelBatchResponse{}), nil
}

func (stubAttendance) PreviewLevelDeletion(context.Context, *connect.Request[attendancev1.PreviewLevelDeletionRequest]) (*connect.Response[attendancev1.PreviewLevelDeletionResponse], error) {
	return connect.NewResponse(&attendancev1.PreviewLevelDeletionResponse{}), nil
}

func (stubAttendance) ArchiveLevel(context.Context, *connect.Request[attendancev1.ArchiveLevelRequest]) (*connect.Response[attendancev1.ArchiveLevelResponse], error) {
	return connect.NewResponse(&attendancev1.ArchiveLevelResponse{}), nil
}

func (stubAttendance) RestoreLevelBatch(context.Context, *connect.Request[attendancev1.RestoreLevelBatchRequest]) (*connect.Response[attendancev1.RestoreLevelBatchResponse], error) {
	return connect.NewResponse(&attendancev1.RestoreLevelBatchResponse{}), nil
}

func (stubFinance) PreviewLevelDeletion(context.Context, *connect.Request[financev1.PreviewLevelDeletionRequest]) (*connect.Response[financev1.PreviewLevelDeletionResponse], error) {
	return connect.NewResponse(&financev1.PreviewLevelDeletionResponse{
		Counts: []*financev1.LevelArchiveCount{{Kind: "tuitionPlans", Count: 1}, {Kind: "charges", Count: 5}, {Kind: "payments", Count: 3}},
	}), nil
}

func (stubFinance) ArchiveLevel(context.Context, *connect.Request[financev1.ArchiveLevelRequest]) (*connect.Response[financev1.ArchiveLevelResponse], error) {
	return connect.NewResponse(&financev1.ArchiveLevelResponse{}), nil
}

func (stubFinance) RestoreLevelBatch(context.Context, *connect.Request[financev1.RestoreLevelBatchRequest]) (*connect.Response[financev1.RestoreLevelBatchResponse], error) {
	return connect.NewResponse(&financev1.RestoreLevelBatchResponse{}), nil
}

func (stubUniforms) PreviewLevelDeletion(context.Context, *connect.Request[uniformsv1.PreviewLevelDeletionRequest]) (*connect.Response[uniformsv1.PreviewLevelDeletionResponse], error) {
	return connect.NewResponse(&uniformsv1.PreviewLevelDeletionResponse{}), nil
}

func (stubUniforms) ArchiveLevel(context.Context, *connect.Request[uniformsv1.ArchiveLevelRequest]) (*connect.Response[uniformsv1.ArchiveLevelResponse], error) {
	return connect.NewResponse(&uniformsv1.ArchiveLevelResponse{}), nil
}

func (stubUniforms) RestoreLevelBatch(context.Context, *connect.Request[uniformsv1.RestoreLevelBatchRequest]) (*connect.Response[uniformsv1.RestoreLevelBatchResponse], error) {
	return connect.NewResponse(&uniformsv1.RestoreLevelBatchResponse{}), nil
}

var levelDeletionRoutes = []struct{ method, path, body string }{
	{http.MethodGet, "/api/v1/config/levels/" + routeLevel + "/deletion-preview", ""},
	{http.MethodPost, "/api/v1/config/levels/" + routeLevel + "/delete", `{"confirmName":"Maternal","confirmToken":"x","password":"director-pw"}`},
	{http.MethodGet, "/api/v1/config/level-trash", ""},
	{http.MethodPost, "/api/v1/config/level-trash/bbbbbbbb-0000-0000-0000-0000000000b1/restore", ""},
}

func TestLevelDeletionRoutesAreOwnerOnly(t *testing.T) {
	gw := newTestGateway(t, allModulesOn)
	for _, token := range []string{"config-writer", "maestro", "cobranza"} {
		for _, rt := range levelDeletionRoutes {
			t.Run(token+" "+rt.method+" "+rt.path, func(t *testing.T) {
				res := doGateway(t, gw, rt.method, rt.path, token, rt.body)
				assert.Equal(t, http.StatusForbidden, res.StatusCode, "config:write is not levels:delete")
			})
		}
	}
}

func TestLevelDeletionRoutesHideWithModuleConfigOff(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	for _, rt := range levelDeletionRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			res := doGateway(t, gw, rt.method, rt.path, "level-deleter", rt.body)
			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})
	}
}

func TestLevelDeletionThroughTheMux(t *testing.T) {
	gw := newTestGateway(t, allModulesOn)

	res := doGateway(t, gw, http.MethodGet, levelDeletionRoutes[0].path, "level-deleter", "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var preview struct {
		Token string           `json:"confirmToken"`
		Keeps map[string]int64 `json:"keeps"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	assert.Equal(t, int64(5), preview.Keeps["charges"])

	wrong := doGateway(t, gw, http.MethodPost, levelDeletionRoutes[1].path, "level-deleter",
		`{"confirmName":"Maternal","confirmToken":"`+preview.Token+`","password":"nope"}`)
	assert.Equal(t, http.StatusUnauthorized, wrong.StatusCode)
	assert.Equal(t, "REAUTH_FAILED", wrong.Header.Get("X-Error-Code"))

	ok := doGateway(t, gw, http.MethodPost, levelDeletionRoutes[1].path, "level-deleter",
		`{"confirmName":"maternal","confirmToken":"`+preview.Token+`","password":"director-pw"}`)
	body, _ := io.ReadAll(ok.Body)
	require.Equal(t, http.StatusOK, ok.StatusCode, string(body))

	trash := doGateway(t, gw, http.MethodGet, levelDeletionRoutes[2].path, "level-deleter", "")
	assert.Equal(t, http.StatusOK, trash.StatusCode)

	restore := doGateway(t, gw, http.MethodPost, levelDeletionRoutes[3].path, "level-deleter", "")
	assert.Equal(t, http.StatusOK, restore.StatusCode)
}

// The delete re-checks a password, so it is throttled like a login: once the
// bucket is spent, even the right password waits (429).
func TestLevelDeletionIsRateLimitedLikeLogin(t *testing.T) {
	frozen := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	limiter := middleware.NewRateLimiter(2, 0.0001, func() time.Time { return frozen })
	gw := newTestGatewayWithReauth(t, allModulesOn, limiter)
	path := levelDeletionRoutes[1].path
	for i := 0; i < 2; i++ {
		res := doGateway(t, gw, http.MethodPost, path, "level-deleter", `{"confirmName":"Maternal","confirmToken":"x","password":"guess"}`)
		assert.NotEqual(t, http.StatusTooManyRequests, res.StatusCode)
	}
	res := doGateway(t, gw, http.MethodPost, path, "level-deleter", `{"confirmName":"Maternal","confirmToken":"x","password":"director-pw"}`)
	assert.Equal(t, http.StatusTooManyRequests, res.StatusCode)
}

// Changing the password checks the current one, so it is throttled like a
// re-authentication: per signed-in user, on the reauth bucket.
func TestChangePasswordIsRateLimitedPerUser(t *testing.T) {
	frozen := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	limiter := middleware.NewRateLimiter(2, 0.0001, func() time.Time { return frozen })
	gw := newTestGatewayWithReauth(t, allModulesOn, limiter)
	body := `{"currentPassword":"guess","newPassword":"newlongpass"}`
	for i := 0; i < 2; i++ {
		res := doGateway(t, gw, http.MethodPost, "/api/v1/auth/change-password", "maestro", body)
		assert.NotEqual(t, http.StatusTooManyRequests, res.StatusCode)
	}
	res := doGateway(t, gw, http.MethodPost, "/api/v1/auth/change-password", "maestro", body)
	assert.Equal(t, http.StatusTooManyRequests, res.StatusCode)
}
