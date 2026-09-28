package grpcadapter

import (
	"context"

	connect "connectrpc.com/connect"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// WithLevelArchive wires grades' step of a level deletion (ADR 0021). The
// composition root always wires it.
func (h *GradesHandler) WithLevelArchive(levels *usecase.LevelArchiveService) *GradesHandler {
	h.levels = levels
	return h
}

// PreviewLevelDeletion counts what deleting the level would archive here.
func (h *GradesHandler) PreviewLevelDeletion(ctx context.Context, req *connect.Request[gradesv1.PreviewLevelDeletionRequest]) (*connect.Response[gradesv1.PreviewLevelDeletionResponse], error) {
	counts, err := h.levels.PreviewLevelDeletion(ctx, toLevelScope(req.Msg.GetScope()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.PreviewLevelDeletionResponse{Counts: toProtoCounts(counts)}), nil
}

// ArchiveLevel moves the level's grades into the trash under the batch.
func (h *GradesHandler) ArchiveLevel(ctx context.Context, req *connect.Request[gradesv1.ArchiveLevelRequest]) (*connect.Response[gradesv1.ArchiveLevelResponse], error) {
	counts, err := h.levels.ArchiveLevel(ctx, req.Msg.GetBatchId(), toLevelScope(req.Msg.GetScope()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.ArchiveLevelResponse{Counts: toProtoCounts(counts)}), nil
}

// RestoreLevelBatch puts a batch back and reports what it skipped.
func (h *GradesHandler) RestoreLevelBatch(ctx context.Context, req *connect.Request[gradesv1.RestoreLevelBatchRequest]) (*connect.Response[gradesv1.RestoreLevelBatchResponse], error) {
	rep, err := h.levels.RestoreLevelBatch(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	skipped := make([]*gradesv1.LevelRestoreSkip, 0, len(rep.Skipped))
	for _, s := range rep.Skipped {
		skipped = append(skipped, &gradesv1.LevelRestoreSkip{Kind: s.Kind, Count: s.Count, Reason: s.Reason})
	}
	return connect.NewResponse(&gradesv1.RestoreLevelBatchResponse{Restored: toProtoCounts(rep.Restored), Skipped: skipped}), nil
}

// PurgeLevelArchives drops the archives past the retention.
func (h *GradesHandler) PurgeLevelArchives(ctx context.Context, _ *connect.Request[gradesv1.PurgeLevelArchivesRequest]) (*connect.Response[gradesv1.PurgeLevelArchivesResponse], error) {
	n, err := h.levels.PurgeLevelArchives(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.PurgeLevelArchivesResponse{PurgedRows: n}), nil
}

// toLevelScope reads the wire scope; a missing scope is an empty one, which
// the use case refuses for its missing level.
func toLevelScope(s *gradesv1.LevelScope) levelarchive.Scope {
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

func toProtoCounts(c levelarchive.Counts) []*gradesv1.LevelArchiveCount {
	sorted := c.Sorted()
	out := make([]*gradesv1.LevelArchiveCount, 0, len(sorted))
	for _, kc := range sorted {
		out = append(out, &gradesv1.LevelArchiveCount{Kind: kc.Kind, Count: kc.Count})
	}
	return out
}
