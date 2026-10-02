//go:build integration

package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/comms/internal/service"
)

const userTemplate = "b0000000-0000-0000-0000-0000000000b1"

// T11: the school's greeting is one row per tenant — absent until saved,
// replaced on every save, and invisible to another tenant (RLS).
func TestBirthdayTemplateRoundTripAndIsolation(t *testing.T) {
	ctx := context.Background()
	a := appRepo(t, tenantA)

	if _, found, err := a.BirthdayTemplate(ctx); err != nil || found {
		t.Fatalf("before saving: found=%v err=%v", found, err)
	}

	first := service.BirthdayTemplate{Title: "¡Feliz día!", Body: "Felicidades, {nombre}.", UpdatedAt: seedNow, UpdatedBy: userTemplate}
	if _, err := a.SaveBirthdayTemplate(ctx, first); err != nil {
		t.Fatalf("save: %v", err)
	}
	second := first
	second.Body = "Muchas felicidades, {nombre}."
	second.UpdatedAt = seedNow.Add(time.Hour)
	saved, err := a.SaveBirthdayTemplate(ctx, second)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if saved.Body != second.Body || saved.IsDefault {
		t.Errorf("saved = %+v", saved)
	}

	got, found, err := a.BirthdayTemplate(ctx)
	if err != nil || !found {
		t.Fatalf("read back: found=%v err=%v", found, err)
	}
	if got.Title != "¡Feliz día!" || got.Body != second.Body || got.UpdatedBy != userTemplate || !got.UpdatedAt.Equal(second.UpdatedAt) {
		t.Errorf("read back = %+v", got)
	}
	if n := countRows(t, tenantA, "birthday_templates", "true"); n != 1 {
		t.Errorf("rows = %d, want one per tenant", n)
	}

	if _, found, err := appRepo(t, tenantB).BirthdayTemplate(ctx); err != nil || found {
		t.Errorf("tenant B sees tenant A's greeting: found=%v err=%v", found, err)
	}
}
