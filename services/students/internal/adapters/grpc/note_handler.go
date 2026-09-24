package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/students/internal/domain"
)

var errNotesNotServed = connect.NewError(connect.CodeUnimplemented, errors.New("notes are not served"))

// AddStudentNote writes a note. The author fields come from the gateway's
// authenticated caller.
func (h *StudentsHandler) AddStudentNote(ctx context.Context, req *connect.Request[studentsv1.AddStudentNoteRequest]) (*connect.Response[studentsv1.AddStudentNoteResponse], error) {
	if h.notes == nil {
		return nil, errNotesNotServed
	}
	m := req.Msg
	note, err := h.notes.AddNote(ctx, m.GetStudentId(), m.GetBody(),
		domain.NoteAuthor{UserID: m.GetAuthorUserId(), Name: m.GetAuthorName()})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.AddStudentNoteResponse{Note: toProtoNote(note)}), nil
}

// ListStudentNotes returns a pupil's notes, newest first.
func (h *StudentsHandler) ListStudentNotes(ctx context.Context, req *connect.Request[studentsv1.ListStudentNotesRequest]) (*connect.Response[studentsv1.ListStudentNotesResponse], error) {
	if h.notes == nil {
		return nil, errNotesNotServed
	}
	notes, err := h.notes.ListNotes(ctx, req.Msg.GetStudentId(), req.Msg.GetIncludeVoided())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*studentsv1.StudentNote, 0, len(notes))
	for _, n := range notes {
		out = append(out, toProtoNote(n))
	}
	return connect.NewResponse(&studentsv1.ListStudentNotesResponse{Notes: out}), nil
}

// VoidStudentNote withdraws a note.
func (h *StudentsHandler) VoidStudentNote(ctx context.Context, req *connect.Request[studentsv1.VoidStudentNoteRequest]) (*connect.Response[studentsv1.VoidStudentNoteResponse], error) {
	if h.notes == nil {
		return nil, errNotesNotServed
	}
	m := req.Msg
	note, err := h.notes.VoidNote(ctx, m.GetStudentId(), m.GetNoteId(), m.GetVoidedBy(), m.GetVoidsAny())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.VoidStudentNoteResponse{Note: toProtoNote(note)}), nil
}

func toProtoNote(n domain.Note) *studentsv1.StudentNote {
	out := &studentsv1.StudentNote{
		Id: n.ID(), StudentId: n.StudentID(), Body: n.Body(),
		AuthorUserId: n.Author().UserID, AuthorName: n.Author().Name,
		CreatedAt: timestamppb.New(n.CreatedAt()),
	}
	if n.IsVoided() {
		out.VoidedAt = timestamppb.New(n.VoidedAt())
		out.VoidedBy = n.VoidedBy()
	}
	return out
}
