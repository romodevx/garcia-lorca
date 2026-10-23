//go:build integration

package postgres_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// An edited contact round-trips with its emergency mark; a removed one is gone;
// another tenant can do neither.
func TestContactEditAndRemove(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000005c1", "CONA050312MDFRNN01")
	c, err := domain.NewContact("c5000000-0000-0000-0000-000000000001", tenantA, studentID, domain.RelationshipOther,
		domain.ContactPerson{FullName: "Rosa"})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if err := repo.Link(ctx, c); err != nil {
		t.Fatalf("Link: %v", err)
	}

	edited, _ := c.Edit(domain.RelationshipGuardian, domain.ContactPerson{FullName: "Rosa Méndez", Phone: "3312345678", Emergency: true})
	if err := appRepo(t, tenantB).UpdateContact(ctx, edited); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B edited tenant A's contact (RLS write breach): %v", err)
	}
	if err := repo.UpdateContact(ctx, edited); err != nil {
		t.Fatalf("UpdateContact: %v", err)
	}
	got, err := repo.ByID(ctx, c.ID())
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.FullName() != "Rosa Méndez" || got.Relationship() != domain.RelationshipGuardian || !got.IsEmergency() || got.Phone() != "3312345678" {
		t.Errorf("stored = %q %v %v %q", got.FullName(), got.Relationship(), got.IsEmergency(), got.Phone())
	}

	if err := appRepo(t, tenantB).RemoveContact(ctx, c.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B removed tenant A's contact (RLS write breach): %v", err)
	}
	if err := repo.RemoveContact(ctx, c.ID()); err != nil {
		t.Fatalf("RemoveContact: %v", err)
	}
	if _, err := repo.ByID(ctx, c.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("the contact is still there: %v", err)
	}
}

// The service checks "no account" and then deletes; access granted in between
// must not be deleted with the row. The DELETE carries the condition itself.
func TestRemoveContactRefusesAContactThatGainedAnAccount(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000005c2", "CONB050312MDFRNN02")
	c, err := domain.NewContact("c5000000-0000-0000-0000-000000000002", tenantA, studentID, domain.RelationshipMother,
		domain.ContactPerson{FullName: "Lucía"})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if err := repo.Link(ctx, c); err != nil {
		t.Fatalf("Link: %v", err)
	}
	// Its own guardian id: other tests count the links of guardianOne.
	granted, err := c.WithAccount("60000000-0000-0000-0000-0000000005c2")
	if err != nil {
		t.Fatalf("WithAccount: %v", err)
	}
	if err := attachOnPupil(ctx, repo, granted); err != nil {
		t.Fatalf("AttachAccount: %v", err)
	}

	if err := repo.RemoveContact(ctx, c.ID()); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("removing a contact with an account: want KindConflict, got %v", err)
	}
	if _, err := repo.ByID(ctx, c.ID()); err != nil {
		t.Errorf("the contact with an account was deleted: %v", err)
	}
}
