package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// WithImportIssues returns the handler serving the pupils' import issues
// (PLAN-import-avisos) through `issues`. Without it ListImportIssues and
// AcceptImportIssue answer Unimplemented.
func (h *StudentsHandler) WithImportIssues(issues *usecase.ImportIssueService) *StudentsHandler {
	cp := *h
	cp.issues = issues
	return &cp
}

var errImportIssuesNotServed = connect.NewError(connect.CodeUnimplemented, errors.New("import issues are not served here"))

// ListImportIssues lists a pupil's import issues. Who may read them and whether
// the module is on is the gateway's call.
func (h *StudentsHandler) ListImportIssues(
	ctx context.Context, req *connect.Request[studentsv1.ListImportIssuesRequest],
) (*connect.Response[studentsv1.ListImportIssuesResponse], error) {
	if h.issues == nil {
		return nil, errImportIssuesNotServed
	}
	issues, err := h.issues.ListImportIssues(ctx, req.Msg.GetStudentId(), req.Msg.GetIncludeResolved())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*studentsv1.StudentImportIssue, 0, len(issues))
	for _, i := range issues {
		out = append(out, toProtoImportIssue(i))
	}
	return connect.NewResponse(&studentsv1.ListImportIssuesResponse{Issues: out}), nil
}

// AcceptImportIssue closes an issue as fine as stored, by the caller the
// gateway names.
func (h *StudentsHandler) AcceptImportIssue(
	ctx context.Context, req *connect.Request[studentsv1.AcceptImportIssueRequest],
) (*connect.Response[studentsv1.AcceptImportIssueResponse], error) {
	if h.issues == nil {
		return nil, errImportIssuesNotServed
	}
	m := req.Msg
	issue, err := h.issues.AcceptImportIssue(ctx, m.GetStudentId(), m.GetIssueId(), m.GetUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.AcceptImportIssueResponse{Issue: toProtoImportIssue(issue)}), nil
}

func toProtoImportIssue(i domain.ImportIssue) *studentsv1.StudentImportIssue {
	return &studentsv1.StudentImportIssue{
		Id:              i.ID(),
		StudentId:       i.StudentID(),
		Field:           string(i.Field()),
		Code:            i.Code(),
		OriginalValue:   i.OriginalValue(),
		StoredValue:     i.StoredValue(),
		ImportBatchId:   i.BatchID(),
		CreatedAt:       optionalTimestamp(i.CreatedAt()),
		ResolvedAt:      optionalTimestamp(i.ResolvedAt()),
		ResolvedBy:      i.ResolvedBy(),
		Resolution:      string(i.Resolution()),
		IntendedGroupId: i.IntendedGroupID(),
	}
}
