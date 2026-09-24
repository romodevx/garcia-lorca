package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// NoteAuthor is who writes a note: the authenticated caller and their name.
type NoteAuthor struct {
	UserID string
	Name   string
}

// StudentNote is one entry in a pupil's history, as the edge speaks it.
type StudentNote struct {
	ID           string     `json:"id"`
	StudentID    string     `json:"studentId"`
	Body         string     `json:"body"`
	AuthorUserID string     `json:"authorUserId"`
	AuthorName   string     `json:"authorName"`
	CreatedAt    time.Time  `json:"createdAt"`
	Voided       bool       `json:"voided"`
	VoidedAt     *time.Time `json:"voidedAt,omitempty"`
}

// AddStudentNote writes a note by `author` about a pupil.
func (c *StudentsClient) AddStudentNote(ctx context.Context, studentID, body string, author NoteAuthor) (StudentNote, error) {
	res, err := c.rpc.AddStudentNote(ctx, connect.NewRequest(&studentsv1.AddStudentNoteRequest{
		StudentId: studentID, Body: body, AuthorUserId: author.UserID, AuthorName: author.Name,
	}))
	if err != nil {
		return StudentNote{}, err
	}
	return fromProtoNote(res.Msg.GetNote()), nil
}

// ListStudentNotes returns a pupil's notes, newest first; voided ones only when
// includeVoided.
func (c *StudentsClient) ListStudentNotes(ctx context.Context, studentID string, includeVoided bool) ([]StudentNote, error) {
	res, err := c.rpc.ListStudentNotes(ctx, connect.NewRequest(&studentsv1.ListStudentNotesRequest{
		StudentId: studentID, IncludeVoided: includeVoided,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]StudentNote, 0, len(res.Msg.GetNotes()))
	for _, n := range res.Msg.GetNotes() {
		out = append(out, fromProtoNote(n))
	}
	return out, nil
}

// VoidStudentNote withdraws a note, as `voidedBy`.
func (c *StudentsClient) VoidStudentNote(ctx context.Context, studentID, noteID, voidedBy string, voidsAny bool) (StudentNote, error) {
	res, err := c.rpc.VoidStudentNote(ctx, connect.NewRequest(&studentsv1.VoidStudentNoteRequest{
		StudentId: studentID, NoteId: noteID, VoidedBy: voidedBy, VoidsAny: voidsAny,
	}))
	if err != nil {
		return StudentNote{}, err
	}
	return fromProtoNote(res.Msg.GetNote()), nil
}

func fromProtoNote(n *studentsv1.StudentNote) StudentNote {
	out := StudentNote{
		ID: n.GetId(), StudentID: n.GetStudentId(), Body: n.GetBody(),
		AuthorUserID: n.GetAuthorUserId(), AuthorName: n.GetAuthorName(),
		CreatedAt: timeFromTS(n.GetCreatedAt()),
	}
	if n.GetVoidedAt() != nil {
		at := timeFromTS(n.GetVoidedAt())
		out.Voided, out.VoidedAt = true, &at
	}
	return out
}
