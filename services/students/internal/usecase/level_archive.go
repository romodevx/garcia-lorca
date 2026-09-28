package usecase

import (
	"context"
	"time"

	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// LevelArchiveService is students' step of a level deletion (ADR 0021): it
// archives the enrollments in the level's groups, clears the level of the
// pupils placed in it (they are kept, "sin nivel, grado ni grupo"), restores
// both, and purges the trash past its retention. It canonicalizes every id
// before the repository sees it; malformed input is KindInvalid.
type LevelArchiveService struct {
	repo ports.LevelArchiveRepository
	now  func() time.Time
}

// NewLevelArchiveService wires the use case; now is injectable for tests.
func NewLevelArchiveService(repo ports.LevelArchiveRepository, now func() time.Time) *LevelArchiveService {
	return &LevelArchiveService{repo: repo, now: now}
}

// PreviewLevelDeletion counts what the deletion would reach, changing nothing.
func (s *LevelArchiveService) PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (ports.LevelPeople, error) {
	sc, err := scope.Normalize()
	if err != nil {
		return ports.LevelPeople{}, err
	}
	return s.repo.PreviewLevelDeletion(ctx, sc)
}

// ArchiveLevel runs the step once per batch.
func (s *LevelArchiveService) ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return nil, err
	}
	sc, err := scope.Normalize()
	if err != nil {
		return nil, err
	}
	return s.repo.ArchiveLevel(ctx, batch, sc)
}

// RestoreLevelBatch puts a batch back, reporting what it skipped.
func (s *LevelArchiveService) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return levelarchive.Report{}, err
	}
	return s.repo.RestoreLevelBatch(ctx, batch)
}

// PurgeLevelArchives drops the trash past levelarchive.Retention.
func (s *LevelArchiveService) PurgeLevelArchives(ctx context.Context) (int64, error) {
	return s.repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(s.now()))
}
