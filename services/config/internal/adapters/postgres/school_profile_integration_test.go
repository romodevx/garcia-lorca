//go:build integration

package postgres_test

import (
	"context"
	"testing"

	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// Task 12: one set of details per school, replaced on save, and invisible to
// another school (RLS).
func TestSchoolProfileUpsertAndIsolation(t *testing.T) {
	ctx := context.Background()
	repoA := pgadapter.NewRepository(appDB(t), tenantA)

	if _, found, err := repoA.GetSchoolProfile(ctx); err != nil || found {
		t.Fatalf("before any save: found %v, err %v", found, err)
	}
	first, err := domain.NewSchoolProfile(tenantA, domain.SchoolProfileInput{Name: "Colegio A", RFC: "EGL150101AB1", CLABE: "012180001234567899"})
	if err != nil {
		t.Fatalf("NewSchoolProfile: %v", err)
	}
	if err := repoA.UpsertSchoolProfile(ctx, first); err != nil {
		t.Fatalf("first save: %v", err)
	}
	second, err := domain.NewSchoolProfile(tenantA, domain.SchoolProfileInput{Name: "Colegio A renombrado", Legend: "Gracias", CLABE: "032180000118359719"})
	if err != nil {
		t.Fatalf("NewSchoolProfile: %v", err)
	}
	if err := repoA.UpsertSchoolProfile(ctx, second); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, found, err := repoA.GetSchoolProfile(ctx)
	if err != nil || !found {
		t.Fatalf("read back: found %v, err %v", found, err)
	}
	if got.CLABE() != "032180000118359719" {
		t.Errorf("CLABE read back %q; want the second save's", got.CLABE())
	}
	if got.Name() != "Colegio A renombrado" || got.RFC() != "" || got.Legend() != "Gracias" {
		t.Errorf("read back %q rfc %q legend %q; want the second save whole", got.Name(), got.RFC(), got.Legend())
	}

	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	if _, found, err := repoB.GetSchoolProfile(ctx); err != nil || found {
		t.Errorf("tenant B sees A's details: found %v, err %v", found, err)
	}
	if n := writeAsTenant(t, tenantB, "UPDATE school_profile SET name = 'hijacked' WHERE tenant_id = ?", tenantA); n != 0 {
		t.Errorf("tenant B updated %d of A's rows", n)
	}
}
