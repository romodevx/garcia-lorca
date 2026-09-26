package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

const historyStudent = "dddddddd-0000-0000-0000-000000000701"

// ListTransfers answers a pupil's group moves; a malformed id is the caller's
// mistake, never a database error.
func TestListTransfers(t *testing.T) {
	svc, _, enrolls, _, _ := newEnrollService(t)
	move, err := domain.NewEnrollmentTransfer("tr-1", "t", "enr-1", "grp-1", "grp-2", "balanceo", time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	enrolls.history = map[string][]domain.EnrollmentTransfer{historyStudent: {move}}

	got, err := svc.ListTransfers(context.Background(), historyStudent)
	if err != nil {
		t.Fatalf("ListTransfers: %v", err)
	}
	if len(got) != 1 || got[0].ID() != "tr-1" {
		t.Fatalf("got %+v, want the pupil's one move", got)
	}

	for _, bad := range []string{"", "nope"} {
		if _, err := svc.ListTransfers(context.Background(), bad); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("student id %q: want KindInvalid, got %v", bad, err)
		}
	}
}
