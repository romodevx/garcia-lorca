//go:build integration

package postgres_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// A pupil's photo reference round-trips, is replaced by a second save, is
// removed, and is invisible to — and unwritable by — another tenant.
func TestPhotoRefRoundTripsAndIsTenantIsolated(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const id = "aaaaaaaa-0000-0000-0000-0000000005d1"
	if err := repo.Create(ctx, applicant(t, id, tenantA, "FOTA050312MDFRNN01", "1° A"), testNumberYear); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.PhotoRef(ctx, id); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("no photo yet: want KindNotFound, got %v", err)
	}
	for _, key := range []string{"t/students/x/1", "t/students/x/2"} {
		if err := repo.SavePhotoRef(ctx, id, ports.PhotoRef{Key: key, ContentType: "image/jpeg", Size: 1234}); err != nil {
			t.Fatalf("SavePhotoRef(%s): %v", key, err)
		}
	}
	got, err := repo.PhotoRef(ctx, id)
	if err != nil || got.Key != "t/students/x/2" || got.ContentType != "image/jpeg" || got.Size != 1234 {
		t.Fatalf("PhotoRef = (%+v, %v)", got, err)
	}

	if _, err := appRepo(t, tenantB).PhotoRef(ctx, id); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B read tenant A's photo reference (RLS read breach): %v", err)
	}
	if err := appRepo(t, tenantB).SavePhotoRef(ctx, id, ports.PhotoRef{Key: "evil", ContentType: "image/png", Size: 1}); err == nil {
		t.Error("tenant B wrote a photo for tenant A's pupil (RLS write breach)")
	}

	if err := repo.DeletePhotoRef(ctx, id); err != nil {
		t.Fatalf("DeletePhotoRef: %v", err)
	}
	if _, err := repo.PhotoRef(ctx, id); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("after delete: want KindNotFound, got %v", err)
	}
}
