//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/garcia-lorca/services/students/internal/domain"
)

// A pupil with no medical record reads as an empty one; a saved record
// round-trips; and a second save replaces the first.
func TestMedicalRecordRoundTrips(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const id = "aaaaaaaa-0000-0000-0000-0000000005b1"
	if err := repo.Create(ctx, applicant(t, id, tenantA, "MEDA050312MDFRNN01", "1° A")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	empty, err := repo.GetMedical(ctx, id)
	if err != nil || empty.BloodType() != "" {
		t.Fatalf("GetMedical before any save = (%+v, %v)", empty, err)
	}
	for _, bt := range []string{"O+", "AB-"} {
		rec, _ := domain.NewMedicalRecord(bt, "Penicilina", "Asma")
		if err := repo.SaveMedical(ctx, id, rec); err != nil {
			t.Fatalf("SaveMedical(%s): %v", bt, err)
		}
	}
	got, err := repo.GetMedical(ctx, id)
	if err != nil {
		t.Fatalf("GetMedical: %v", err)
	}
	if got.BloodType() != "AB-" || got.Allergies() != "Penicilina" || got.Conditions() != "Asma" {
		t.Errorf("record = %q %q %q", got.BloodType(), got.Allergies(), got.Conditions())
	}
}

// RLS: tenant B can neither read nor overwrite tenant A's medical record.
func TestMedicalRecordIsTenantIsolated(t *testing.T) {
	ctx := context.Background()
	const id = "aaaaaaaa-0000-0000-0000-0000000005b2"
	if err := appRepo(t, tenantA).Create(ctx, applicant(t, id, tenantA, "MEDB050312MDFRNN02", "1° A")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rec, _ := domain.NewMedicalRecord("O+", "Nueces", "")
	if err := appRepo(t, tenantA).SaveMedical(ctx, id, rec); err != nil {
		t.Fatalf("SaveMedical A: %v", err)
	}

	seen, err := appRepo(t, tenantB).GetMedical(ctx, id)
	if err == nil && seen.Allergies() != "" {
		t.Fatal("tenant B read tenant A's medical record (RLS read breach)")
	}
	intruder, _ := domain.NewMedicalRecord("A-", "nada", "")
	if err := appRepo(t, tenantB).SaveMedical(ctx, id, intruder); err == nil {
		t.Error("tenant B wrote tenant A's medical record (RLS write breach)")
	}
	still, _ := appRepo(t, tenantA).GetMedical(ctx, id)
	if still.BloodType() != "O+" {
		t.Errorf("tenant A's record changed to %q", still.BloodType())
	}
}
