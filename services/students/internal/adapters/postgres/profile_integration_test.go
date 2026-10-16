//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/students/internal/domain"
)

// The file's editable half round-trips: names, CURP, birth date, sex, the
// pupil's email and phones, and the address.
func TestUpdateDetailsRoundTrips(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	const id = "aaaaaaaa-0000-0000-0000-0000000005a1"
	if err := repo.Create(ctx, applicant(t, id, tenantA, "PEAA050312MDFRNN01", "1° A"), testNumberYear); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stored, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	profile, err := domain.NewProfile(domain.ProfileInput{
		BirthDate: "2019-03-12", Sex: "F", Email: "ana@correo.mx",
		Phones:  []domain.Phone{{Kind: "celular", Number: "5512345678"}, {Kind: "casa", Number: "3312345678"}},
		Address: domain.Address{Street: "Av. Juárez", ExtNumber: "123", PostalCode: "44100", Municipality: "Guadalajara", State: "Jalisco"},
	}, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	curp, _ := domain.NewCURP("PEAA050312MDFRNN02")
	if err := stored.UpdateDetails("Ana María", "Pérez", "López", curp, profile); err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	if err := repo.UpdateDetails(ctx, stored); err != nil {
		t.Fatalf("repo.UpdateDetails: %v", err)
	}

	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	p := got.Profile()
	if got.FirstName() != "Ana María" || got.CURP().String() != "PEAA050312MDFRNN02" {
		t.Errorf("identity = %q %q", got.FirstName(), got.CURP())
	}
	if got.LastName() != "Pérez" || got.MaternalLastName() != "López" {
		t.Errorf("surnames = %q %q, want Pérez López", got.LastName(), got.MaternalLastName())
	}
	if p.BirthDate().Format("2006-01-02") != "2019-03-12" || p.Sex() != domain.SexFemale || p.Email() != "ana@correo.mx" {
		t.Errorf("profile = %v %q %q", p.BirthDate(), p.Sex(), p.Email())
	}
	if phones := p.Phones(); len(phones) != 2 || phones[1].Kind != "casa" || phones[1].Number != "3312345678" {
		t.Errorf("phones = %+v", phones)
	}
	if a := p.Address(); a.Street != "Av. Juárez" || a.PostalCode != "44100" || a.State != "Jalisco" {
		t.Errorf("address = %+v", a)
	}
}
