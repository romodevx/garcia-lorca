package usecase

import (
	"context"
	"time"

	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// LevelArchiveService is grades' step of a level deletion (ADR 0021): it
// checks the scope and batch id at the boundary and hands them to the
// archive. The saga itself lives in the gateway.
type LevelArchiveService struct {
	repo ports.LevelArchiveRepository
	now  func() time.Time
}

// NewLevelArchiveService wires the use case; now is injectable for the purge.
func NewLevelArchiveService(repo ports.LevelArchiveRepository, now func() time.Time) *LevelArchiveService {
	return &LevelArchiveService{repo: repo, now: now}
}

// PreviewLevelDeletion counts what deleting the level would archive here.
func (s *LevelArchiveService) PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (levelarchive.Counts, error) {
	scope, err := scope.Normalize()
	if err != nil {
		return nil, err
	}
	return s.repo.PreviewLevelDeletion(ctx, scope)
}

// ArchiveLevel moves the level's grades into the trash under batchID.
func (s *LevelArchiveService) ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return nil, err
	}
	scope, err = scope.Normalize()
	if err != nil {
		return nil, err
	}
	return s.repo.ArchiveLevel(ctx, batch, scope)
}

// RestoreLevelBatch puts a batch back, reporting what it skipped. Grades of
// skipEnrollmentIDs (enrollments students could not restore) stay archived.
func (s *LevelArchiveService) RestoreLevelBatch(ctx context.Context, batchID string, skipEnrollmentIDs []string) (levelarchive.Report, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return levelarchive.Report{}, err
	}
	skip, err := levelarchive.NormalizeIDs(skipEnrollmentIDs, "skipped enrollment ids")
	if err != nil {
		return levelarchive.Report{}, err
	}
	return s.repo.RestoreLevelBatch(ctx, batch, skip)
}

// PurgeLevelBatch drops one batch's archive (the gateway's retention sweep, on
// config's clock).
func (s *LevelArchiveService) PurgeLevelBatch(ctx context.Context, batchID string) (int64, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return 0, err
	}
	return s.repo.PurgeLevelBatch(ctx, batch)
}

// PurgeLevelArchives drops the archives past the retention.
func (s *LevelArchiveService) PurgeLevelArchives(ctx context.Context) (int64, error) {
	return s.repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(s.now()))
}
