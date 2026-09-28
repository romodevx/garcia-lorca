package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
	"github.com/garcia-lorca/pkg/levelarchive"
	grpcadapter "github.com/garcia-lorca/services/uniforms/internal/adapters/grpc"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

type fakeTrash struct {
	scope levelarchive.Scope
	batch string
}

func (f *fakeTrash) PreviewLevelDeletion(_ context.Context, s levelarchive.Scope) (levelarchive.Counts, error) {
	f.scope = s
	return levelarchive.Counts{"uniformItems": 5}, nil
}

func (f *fakeTrash) ArchiveLevel(_ context.Context, b string, s levelarchive.Scope) (levelarchive.Counts, error) {
	f.batch, f.scope = b, s
	return levelarchive.Counts{"uniformItems": 5}, nil
}

func (f *fakeTrash) RestoreLevelBatch(_ context.Context, b string) (levelarchive.Report, error) {
	f.batch = b
	rep := levelarchive.NewReport()
	rep.Record("uniformItems", 4, 1, "la prenda ya se reasignó a otro nivel")
	return rep, nil
}

func (f *fakeTrash) PurgeLevelArchives(context.Context, time.Time) (int64, error) { return 7, nil }

func (f *fakeTrash) PurgeLevelBatch(_ context.Context, b string) (int64, error) {
	f.batch = b
	return 9, nil
}

const (
	hLevel = "aaaaaaaa-0000-0000-0000-000000000001"
	hGroup = "aaaaaaaa-0000-0000-0000-000000000002"
	hBatch = "aaaaaaaa-0000-0000-0000-000000000003"
)

func trashHandler(t *testing.T) (*grpcadapter.UniformsHandler, *fakeTrash) {
	t.Helper()
	f := &fakeTrash{}
	h := grpcadapter.NewUniformsHandler(nil, nil).WithLevelArchive(usecase.NewLevelArchiveService(f, time.Now))
	return h, f
}

func TestLevelArchiveWire(t *testing.T) {
	ctx := context.Background()
	h, f := trashHandler(t)
	scope := &uniformsv1.LevelScope{LevelId: hLevel, GroupIds: []string{hGroup}, EnrollmentIds: []string{hGroup}, SubjectIds: []string{hGroup}}

	prev, err := h.PreviewLevelDeletion(ctx, connect.NewRequest(&uniformsv1.PreviewLevelDeletionRequest{Scope: scope}))
	require.NoError(t, err)
	require.Len(t, prev.Msg.GetCounts(), 1)
	assert.Equal(t, "uniformItems", prev.Msg.GetCounts()[0].GetKind())
	assert.Equal(t, int64(5), prev.Msg.GetCounts()[0].GetCount())
	assert.Equal(t, []string{hGroup}, f.scope.GroupIDs)
	assert.Equal(t, []string{hGroup}, f.scope.EnrollmentIDs)
	assert.Equal(t, []string{hGroup}, f.scope.SubjectIDs)

	arch, err := h.ArchiveLevel(ctx, connect.NewRequest(&uniformsv1.ArchiveLevelRequest{BatchId: hBatch, Scope: scope}))
	require.NoError(t, err)
	assert.Len(t, arch.Msg.GetCounts(), 1)
	assert.Equal(t, hBatch, f.batch)

	rest, err := h.RestoreLevelBatch(ctx, connect.NewRequest(&uniformsv1.RestoreLevelBatchRequest{BatchId: hBatch}))
	require.NoError(t, err)
	require.Len(t, rest.Msg.GetSkipped(), 1)
	assert.Equal(t, int64(1), rest.Msg.GetSkipped()[0].GetCount())
	assert.NotEmpty(t, rest.Msg.GetSkipped()[0].GetReason())
	assert.Equal(t, int64(4), rest.Msg.GetRestored()[0].GetCount())

	purge, err := h.PurgeLevelArchives(ctx, connect.NewRequest(&uniformsv1.PurgeLevelArchivesRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int64(7), purge.Msg.GetPurgedRows())

	one, err := h.PurgeLevelBatch(ctx, connect.NewRequest(&uniformsv1.PurgeLevelBatchRequest{BatchId: hBatch}))
	require.NoError(t, err)
	assert.Equal(t, int64(9), one.Msg.GetPurgedRows())
	assert.Equal(t, hBatch, f.batch)
}

func TestLevelArchiveWireRefusesMalformedIDs(t *testing.T) {
	ctx := context.Background()
	h, _ := trashHandler(t)
	_, err := h.ArchiveLevel(ctx, connect.NewRequest(&uniformsv1.ArchiveLevelRequest{BatchId: "x", Scope: &uniformsv1.LevelScope{LevelId: hLevel}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = h.PreviewLevelDeletion(ctx, connect.NewRequest(&uniformsv1.PreviewLevelDeletionRequest{}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "a missing scope has no level")
}

func TestLevelArchiveWireUnwiredIsUnimplemented(t *testing.T) {
	h := grpcadapter.NewUniformsHandler(nil, nil)
	_, err := h.RestoreLevelBatch(context.Background(), connect.NewRequest(&uniformsv1.RestoreLevelBatchRequest{BatchId: hBatch}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
