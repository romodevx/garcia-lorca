package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// fakeTrash records what the use case hands the repository.
type fakeTrash struct {
	previewed   string
	begun       ports.BeginLevelDeletionInput
	archived    ports.ArchiveLevelInput
	restoredID  string
	partial     bool
	failed      []string
	failSkips   []levelarchive.Skip
	stallAfter  time.Duration
	purgedBatch string
	purgeCutoff time.Time
	restoreRun  []string
}

func (f *fakeTrash) BeginLevelRestore(_ context.Context, batchID string) (domain.LevelDeletion, error) {
	f.restoreRun = append(f.restoreRun, "begin "+batchID)
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionRestoring}, nil
}

func (f *fakeTrash) AbandonLevelRestore(_ context.Context, batchID string) (domain.LevelDeletion, error) {
	f.restoreRun = append(f.restoreRun, "abandon "+batchID)
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionDeleted}, nil
}

func (f *fakeTrash) PreviewLevelDeletion(_ context.Context, levelID string) (ports.LevelStructure, error) {
	f.previewed = levelID
	return ports.LevelStructure{LevelID: levelID, LevelName: "Primaria", Counts: levelarchive.Counts{"groups": 1}}, nil
}

func (f *fakeTrash) BeginLevelDeletion(_ context.Context, in ports.BeginLevelDeletionInput) (domain.LevelDeletion, error) {
	f.begun = in
	return domain.LevelDeletion{BatchID: in.BatchID, Status: domain.LevelDeletionDeleting}, nil
}

func (f *fakeTrash) ArchiveLevel(_ context.Context, in ports.ArchiveLevelInput) (levelarchive.Counts, domain.LevelDeletion, error) {
	f.archived = in
	return levelarchive.Counts{"groups": 1}, domain.LevelDeletion{BatchID: in.BatchID}, nil
}

func (f *fakeTrash) RestoreLevelBatch(_ context.Context, batchID string) (levelarchive.Report, error) {
	f.restoredID = batchID
	return levelarchive.NewReport(), nil
}

func (f *fakeTrash) MarkLevelDeletionRestored(_ context.Context, batchID string, partial bool) (domain.LevelDeletion, error) {
	f.partial = partial
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionRestored}, nil
}

func (f *fakeTrash) CancelLevelDeletion(_ context.Context, batchID string) (domain.LevelDeletion, error) {
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionCancelled}, nil
}

func (f *fakeTrash) FailLevelDeletion(_ context.Context, batchID string, pending []string, skipped []levelarchive.Skip) (domain.LevelDeletion, error) {
	f.failed, f.failSkips = pending, skipped
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionFailedPartial}, nil
}

func (f *fakeTrash) SweepLevelDeletions(_ context.Context, stalledAfter time.Duration) ([]domain.LevelDeletion, int64, error) {
	f.stallAfter = stalledAfter
	return []domain.LevelDeletion{{BatchID: "b"}}, 1, nil
}

func (f *fakeTrash) MarkLevelDeletionPurged(_ context.Context, batchID string) (domain.LevelDeletion, error) {
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionPurged}, nil
}

func (f *fakeTrash) PurgeLevelBatch(_ context.Context, batchID string) (int64, error) {
	f.purgedBatch = batchID
	return 5, nil
}

func (f *fakeTrash) ListLevelDeletions(context.Context) ([]domain.LevelDeletion, error) {
	return []domain.LevelDeletion{{BatchID: "b"}}, nil
}

func (f *fakeTrash) GetLevelDeletion(_ context.Context, batchID string) (domain.LevelDeletion, error) {
	return domain.LevelDeletion{BatchID: batchID}, nil
}

func (f *fakeTrash) PurgeLevelArchives(_ context.Context, cutoff time.Time) (int64, error) {
	f.purgeCutoff = cutoff
	return 3, nil
}

var trashNow = time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)

func newTrash() (*usecase.LevelDeletionService, *fakeTrash) {
	f := &fakeTrash{}
	return usecase.NewLevelDeletionService(f, func() time.Time { return trashNow }), f
}

