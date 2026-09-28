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
		GradeLevelIds: st.GradeLevelIDs, GroupIds: st.GroupIDs, SubjectIds: st.SubjectIDs, TermIds: st.TermIDs,
	}), nil
}

// ArchiveLevel is the saga's last step: the structure into the trash, the batch
// into the ledger.
func (h *ConfigHandler) ArchiveLevel(ctx context.Context, req *connect.Request[configv1.ArchiveLevelRequest]) (*connect.Response[configv1.ArchiveLevelResponse], error) {
	counts, rec, err := h.trash.ArchiveLevel(ctx, usecase.ArchiveLevelCommand{
		BatchID: req.Msg.GetBatchId(), LevelID: req.Msg.GetLevelId(),
		DeletedBy: req.Msg.GetDeletedBy(), DeletedByName: req.Msg.GetDeletedByName(),
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
	rec, err := h.trash.MarkLevelDeletionRestored(ctx, req.Msg.GetBatchId())
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
		Status: string(d.Status),
	}
}
