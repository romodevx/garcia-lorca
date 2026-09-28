package handler_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/attendance/internal/handler"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

type fakeTrash struct {
	scope  levelarchive.Scope
	batch  string
	skip   []string
	missed bool
}

func (f *fakeTrash) PreviewLevelDeletion(_ context.Context, s levelarchive.Scope) (levelarchive.Counts, error) {
	f.scope = s
	return levelarchive.Counts{"attendanceSessions": 2, "attendanceMarks": 5}, nil
}

func (f *fakeTrash) ArchiveLevel(_ context.Context, b string, s levelarchive.Scope) (levelarchive.Counts, error) {
	f.batch, f.scope = b, s
	return levelarchive.Counts{"attendanceMarks": 5, "attendanceSessions": 2}, nil
}

func (f *fakeTrash) RestoreLevelBatch(_ context.Context, b string, skip []string) (levelarchive.Report, error) {
	f.batch, f.skip = b, skip
	if f.missed {
		return levelarchive.Report{}, levelarchive.ErrBatchNotFound
	}
	rep := levelarchive.NewReport()
	rep.Record("attendanceMarks", 4, 1, "ya hay otra lista o marca para ese día")
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

func trashHandler(t *testing.T) (*handler.AttendanceHandler, *fakeTrash) {
	t.Helper()
	f := &fakeTrash{}
	h := handler.NewAttendanceHandler(nil).WithLevelArchive(service.NewLevelArchiveService(f, time.Now))
	return h, f
}

func TestLevelArchiveWire(t *testing.T) {
	ctx := context.Background()
	h, f := trashHandler(t)
	scope := &attendancev1.LevelScope{LevelId: hLevel, GroupIds: []string{hGroup}, EnrollmentIds: []string{hGroup}, SubjectIds: []string{hGroup}}

	prev, err := h.PreviewLevelDeletion(ctx, connect.NewRequest(&attendancev1.PreviewLevelDeletionRequest{Scope: scope}))
	require.NoError(t, err)
	require.Len(t, prev.Msg.GetCounts(), 2)
	assert.Equal(t, "attendanceMarks", prev.Msg.GetCounts()[0].GetKind(), "counts travel sorted by kind")
	assert.Equal(t, int64(5), prev.Msg.GetCounts()[0].GetCount())
	assert.Equal(t, []string{hGroup}, f.scope.GroupIDs)
	assert.Equal(t, []string{hGroup}, f.scope.EnrollmentIDs)
	assert.Equal(t, []string{hGroup}, f.scope.SubjectIDs)

	arch, err := h.ArchiveLevel(ctx, connect.NewRequest(&attendancev1.ArchiveLevelRequest{BatchId: hBatch, Scope: scope}))
	require.NoError(t, err)
	assert.Len(t, arch.Msg.GetCounts(), 2)
	assert.Equal(t, hBatch, f.batch)

	rest, err := h.RestoreLevelBatch(ctx, connect.NewRequest(&attendancev1.RestoreLevelBatchRequest{BatchId: hBatch, SkipEnrollmentIds: []string{hGroup}}))
	require.NoError(t, err)
	assert.Equal(t, []string{hGroup}, f.skip, "the enrollments students could not restore reach the repository")
	require.Len(t, rest.Msg.GetSkipped(), 1)
	assert.Equal(t, int64(1), rest.Msg.GetSkipped()[0].GetCount())
	assert.NotEmpty(t, rest.Msg.GetSkipped()[0].GetReason())
	assert.Equal(t, int64(4), rest.Msg.GetRestored()[0].GetCount())

	purge, err := h.PurgeLevelArchives(ctx, connect.NewRequest(&attendancev1.PurgeLevelArchivesRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int64(7), purge.Msg.GetPurgedRows())

	one, err := h.PurgeLevelBatch(ctx, connect.NewRequest(&attendancev1.PurgeLevelBatchRequest{BatchId: hBatch}))
	require.NoError(t, err)
	assert.Equal(t, int64(9), one.Msg.GetPurgedRows())
	assert.Equal(t, hBatch, f.batch)
}

func TestLevelArchiveWireMissingArchiveIsNotFound(t *testing.T) {
	h, f := trashHandler(t)
	f.missed = true
	_, err := h.RestoreLevelBatch(context.Background(), connect.NewRequest(&attendancev1.RestoreLevelBatchRequest{BatchId: hBatch}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "never a silent success")
}

func TestLevelArchiveWireRefusesMalformedIDs(t *testing.T) {
	ctx := context.Background()
	h, _ := trashHandler(t)
	_, err := h.ArchiveLevel(ctx, connect.NewRequest(&attendancev1.ArchiveLevelRequest{BatchId: "x", Scope: &attendancev1.LevelScope{LevelId: hLevel}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = h.PreviewLevelDeletion(ctx, connect.NewRequest(&attendancev1.PreviewLevelDeletionRequest{}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "a missing scope has no level")
}

func TestLevelArchiveWireUnwiredIsUnimplemented(t *testing.T) {
	h := handler.NewAttendanceHandler(nil)
	_, err := h.RestoreLevelBatch(context.Background(), connect.NewRequest(&attendancev1.RestoreLevelBatchRequest{BatchId: hBatch}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
