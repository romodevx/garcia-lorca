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
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

type stubLevelConfig struct {
	configv1connect.UnimplementedConfigServiceHandler
	gotSummary map[string]int64
	gotBegin   *configv1.BeginLevelDeletionRequest
	gotFail    *configv1.FailLevelDeletionRequest
	gotPartial bool
	gotAbandon string
}

func (s *stubLevelConfig) BeginLevelDeletion(_ context.Context, req *connect.Request[configv1.BeginLevelDeletionRequest]) (*connect.Response[configv1.BeginLevelDeletionResponse], error) {
	s.gotBegin = req.Msg
	return connect.NewResponse(&configv1.BeginLevelDeletionResponse{Deletion: &configv1.LevelDeletion{BatchId: req.Msg.GetBatchId(), Status: "deleting"}}), nil
}

func (s *stubLevelConfig) FailLevelDeletion(_ context.Context, req *connect.Request[configv1.FailLevelDeletionRequest]) (*connect.Response[configv1.FailLevelDeletionResponse], error) {
	s.gotFail = req.Msg
	return connect.NewResponse(&configv1.FailLevelDeletionResponse{Deletion: &configv1.LevelDeletion{
		BatchId: req.Msg.GetBatchId(), Status: "failed_partial", PendingSteps: req.Msg.GetPendingSteps(), Skipped: req.Msg.GetSkipped(),
	}}), nil
}

func (s *stubLevelConfig) MarkLevelDeletionRestored(_ context.Context, req *connect.Request[configv1.MarkLevelDeletionRestoredRequest]) (*connect.Response[configv1.MarkLevelDeletionRestoredResponse], error) {
	s.gotPartial = req.Msg.GetPartial()
	return connect.NewResponse(&configv1.MarkLevelDeletionRestoredResponse{Deletion: &configv1.LevelDeletion{Status: "restored_partial"}}), nil
}

func (s *stubLevelConfig) BeginLevelRestore(_ context.Context, req *connect.Request[configv1.BeginLevelRestoreRequest]) (*connect.Response[configv1.BeginLevelRestoreResponse], error) {
	return connect.NewResponse(&configv1.BeginLevelRestoreResponse{Deletion: &configv1.LevelDeletion{
		BatchId: req.Msg.GetBatchId(), Status: "restoring", RestoringFrom: "failed_partial", PendingSteps: []string{"grades"},
	}}), nil
}

func (s *stubLevelConfig) AbandonLevelRestore(_ context.Context, req *connect.Request[configv1.AbandonLevelRestoreRequest]) (*connect.Response[configv1.AbandonLevelRestoreResponse], error) {
	s.gotAbandon = req.Msg.GetBatchId()
	return connect.NewResponse(&configv1.AbandonLevelRestoreResponse{Deletion: &configv1.LevelDeletion{
		BatchId: req.Msg.GetBatchId(), Status: "failed_partial",
	}}), nil
}

func (s *stubLevelConfig) SweepLevelDeletions(context.Context, *connect.Request[configv1.SweepLevelDeletionsRequest]) (*connect.Response[configv1.SweepLevelDeletionsResponse], error) {
	return connect.NewResponse(&configv1.SweepLevelDeletionsResponse{Expired: []*configv1.LevelDeletion{{BatchId: "old"}}, Stalled: 1}), nil
}

func (s *stubLevelConfig) PurgeLevelBatch(_ context.Context, req *connect.Request[configv1.PurgeLevelBatchRequest]) (*connect.Response[configv1.PurgeLevelBatchResponse], error) {
	return connect.NewResponse(&configv1.PurgeLevelBatchResponse{PurgedRows: 4}), nil
}

