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

// stubTrash answers fixed results, or restoreErr from a restore / beginErr
// from a begin.
type stubTrash struct {
	restoreErr error
	beginErr   error
	summary    levelarchive.Counts
	begun      ports.BeginLevelDeletionInput
	partial    bool
	pending    []string
	skipped    []levelarchive.Skip
}

func (s *stubTrash) PreviewLevelDeletion(_ context.Context, levelID string) (ports.LevelStructure, error) {
	return ports.LevelStructure{
		LevelID: levelID, LevelName: "Primaria", Deleting: true, GradeLevelIDs: []string{"g1"}, GroupIDs: []string{"gr1"},
		SubjectIDs: []string{"s1"}, TermIDs: []string{"t1"}, Counts: levelarchive.Counts{"groups": 1, "levels": 1},
	}, nil
}

func (s *stubTrash) BeginLevelDeletion(_ context.Context, in ports.BeginLevelDeletionInput) (domain.LevelDeletion, error) {
	s.begun = in
	if s.beginErr != nil {
		return domain.LevelDeletion{}, s.beginErr
	}
	d, err := domain.NewLevelDeletion(in.BatchID, in.LevelID, "Primaria", in.DeletedBy, in.DeletedByName, nil, hAt)
	d.Status = domain.LevelDeletionDeleting
	return d, err
}

func (s *stubTrash) ArchiveLevel(_ context.Context, in ports.ArchiveLevelInput) (levelarchive.Counts, domain.LevelDeletion, error) {
	s.summary = in.Summary
	d, err := domain.NewLevelDeletion(in.BatchID, in.LevelID, "Primaria", hUser, "Laura", levelarchive.Counts{"groups": 1}, hAt)
	return levelarchive.Counts{"groups": 1}, d, err
}

func (s *stubTrash) RestoreLevelBatch(context.Context, string) (levelarchive.Report, error) {
	if s.restoreErr != nil {
		return levelarchive.Report{}, s.restoreErr
	}
	rep := levelarchive.NewReport()
	rep.Record("groups", 1, 2, "choca")
	return rep, nil
}

func (s *stubTrash) MarkLevelDeletionRestored(_ context.Context, batchID string, partial bool) (domain.LevelDeletion, error) {
	s.partial = partial
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionRestored, DeletedAt: hAt, RestorableUntil: hAt}, nil
}

func (s *stubTrash) CancelLevelDeletion(_ context.Context, batchID string) (domain.LevelDeletion, error) {
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionCancelled, DeletedAt: hAt, RestorableUntil: hAt}, nil
}

func (s *stubTrash) FailLevelDeletion(_ context.Context, batchID string, pending []string, skipped []levelarchive.Skip) (domain.LevelDeletion, error) {
	s.pending, s.skipped = pending, skipped
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionFailedPartial, PendingSteps: pending, Skipped: skipped,
		DeletedAt: hAt, RestorableUntil: hAt}, nil
}

func (s *stubTrash) SweepLevelDeletions(context.Context, time.Duration) ([]domain.LevelDeletion, int64, error) {
	return []domain.LevelDeletion{{BatchID: hBatch, Status: domain.LevelDeletionDeleted, DeletedAt: hAt, RestorableUntil: hAt}}, 2, nil
}

func (s *stubTrash) MarkLevelDeletionPurged(_ context.Context, batchID string) (domain.LevelDeletion, error) {
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionPurged, DeletedAt: hAt, RestorableUntil: hAt}, nil
}

