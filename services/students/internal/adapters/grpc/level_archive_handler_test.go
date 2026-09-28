package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

type stubLevelArchive struct {
	scope  levelarchive.Scope
	purged string
	missed bool
}

func (s *stubLevelArchive) PreviewLevelDeletion(_ context.Context, sc levelarchive.Scope) (ports.LevelPeople, error) {
	s.scope = sc
	return ports.LevelPeople{
		Counts:        levelarchive.Counts{"students": 2, "enrollments": 1, "enrollmentTransfers": 0},
		StudentIDs:    []string{"s1", "s2"},
		EnrollmentIDs: []string{"e1"},
	}, nil
}
func (s *stubLevelArchive) ArchiveLevel(_ context.Context, _ string, sc levelarchive.Scope) (levelarchive.Counts, error) {
	s.scope = sc
	return levelarchive.Counts{"students": 2, "enrollments": 1}, nil
}
func (s *stubLevelArchive) RestoreLevelBatch(context.Context, string) (ports.LevelRestore, error) {
	if s.missed {
		return ports.LevelRestore{}, levelarchive.ErrBatchNotFound
	}
	rep := levelarchive.NewReport()
	rep.Record("students", 1, 1, "el alumno ya tiene otro nivel")
	return ports.LevelRestore{Report: rep, UnrestoredEnrollmentIDs: []string{haGroup}}, nil
}
func (s *stubLevelArchive) PurgeLevelBatch(_ context.Context, b string) (int64, error) {
	s.purged = b
	return 6, nil
}
func (s *stubLevelArchive) PurgeLevelArchives(context.Context, time.Time) (int64, error) {
	return 4, nil
}

const (
	haLevel = "aaaaaaaa-0000-0000-0000-000000000001"
	haGroup = "aaaaaaaa-0000-0000-0000-000000000002"
	haBatch = "aaaaaaaa-0000-0000-0000-000000000003"
)

func newLevelArchiveHandler(stub *stubLevelArchive) *grpcadapter.StudentsHandler {
	svc := usecase.NewLevelArchiveService(stub, time.Now)
	return grpcadapter.NewStudentsHandler(nil, nil, nil, usecase.NewHealthService("students")).WithLevelArchive(svc)
}

func TestLevelArchiveRPCsMapTheWire(t *testing.T) {
	stub := &stubLevelArchive{}
	h := newLevelArchiveHandler(stub)
	ctx := context.Background()
	scope := &studentsv1.LevelScope{LevelId: haLevel, GroupIds: []string{haGroup}}

	prev, err := h.PreviewLevelDeletion(ctx, connect.NewRequest(&studentsv1.PreviewLevelDeletionRequest{Scope: scope}))
	require.NoError(t, err)
	assert.Equal(t, []string{"s1", "s2"}, prev.Msg.GetStudentIds())
	assert.Equal(t, []string{"e1"}, prev.Msg.GetEnrollmentIds())
	require.Len(t, prev.Msg.GetCounts(), 3)
	assert.Equal(t, "enrollmentTransfers", prev.Msg.GetCounts()[0].GetKind(), "counts are sorted by kind")
	assert.Equal(t, []string{haGroup}, stub.scope.GroupIDs)

	arch, err := h.ArchiveLevel(ctx, connect.NewRequest(&studentsv1.ArchiveLevelRequest{BatchId: haBatch, Scope: scope}))
	require.NoError(t, err)
	assert.Len(t, arch.Msg.GetCounts(), 2)

	rest, err := h.RestoreLevelBatch(ctx, connect.NewRequest(&studentsv1.RestoreLevelBatchRequest{BatchId: haBatch}))
	require.NoError(t, err)
	require.Len(t, rest.Msg.GetSkipped(), 1)
	assert.Equal(t, "el alumno ya tiene otro nivel", rest.Msg.GetSkipped()[0].GetReason())
	assert.Equal(t, int64(1), rest.Msg.GetRestored()[0].GetCount())
	assert.Equal(t, []string{haGroup}, rest.Msg.GetUnrestoredEnrollmentIds())

	one, err := h.PurgeLevelBatch(ctx, connect.NewRequest(&studentsv1.PurgeLevelBatchRequest{BatchId: haBatch}))
	require.NoError(t, err)
	assert.Equal(t, int64(6), one.Msg.GetPurgedRows())
	assert.Equal(t, haBatch, stub.purged)

	purge, err := h.PurgeLevelArchives(ctx, connect.NewRequest(&studentsv1.PurgeLevelArchivesRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int64(4), purge.Msg.GetPurgedRows())
}

func TestLevelArchiveRPCsRefuseMalformedInput(t *testing.T) {
	h := newLevelArchiveHandler(&stubLevelArchive{})
	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
	}{
		{"preview without scope", func() error {
			_, err := h.PreviewLevelDeletion(ctx, connect.NewRequest(&studentsv1.PreviewLevelDeletionRequest{}))
			return err
		}},
		{"archive bad batch", func() error {
			_, err := h.ArchiveLevel(ctx, connect.NewRequest(&studentsv1.ArchiveLevelRequest{BatchId: "x", Scope: &studentsv1.LevelScope{LevelId: haLevel}}))
			return err
		}},
		{"restore bad batch", func() error {
			_, err := h.RestoreLevelBatch(ctx, connect.NewRequest(&studentsv1.RestoreLevelBatchRequest{BatchId: "nope"}))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(tc.call()))
		})
	}
}

func TestLevelArchiveRPCsWithoutTheServiceAreUnimplemented(t *testing.T) {
	h := grpcadapter.NewStudentsHandler(nil, nil, nil, usecase.NewHealthService("students"))
	_, err := h.PurgeLevelArchives(context.Background(), connect.NewRequest(&studentsv1.PurgeLevelArchivesRequest{}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestLevelArchiveRestoreOfAMissingBatchIsNotFound(t *testing.T) {
	h := newLevelArchiveHandler(&stubLevelArchive{missed: true})
	_, err := h.RestoreLevelBatch(context.Background(), connect.NewRequest(&studentsv1.RestoreLevelBatchRequest{BatchId: haBatch}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "never a silent success")
}
