package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// ADR 0033: the BBVA reference travels in and out of every Student.
func TestTheBankReferenceRoundTrips(t *testing.T) {
	h, _ := newHandler(t)
	created, err := h.CreateApplicant(context.Background(), connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp: validCURP, FirstName: "Ana", LastName: "Herrera",
		LevelId: "e1000000-0000-0000-0000-000000000001", BankReference: " 0012345 ",
	}))
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}
	if got := created.Msg.GetStudent().GetBankReference(); got != "0012345" {
		t.Fatalf("created bank_reference = %q; want 0012345", got)
	}

	// Unset keeps it.
	kept, err := h.UpdateStudent(context.Background(), connect.NewRequest(&studentsv1.UpdateStudentRequest{
		Id: "stu-1", FirstName: "Ana", LastName: "Herrera", Curp: validCURP,
	}))
	if err != nil {
		t.Fatalf("UpdateStudent (unset): %v", err)
	}
	if got := kept.Msg.GetStudent().GetBankReference(); got != "0012345" {
		t.Fatalf("unset bank_reference must keep it; got %q", got)
	}

	// "" clears it.
	cleared, err := h.UpdateStudent(context.Background(), connect.NewRequest(&studentsv1.UpdateStudentRequest{
		Id: "stu-1", FirstName: "Ana", LastName: "Herrera", Curp: validCURP, BankReference: proto.String(""),
	}))
	if err != nil {
		t.Fatalf("UpdateStudent (clear): %v", err)
	}
	if got := cleared.Msg.GetStudent().GetBankReference(); got != "" {
		t.Fatalf("empty bank_reference must clear it; got %q", got)
	}

	got, err := h.GetStudent(context.Background(), connect.NewRequest(&studentsv1.GetStudentRequest{Id: "stu-1"}))
	if err != nil || got.Msg.GetStudent().GetBankReference() != "" {
		t.Fatalf("GetStudent bank_reference = %q (err %v); want cleared", got.Msg.GetStudent().GetBankReference(), err)
	}
}

func TestAMalformedBankReferenceIsInvalidArgument(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.CreateApplicant(context.Background(), connect.NewRequest(&studentsv1.CreateApplicantRequest{
		Curp: validCURP, FirstName: "Ana", LastName: "Herrera",
		LevelId: "e1000000-0000-0000-0000-000000000001", BankReference: "S/R",
	}))
	assertCode(t, err, connect.CodeInvalidArgument)

	createValid(t, h)
	_, err = h.UpdateStudent(context.Background(), connect.NewRequest(&studentsv1.UpdateStudentRequest{
		Id: "stu-1", FirstName: "Ana", LastName: "Herrera", Curp: validCURP, BankReference: proto.String("12-34"),
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

// Finance reads Enrollment.origin to skip an imported pupil's inscription:
// every enrollment on the wire must carry it.
func TestEveryEnrollmentCarriesItsOrigin(t *testing.T) {
	enrollments := newFakeEnrollmentRepo()
	h, studentID := enrollmentHandlerWith(t, enrollments)
	manual, err := h.Enroll(context.Background(), connect.NewRequest(&studentsv1.EnrollRequest{StudentId: studentID, GroupId: enrollGroup}))
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if got := manual.Msg.GetEnrollment().GetOrigin(); got != "manual" {
		t.Fatalf("Enroll origin = %q; want manual", got)
	}

	imported, err := domain.NewEnrollment("enr-imp", testTenant, "stu-imp", enrollCycle, enrollGroup, "",
		domain.EnrollmentStatusActive, time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewEnrollment: %v", err)
	}
	importedAt := time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC)
	enrollments.byID["enr-imp"] = imported.WithOrigin(domain.EnrollmentOriginImport).WithImportedAt(importedAt)
	if got := manual.Msg.GetEnrollment().GetImportedAt(); got != nil {
		t.Fatalf("a manual enrollment carries imported_at %v", got)
	}

	one, err := h.GetEnrollment(context.Background(), connect.NewRequest(&studentsv1.GetEnrollmentRequest{Id: "enr-imp"}))
	if err != nil || one.Msg.GetEnrollment().GetOrigin() != "import" {
		t.Fatalf("GetEnrollment origin = %q (err %v); want import", one.Msg.GetEnrollment().GetOrigin(), err)
	}
	if got := one.Msg.GetEnrollment().GetImportedAt(); got == nil || !got.AsTime().Equal(importedAt) {
		t.Fatalf("GetEnrollment imported_at = %v; want %v", got, importedAt)
	}
	list, err := h.ListEnrollments(context.Background(), connect.NewRequest(&studentsv1.ListEnrollmentsRequest{StudentId: "stu-imp"}))
	if err != nil || len(list.Msg.GetEnrollments()) != 1 || list.Msg.GetEnrollments()[0].GetOrigin() != "import" {
		t.Fatalf("ListEnrollments = %+v (err %v); want one import enrollment", list.Msg.GetEnrollments(), err)
	}
}
