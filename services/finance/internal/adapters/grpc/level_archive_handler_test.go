package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/pkg/levelarchive"
	grpcadapter "github.com/garcia-lorca/services/finance/internal/adapters/grpc"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

const (
	hLevel = "aaaaaaaa-0000-0000-0000-000000000001"
	hBatch = "bbbbbbbb-0000-0000-0000-000000000001"
	hEnrol = "cccccccc-0000-0000-0000-000000000001"
)

// stubLevelArchive is the repository behind a real LevelArchiveService.
type stubLevelArchive struct{ scope levelarchive.Scope }

func (s *stubLevelArchive) PreviewLevelDeletion(_ context.Context, sc levelarchive.Scope) (levelarchive.Counts, error) {
	s.scope = sc
	return levelarchive.Counts{"charges": 2, "tuitionPlans": 5}, nil
}
func (s *stubLevelArchive) ArchiveLevel(_ context.Context, _ string, sc levelarchive.Scope) (levelarchive.Counts, error) {
	s.scope = sc
	return levelarchive.Counts{"tuitionPlans": 5}, nil
}
func (s *stubLevelArchive) RestoreLevelBatch(context.Context, string) (levelarchive.Report, error) {
	rep := levelarchive.NewReport()
	rep.Record("tuitionPlans", 4, 1, "ya existe")
	return rep, nil
}
func (s *stubLevelArchive) PurgeLevelArchives(context.Context, time.Time) (int64, error) {
	return 9, nil
}

func levelArchiveHandler(repo *stubLevelArchive) *grpcadapter.FinanceHandler {
	return grpcadapter.NewFinanceHandler(nil, usecase.NewHealthService("finance")).
		WithLevelArchive(usecase.NewLevelArchiveService(repo, time.Now))
}

func TestLevelArchiveHandlerWireMapping(t *testing.T) {
	ctx := context.Background()
	repo := &stubLevelArchive{}
	h := levelArchiveHandler(repo)

	prev, err := h.PreviewLevelDeletion(ctx, connect.NewRequest(&financev1.PreviewLevelDeletionRequest{
		Scope: &financev1.LevelScope{LevelId: hLevel, EnrollmentIds: []string{hEnrol}},
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{hEnrol}, repo.scope.EnrollmentIDs, "the scope reaches the use case")
	require.Len(t, prev.Msg.GetCounts(), 2)
	assert.Equal(t, "charges", prev.Msg.GetCounts()[0].GetKind(), "counts are sorted by kind")
	assert.Equal(t, int64(5), prev.Msg.GetCounts()[1].GetCount())

	arch, err := h.ArchiveLevel(ctx, connect.NewRequest(&financev1.ArchiveLevelRequest{
		BatchId: hBatch, Scope: &financev1.LevelScope{LevelId: hLevel},
	}))
	require.NoError(t, err)
	assert.Equal(t, int64(5), arch.Msg.GetCounts()[0].GetCount())

	rest, err := h.RestoreLevelBatch(ctx, connect.NewRequest(&financev1.RestoreLevelBatchRequest{BatchId: hBatch}))
	require.NoError(t, err)
	assert.Equal(t, int64(4), rest.Msg.GetRestored()[0].GetCount())
	require.Len(t, rest.Msg.GetSkipped(), 1)
	assert.Equal(t, "ya existe", rest.Msg.GetSkipped()[0].GetReason())

	purge, err := h.PurgeLevelArchives(ctx, connect.NewRequest(&financev1.PurgeLevelArchivesRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int64(9), purge.Msg.GetPurgedRows())
}

func TestLevelArchiveHandlerRefusesAMalformedScope(t *testing.T) {
	h := levelArchiveHandler(&stubLevelArchive{})
	cases := []struct {
		name string
		call func() error
	}{
		{"preview without scope", func() error {
			_, err := h.PreviewLevelDeletion(context.Background(), connect.NewRequest(&financev1.PreviewLevelDeletionRequest{}))
			return err
		}},
		{"archive with a bad batch", func() error {
			_, err := h.ArchiveLevel(context.Background(), connect.NewRequest(&financev1.ArchiveLevelRequest{
				BatchId: "x", Scope: &financev1.LevelScope{LevelId: hLevel}}))
			return err
		}},
		{"restore with a bad batch", func() error {
			_, err := h.RestoreLevelBatch(context.Background(), connect.NewRequest(&financev1.RestoreLevelBatchRequest{BatchId: "x"}))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(tc.call()))
		})
	}
}
