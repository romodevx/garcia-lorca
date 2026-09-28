package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

type fakeLevelArchive struct {
	scope  levelarchive.Scope
	batch  string
	cutoff time.Time
}

func (f *fakeLevelArchive) PreviewLevelDeletion(_ context.Context, s levelarchive.Scope) (ports.LevelPeople, error) {
	f.scope = s
	return ports.LevelPeople{Counts: levelarchive.Counts{"students": 1}}, nil
}
func (f *fakeLevelArchive) ArchiveLevel(_ context.Context, b string, s levelarchive.Scope) (levelarchive.Counts, error) {
	f.batch, f.scope = b, s
	return levelarchive.Counts{"enrollments": 2}, nil
}
func (f *fakeLevelArchive) RestoreLevelBatch(_ context.Context, b string) (levelarchive.Report, error) {
	f.batch = b
	return levelarchive.NewReport(), nil
}
func (f *fakeLevelArchive) PurgeLevelArchives(_ context.Context, c time.Time) (int64, error) {
	f.cutoff = c
	return 3, nil
}

const (
	lvlID   = "AAAAAAAA-0000-0000-0000-000000000001"
	batchID = "bbbbbbbb-0000-0000-0000-000000000001"
)

func TestLevelArchiveServiceNormalizesAndRefuses(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		call    func(*usecase.LevelArchiveService) error
		wantErr bool
	}{
		{"preview ok", func(s *usecase.LevelArchiveService) error {
			_, err := s.PreviewLevelDeletion(context.Background(), levelarchive.Scope{LevelID: lvlID})
			return err
		}, false},
		{"preview bad level", func(s *usecase.LevelArchiveService) error {
			_, err := s.PreviewLevelDeletion(context.Background(), levelarchive.Scope{LevelID: "kinder"})
			return err
		}, true},
		{"archive ok", func(s *usecase.LevelArchiveService) error {
			_, err := s.ArchiveLevel(context.Background(), batchID, levelarchive.Scope{LevelID: lvlID})
			return err
		}, false},
		{"archive bad batch", func(s *usecase.LevelArchiveService) error {
			_, err := s.ArchiveLevel(context.Background(), "x", levelarchive.Scope{LevelID: lvlID})
			return err
		}, true},
		{"archive bad group", func(s *usecase.LevelArchiveService) error {
			_, err := s.ArchiveLevel(context.Background(), batchID, levelarchive.Scope{LevelID: lvlID, GroupIDs: []string{"g"}})
			return err
		}, true},
		{"restore bad batch", func(s *usecase.LevelArchiveService) error {
			_, err := s.RestoreLevelBatch(context.Background(), "")
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
			require.NoError(t, err)
		})
	}
}

func TestLevelArchiveServicePassesCanonicalIDs(t *testing.T) {
	repo := &fakeLevelArchive{}
	svc := usecase.NewLevelArchiveService(repo, time.Now)
	_, err := svc.ArchiveLevel(context.Background(), " BBBBBBBB-0000-0000-0000-000000000001", levelarchive.Scope{LevelID: lvlID})
	require.NoError(t, err)
	assert.Equal(t, batchID, repo.batch)
	assert.Equal(t, "aaaaaaaa-0000-0000-0000-000000000001", repo.scope.LevelID)
}

func TestLevelArchiveServicePurgesPastTheRetention(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	repo := &fakeLevelArchive{}
	n, err := usecase.NewLevelArchiveService(repo, func() time.Time { return now }).PurgeLevelArchives(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	assert.Equal(t, now.Add(-7*24*time.Hour), repo.cutoff)
}