func (s *stubTrash) PurgeLevelBatch(context.Context, string) (int64, error) { return 11, nil }

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

	assert.Equal(t, configv1.LevelStatus_LEVEL_STATUS_DELETING, res.Msg.GetLevelStatus())

	_, err = h.PreviewLevelDeletion(context.Background(), connect.NewRequest(&configv1.PreviewLevelDeletionRequest{LevelId: "kinder"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestArchiveLevelWireCarriesTheSummaryAndTheLedgerRow(t *testing.T) {
	stub := &stubTrash{}
	h := trashHandler(stub)
	res, err := h.ArchiveLevel(context.Background(), connect.NewRequest(&configv1.ArchiveLevelRequest{
		BatchId: hBatch, LevelId: hLevel,
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
	stub := &stubTrash{}
	res, err := trashHandler(stub).MarkLevelDeletionRestored(context.Background(),
		connect.NewRequest(&configv1.MarkLevelDeletionRestoredRequest{BatchId: hBatch, Partial: true}))
	require.NoError(t, err)
	assert.Equal(t, "restored", res.Msg.GetDeletion().GetStatus())
	assert.True(t, stub.partial)
}

const hHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestBeginLevelDeletionWire(t *testing.T) {
	stub := &stubTrash{}
	res, err := trashHandler(stub).BeginLevelDeletion(context.Background(), connect.NewRequest(&configv1.BeginLevelDeletionRequest{
		BatchId: hBatch, LevelId: hLevel, TokenHash: hHash, DeletedBy: hUser, DeletedByName: "Laura",
	}))
	require.NoError(t, err)
	assert.Equal(t, "deleting", res.Msg.GetDeletion().GetStatus())
	assert.Equal(t, hHash, stub.begun.TokenHash)
	assert.Equal(t, "Laura", stub.begun.DeletedByName)
}

func TestBeginLevelDeletionForwardsItsRefusals(t *testing.T) {
	cases := []struct {
		err  error
		meta string
	}{
		{domain.DeletionInProgress(), "DELETION_IN_PROGRESS"},
		{domain.PreviewTokenUsed(), "PREVIEW_STALE"},
	}
	for _, tc := range cases {
		t.Run(tc.meta, func(t *testing.T) {
			_, err := trashHandler(&stubTrash{beginErr: tc.err}).BeginLevelDeletion(context.Background(),
				connect.NewRequest(&configv1.BeginLevelDeletionRequest{BatchId: hBatch, LevelId: hLevel, TokenHash: hHash, DeletedBy: hUser, DeletedByName: "L"}))
			assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
			var ce *connect.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, tc.meta, ce.Meta().Get(grpcadapter.ErrorCodeMeta))
		})
	}
}

func TestLevelBeingDeletedCarriesItsCode(t *testing.T) {
	_, err := trashHandler(&stubTrash{restoreErr: levelarchive.LevelBeingDeleted()}).RestoreLevelBatch(context.Background(),
		connect.NewRequest(&configv1.RestoreLevelBatchRequest{BatchId: hBatch}))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeAlreadyExists, ce.Code())
	assert.Equal(t, "LEVEL_BEING_DELETED", ce.Meta().Get(grpcadapter.ErrorCodeMeta))
}

func TestSagaLedgerWire(t *testing.T) {
	stub := &stubTrash{}
	h := trashHandler(stub)
	ctx := context.Background()

	c, err := h.CancelLevelDeletion(ctx, connect.NewRequest(&configv1.CancelLevelDeletionRequest{BatchId: hBatch}))
	require.NoError(t, err)
	assert.Equal(t, "cancelled", c.Msg.GetDeletion().GetStatus())

	f, err := h.FailLevelDeletion(ctx, connect.NewRequest(&configv1.FailLevelDeletionRequest{
		BatchId: hBatch, PendingSteps: []string{"grades", "config"},
		Skipped: []*configv1.LevelRestoreSkip{{Kind: "enrollments", Count: 2, Reason: "r"}},
	}))
	require.NoError(t, err)
	assert.Equal(t, "failed_partial", f.Msg.GetDeletion().GetStatus())
	assert.Equal(t, []string{"grades", "config"}, f.Msg.GetDeletion().GetPendingSteps())
	require.Len(t, f.Msg.GetDeletion().GetSkipped(), 1)
	assert.Equal(t, []levelarchive.Skip{{Kind: "enrollments", Count: 2, Reason: "r"}}, stub.skipped)

	sw, err := h.SweepLevelDeletions(ctx, connect.NewRequest(&configv1.SweepLevelDeletionsRequest{}))
	require.NoError(t, err)
	assert.Len(t, sw.Msg.GetExpired(), 1)
	assert.Equal(t, int64(2), sw.Msg.GetStalled())

	p, err := h.PurgeLevelBatch(ctx, connect.NewRequest(&configv1.PurgeLevelBatchRequest{BatchId: hBatch}))
	require.NoError(t, err)
	assert.Equal(t, int64(11), p.Msg.GetPurgedRows())

	m, err := h.MarkLevelDeletionPurged(ctx, connect.NewRequest(&configv1.MarkLevelDeletionPurgedRequest{BatchId: hBatch}))
	require.NoError(t, err)
	assert.Equal(t, "purged", m.Msg.GetDeletion().GetStatus())
}
