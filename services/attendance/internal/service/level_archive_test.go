package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

type fakeLevelArchiveRepo struct {
	scope  levelarchive.Scope
	batch  string
	cutoff time.Time
}

func (f *fakeLevelArchiveRepo) PreviewLevelDeletion(_ context.Context, s levelarchive.Scope) (levelarchive.Counts, error) {
	f.scope = s
	return levelarchive.Counts{"attendanceMarks": 1}, nil
}

func (f *fakeLevelArchiveRepo) ArchiveLevel(_ context.Context, b string, s levelarchive.Scope) (levelarchive.Counts, error) {
	f.batch, f.scope = b, s
	return levelarchive.Counts{"attendanceMarks": 1}, nil
}

func (f *fakeLevelArchiveRepo) RestoreLevelBatch(_ context.Context, b string) (levelarchive.Report, error) {
	f.batch = b
	return levelarchive.NewReport(), nil
}

func (f *fakeLevelArchiveRepo) PurgeLevelArchives(_ context.Context, c time.Time) (int64, error) {
	f.cutoff = c
	return 3, nil
}

const (
	archLevel = "aaaaaaaa-0000-0000-0000-000000000001"
	archGroup = "aaaaaaaa-0000-0000-0000-000000000002"
	archBatch = "aaaaaaaa-0000-0000-0000-000000000003"
)

func TestLevelArchiveServiceValidatesAtTheBoundary(t *testing.T) {
	cases := []struct {
		name    string
		batch   string
		scope   levelarchive.Scope
		wantErr bool
	}{
		{"valid", archBatch, levelarchive.Scope{LevelID: archLevel, GroupIDs: []string{archGroup}}, false},
		{"uppercase ids are canonicalized", "AAAAAAAA-0000-0000-0000-000000000003", levelarchive.Scope{LevelID: "AAAAAAAA-0000-0000-0000-000000000001"}, false},
		{"bad batch", "x", levelarchive.Scope{LevelID: archLevel}, true},
		{"missing level", archBatch, levelarchive.Scope{}, true},
		{"bad member", archBatch, levelarchive.Scope{LevelID: archLevel, EnrollmentIDs: []string{"nope"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeLevelArchiveRepo{}
			svc := service.NewLevelArchiveService(repo, time.Now)
			_, err := svc.ArchiveLevel(context.Background(), tc.batch, tc.scope)
			if tc.wantErr {
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
				assert.Empty(t, repo.batch, "nothing reaches the repository")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, archBatch, repo.batch)
			assert.Equal(t, archLevel, repo.scope.LevelID)
		})
	}
}

func TestLevelArchiveServicePreviewAndRestoreValidate(t *testing.T) {
	svc := service.NewLevelArchiveService(&fakeLevelArchiveRepo{}, time.Now)
	_, err := svc.PreviewLevelDeletion(context.Background(), levelarchive.Scope{LevelID: "kinder"})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	_, err = svc.RestoreLevelBatch(context.Background(), "x")
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))

	got, err := svc.PreviewLevelDeletion(context.Background(), levelarchive.Scope{LevelID: archLevel})
	require.NoError(t, err)
	assert.Equal(t, int64(1), got["attendanceMarks"])
}

func TestLevelArchiveServicePurgesPastTheRetention(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	repo := &fakeLevelArchiveRepo{}
	svc := service.NewLevelArchiveService(repo, func() time.Time { return now })
	n, err := svc.PurgeLevelArchives(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	assert.Equal(t, now.Add(-7*24*time.Hour), repo.cutoff)
}
