package service

import (
	"context"
	"time"

	"github.com/garcia-lorca/pkg/levelarchive"
)

// LevelArchiveRepo is attendance's half of the level-deletion trash (ADR 0021):
// it files the scope's marks and pass lists in the archive and puts them back.
// Every method runs in ONE tenant-bound transaction; archive and restore are
// idempotent per batch.
type LevelArchiveRepo interface {
	PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (levelarchive.Counts, error)
	ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error)
	RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error)
	PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error)
}

// LevelArchiveService validates a level deletion's ids at the boundary and
// hands the work to the repository. The gateway's saga drives it.
type LevelArchiveService struct {
	repo LevelArchiveRepo
	now  func() time.Time
}

// NewLevelArchiveService wires the service; now is injectable for tests.
func NewLevelArchiveService(repo LevelArchiveRepo, now func() time.Time) *LevelArchiveService {
	return &LevelArchiveService{repo: repo, now: now}
}

// PreviewLevelDeletion counts what the scope would archive (KindInvalid on a
// malformed id).
func (s *LevelArchiveService) PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (levelarchive.Counts, error) {
	norm, err := scope.Normalize()
	if err != nil {
		return nil, err
	}
	return s.repo.PreviewLevelDeletion(ctx, norm)
}

// ArchiveLevel archives the scope under batchID.
func (s *LevelArchiveService) ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return nil, err
	}
	norm, err := scope.Normalize()
	if err != nil {
		return nil, err
	}
	return s.repo.ArchiveLevel(ctx, batch, norm)
}

// RestoreLevelBatch puts a batch back and reports what it skipped.
func (s *LevelArchiveService) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return levelarchive.Report{}, err
	}
	return s.repo.RestoreLevelBatch(ctx, batch)
}

// PurgeLevelArchives drops archives past the retention.
func (s *LevelArchiveService) PurgeLevelArchives(ctx context.Context) (int64, error) {
	return s.repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(s.now()))
}
