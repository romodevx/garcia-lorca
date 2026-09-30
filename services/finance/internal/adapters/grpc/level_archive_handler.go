package grpcadapter

import (
	"context"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// WithLevelArchive wires finance's step of a level deletion (ADR 0021). The
// composition root always wires it.
func (h *FinanceHandler) WithLevelArchive(levels *usecase.LevelArchiveService) *FinanceHandler {
	h.levels = levels
	return h
}

// PreviewLevelDeletion counts what deleting the level would archive here.
func (h *FinanceHandler) PreviewLevelDeletion(ctx context.Context, req *connect.Request[financev1.PreviewLevelDeletionRequest]) (*connect.Response[financev1.PreviewLevelDeletionResponse], error) {
	counts, err := h.levels.PreviewLevelDeletion(ctx, toLevelScope(req.Msg.GetScope()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.PreviewLevelDeletionResponse{Counts: toProtoCounts(counts)}), nil
}

// ArchiveLevel moves the level's tuition plans into the trash under the batch.
func (h *FinanceHandler) ArchiveLevel(ctx context.Context, req *connect.Request[financev1.ArchiveLevelRequest]) (*connect.Response[financev1.ArchiveLevelResponse], error) {
	counts, err := h.levels.ArchiveLevel(ctx, req.Msg.GetBatchId(), toLevelScope(req.Msg.GetScope()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ArchiveLevelResponse{Counts: toProtoCounts(counts)}), nil
}

// RestoreLevelBatch puts a batch back and reports what it skipped.
func (h *FinanceHandler) RestoreLevelBatch(ctx context.Context, req *connect.Request[financev1.RestoreLevelBatchRequest]) (*connect.Response[financev1.RestoreLevelBatchResponse], error) {
	rep, err := h.levels.RestoreLevelBatch(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	skipped := make([]*financev1.LevelRestoreSkip, 0, len(rep.Skipped))
	for _, s := range rep.Skipped {
		skipped = append(skipped, &financev1.LevelRestoreSkip{Kind: s.Kind, Count: s.Count, Reason: s.Reason})
	}
	return connect.NewResponse(&financev1.RestoreLevelBatchResponse{Restored: toProtoCounts(rep.Restored), Skipped: skipped}), nil
}

// PurgeLevelArchives drops the archives past the retention.
func (h *FinanceHandler) PurgeLevelArchives(ctx context.Context, _ *connect.Request[financev1.PurgeLevelArchivesRequest]) (*connect.Response[financev1.PurgeLevelArchivesResponse], error) {
	n, err := h.levels.PurgeLevelArchives(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.PurgeLevelArchivesResponse{PurgedRows: n}), nil
}

// PurgeLevelBatch drops one batch's archive (the gateway's retention sweep).
func (h *FinanceHandler) PurgeLevelBatch(ctx context.Context, req *connect.Request[financev1.PurgeLevelBatchRequest]) (*connect.Response[financev1.PurgeLevelBatchResponse], error) {
	n, err := h.levels.PurgeLevelBatch(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.PurgeLevelBatchResponse{PurgedRows: n}), nil
}

// toLevelScope reads the wire scope; a missing scope is an empty one, which
// the use case refuses for its missing level.
func toLevelScope(s *financev1.LevelScope) levelarchive.Scope {
	return levelarchive.Scope{
		LevelID:       s.GetLevelId(),
		GradeLevelIDs: s.GetGradeLevelIds(),
		GroupIDs:      s.GetGroupIds(),
		SubjectIDs:    s.GetSubjectIds(),
		TermIDs:       s.GetTermIds(),
		EnrollmentIDs: s.GetEnrollmentIds(),
		StudentIDs:    s.GetStudentIds(),
	}
}

func toProtoCounts(c levelarchive.Counts) []*financev1.LevelArchiveCount {
	sorted := c.Sorted()
	out := make([]*financev1.LevelArchiveCount, 0, len(sorted))
	for _, kc := range sorted {
		out = append(out, &financev1.LevelArchiveCount{Kind: kc.Kind, Count: kc.Count})
	}
	return out
}
