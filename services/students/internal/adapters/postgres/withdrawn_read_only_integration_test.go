//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// T14-009 gate: a withdrawal committed between the use case's read and the
// write must still win. The write itself refuses a withdrawn row, so a stale
// in-memory pupil (read as pending) cannot correct a file closed meanwhile.
func TestWritesRefuseAPupilWithdrawnMeanwhile(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const id = "aaaaaaaa-0000-0000-0000-0000000005e1"
	if err := repo.Create(ctx, applicant(t, id, tenantA, "BAJA050312MDFRNN01", "1° A"), testNumberYear); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stale, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if err := repo.SavePhotoRef(ctx, id, ports.PhotoRef{Key: "t/students/x/1", ContentType: "image/jpeg", Size: 1}); err != nil {
		t.Fatalf("SavePhotoRef before the withdrawal: %v", err)
	}

	withdrawing, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if _, err := withdrawing.Withdraw("cambio de escuela"); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if err := repo.Update(ctx, withdrawing); err != nil {
		t.Fatalf("Update (withdrawal): %v", err)
	}

	if _, err := repo.UpdateDetails(ctx, stale, ports.DetailsCorrection{}); !errors.Is(err, domain.ErrWithdrawnReadOnly) {
		t.Errorf("UpdateDetails on a withdrawn row = %v, want ErrWithdrawnReadOnly", err)
	}
	rec, _ := domain.NewMedicalRecord("O+", "", "")
	if err := repo.SaveMedical(ctx, id, rec); !errors.Is(err, domain.ErrWithdrawnReadOnly) {
		t.Errorf("SaveMedical on a withdrawn row = %v, want ErrWithdrawnReadOnly", err)
	}
	if err := repo.SavePhotoRef(ctx, id, ports.PhotoRef{Key: "t/students/x/2", ContentType: "image/jpeg", Size: 1}); !errors.Is(err, domain.ErrWithdrawnReadOnly) {
		t.Errorf("SavePhotoRef on a withdrawn row = %v, want ErrWithdrawnReadOnly", err)
	}
	if err := repo.DeletePhotoRef(ctx, id); !errors.Is(err, domain.ErrWithdrawnReadOnly) {
		t.Errorf("DeletePhotoRef on a withdrawn row = %v, want ErrWithdrawnReadOnly", err)
	}
	if got, err := repo.PhotoRef(ctx, id); err != nil || got.Key != "t/students/x/1" {
		t.Errorf("the photo changed after the withdrawal: (%+v, %v)", got, err)
	}
}
