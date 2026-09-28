package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

// WithLevelArchive wires the level-deletion trash (ADR 0021). The composition
// root always wires it; without it the four RPCs answer Unimplemented.
func (h *UniformsHandler) WithLevelArchive(a *usecase.LevelArchiveService) *UniformsHandler {
	h.trash = a
	return h
}

var errNoTrash = connect.NewError(connect.CodeUnimplemented, errors.New("level archive not wired"))

// PreviewLevelDeletion counts what deleting the level would archive here.
func (h *UniformsHandler) PreviewLevelDeletion(ctx context.Context, req *connect.Request[uniformsv1.PreviewLevelDeletionRequest]) (*connect.Response[uniformsv1.PreviewLevelDeletionResponse], error) {
	if h.trash == nil {
		return nil, errNoTrash
	}
	counts, err := h.trash.PreviewLevelDeletion(ctx, fromProtoScope(req.Msg.GetScope()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.PreviewLevelDeletionResponse{Counts: toProtoCounts(counts)}), nil
}

// ArchiveLevel moves the level's garments into the trash under the batch.
func (h *UniformsHandler) ArchiveLevel(ctx context.Context, req *connect.Request[uniformsv1.ArchiveLevelRequest]) (*connect.Response[uniformsv1.ArchiveLevelResponse], error) {
	if h.trash == nil {
		return nil, errNoTrash
	}
	counts, err := h.trash.ArchiveLevel(ctx, req.Msg.GetBatchId(), fromProtoScope(req.Msg.GetScope()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.ArchiveLevelResponse{Counts: toProtoCounts(counts)}), nil
}

// RestoreLevelBatch puts a batch back and reports what it skipped.
func (h *UniformsHandler) RestoreLevelBatch(ctx context.Context, req *connect.Request[uniformsv1.RestoreLevelBatchRequest]) (*connect.Response[uniformsv1.RestoreLevelBatchResponse], error) {
	if h.trash == nil {
		return nil, errNoTrash
	}
	rep, err := h.trash.RestoreLevelBatch(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	skipped := make([]*uniformsv1.LevelRestoreSkip, 0, len(rep.Skipped))
	for _, s := range rep.Skipped {
		skipped = append(skipped, &uniformsv1.LevelRestoreSkip{Kind: s.Kind, Count: s.Count, Reason: s.Reason})
	}
	return connect.NewResponse(&uniformsv1.RestoreLevelBatchResponse{
		Restored: toProtoCounts(rep.Restored), Skipped: skipped,
	}), nil
}

// PurgeLevelArchives drops archives past the retention.
func (h *UniformsHandler) PurgeLevelArchives(ctx context.Context, _ *connect.Request[uniformsv1.PurgeLevelArchivesRequest]) (*connect.Response[uniformsv1.PurgeLevelArchivesResponse], error) {
	if h.trash == nil {
		return nil, errNoTrash
	}
	n, err := h.trash.PurgeLevelArchives(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&uniformsv1.PurgeLevelArchivesResponse{PurgedRows: n}), nil
}

func fromProtoScope(s *uniformsv1.LevelScope) levelarchive.Scope {
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

func toProtoCounts(c levelarchive.Counts) []*uniformsv1.LevelArchiveCount {
	sorted := c.Sorted()
	out := make([]*uniformsv1.LevelArchiveCount, 0, len(sorted))
	for _, kc := range sorted {
		out = append(out, &uniformsv1.LevelArchiveCount{Kind: kc.Kind, Count: kc.Count})
	}
	return out
}
