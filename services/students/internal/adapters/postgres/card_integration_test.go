//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// Task 13: a card code is stored, found by its token, revoked (kept, no longer
// active), and invisible to another school.
func TestCardTokensRoundTripAndIsolation(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	pupil := applicant(t, "aaaaaaaa-0000-0000-0000-0000000013e1", tenantA, "CARE050312MDFRNN07", "")
	if err := repo.Create(ctx, pupil, testNumberYear); err != nil {
		t.Fatalf("Create pupil: %v", err)
	}
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	card, err := domain.NewCardToken("cccccccc-0000-0000-0000-0000000013e1", tenantA, pupil.ID(), [16]byte{9, 9, 9}, at)
	if err != nil {
		t.Fatalf("NewCardToken: %v", err)
	}
	if err := repo.SaveCard(ctx, card); err != nil {
		t.Fatalf("SaveCard: %v", err)
	}

	active, found, err := repo.ActiveCard(ctx, pupil.ID())
	if err != nil || !found || active.Token() != card.Token() {
		t.Fatalf("ActiveCard = %q found %v (%v)", active.Token(), found, err)
	}
	if got, err := repo.FindCard(ctx, card.Token()); err != nil || got.StudentID() != pupil.ID() {
		t.Fatalf("FindCard = %+v (%v)", got, err)
	}
	if _, err := appRepo(t, tenantB).FindCard(ctx, card.Token()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B finds A's card: %v", err)
	}

	if err := repo.RevokeCards(ctx, pupil.ID(), at.Add(time.Hour)); err != nil {
		t.Fatalf("RevokeCards: %v", err)
	}
	if _, found, _ := repo.ActiveCard(ctx, pupil.ID()); found {
		t.Error("a revoked card is still active")
	}
	if ever, err := repo.EverIssued(ctx, pupil.ID()); err != nil || !ever {
		t.Errorf("EverIssued after a revoke = %v (%v), want true", ever, err)
	}
	kept, err := repo.FindCard(ctx, card.Token())
	if err != nil || kept.Active() {
		t.Errorf("the revoked card = active %v (%v); want kept and inactive", kept.Active(), err)
	}
}

// Review of task 13: a card stored already revoked (revoked before the family
// ever opened it) is kept, counts as issued, and is not active.
func TestACardBornRevokedIsStoredRevoked(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	pupil := applicant(t, "aaaaaaaa-0000-0000-0000-0000000013e2", tenantA, "CARF050312MDFRNN08", "")
	if err := repo.Create(ctx, pupil, testNumberYear); err != nil {
		t.Fatalf("Create pupil: %v", err)
	}
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	card, err := domain.NewCardToken("cccccccc-0000-0000-0000-0000000013e2", tenantA, pupil.ID(), [16]byte{4, 4}, at)
	if err != nil {
		t.Fatalf("NewCardToken: %v", err)
	}
	if err := repo.SaveCard(ctx, card.Revoke(at)); err != nil {
		t.Fatalf("SaveCard: %v", err)
	}
	if _, found, _ := repo.ActiveCard(ctx, pupil.ID()); found {
		t.Error("a card born revoked is active")
	}
	if ever, err := repo.EverIssued(ctx, pupil.ID()); err != nil || !ever {
		t.Errorf("EverIssued = %v (%v), want true", ever, err)
	}
}
