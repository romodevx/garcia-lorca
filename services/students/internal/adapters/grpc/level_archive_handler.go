package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

var errLevelArchiveNotServed = connect.NewError(connect.CodeUnimplemented, errors.New("level deletion is not served"))

// WithLevelArchive returns the handler serving students' step of a level
// deletion (ADR 0021). Without it those RPCs answer Unimplemented.
func (h *StudentsHandler) WithLevelArchive(levels *usecase.LevelArchiveService) *StudentsHandler {
	cp := *h
	cp.levels = levels
	return &cp
}

func (h *StudentsHandler) PreviewLevelDeletion(ctx context.Context, req *connect.Request[studentsv1.PreviewLevelDeletionRequest]) (*connect.Response[studentsv1.PreviewLevelDeletionResponse], error) {
	if h.levels == nil {
		return nil, errLevelArchiveNotServed
	}
	people, err := h.levels.PreviewLevelDeletion(ctx, toLevelScope(req.Msg.GetScope()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.PreviewLevelDeletionResponse{
		Counts:        toProtoLevelCounts(people.Counts),
		StudentIds:    people.StudentIDs,
		EnrollmentIds: people.EnrollmentIDs,
	}), nil
}

func (h *StudentsHandler) ArchiveLevel(ctx context.Context, req *connect.Request[studentsv1.ArchiveLevelRequest]) (*connect.Response[studentsv1.ArchiveLevelResponse], error) {
	if h.levels == nil {
		return nil, errLevelArchiveNotServed
	}
	counts, err := h.levels.ArchiveLevel(ctx, req.Msg.GetBatchId(), toLevelScope(req.Msg.GetScope()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.ArchiveLevelResponse{Counts: toProtoLevelCounts(counts)}), nil
}

func (h *StudentsHandler) RestoreLevelBatch(ctx context.Context, req *connect.Request[studentsv1.RestoreLevelBatchRequest]) (*connect.Response[studentsv1.RestoreLevelBatchResponse], error) {
	if h.levels == nil {
		return nil, errLevelArchiveNotServed
	}
	rep, err := h.levels.RestoreLevelBatch(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	skipped := make([]*studentsv1.LevelRestoreSkip, 0, len(rep.Skipped))
	for _, s := range rep.Skipped {
		skipped = append(skipped, &studentsv1.LevelRestoreSkip{Kind: s.Kind, Count: s.Count, Reason: s.Reason})
	}
	return connect.NewResponse(&studentsv1.RestoreLevelBatchResponse{
		Restored: toProtoLevelCounts(rep.Restored), Skipped: skipped,
	}), nil
}

func (h *StudentsHandler) PurgeLevelArchives(ctx context.Context, _ *connect.Request[studentsv1.PurgeLevelArchivesRequest]) (*connect.Response[studentsv1.PurgeLevelArchivesResponse], error) {
	if h.levels == nil {
		return nil, errLevelArchiveNotServed
	}
	n, err := h.levels.PurgeLevelArchives(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.PurgeLevelArchivesResponse{PurgedRows: n}), nil
}

func toLevelScope(s *studentsv1.LevelScope) levelarchive.Scope {
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

func toProtoLevelCounts(c levelarchive.Counts) []*studentsv1.LevelArchiveCount {
	sorted := c.Sorted()
	out := make([]*studentsv1.LevelArchiveCount, 0, len(sorted))
	for _, kc := range sorted {
		out = append(out, &studentsv1.LevelArchiveCount{Kind: kc.Kind, Count: kc.Count})
	}
	return out
}
