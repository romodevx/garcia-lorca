package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

type memNotes struct{ byID map[string]domain.Note }

func (m *memNotes) AddNote(_ context.Context, n domain.Note) error { m.byID[n.ID()] = n; return nil }
func (m *memNotes) NotesOf(_ context.Context, studentID string) ([]domain.Note, error) {
	var out []domain.Note
	for _, n := range m.byID {
		if n.StudentID() == studentID {
			out = append(out, n)
		}
	}
	return out, nil
}
func (m *memNotes) NoteByID(_ context.Context, id string) (domain.Note, error) {
	n, ok := m.byID[id]
	if !ok {
		return domain.Note{}, glerrors.ErrNotFound
	}
	return n, nil
}
func (m *memNotes) VoidNote(_ context.Context, n domain.Note) error { m.byID[n.ID()] = n; return nil }

func newNotesHandler(t *testing.T) *grpcadapter.StudentsHandler {
	t.Helper()
	repo := newFakeRepo()
	svc := usecase.NewStudentService(repo, &fakePublisher{}, testLevels, testTenant, func() string { return "stu-1" })
	notes := usecase.NewNoteService(repo, &memNotes{byID: map[string]domain.Note{}}, testTenant,
		func() string { return "note-1" }, func() time.Time { return time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC) })
	h := grpcadapter.NewStudentsHandler(svc, nil, nil, usecase.NewHealthService("students")).WithNotes(notes)
	createValid(t, h)
	return h
}

func TestStudentNotesAreAddedListedAndVoided(t *testing.T) {
	h := newNotesHandler(t)
	ctx := context.Background()
	added, err := h.AddStudentNote(ctx, connect.NewRequest(&studentsv1.AddStudentNoteRequest{
		StudentId: "stu-1", Body: "Llegó con fiebre", AuthorUserId: "u-1", AuthorName: "Gabriela Rojas",
	}))
	if err != nil {
		t.Fatalf("AddStudentNote: %v", err)
	}
	n := added.Msg.GetNote()
	if n.GetId() != "note-1" || n.GetAuthorName() != "Gabriela Rojas" || n.GetCreatedAt() == nil || n.GetVoidedAt() != nil {
		t.Errorf("note = %+v", n)
	}

	list, err := h.ListStudentNotes(ctx, connect.NewRequest(&studentsv1.ListStudentNotesRequest{StudentId: "stu-1"}))
	if err != nil || len(list.Msg.GetNotes()) != 1 {
		t.Fatalf("ListStudentNotes = %v, %v", list, err)
	}

	_, err = h.VoidStudentNote(ctx, connect.NewRequest(&studentsv1.VoidStudentNoteRequest{StudentId: "stu-1", NoteId: "note-1", VoidedBy: "u-other"}))
	assertCode(t, err, connect.CodePermissionDenied)

	voided, err := h.VoidStudentNote(ctx, connect.NewRequest(&studentsv1.VoidStudentNoteRequest{StudentId: "stu-1", NoteId: "note-1", VoidedBy: "u-1"}))
	if err != nil || voided.Msg.GetNote().GetVoidedAt() == nil || voided.Msg.GetNote().GetVoidedBy() != "u-1" {
		t.Fatalf("VoidStudentNote = %v, %v", voided, err)
	}
	after, _ := h.ListStudentNotes(ctx, connect.NewRequest(&studentsv1.ListStudentNotesRequest{StudentId: "stu-1"}))
	if len(after.Msg.GetNotes()) != 0 {
		t.Errorf("a voided note is still in the ordinary history")
	}
	all, _ := h.ListStudentNotes(ctx, connect.NewRequest(&studentsv1.ListStudentNotesRequest{StudentId: "stu-1", IncludeVoided: true}))
	if len(all.Msg.GetNotes()) != 1 {
		t.Errorf("include_voided lost the voided note")
	}
}

// Without notes wired the RPCs say so instead of panicking.
func TestStudentNotesWithoutAServiceAreUnimplemented(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.ListStudentNotes(context.Background(), connect.NewRequest(&studentsv1.ListStudentNotesRequest{StudentId: "stu-1"}))
	assertCode(t, err, connect.CodeUnimplemented)
}
