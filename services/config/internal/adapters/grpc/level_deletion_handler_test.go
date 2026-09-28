package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	grpcadapter "github.com/garcia-lorca/services/config/internal/adapters/grpc"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

const (
	hBatch = "aaaaaaaa-0000-0000-0000-00000000000b"
	hLevel = "aaaaaaaa-0000-0000-0000-00000000000c"
	hUser  = "aaaaaaaa-0000-0000-0000-00000000000d"
)

var hAt = time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)

// stubTrash answers fixed results, or restoreErr from a restore.
type stubTrash struct {
	restoreErr error
	summary    levelarchive.Counts
}

func (s *stubTrash) PreviewLevelDeletion(_ context.Context, levelID string) (ports.LevelStructure, error) {
	return ports.LevelStructure{
		LevelID: levelID, LevelName: "Primaria", GradeLevelIDs: []string{"g1"}, GroupIDs: []string{"gr1"},
		SubjectIDs: []string{"s1"}, TermIDs: []string{"t1"}, Counts: levelarchive.Counts{"groups": 1, "levels": 1},
	}, nil
}

func (s *stubTrash) ArchiveLevel(_ context.Context, in ports.ArchiveLevelInput) (levelarchive.Counts, domain.LevelDeletion, error) {
	s.summary = in.Summary
	d, err := domain.NewLevelDeletion(in.BatchID, in.LevelID, "Primaria", in.DeletedBy, in.DeletedByName, levelarchive.Counts{"groups": 1}, hAt)
	return levelarchive.Counts{"groups": 1}, d, err
}

func (s *stubTrash) RestoreLevelBatch(context.Context, string, time.Time) (levelarchive.Report, error) {
	if s.restoreErr != nil {
		return levelarchive.Report{}, s.restoreErr
	}
	rep := levelarchive.NewReport()
	rep.Record("groups", 1, 2, "choca")
	return rep, nil
}

func (s *stubTrash) MarkLevelDeletionRestored(_ context.Context, batchID string) (domain.LevelDeletion, error) {
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionRestored, DeletedAt: hAt, RestorableUntil: hAt}, nil
}

func (s *stubTrash) ListLevelDeletions(context.Context) ([]domain.LevelDeletion, error) {
	d, err := domain.NewLevelDeletion(hBatch, hLevel, "Primaria", hUser, "Laura", levelarchive.Counts{"students": 4}, hAt)
	return []domain.LevelDeletion{d}, err
}

func (s *stubTrash) GetLevelDeletion(context.Context, string) (domain.LevelDeletion, error) {
	return domain.LevelDeletion{}, glerrors.ErrNotFound
}

func (s *stubTrash) PurgeLevelArchives(context.Context, time.Time) (int64, error) { return 7, nil }

func trashHandler(stub *stubTrash) *grpcadapter.ConfigHandler {
	svc := usecase.NewLevelDeletionService(stub, func() time.Time { return hAt })
	return grpcadapter.NewConfigHandler(nil, nil).WithLevelDeletion(svc)
}

func TestPreviewLevelDeletionWire(t *testing.T) {
	h := trashHandler(&stubTrash{})
	res, err := h.PreviewLevelDeletion(context.Background(), connect.NewRequest(&configv1.PreviewLevelDeletionRequest{LevelId: hLevel}))
	require.NoError(t, err)
	assert.Equal(t, "Primaria", res.Msg.GetLevelName())
	assert.Equal(t, []string{"gr1"}, res.Msg.GetGroupIds())
	require.Len(t, res.Msg.GetCounts(), 2)
	assert.Equal(t, "groups", res.Msg.GetCounts()[0].GetKind(), "counts are sorted by kind")

	_, err = h.PreviewLevelDeletion(context.Background(), connect.NewRequest(&configv1.PreviewLevelDeletionRequest{LevelId: "kinder"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestArchiveLevelWireCarriesTheSummaryAndTheLedgerRow(t *testing.T) {
	stub := &stubTrash{}
	h := trashHandler(stub)
	res, err := h.ArchiveLevel(context.Background(), connect.NewRequest(&configv1.ArchiveLevelRequest{
		BatchId: hBatch, LevelId: hLevel, DeletedBy: hUser, DeletedByName: "Laura",
		Summary: []*configv1.LevelArchiveCount{{Kind: "students", Count: 4}},
	}))
	require.NoError(t, err)
	assert.Equal(t, int64(4), stub.summary["students"])
	assert.Equal(t, hBatch, res.Msg.GetDeletion().GetBatchId())
	assert.Equal(t, "deleted", res.Msg.GetDeletion().GetStatus())
	assert.True(t, res.Msg.GetDeletion().GetRestorableUntil().AsTime().Equal(hAt.Add(levelarchive.Retention)))
}

func TestRestoreLevelBatchWire(t *testing.T) {
	res, err := trashHandler(&stubTrash{}).RestoreLevelBatch(context.Background(),
		connect.NewRequest(&configv1.RestoreLevelBatchRequest{BatchId: hBatch}))
	require.NoError(t, err)
	require.Len(t, res.Msg.GetSkipped(), 1)
	assert.Equal(t, int64(2), res.Msg.GetSkipped()[0].GetCount())
	assert.Equal(t, "choca", res.Msg.GetSkipped()[0].GetReason())
}

func TestRestoreLevelBatchForwardsTheRefusalCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code connect.Code
		meta string
	}{
		{"name taken", domain.LevelNameTaken(), connect.CodeAlreadyExists, "LEVEL_NAME_TAKEN"},
		{"not in trash", glerrors.ErrNotFound, connect.CodeNotFound, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := trashHandler(&stubTrash{restoreErr: tc.err}).RestoreLevelBatch(context.Background(),
				connect.NewRequest(&configv1.RestoreLevelBatchRequest{BatchId: hBatch}))
			assert.Equal(t, tc.code, connect.CodeOf(err))
			var ce *connect.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, tc.meta, ce.Meta().Get(grpcadapter.ErrorCodeMeta))
		})
	}
}

func TestListAndGetLevelDeletionsWire(t *testing.T) {
	h := trashHandler(&stubTrash{})
	list, err := h.ListLevelDeletions(context.Background(), connect.NewRequest(&configv1.ListLevelDeletionsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetDeletions(), 1)
	assert.Equal(t, "Laura", list.Msg.GetDeletions()[0].GetDeletedByName())

	_, err = h.GetLevelDeletion(context.Background(), connect.NewRequest(&configv1.GetLevelDeletionRequest{BatchId: hBatch}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestPurgeLevelArchivesWire(t *testing.T) {
	res, err := trashHandler(&stubTrash{}).PurgeLevelArchives(context.Background(), connect.NewRequest(&configv1.PurgeLevelArchivesRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int64(7), res.Msg.GetPurgedRows())
}

func TestMarkLevelDeletionRestoredWire(t *testing.T) {
	res, err := trashHandler(&stubTrash{}).MarkLevelDeletionRestored(context.Background(),
		connect.NewRequest(&configv1.MarkLevelDeletionRestoredRequest{BatchId: hBatch}))
	require.NoError(t, err)
	assert.Equal(t, "restored", res.Msg.GetDeletion().GetStatus())
}
