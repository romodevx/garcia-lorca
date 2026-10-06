package grpcadapter

import (
	"context"
	"errors"
	"fmt"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// WithImports returns the handler serving the legacy import (ADR 0033) through
// `imports`. Without it ImportStudents answers Unimplemented.
func (h *StudentsHandler) WithImports(imports *usecase.ImportService) *StudentsHandler {
	cp := *h
	cp.imports = imports
	return &cp
}

var errImportNotServed = connect.NewError(connect.CodeUnimplemented, errors.New("the import is not served here"))

// ImportStudents imports (or, validating, judges) the pupils of the previous
// system. Who may call it and whether the module is on is the gateway's call
// (module flags + students:import); this adapter only translates.
func (h *StudentsHandler) ImportStudents(
	ctx context.Context, req *connect.Request[studentsv1.ImportStudentsRequest],
) (*connect.Response[studentsv1.ImportStudentsResponse], error) {
	if h.imports == nil {
		return nil, errImportNotServed
	}
	m := req.Msg
	if len(m.GetRows()) > usecase.MaxImportRows {
		// Refused before a thousand rows are copied into the use case's shape.
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("an import carries at most %d rows", usecase.MaxImportRows))
	}
	rows := make([]usecase.ImportRow, 0, len(m.GetRows()))
	for _, r := range m.GetRows() {
		rows = append(rows, usecase.ImportRow{
			RowNumber:        int(r.GetRowNumber()),
			CURP:             r.GetCurp(),
			FirstName:        r.GetFirstName(),
			LastName:         r.GetLastName(),
			MaternalLastName: r.GetMaternalLastName(),
			GroupID:          r.GetGroupId(),
			BankReference:    r.GetBankReference(),
			Profile:          toDomainProfileInput(r.GetProfile()),
		})
	}
	results, err := h.imports.ImportStudents(ctx, rows, m.GetValidateOnly(), m.GetBatchId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*studentsv1.ImportRowResult, 0, len(results))
	for _, r := range results {
		out = append(out, &studentsv1.ImportRowResult{
			RowNumber:        int32(r.RowNumber),
			Outcome:          importOutcomes[r.Outcome],
			StudentId:        r.StudentID,
			EnrollmentNumber: r.EnrollmentNumber,
			Warnings:         r.Warnings,
			Reason:           r.Reason,
		})
	}
	return connect.NewResponse(&studentsv1.ImportStudentsResponse{Results: out}), nil
}

var importOutcomes = map[usecase.ImportOutcome]studentsv1.ImportRowOutcome{
	usecase.ImportImported:        studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_IMPORTED,
	usecase.ImportAlreadyExists:   studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_ALREADY_EXISTS,
	usecase.ImportAlreadyImported: studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_ALREADY_IMPORTED,
	usecase.ImportRejected:        studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_REJECTED,
}
