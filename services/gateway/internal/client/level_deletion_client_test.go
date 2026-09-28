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
}

func (s *stubLevelConfig) PreviewLevelDeletion(_ context.Context, req *connect.Request[configv1.PreviewLevelDeletionRequest]) (*connect.Response[configv1.PreviewLevelDeletionResponse], error) {
	return connect.NewResponse(&configv1.PreviewLevelDeletionResponse{
		LevelId: req.Msg.GetLevelId(), LevelName: "Primaria", GroupIds: []string{"g1"}, TermIds: []string{"t1"},
		Counts: []*configv1.LevelArchiveCount{{Kind: "groups", Count: 1}, {Kind: "terms", Count: 2}},
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
