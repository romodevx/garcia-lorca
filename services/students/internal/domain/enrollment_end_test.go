package domain_test

import (
	"testing"
	"time"

	"github.com/garcia-lorca/services/students/internal/domain"
)

// An enrollment knows when it stopped being active (withdrawn or transferred);
// an active one has not ended, whatever it is told.
func TestEnrollmentEndedAt(t *testing.T) {
	ended := time.Date(2026, 10, 3, 17, 0, 0, 0, time.UTC)
	build := func(status domain.EnrollmentStatus) domain.Enrollment {
		t.Helper()
		e, err := domain.NewEnrollment("enr-1", "t", "stu-1", "cyc-1", "grp-1", "", status, ended.AddDate(0, -1, 0))
		if err != nil {
			t.Fatal(err)
		}
		return e.WithEndedAt(ended)
	}
	if got := build(domain.EnrollmentStatusWithdrawn).EndedAt(); !got.Equal(ended) {
		t.Errorf("withdrawn EndedAt = %v, want %v", got, ended)
	}
	if got := build(domain.EnrollmentStatusActive).EndedAt(); !got.IsZero() {
		t.Errorf("active EndedAt = %v, want zero", got)
	}
}