func (s *stubLevelConfig) PreviewLevelDeletion(_ context.Context, req *connect.Request[configv1.PreviewLevelDeletionRequest]) (*connect.Response[configv1.PreviewLevelDeletionResponse], error) {
	return connect.NewResponse(&configv1.PreviewLevelDeletionResponse{
		LevelId: req.Msg.GetLevelId(), LevelName: "Primaria", GroupIds: []string{"g1"}, TermIds: []string{"t1"},
		LevelStatus: configv1.LevelStatus_LEVEL_STATUS_DELETING,
		Counts:      []*configv1.LevelArchiveCount{{Kind: "groups", Count: 1}, {Kind: "terms", Count: 2}},
	}), nil
}

func (s *stubLevelConfig) ArchiveLevel(_ context.Context, req *connect.Request[configv1.ArchiveLevelRequest]) (*connect.Response[configv1.ArchiveLevelResponse], error) {
	s.gotSummary = map[string]int64{}
	for _, kc := range req.Msg.GetSummary() {
		s.gotSummary[kc.GetKind()] = kc.GetCount()
	}
	return connect.NewResponse(&configv1.ArchiveLevelResponse{Deletion: &configv1.LevelDeletion{BatchId: req.Msg.GetBatchId(), Status: "deleted"}}), nil
}

func (s *stubLevelConfig) RestoreLevelBatch(context.Context, *connect.Request[configv1.RestoreLevelBatchRequest]) (*connect.Response[configv1.RestoreLevelBatchResponse], error) {
	return connect.NewResponse(&configv1.RestoreLevelBatchResponse{
		Restored: []*configv1.LevelArchiveCount{{Kind: "groups", Count: 1}},
		Skipped:  []*configv1.LevelRestoreSkip{{Kind: "teachingAssignments", Count: 1, Reason: "docente ya no existe"}},
	}), nil
}

type stubLevelStudents struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	gotScope *studentsv1.LevelScope
}

func (s *stubLevelStudents) RestoreLevelBatch(context.Context, *connect.Request[studentsv1.RestoreLevelBatchRequest]) (*connect.Response[studentsv1.RestoreLevelBatchResponse], error) {
	return connect.NewResponse(&studentsv1.RestoreLevelBatchResponse{UnrestoredEnrollmentIds: []string{"e9"}}), nil
}

func (s *stubLevelStudents) PreviewLevelDeletion(_ context.Context, req *connect.Request[studentsv1.PreviewLevelDeletionRequest]) (*connect.Response[studentsv1.PreviewLevelDeletionResponse], error) {
	s.gotScope = req.Msg.GetScope()
	return connect.NewResponse(&studentsv1.PreviewLevelDeletionResponse{
		Counts: []*studentsv1.LevelArchiveCount{{Kind: "students", Count: 3}}, StudentIds: []string{"s1"}, EnrollmentIds: []string{"e1"},
	}), nil
}

func serve(t *testing.T) func(path string, h http.Handler) string {
	t.Helper()
	return func(path string, h http.Handler) string { return serveAt(t, path, h) }
}

func serveAt(t *testing.T, path string, h http.Handler) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestLevelDeletionWireRoundTrip(t *testing.T) {
	ctx := context.Background()
	cfgStub := &stubLevelConfig{}
	cfg := client.NewConfigClient(serve(t)(configv1connect.NewConfigServiceHandler(cfgStub)), nil)
	stuStub := &stubLevelStudents{}
	stu := client.NewStudentsClient(serve(t)(studentsv1connect.NewStudentsServiceHandler(stuStub)), nil)

	st, err := cfg.PreviewLevelDeletion(ctx, "lvl-1")
	require.NoError(t, err)
	assert.Equal(t, client.LevelCounts{"groups": 1, "terms": 2}, st.Counts)
	assert.Equal(t, []string{"g1"}, st.GroupIDs)

	people, err := stu.PreviewLevelDeletion(ctx, client.LevelScope{LevelID: "lvl-1", GroupIDs: st.GroupIDs, TermIDs: st.TermIDs})
	require.NoError(t, err)
	assert.Equal(t, []string{"g1"}, stuStub.gotScope.GetGroupIds(), "the scope crosses the wire whole")
	assert.Equal(t, []string{"t1"}, stuStub.gotScope.GetTermIds())
	assert.Equal(t, []string{"e1"}, people.EnrollmentIDs)
	assert.Equal(t, int64(3), people.Counts["students"])

	_, rec, err := cfg.ArchiveLevel(ctx, client.ArchiveLevelRecord{BatchID: "b1", LevelID: "lvl-1", Summary: client.LevelCounts{"students": 3}})
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{"students": 3}, cfgStub.gotSummary)
	assert.Equal(t, "b1", rec.BatchID)

	rep, err := cfg.RestoreLevelBatch(ctx, "b1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), rep.Restored["groups"])
	require.Len(t, rep.Skipped, 1)
	assert.Equal(t, "docente ya no existe", rep.Skipped[0].Reason)
}

