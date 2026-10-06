package grpcadapter_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// fakeImportRepo stores what the handler hands it; every CURP is new.
type fakeImportRepo struct{ stored []ports.ImportCommand }

func (r *fakeImportRepo) StudentsByCURP(_ context.Context, _ []string) (map[string]ports.ExistingPupil, error) {
	return map[string]ports.ExistingPupil{}, nil
}

func (r *fakeImportRepo) BankReferencesTaken(_ context.Context, _ []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (r *fakeImportRepo) ImportStudent(_ context.Context, cmd ports.ImportCommand) (ports.ImportedPupil, error) {
	r.stored = append(r.stored, cmd)
	return ports.ImportedPupil{EnrollmentNumber: "2026-00042"}, nil
}

const importHandlerBatch = "b0000000-0000-0000-0000-000000000001"

func importHandler(t *testing.T) (*grpcadapter.StudentsHandler, *fakeImportRepo) {
	t.Helper()
	repo := &fakeImportRepo{}
	catalog := &fakeCatalogValidator{groups: map[string]ports.GroupRef{
		enrollGroup: {ID: enrollGroup, CycleID: enrollCycle, Name: "2° C", GradeLevelID: "grade-2"},
	}}
	ids := 0
	svc := usecase.NewImportService(repo, catalog, testLevels, testTenant,
		func() string { ids++; return "00000000-0000-0000-0000-00000000000" + string(rune('0'+ids)) },
		func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) })
	h, _ := newHandler(t)
	return h.WithImports(svc), repo
}

func TestImportStudentsMapsTheWire(t *testing.T) {
	h, repo := importHandler(t)
	res, err := h.ImportStudents(context.Background(), connect.NewRequest(&studentsv1.ImportStudentsRequest{
		BatchId: importHandlerBatch,
		Rows: []*studentsv1.ImportStudentRow{
			{
				RowNumber: 12, Curp: validCURP, FirstName: "Ana", LastName: "Herrera", MaternalLastName: "Ríos",
				GroupId: enrollGroup, BankReference: "S/R",
				Profile: &studentsv1.StudentProfile{
					Sex: "F", Phones: []*studentsv1.StudentPhone{{Kind: "casa", Number: "52 443 123 4567"}},
					Address: &studentsv1.StudentAddress{State: "MICHOACAN DE OCAMPO"},
				},
			},
			{RowNumber: 13, Curp: "nope", FirstName: "Luis", LastName: "Pérez", GroupId: enrollGroup},
		},
	}))
	if err != nil {
		t.Fatalf("ImportStudents: %v", err)
	}
	got := res.Msg.GetResults()
	if len(got) != 2 {
		t.Fatalf("results = %d; want 2", len(got))
	}
	first, second := got[0], got[1]
	if first.GetRowNumber() != 12 || first.GetOutcome() != studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_IMPORTED ||
		first.GetStudentId() == "" || first.GetEnrollmentNumber() != "2026-00042" ||
		!reflect.DeepEqual(first.GetWarnings(), []string{usecase.WarnBankReferenceInvalid}) {
		t.Errorf("row 12 = %+v", first)
	}
	if second.GetRowNumber() != 13 || second.GetOutcome() != studentsv1.ImportRowOutcome_IMPORT_ROW_OUTCOME_REJECTED ||
		second.GetReason() != usecase.ReasonCURPInvalid {
		t.Errorf("row 13 = %+v", second)
	}
	if len(repo.stored) != 1 {
		t.Fatalf("stored %d pupils; want 1", len(repo.stored))
	}
	s := repo.stored[0].Student
	if s.MaternalLastName() != "Ríos" || s.Profile().Address().State != "Michoacán" || s.Profile().Phones()[0].Number != "4431234567" {
		t.Errorf("profile not carried: %q %+v %+v", s.MaternalLastName(), s.Profile().Address(), s.Profile().Phones())
	}
}

func TestImportStudentsRefusesABadRequest(t *testing.T) {
	h, _ := importHandler(t)
	rows := make([]*studentsv1.ImportStudentRow, usecase.MaxImportRows+1)
	for i := range rows {
		rows[i] = &studentsv1.ImportStudentRow{}
	}
	_, err := h.ImportStudents(context.Background(), connect.NewRequest(&studentsv1.ImportStudentsRequest{Rows: rows, ValidateOnly: true}))
	assertCode(t, err, connect.CodeInvalidArgument)

	_, err = h.ImportStudents(context.Background(), connect.NewRequest(&studentsv1.ImportStudentsRequest{
		Rows: []*studentsv1.ImportStudentRow{{RowNumber: 1}},
	}))
	assertCode(t, err, connect.CodeInvalidArgument) // a commit without a batch id
}

func TestImportStudentsWithoutTheServiceIsUnimplemented(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.ImportStudents(context.Background(), connect.NewRequest(&studentsv1.ImportStudentsRequest{}))
	assertCode(t, err, connect.CodeUnimplemented)
}
