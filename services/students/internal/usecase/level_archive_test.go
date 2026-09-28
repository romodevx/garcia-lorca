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
	purged string
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
func (f *fakeLevelArchive) RestoreLevelBatch(_ context.Context, b string) (ports.LevelRestore, error) {
	f.batch = b
	return ports.LevelRestore{Report: levelarchive.NewReport(), UnrestoredEnrollmentIDs: []string{"e1"}}, nil
}
func (f *fakeLevelArchive) PurgeLevelBatch(_ context.Context, b string) (int64, error) {
	f.purged = b
	return 5, nil
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

func TestLevelArchiveServicePurgesOneBatch(t *testing.T) {
	repo := &fakeLevelArchive{}
	svc := usecase.NewLevelArchiveService(repo, time.Now)
	n, err := svc.PurgeLevelBatch(context.Background(), "BBBBBBBB-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)
	assert.Equal(t, batchID, repo.purged, "canonicalized")
	_, err = svc.PurgeLevelBatch(context.Background(), "nope")
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Equal(t, batchID, repo.purged, "a malformed id never reaches the repository")
}

func TestLevelArchiveServiceRestoreCarriesUnrestoredEnrollments(t *testing.T) {
	res, err := usecase.NewLevelArchiveService(&fakeLevelArchive{}, time.Now).RestoreLevelBatch(context.Background(), batchID)
	require.NoError(t, err)
	assert.Equal(t, []string{"e1"}, res.UnrestoredEnrollmentIDs)
}