// Review fixes: the lock, the single-use token, the ledger, the restore's
// enrollments hand-off and the sweep cross the wire whole.
func TestLevelDeletionSagaLedgerWire(t *testing.T) {
	ctx := context.Background()
	cfgStub := &stubLevelConfig{}
	cfg := client.NewConfigClient(serve(t)(configv1connect.NewConfigServiceHandler(cfgStub)), nil)
	stu := client.NewStudentsClient(serve(t)(studentsv1connect.NewStudentsServiceHandler(&stubLevelStudents{})), nil)

	st, err := cfg.PreviewLevelDeletion(ctx, "lvl-1")
	require.NoError(t, err)
	assert.True(t, st.Deleting)

	rec, err := cfg.BeginLevelDeletion(ctx, client.BeginLevelDeletionRecord{BatchID: "b1", LevelID: "lvl-1", TokenHash: "h", DeletedBy: "u", DeletedByName: "Laura"})
	require.NoError(t, err)
	assert.Equal(t, "deleting", rec.Status)
	assert.Equal(t, "h", cfgStub.gotBegin.GetTokenHash())
	assert.Equal(t, "Laura", cfgStub.gotBegin.GetDeletedByName())

	failed, err := cfg.FailLevelDeletion(ctx, "b1", []string{"grades"}, []client.LevelRestoreSkip{{Kind: "enrollments", Count: 2, Reason: "r"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"grades"}, failed.PendingSteps)
	require.Len(t, failed.Skipped, 1)
	assert.Equal(t, int64(2), failed.Skipped[0].Count)

	_, err = cfg.MarkLevelDeletionRestored(ctx, "b1", true)
	require.NoError(t, err)
	assert.True(t, cfgStub.gotPartial)

	expired, stalled, err := cfg.SweepLevelDeletions(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stalled)
	require.Len(t, expired, 1)
	n, err := cfg.PurgeLevelBatch(ctx, "old")
	require.NoError(t, err)
	assert.Equal(t, int64(4), n)

	rep, err := stu.RestoreLevelBatch(ctx, "b1")
	require.NoError(t, err)
	assert.Equal(t, []string{"e9"}, rep.Unrestored, "grades and attendance get these as skip ids")
}

// Confirmation review (gap 4): the restore's start and fall-back reach config,
// and restoring_from comes back.
func TestLevelRestoreStartWire(t *testing.T) {
	ctx := context.Background()
	stub := &stubLevelConfig{}
	cfg := client.NewConfigClient(serve(t)(configv1connect.NewConfigServiceHandler(stub)), nil)

	begun, err := cfg.BeginLevelRestore(ctx, "b1")
	require.NoError(t, err)
	assert.Equal(t, "restoring", begun.Status)
	assert.Equal(t, "failed_partial", begun.RestoringFrom)
	assert.Equal(t, []string{"grades"}, begun.PendingSteps)

	back, err := cfg.AbandonLevelRestore(ctx, "b1")
	require.NoError(t, err)
	assert.Equal(t, "failed_partial", back.Status)
	assert.Equal(t, "b1", stub.gotAbandon)
}
