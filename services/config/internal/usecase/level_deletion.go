package usecase

import (
	"context"
	"time"

	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// LevelDeletionService is config's part of deleting a level (ADR 0021): it
// resolves the level's structure for the gateway's saga, moves that structure
// into the trash as the saga's last step, brings it back first on a restore,
// and keeps the trash's ledger. Ids are canonicalized here (uuid, lowercase)
// before anything compares them; instants come from the injected clock.
type LevelDeletionService struct {
	repo ports.LevelDeletionRepository
	now  func() time.Time
}

// NewLevelDeletionService wires the use case.
func NewLevelDeletionService(repo ports.LevelDeletionRepository, now func() time.Time) *LevelDeletionService {
	return &LevelDeletionService{repo: repo, now: now}
}

// ArchiveLevelCommand is the saga's config step. DeletedBy/DeletedByName are
// the verified principal; Summary is what the other services archived.
type ArchiveLevelCommand struct {
	BatchID       string
	LevelID       string
	DeletedBy     string
	DeletedByName string
	Summary       levelarchive.Counts
}

// PreviewLevelDeletion resolves a level's structure and counts.
func (s *LevelDeletionService) PreviewLevelDeletion(ctx context.Context, levelID string) (ports.LevelStructure, error) {
	id, err := levelarchive.NormalizeID(levelID, "level id")
	if err != nil {
		return ports.LevelStructure{}, err
	}
	return s.repo.PreviewLevelDeletion(ctx, id)
}

// ArchiveLevel moves the level's structure into the trash and records the
// batch, stamped with this service's clock.
func (s *LevelDeletionService) ArchiveLevel(ctx context.Context, cmd ArchiveLevelCommand) (levelarchive.Counts, domain.LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(cmd.BatchID)
	if err != nil {
		return nil, domain.LevelDeletion{}, err
	}
	level, err := levelarchive.NormalizeID(cmd.LevelID, "level id")
	if err != nil {
		return nil, domain.LevelDeletion{}, err
	}
	return s.repo.ArchiveLevel(ctx, ports.ArchiveLevelInput{
		BatchID: batch, LevelID: level, DeletedBy: cmd.DeletedBy, DeletedByName: cmd.DeletedByName,
		Summary: cmd.Summary, At: s.now().UTC(),
	})
}

// RestoreLevelBatch puts a batch's structure back.
func (s *LevelDeletionService) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return levelarchive.Report{}, err
	}
	return s.repo.RestoreLevelBatch(ctx, batch, s.now().UTC())
}

// MarkLevelDeletionRestored closes a fully restored batch.
func (s *LevelDeletionService) MarkLevelDeletionRestored(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return domain.LevelDeletion{}, err
	}
	return s.repo.MarkLevelDeletionRestored(ctx, batch)
}

// ListLevelDeletions is the trash.
func (s *LevelDeletionService) ListLevelDeletions(ctx context.Context) ([]domain.LevelDeletion, error) {
	return s.repo.ListLevelDeletions(ctx)
}

// GetLevelDeletion fetches one batch.
func (s *LevelDeletionService) GetLevelDeletion(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return domain.LevelDeletion{}, err
	}
	return s.repo.GetLevelDeletion(ctx, batch)
}

// PurgeLevelArchives drops what is past the retention.
func (s *LevelDeletionService) PurgeLevelArchives(ctx context.Context) (int64, error) {
	return s.repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(s.now().UTC()))
}
