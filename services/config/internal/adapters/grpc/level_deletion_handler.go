package grpcadapter

import (
	"context"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// --- level deletion (ADR 0021) -------------------------------------------------

// PreviewLevelDeletion resolves the level's structure for the gateway's saga.
func (h *ConfigHandler) PreviewLevelDeletion(ctx context.Context, req *connect.Request[configv1.PreviewLevelDeletionRequest]) (*connect.Response[configv1.PreviewLevelDeletionResponse], error) {
	st, err := h.trash.PreviewLevelDeletion(ctx, req.Msg.GetLevelId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.PreviewLevelDeletionResponse{
		LevelId: st.LevelID, LevelName: st.LevelName, Counts: toProtoCounts(st.Counts),
		LevelStatus:   toProtoLevelStatus(st.Deleting),
		GradeLevelIds: st.GradeLevelIDs, GroupIds: st.GroupIDs, SubjectIds: st.SubjectIDs, TermIds: st.TermIDs,
	}), nil
}

// BeginLevelDeletion is the saga's first step and its lock.
func (h *ConfigHandler) BeginLevelDeletion(ctx context.Context, req *connect.Request[configv1.BeginLevelDeletionRequest]) (*connect.Response[configv1.BeginLevelDeletionResponse], error) {
	rec, err := h.trash.BeginLevelDeletion(ctx, usecase.BeginLevelDeletionCommand{
		BatchID: req.Msg.GetBatchId(), LevelID: req.Msg.GetLevelId(), TokenHash: req.Msg.GetTokenHash(),
		DeletedBy: req.Msg.GetDeletedBy(), DeletedByName: req.Msg.GetDeletedByName(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.BeginLevelDeletionResponse{Deletion: toProtoLevelDeletion(rec)}), nil
}

// ArchiveLevel is the saga's last step: the structure into the trash, the
// begun batch completed. The request's deleted_by fields are ignored: who
// deleted was recorded at BeginLevelDeletion.
func (h *ConfigHandler) ArchiveLevel(ctx context.Context, req *connect.Request[configv1.ArchiveLevelRequest]) (*connect.Response[configv1.ArchiveLevelResponse], error) {
	counts, rec, err := h.trash.ArchiveLevel(ctx, usecase.ArchiveLevelCommand{
		BatchID: req.Msg.GetBatchId(), LevelID: req.Msg.GetLevelId(),
		Summary: fromProtoCounts(req.Msg.GetSummary()),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.ArchiveLevelResponse{Counts: toProtoCounts(counts), Deletion: toProtoLevelDeletion(rec)}), nil
}

// RestoreLevelBatch puts the structure back (the restore's first step).
func (h *ConfigHandler) RestoreLevelBatch(ctx context.Context, req *connect.Request[configv1.RestoreLevelBatchRequest]) (*connect.Response[configv1.RestoreLevelBatchResponse], error) {
	rep, err := h.trash.RestoreLevelBatch(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.RestoreLevelBatchResponse{
		Restored: toProtoCounts(rep.Restored), Skipped: toProtoSkips(rep.Skipped),
	}), nil
}

// MarkLevelDeletionRestored closes a batch every service restored.
func (h *ConfigHandler) MarkLevelDeletionRestored(ctx context.Context, req *connect.Request[configv1.MarkLevelDeletionRestoredRequest]) (*connect.Response[configv1.MarkLevelDeletionRestoredResponse], error) {
	rec, err := h.trash.MarkLevelDeletionRestored(ctx, req.Msg.GetBatchId(), req.Msg.GetPartial())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.MarkLevelDeletionRestoredResponse{Deletion: toProtoLevelDeletion(rec)}), nil
}

// ListLevelDeletions is the trash.
func (h *ConfigHandler) ListLevelDeletions(ctx context.Context, _ *connect.Request[configv1.ListLevelDeletionsRequest]) (*connect.Response[configv1.ListLevelDeletionsResponse], error) {
	recs, err := h.trash.ListLevelDeletions(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.LevelDeletion, 0, len(recs))
	for _, r := range recs {
		out = append(out, toProtoLevelDeletion(r))
	}
	return connect.NewResponse(&configv1.ListLevelDeletionsResponse{Deletions: out}), nil
}

// GetLevelDeletion fetches one batch.
func (h *ConfigHandler) GetLevelDeletion(ctx context.Context, req *connect.Request[configv1.GetLevelDeletionRequest]) (*connect.Response[configv1.GetLevelDeletionResponse], error) {
	rec, err := h.trash.GetLevelDeletion(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetLevelDeletionResponse{Deletion: toProtoLevelDeletion(rec)}), nil
}

// PurgeLevelArchives drops what is past the retention.
func (h *ConfigHandler) PurgeLevelArchives(ctx context.Context, _ *connect.Request[configv1.PurgeLevelArchivesRequest]) (*connect.Response[configv1.PurgeLevelArchivesResponse], error) {
	n, err := h.trash.PurgeLevelArchives(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.PurgeLevelArchivesResponse{PurgedRows: n}), nil
}

// BeginLevelRestore marks a trash batch restoring (ADR 0021).
func (h *ConfigHandler) BeginLevelRestore(ctx context.Context, req *connect.Request[configv1.BeginLevelRestoreRequest]) (*connect.Response[configv1.BeginLevelRestoreResponse], error) {
	rec, err := h.trash.BeginLevelRestore(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.BeginLevelRestoreResponse{Deletion: toProtoLevelDeletion(rec)}), nil
}

// AbandonLevelRestore puts a batch whose restore failed back where it was.
func (h *ConfigHandler) AbandonLevelRestore(ctx context.Context, req *connect.Request[configv1.AbandonLevelRestoreRequest]) (*connect.Response[configv1.AbandonLevelRestoreResponse], error) {
	rec, err := h.trash.AbandonLevelRestore(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.AbandonLevelRestoreResponse{Deletion: toProtoLevelDeletion(rec)}), nil
}

// CancelLevelDeletion closes a fully compensated saga.
func (h *ConfigHandler) CancelLevelDeletion(ctx context.Context, req *connect.Request[configv1.CancelLevelDeletionRequest]) (*connect.Response[configv1.CancelLevelDeletionResponse], error) {
	rec, err := h.trash.CancelLevelDeletion(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CancelLevelDeletionResponse{Deletion: toProtoLevelDeletion(rec)}), nil
}

// FailLevelDeletion records a saga its compensation could not fully undo.
func (h *ConfigHandler) FailLevelDeletion(ctx context.Context, req *connect.Request[configv1.FailLevelDeletionRequest]) (*connect.Response[configv1.FailLevelDeletionResponse], error) {
	rec, err := h.trash.FailLevelDeletion(ctx, req.Msg.GetBatchId(), req.Msg.GetPendingSteps(), fromProtoSkips(req.Msg.GetSkipped()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.FailLevelDeletionResponse{Deletion: toProtoLevelDeletion(rec)}), nil
}

// SweepLevelDeletions unsticks dead sagas and lists the expired batches.
func (h *ConfigHandler) SweepLevelDeletions(ctx context.Context, _ *connect.Request[configv1.SweepLevelDeletionsRequest]) (*connect.Response[configv1.SweepLevelDeletionsResponse], error) {
	recs, stalled, err := h.trash.SweepLevelDeletions(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.LevelDeletion, 0, len(recs))
	for _, r := range recs {
		out = append(out, toProtoLevelDeletion(r))
	}
	return connect.NewResponse(&configv1.SweepLevelDeletionsResponse{Expired: out, Stalled: stalled}), nil
}

// MarkLevelDeletionPurged closes a batch every service purged.
func (h *ConfigHandler) MarkLevelDeletionPurged(ctx context.Context, req *connect.Request[configv1.MarkLevelDeletionPurgedRequest]) (*connect.Response[configv1.MarkLevelDeletionPurgedResponse], error) {
	rec, err := h.trash.MarkLevelDeletionPurged(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.MarkLevelDeletionPurgedResponse{Deletion: toProtoLevelDeletion(rec)}), nil
}

// PurgeLevelBatch drops config's archive of one batch.
func (h *ConfigHandler) PurgeLevelBatch(ctx context.Context, req *connect.Request[configv1.PurgeLevelBatchRequest]) (*connect.Response[configv1.PurgeLevelBatchResponse], error) {
	n, err := h.trash.PurgeLevelBatch(ctx, req.Msg.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.PurgeLevelBatchResponse{PurgedRows: n}), nil
}

func fromProtoSkips(in []*configv1.LevelRestoreSkip) []levelarchive.Skip {
	out := make([]levelarchive.Skip, 0, len(in))
	for _, s := range in {
		out = append(out, levelarchive.Skip{Kind: s.GetKind(), Count: s.GetCount(), Reason: s.GetReason()})
	}
	return out
}

func toProtoCounts(c levelarchive.Counts) []*configv1.LevelArchiveCount {
	sorted := c.Sorted()
	out := make([]*configv1.LevelArchiveCount, 0, len(sorted))
	for _, kc := range sorted {
		out = append(out, &configv1.LevelArchiveCount{Kind: kc.Kind, Count: kc.Count})
	}
	return out
}

func fromProtoCounts(in []*configv1.LevelArchiveCount) levelarchive.Counts {
	out := levelarchive.Counts{}
	for _, kc := range in {
		out[kc.GetKind()] += kc.GetCount()
	}
	return out
}

func toProtoSkips(in []levelarchive.Skip) []*configv1.LevelRestoreSkip {
	out := make([]*configv1.LevelRestoreSkip, 0, len(in))
	for _, s := range in {
		out = append(out, &configv1.LevelRestoreSkip{Kind: s.Kind, Count: s.Count, Reason: s.Reason})
	}
	return out
}

func toProtoLevelDeletion(d domain.LevelDeletion) *configv1.LevelDeletion {
	return &configv1.LevelDeletion{
		BatchId: d.BatchID, LevelId: d.LevelID, LevelName: d.LevelName, Counts: toProtoCounts(d.Counts),
		DeletedBy: d.DeletedBy, DeletedByName: d.DeletedByName,
		DeletedAt: timestamppb.New(d.DeletedAt), RestorableUntil: timestamppb.New(d.RestorableUntil),
		Status: string(d.Status), PendingSteps: d.PendingSteps, Skipped: toProtoSkips(d.Skipped),
		RestoringFrom: string(d.RestoringFrom),
	}
}
