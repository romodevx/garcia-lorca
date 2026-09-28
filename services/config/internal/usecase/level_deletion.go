package usecase

import (
	"context"
	"regexp"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// LevelDeletionService is config's part of deleting a level (ADR 0021): it
// resolves the level's structure for the gateway's saga, holds the saga's lock
// (BeginLevelDeletion, the first step), moves that structure into the trash as
// the saga's last step, brings it back first on a restore, and keeps the
// trash's ledger. Ids are canonicalized here (uuid, lowercase) before anything
// compares them. Every instant of the trash is the database's clock; the
// injected clock only serves the legacy age-based purge.
type LevelDeletionService struct {
	repo ports.LevelDeletionRepository
	now  func() time.Time
}

// NewLevelDeletionService wires the use case.
func NewLevelDeletionService(repo ports.LevelDeletionRepository, now func() time.Time) *LevelDeletionService {
	return &LevelDeletionService{repo: repo, now: now}
}

// BeginLevelDeletionCommand opens a saga. TokenHash is the SHA-256 (hex) of
// the confirm token; DeletedBy/DeletedByName are the verified principal.
type BeginLevelDeletionCommand struct {
	BatchID       string
	LevelID       string
	TokenHash     string
	DeletedBy     string
	DeletedByName string
}

// ArchiveLevelCommand is the saga's config step. Summary is what the other
// services archived.
type ArchiveLevelCommand struct {
	BatchID string
	LevelID string
	Summary levelarchive.Counts
}

var tokenHashShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

// PreviewLevelDeletion resolves a level's structure and counts.
func (s *LevelDeletionService) PreviewLevelDeletion(ctx context.Context, levelID string) (ports.LevelStructure, error) {
	id, err := levelarchive.NormalizeID(levelID, "level id")
	if err != nil {
		return ports.LevelStructure{}, err
	}
	return s.repo.PreviewLevelDeletion(ctx, id)
}

// BeginLevelDeletion freezes the level and records the running batch.
func (s *LevelDeletionService) BeginLevelDeletion(ctx context.Context, cmd BeginLevelDeletionCommand) (domain.LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(cmd.BatchID)
	if err != nil {
		return domain.LevelDeletion{}, err
	}
	level, err := levelarchive.NormalizeID(cmd.LevelID, "level id")
	if err != nil {
		return domain.LevelDeletion{}, err
	}
	if !tokenHashShape.MatchString(cmd.TokenHash) {
		return domain.LevelDeletion{}, glerrors.New(glerrors.KindInvalid, "token hash must be a hex sha-256")
	}
	return s.repo.BeginLevelDeletion(ctx, ports.BeginLevelDeletionInput{
		BatchID: batch, LevelID: level, TokenHash: cmd.TokenHash,
		DeletedBy: cmd.DeletedBy, DeletedByName: cmd.DeletedByName,
	})
}

// ArchiveLevel moves the level's structure into the trash and completes the
// begun batch.
func (s *LevelDeletionService) ArchiveLevel(ctx context.Context, cmd ArchiveLevelCommand) (levelarchive.Counts, domain.LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(cmd.BatchID)
	if err != nil {
		return nil, domain.LevelDeletion{}, err
	}
	level, err := levelarchive.NormalizeID(cmd.LevelID, "level id")
	if err != nil {
		return nil, domain.LevelDeletion{}, err
	}
	return s.repo.ArchiveLevel(ctx, ports.ArchiveLevelInput{BatchID: batch, LevelID: level, Summary: cmd.Summary})
}

// RestoreLevelBatch puts a batch's structure back.
func (s *LevelDeletionService) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return levelarchive.Report{}, err
	}
	return s.repo.RestoreLevelBatch(ctx, batch)
}

// MarkLevelDeletionRestored closes a restored batch (partial: some rows did
// not fit, it stays in the trash).
func (s *LevelDeletionService) MarkLevelDeletionRestored(ctx context.Context, batchID string, partial bool) (domain.LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return domain.LevelDeletion{}, err
	}
	return s.repo.MarkLevelDeletionRestored(ctx, batch, partial)
}

// CancelLevelDeletion closes a fully compensated saga.
func (s *LevelDeletionService) CancelLevelDeletion(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return domain.LevelDeletion{}, err
	}
	return s.repo.CancelLevelDeletion(ctx, batch)
}

// FailLevelDeletion records a saga its compensation could not fully undo.
// Pending steps must be saga steps.
func (s *LevelDeletionService) FailLevelDeletion(ctx context.Context, batchID string, pending []string, skipped []levelarchive.Skip) (domain.LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return domain.LevelDeletion{}, err
	}
	for _, p := range pending {
		if !isSagaStep(p) {
			return domain.LevelDeletion{}, glerrors.New(glerrors.KindInvalid, "unknown level deletion step")
		}
	}
	return s.repo.FailLevelDeletion(ctx, batch, pending, skipped)
}

func isSagaStep(name string) bool {
	for _, s := range domain.LevelDeletionSteps {
		if s == name {
			return true
		}
	}
	return false
}

// SweepLevelDeletions unsticks dead sagas and lists the expired batches.
func (s *LevelDeletionService) SweepLevelDeletions(ctx context.Context) ([]domain.LevelDeletion, int64, error) {
	return s.repo.SweepLevelDeletions(ctx, domain.SagaStallAfter)
}

// MarkLevelDeletionPurged closes a batch every service purged.
func (s *LevelDeletionService) MarkLevelDeletionPurged(ctx context.Context, batchID string) (domain.LevelDeletion, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return domain.LevelDeletion{}, err
	}
	return s.repo.MarkLevelDeletionPurged(ctx, batch)
}

// PurgeLevelBatch drops config's archive of one batch.
func (s *LevelDeletionService) PurgeLevelBatch(ctx context.Context, batchID string) (int64, error) {
	batch, err := levelarchive.NormalizeBatchID(batchID)
	if err != nil {
		return 0, err
	}
	return s.repo.PurgeLevelBatch(ctx, batch)
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

// PurgeLevelArchives drops archives filed before the retention (legacy; the
// gateway's sweep purges per batch on restorable_until).
func (s *LevelDeletionService) PurgeLevelArchives(ctx context.Context) (int64, error) {
	return s.repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(s.now().UTC()))
}
