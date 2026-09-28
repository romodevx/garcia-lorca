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
	archived    ports.ArchiveLevelInput
	restoredAt  time.Time
	restoredID  string
	purgeCutoff time.Time
}

func (f *fakeTrash) PreviewLevelDeletion(_ context.Context, levelID string) (ports.LevelStructure, error) {
	f.previewed = levelID
	return ports.LevelStructure{LevelID: levelID, LevelName: "Primaria", Counts: levelarchive.Counts{"groups": 1}}, nil
}

func (f *fakeTrash) ArchiveLevel(_ context.Context, in ports.ArchiveLevelInput) (levelarchive.Counts, domain.LevelDeletion, error) {
	f.archived = in
	return levelarchive.Counts{"groups": 1}, domain.LevelDeletion{BatchID: in.BatchID}, nil
}

func (f *fakeTrash) RestoreLevelBatch(_ context.Context, batchID string, now time.Time) (levelarchive.Report, error) {
	f.restoredID, f.restoredAt = batchID, now
	return levelarchive.NewReport(), nil
}

func (f *fakeTrash) MarkLevelDeletionRestored(_ context.Context, batchID string) (domain.LevelDeletion, error) {
	return domain.LevelDeletion{BatchID: batchID, Status: domain.LevelDeletionRestored}, nil
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
)

func TestLevelDeletionServiceRefusesMalformedIDs(t *testing.T) {
	svc, _ := newTrash()
	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
	}{
		{"preview", func() error { _, err := svc.PreviewLevelDeletion(ctx, "kinder"); return err }},
		{"archive batch", func() error {
			_, _, err := svc.ArchiveLevel(ctx, usecase.ArchiveLevelCommand{BatchID: "x", LevelID: okLevel, DeletedBy: okUser, DeletedByName: "L"})
			return err
		}},
		{"archive level", func() error {
			_, _, err := svc.ArchiveLevel(ctx, usecase.ArchiveLevelCommand{BatchID: okBatch, LevelID: "x", DeletedBy: okUser, DeletedByName: "L"})
			return err
		}},
		{"restore", func() error { _, err := svc.RestoreLevelBatch(ctx, "x"); return err }},
		{"mark", func() error { _, err := svc.MarkLevelDeletionRestored(ctx, "x"); return err }},
		{"get", func() error { _, err := svc.GetLevelDeletion(ctx, "x"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(tc.call()))
		})
	}
}

func TestLevelDeletionServiceCanonicalizesAndStampsTheClock(t *testing.T) {
	svc, f := newTrash()
	ctx := context.Background()

	_, err := svc.PreviewLevelDeletion(ctx, " AAAAAAAA-0000-0000-0000-00000000000C ")
	require.NoError(t, err)
	assert.Equal(t, okLevel, f.previewed)

	_, _, err = svc.ArchiveLevel(ctx, usecase.ArchiveLevelCommand{
		BatchID: okBatch, LevelID: okLevel, DeletedBy: okUser, DeletedByName: "Laura",
		Summary: levelarchive.Counts{"students": 2},
	})
	require.NoError(t, err)
	assert.Equal(t, trashNow, f.archived.At, "the deletion instant is the service's clock, never the caller's")
	assert.Equal(t, int64(2), f.archived.Summary["students"])

	_, err = svc.RestoreLevelBatch(ctx, okBatch)
	require.NoError(t, err)
	assert.Equal(t, trashNow, f.restoredAt)

	n, err := svc.PurgeLevelArchives(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	assert.Equal(t, trashNow.Add(-levelarchive.Retention), f.purgeCutoff, "only archives past the 7-day retention")
}
