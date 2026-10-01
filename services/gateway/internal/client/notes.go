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
	// The last edit; absent on a note never edited.
	UpdatedAt     *time.Time `json:"updatedAt,omitempty"`
	UpdatedByName string     `json:"updatedByName,omitempty"`
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

// EditStudentNote replaces a note's text, as `editor`; editsAny lets the
// caller edit a note they did not write; expectedBody (when set) refuses an
// edit over a note that changed meanwhile.
func (c *StudentsClient) EditStudentNote(ctx context.Context, studentID, noteID, body, expectedBody string, editor NoteAuthor, editsAny bool) (StudentNote, error) {
	res, err := c.rpc.EditStudentNote(ctx, connect.NewRequest(&studentsv1.EditStudentNoteRequest{
		StudentId: studentID, NoteId: noteID, Body: body, ExpectedBody: expectedBody,
		EditorUserId: editor.UserID, EditorName: editor.Name, EditsAny: editsAny,
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
	if n.GetUpdatedAt() != nil {
		at := timeFromTS(n.GetUpdatedAt())
		out.UpdatedAt, out.UpdatedByName = &at, n.GetUpdatedByName()
	}
	return out
}
