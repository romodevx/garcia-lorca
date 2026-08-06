package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestEnrollmentStatusStringAndValid(t *testing.T) {
	if domain.EnrollmentStatusActive.String() != "active" ||
		domain.EnrollmentStatusTransferred.String() != "transferred" ||
		domain.EnrollmentStatusWithdrawn.String() != "withdrawn" {
		t.Errorf("unexpected status names: %q %q %q",
			domain.EnrollmentStatusActive, domain.EnrollmentStatusTransferred, domain.EnrollmentStatusWithdrawn)
	}
	if !domain.EnrollmentStatusActive.Valid() || !domain.EnrollmentStatusWithdrawn.Valid() {
		t.Error("active and withdrawn are valid")
	}
	if domain.EnrollmentStatusUnspecified.Valid() {
		t.Error("unspecified is not valid")
	}
	if domain.EnrollmentStatusUnspecified.String() != "" {
		t.Errorf("unspecified String() = %q, want empty", domain.EnrollmentStatusUnspecified.String())
	}
}

func TestParseEnrollmentStatus(t *testing.T) {
	got, err := domain.ParseEnrollmentStatus("active")
	if err != nil || got != domain.EnrollmentStatusActive {
		t.Errorf("ParseEnrollmentStatus(active) = %v, %v", got, err)
	}
	if _, err := domain.ParseEnrollmentStatus("bogus"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("ParseEnrollmentStatus(bogus): want KindInvalid, got %v", err)
	}
}

func validEnrollment(t *testing.T) domain.Enrollment {
	t.Helper()
	e, err := domain.NewEnrollment("enr-1", "t1", "stu-1", "cyc-1", "grp-1", "term-1",
		domain.EnrollmentStatusActive, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewEnrollment: %v", err)
	}
	return e
}

func TestNewEnrollmentAccessors(t *testing.T) {
	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	e, err := domain.NewEnrollment("enr-1", "t1", "stu-1", "cyc-1", "grp-1", "term-1",
		domain.EnrollmentStatusActive, at)
	if err != nil {
		t.Fatalf("NewEnrollment: %v", err)
	}
	if e.ID() != "enr-1" || e.TenantID() != "t1" || e.StudentID() != "stu-1" ||
		e.CycleID() != "cyc-1" || e.GroupID() != "grp-1" || e.TermID() != "term-1" ||
		e.Status() != domain.EnrollmentStatusActive || !e.EnrolledAt().Equal(at) {
		t.Errorf("accessors mismatch: %+v", e)
	}
}

func TestNewEnrollmentTermOptional(t *testing.T) {
	e, err := domain.NewEnrollment("enr-1", "t1", "stu-1", "cyc-1", "grp-1", "",
		domain.EnrollmentStatusActive, time.Now())
	if err != nil {
		t.Fatalf("empty termID must be allowed: %v", err)
	}
	if e.TermID() != "" {
		t.Errorf("TermID() = %q, want empty", e.TermID())
	}
}

func TestNewEnrollmentValidation(t *testing.T) {
	at := time.Now()
	cases := []struct {
		name                                      string
		id, tenantID, studentID, cycleID, groupID string
		status                                    domain.EnrollmentStatus
	}{
		{"empty id", "", "t1", "stu-1", "cyc-1", "grp-1", domain.EnrollmentStatusActive},
		{"empty tenant", "enr-1", "", "stu-1", "cyc-1", "grp-1", domain.EnrollmentStatusActive},
		{"empty student", "enr-1", "t1", "", "cyc-1", "grp-1", domain.EnrollmentStatusActive},
		{"empty cycle", "enr-1", "t1", "stu-1", "", "grp-1", domain.EnrollmentStatusActive},
		{"empty group", "enr-1", "t1", "stu-1", "cyc-1", "", domain.EnrollmentStatusActive},
		{"invalid status", "enr-1", "t1", "stu-1", "cyc-1", "grp-1", domain.EnrollmentStatusUnspecified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewEnrollment(tc.id, tc.tenantID, tc.studentID, tc.cycleID, tc.groupID, "term-1", tc.status, at)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestEnrollmentGuards(t *testing.T) {
	active := validEnrollment(t)
	if !active.CanTransfer() || !active.CanWithdraw() {
		t.Error("an active enrollment can transfer and withdraw")
	}

	withdrawn, err := domain.NewEnrollment("enr-2", "t1", "stu-1", "cyc-1", "grp-1", "",
		domain.EnrollmentStatusWithdrawn, time.Now())
	if err != nil {
		t.Fatalf("NewEnrollment: %v", err)
	}
	if withdrawn.CanTransfer() || withdrawn.CanWithdraw() {
		t.Error("a withdrawn enrollment cannot transfer or withdraw")
	}

	transferred, err := domain.NewEnrollment("enr-3", "t1", "stu-1", "cyc-1", "grp-1", "",
		domain.EnrollmentStatusTransferred, time.Now())
	if err != nil {
		t.Fatalf("NewEnrollment: %v", err)
	}
	if transferred.CanTransfer() || transferred.CanWithdraw() {
		t.Error("a transferred enrollment cannot transfer or withdraw")
	}
}
