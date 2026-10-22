package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// StudentImportIssue is a value the legacy import could not keep as the file
// wrote it, stored with the pupil (PLAN-import-avisos). ResolvedAt is zero
// while it is open.
type StudentImportIssue struct {
	ID              string
	StudentID       string
	Field           string
	Code            string
	OriginalValue   string
	StoredValue     string
	CreatedAt       time.Time
	ResolvedAt      time.Time
	ResolvedBy      string
	Resolution      string
	IntendedGroupID string
}

// ListImportIssues is a pupil's import issues, open first (resolved ones only
// with includeResolved). An unknown pupil is NotFound.
func (c *StudentsClient) ListImportIssues(ctx context.Context, studentID string, includeResolved bool) ([]StudentImportIssue, error) {
	res, err := c.rpc.ListImportIssues(ctx, connect.NewRequest(&studentsv1.ListImportIssuesRequest{
		StudentId: studentID, IncludeResolved: includeResolved,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]StudentImportIssue, 0, len(res.Msg.GetIssues()))
	for _, i := range res.Msg.GetIssues() {
		out = append(out, toStudentImportIssue(i))
	}
	return out, nil
}

// AcceptImportIssue closes an open issue as fine as stored, by userID. A
// draft's curp issue is refused (STUDENT_DRAFT).
func (c *StudentsClient) AcceptImportIssue(ctx context.Context, studentID, issueID, userID string) (StudentImportIssue, error) {
	res, err := c.rpc.AcceptImportIssue(ctx, connect.NewRequest(&studentsv1.AcceptImportIssueRequest{
		StudentId: studentID, IssueId: issueID, UserId: userID,
	}))
	if err != nil {
		return StudentImportIssue{}, err
	}
	return toStudentImportIssue(res.Msg.GetIssue()), nil
}

func toStudentImportIssue(i *studentsv1.StudentImportIssue) StudentImportIssue {
	out := StudentImportIssue{
		ID: i.GetId(), StudentID: i.GetStudentId(), Field: i.GetField(), Code: i.GetCode(),
		OriginalValue: i.GetOriginalValue(), StoredValue: i.GetStoredValue(), ResolvedBy: i.GetResolvedBy(),
		Resolution: i.GetResolution(), IntendedGroupID: i.GetIntendedGroupId(),
	}
	if i.GetCreatedAt() != nil {
		out.CreatedAt = i.GetCreatedAt().AsTime()
	}
	if i.GetResolvedAt() != nil {
		out.ResolvedAt = i.GetResolvedAt().AsTime()
	}
	return out
}
