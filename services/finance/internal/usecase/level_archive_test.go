package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

const (
	laLevel = "aaaaaaaa-0000-0000-0000-000000000001"
	laBatch = "bbbbbbbb-0000-0000-0000-000000000001"
)

// fakeLevelArchive records what the use case hands the repository.
type fakeLevelArchive struct {
	scope  levelarchive.Scope
	batch  string
	cutoff time.Time
}

func (f *fakeLevelArchive) PreviewLevelDeletion(_ context.Context, s levelarchive.Scope) (levelarchive.Counts, error) {
	f.scope = s
	return levelarchive.Counts{"tuitionPlans": 3}, nil
}

func (f *fakeLevelArchive) ArchiveLevel(_ context.Context, batch string, s levelarchive.Scope) (levelarchive.Counts, error) {
	f.batch, f.scope = batch, s
	return levelarchive.Counts{"tuitionPlans": 3}, nil
}

func (f *fakeLevelArchive) RestoreLevelBatch(_ context.Context, batch string) (levelarchive.Report, error) {
	f.batch = batch
	return levelarchive.NewReport(), nil
}

func (f *fakeLevelArchive) PurgeLevelArchives(_ context.Context, cutoff time.Time) (int64, error) {
	f.cutoff = cutoff
	return 7, nil
}

func TestLevelArchiveServiceValidatesAtTheBoundary(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		call    func(*usecase.LevelArchiveService) error
		wantErr bool
	}{
		{"preview canonical scope", func(s *usecase.LevelArchiveService) error {
			_, err := s.PreviewLevelDeletion(context.Background(), levelarchive.Scope{LevelID: laLevel})
			return err
		}, false},
		{"preview malformed level", func(s *usecase.LevelArchiveService) error {
			_, err := s.PreviewLevelDeletion(context.Background(), levelarchive.Scope{LevelID: "kinder"})
			return err
		}, true},
		{"archive malformed batch", func(s *usecase.LevelArchiveService) error {
			_, err := s.ArchiveLevel(context.Background(), "nope", levelarchive.Scope{LevelID: laLevel})
			return err
		}, true},
		{"archive malformed enrollment", func(s *usecase.LevelArchiveService) error {
			_, err := s.ArchiveLevel(context.Background(), laBatch, levelarchive.Scope{LevelID: laLevel, EnrollmentIDs: []string{"x"}})
			return err
		}, true},
		{"restore malformed batch", func(s *usecase.LevelArchiveService) error {
			_, err := s.RestoreLevelBatch(context.Background(), "nope")
			return err
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := usecase.NewLevelArchiveService(&fakeLevelArchive{}, func() time.Time { return now })
			err := tc.call(svc)
			if tc.wantErr {
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestLevelArchiveServiceCanonicalizesIDs(t *testing.T) {
	repo := &fakeLevelArchive{}
	svc := usecase.NewLevelArchiveService(repo, time.Now)
	_, err := svc.ArchiveLevel(context.Background(), " BBBBBBBB-0000-0000-0000-000000000001",
		levelarchive.Scope{LevelID: "AAAAAAAA-0000-0000-0000-000000000001"})
	require.NoError(t, err)
	assert.Equal(t, laBatch, repo.batch)
	assert.Equal(t, laLevel, repo.scope.LevelID)
}

func TestLevelArchiveServicePurgesPastTheRetention(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	repo := &fakeLevelArchive{}
	n, err := usecase.NewLevelArchiveService(repo, func() time.Time { return now }).PurgeLevelArchives(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(7), n)
	assert.Equal(t, now.Add(-7*24*time.Hour), repo.cutoff)
}