const (
	okBatch = "aaaaaaaa-0000-0000-0000-00000000000b"
	okLevel = "aaaaaaaa-0000-0000-0000-00000000000c"
	okUser  = "aaaaaaaa-0000-0000-0000-00000000000d"
	okHash  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestLevelDeletionServiceRefusesMalformedIDs(t *testing.T) {
	svc, _ := newTrash()
	ctx := context.Background()
	begin := func(c usecase.BeginLevelDeletionCommand) func() error {
		return func() error { _, err := svc.BeginLevelDeletion(ctx, c); return err }
	}
	cases := []struct {
		name string
		call func() error
	}{
		{"preview", func() error { _, err := svc.PreviewLevelDeletion(ctx, "kinder"); return err }},
		{"begin batch", begin(usecase.BeginLevelDeletionCommand{BatchID: "x", LevelID: okLevel, TokenHash: okHash, DeletedBy: okUser, DeletedByName: "L"})},
		{"begin level", begin(usecase.BeginLevelDeletionCommand{BatchID: okBatch, LevelID: "x", TokenHash: okHash, DeletedBy: okUser, DeletedByName: "L"})},
		{"begin token", begin(usecase.BeginLevelDeletionCommand{BatchID: okBatch, LevelID: okLevel, TokenHash: "nope", DeletedBy: okUser, DeletedByName: "L"})},
		{"archive batch", func() error {
			_, _, err := svc.ArchiveLevel(ctx, usecase.ArchiveLevelCommand{BatchID: "x", LevelID: okLevel})
			return err
		}},
		{"archive level", func() error {
			_, _, err := svc.ArchiveLevel(ctx, usecase.ArchiveLevelCommand{BatchID: okBatch, LevelID: "x"})
			return err
		}},
		{"restore", func() error { _, err := svc.RestoreLevelBatch(ctx, "x"); return err }},
		{"mark", func() error { _, err := svc.MarkLevelDeletionRestored(ctx, "x", false); return err }},
		{"cancel", func() error { _, err := svc.CancelLevelDeletion(ctx, "x"); return err }},
		{"fail", func() error { _, err := svc.FailLevelDeletion(ctx, "x", nil, nil); return err }},
		{"fail step", func() error { _, err := svc.FailLevelDeletion(ctx, okBatch, []string{"payroll"}, nil); return err }},
		{"purged", func() error { _, err := svc.MarkLevelDeletionPurged(ctx, "x"); return err }},
		{"purge batch", func() error { _, err := svc.PurgeLevelBatch(ctx, "x"); return err }},
		{"get", func() error { _, err := svc.GetLevelDeletion(ctx, "x"); return err }},
		{"restore start", func() error { _, err := svc.BeginLevelRestore(ctx, "x"); return err }},
		{"restore abandon", func() error { _, err := svc.AbandonLevelRestore(ctx, "x"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(tc.call()))
		})
	}
}

func TestLevelDeletionServiceCanonicalizes(t *testing.T) {
	svc, f := newTrash()
	ctx := context.Background()

	_, err := svc.PreviewLevelDeletion(ctx, " AAAAAAAA-0000-0000-0000-00000000000C ")
	require.NoError(t, err)
	assert.Equal(t, okLevel, f.previewed)

	_, err = svc.BeginLevelDeletion(ctx, usecase.BeginLevelDeletionCommand{
		BatchID: "AAAAAAAA-0000-0000-0000-00000000000B", LevelID: okLevel, TokenHash: okHash,
		DeletedBy: okUser, DeletedByName: "Laura",
	})
	require.NoError(t, err)
	assert.Equal(t, okBatch, f.begun.BatchID)
	assert.Equal(t, okHash, f.begun.TokenHash)

	_, _, err = svc.ArchiveLevel(ctx, usecase.ArchiveLevelCommand{
		BatchID: okBatch, LevelID: okLevel, Summary: levelarchive.Counts{"students": 2},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), f.archived.Summary["students"])

	_, err = svc.RestoreLevelBatch(ctx, okBatch)
	require.NoError(t, err)
	assert.Equal(t, okBatch, f.restoredID)

	_, err = svc.MarkLevelDeletionRestored(ctx, okBatch, true)
	require.NoError(t, err)
	assert.True(t, f.partial)

	skips := []levelarchive.Skip{{Kind: "enrollments", Count: 1, Reason: "r"}}
	_, err = svc.FailLevelDeletion(ctx, okBatch, []string{"students", "grades"}, skips)
	require.NoError(t, err)
	assert.Equal(t, []string{"students", "grades"}, f.failed)
	assert.Equal(t, skips, f.failSkips)

	got, stalled, err := svc.SweepLevelDeletions(ctx)
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, int64(1), stalled)
	assert.Equal(t, domain.SagaStallAfter, f.stallAfter)

	n, err := svc.PurgeLevelBatch(ctx, okBatch)
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)
	assert.Equal(t, okBatch, f.purgedBatch)

	n, err = svc.PurgeLevelArchives(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	assert.Equal(t, trashNow.Add(-levelarchive.Retention), f.purgeCutoff)
}

// Confirmation review (gap 4): the restore's start and fall-back canonicalize
// the batch id before the repository compares it.
func TestLevelRestoreStartAndAbandonCanonicalize(t *testing.T) {
	svc, f := newTrash()
	ctx := context.Background()
	_, err := svc.BeginLevelRestore(ctx, " AAAAAAAA-0000-0000-0000-00000000000B")
	require.NoError(t, err)
	_, err = svc.AbandonLevelRestore(ctx, "AAAAAAAA-0000-0000-0000-00000000000B")
	require.NoError(t, err)
	assert.Equal(t, []string{"begin " + okBatch, "abandon " + okBatch}, f.restoreRun)
}
