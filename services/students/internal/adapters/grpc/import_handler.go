package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// ImportStudents is the legacy import (ADR 0033). Wired in the next step of the
// module; until then the contract answers Unimplemented.
func (h *StudentsHandler) ImportStudents(
	_ context.Context, _ *connect.Request[studentsv1.ImportStudentsRequest],
) (*connect.Response[studentsv1.ImportStudentsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("import not implemented yet"))
}
