package usecase_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// Task 12: the school's details, as a receipt prints them. A school that has not
// filled them yet reads as "not set", so the receipt can fall back.
func TestSchoolProfileIsSetAndRead(t *testing.T) {
	svc, _ := newService(t)

	if _, found, err := svc.GetSchoolProfile(ctx()); err != nil || found {
		t.Fatalf("before any save: found %v, err %v; want not set and no error", found, err)
	}

	saved, err := svc.SetSchoolProfile(ctx(), domain.SchoolProfileInput{Name: "Colegio García Lorca", RFC: "egl150101ab1"})
	if err != nil {
		t.Fatalf("SetSchoolProfile: %v", err)
	}
	if saved.RFC() != "EGL150101AB1" {
		t.Errorf("saved RFC = %q, want it upper-cased", saved.RFC())
	}
	got, found, err := svc.GetSchoolProfile(ctx())
	if err != nil || !found || got.Name() != "Colegio García Lorca" {
		t.Errorf("read back: %q, found %v, err %v", got.Name(), found, err)
	}

	if _, err := svc.SetSchoolProfile(ctx(), domain.SchoolProfileInput{Name: ""}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("a profile with no name: err = %v, want KindInvalid", err)
	}
}
