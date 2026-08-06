package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestNewEnrollmentTransferAccessors(t *testing.T) {
	at := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	tr, err := domain.NewEnrollmentTransfer("mov-1", "t1", "enr-1", "grp-1", "grp-2", "family relocation", at)
	if err != nil {
		t.Fatalf("NewEnrollmentTransfer: %v", err)
	}
	if tr.ID() != "mov-1" || tr.TenantID() != "t1" || tr.EnrollmentID() != "enr-1" ||
		tr.FromGroupID() != "grp-1" || tr.ToGroupID() != "grp-2" ||
		tr.Reason() != "family relocation" || !tr.MovedAt().Equal(at) {
		t.Errorf("accessors mismatch: %+v", tr)
	}
}

func TestNewEnrollmentTransferValidation(t *testing.T) {
	at := time.Now()
	cases := []struct {
		name                                                   string
		id, tenantID, enrollmentID, fromGroup, toGroup, reason string
	}{
		{"empty id", "", "t1", "enr-1", "grp-1", "grp-2", "why"},
		{"empty tenant", "mov-1", "", "enr-1", "grp-1", "grp-2", "why"},
		{"empty enrollment", "mov-1", "t1", "", "grp-1", "grp-2", "why"},
		{"empty fromGroup", "mov-1", "t1", "enr-1", "", "grp-2", "why"},
		{"empty toGroup", "mov-1", "t1", "enr-1", "grp-1", "", "why"},
		{"empty reason", "mov-1", "t1", "enr-1", "grp-1", "grp-2", ""},
		{"same group", "mov-1", "t1", "enr-1", "grp-1", "grp-1", "why"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewEnrollmentTransfer(tc.id, tc.tenantID, tc.enrollmentID, tc.fromGroup, tc.toGroup, tc.reason, at)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}
