//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

func withBankReference(t *testing.T, s *domain.Student, raw string) *domain.Student {
	t.Helper()
	ref, err := domain.NewBankReference(raw)
	if err != nil {
		t.Fatalf("bank reference %q: %v", raw, err)
	}
	s.ChangeBankReference(ref)
	return s
}

func TestBankReferenceIsStoredAndRead(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const id = "aaaaaaaa-0b00-0000-0000-000000000001"
	if err := repo.Create(ctx, withBankReference(t, applicant(t, id, tenantA, "BAAC050312MDFRNN05", "1° A"), "7700112233")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.FindByID(ctx, id)
	if err != nil || got.BankReference() != "7700112233" {
		t.Fatalf("FindByID bank reference = %q (err %v)", got.BankReference(), err)
	}
	listed, _, err := repo.List(ctx, ports.ListFilter{IDs: []string{id}})
	if err != nil || len(listed) != 1 || listed[0].BankReference() != "7700112233" {
		t.Fatalf("List lost the bank reference: %v (err %v)", listed, err)
	}

	// UpdateDetails writes it, and clears it.
	got.ChangeBankReference("")
	if err := repo.UpdateDetails(ctx, got); err != nil {
		t.Fatalf("UpdateDetails (clear): %v", err)
	}
	cleared, _ := repo.FindByID(ctx, id)
	if !cleared.BankReference().IsZero() {
		t.Fatalf("cleared reference reads %q", cleared.BankReference())
	}
}

// A unique violation is told apart by its constraint: a taken bank reference
// is NOT "this CURP already exists".
func TestATakenBankReferenceIsItsOwnConflict(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	first := withBankReference(t, applicant(t, "aaaaaaaa-0b00-0000-0000-000000000010", tenantA, "BAAD050312MDFRNN06", "1° A"), "5500")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}

	second := withBankReference(t, applicant(t, "aaaaaaaa-0b00-0000-0000-000000000011", tenantA, "BAAE050312MDFRNN07", "1° A"), "5500")
	err := repo.Create(ctx, second)
	if !errors.Is(err, domain.ErrBankReferenceTaken) || glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("Create with a taken reference: err = %v; want ErrBankReferenceTaken", err)
	}

	third := applicant(t, "aaaaaaaa-0b00-0000-0000-000000000012", tenantA, "BAAF050312MDFRNN08", "1° A")
	if err := repo.Create(ctx, third); err != nil {
		t.Fatalf("Create third: %v", err)
	}
	withBankReference(t, third, "5500")
	if err := repo.UpdateDetails(ctx, third); !errors.Is(err, domain.ErrBankReferenceTaken) {
		t.Fatalf("UpdateDetails with a taken reference: err = %v; want ErrBankReferenceTaken", err)
	}

	dupCURP := applicant(t, "aaaaaaaa-0b00-0000-0000-000000000013", tenantA, "BAAD050312MDFRNN06", "1° A")
	if err := repo.Create(ctx, dupCURP); !errors.Is(err, domain.ErrCURPTaken) || errors.Is(err, domain.ErrBankReferenceTaken) {
		t.Fatalf("Create with a taken CURP: err = %v; want ErrCURPTaken only", err)
	}
}

func TestEnrollmentOriginRoundTrips(t *testing.T) {
	ctx := context.Background()
	sid := seedStudent(t, tenantA, "aaaaaaaa-0b00-0000-0000-000000000020", "BAAG050312MDFRNN09")
	repo := appRepo(t, tenantA)
	e := enrollment(t, "e0b00000-0000-0000-0000-000000000001", tenantA, sid, groupOne).WithOrigin(domain.EnrollmentOriginImport)
	if err := repo.CreateEnrollment(ctx, e); err != nil {
		t.Fatalf("CreateEnrollment: %v", err)
	}
	got, err := repo.GetEnrollment(ctx, e.ID())
	if err != nil || got.Origin() != domain.EnrollmentOriginImport {
		t.Fatalf("GetEnrollment origin = %q (err %v); want import", got.Origin(), err)
	}
	listed, err := repo.ListEnrollments(ctx, sid, "", "")
	if err != nil || len(listed) != 1 || listed[0].Origin() != domain.EnrollmentOriginImport {
		t.Fatalf("ListEnrollments = %v (err %v); want one import enrollment", listed, err)
	}
}
