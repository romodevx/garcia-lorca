package usecase_test

import (
	"context"
	"errors"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// TestResendInvitationMailsTheExistingUser: the person never got their link, or
// forgot their password. Same operation, and it must not touch anything else.
func TestResendInvitationMailsTheExistingUser(t *testing.T) {
	prov := &fakeProvisioner{}
	svc, _ := inviteService(t, prov)
	invited := seedInvited(t, svc, "rezagada@garcialorca.mx")
	// Forget the seed invitation: these assertions are about the RESEND.
	prov.created, prov.sent = nil, nil

	if err := svc.ResendInvitation(context.Background(), invited); err != nil {
		t.Fatalf("resend: %v", err)
	}
	if len(prov.sent) != 1 || prov.sent[0] != "rezagada@garcialorca.mx" {
		t.Fatalf("sent = %v", prov.sent)
	}
	// No second account, and no row rewritten: a resend is a mail, nothing else.
	if len(prov.created) != 0 {
		t.Fatalf("resend must not create an account: %v", prov.created)
	}
}

// TestResendInvitationForAnUnknownUserIsNotFound. Keyed by id precisely so it
// cannot be aimed at an address that is not already a user here.
func TestResendInvitationForAnUnknownUserIsNotFound(t *testing.T) {
	prov := &fakeProvisioner{}
	svc, _ := inviteService(t, prov)

	err := svc.ResendInvitation(context.Background(), "no-existe")
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("kind = %v, want NotFound (err=%v)", glerrors.KindOf(err), err)
	}
	if len(prov.sent) != 0 {
		t.Fatal("nothing may be mailed for an unknown user")
	}
}

// TestResendInvitationWithoutProviderIsInternal — the environment has no
// provider, so there is no link to send and no pretending otherwise.
func TestResendInvitationWithoutProviderIsInternal(t *testing.T) {
	// No provider at all: there is no link to send, and the refusal must say so
	// rather than report a success nobody can act on.
	svc, _ := inviteService(t, nil)

	err := svc.ResendInvitation(context.Background(), "cualquiera")
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("kind = %v, want Internal (err=%v)", glerrors.KindOf(err), err)
	}
}

// TestResendInvitationReportsAFailedSend: silence here means the person keeps
// waiting for a mail nobody knows never went.
func TestResendInvitationReportsAFailedSend(t *testing.T) {
	prov := &fakeProvisioner{}
	svc, _ := inviteService(t, prov)
	invited := seedInvited(t, svc, "otra@garcialorca.mx")
	prov.sendErr = errors.New("quota exceeded")

	if err := svc.ResendInvitation(context.Background(), invited); err == nil {
		t.Fatal("a failed send must be reported")
	}
}

// seedInvited creates a real invited user through the service, so the resend
// tests run against the state invitations actually produce.
func seedInvited(t *testing.T, svc *usecase.AdminService, email string) string {
	t.Helper()
	u, err := svc.InviteUser(context.Background(), email, "Persona", nil)
	if err != nil {
		t.Fatalf("seed invite: %v", err)
	}
	return u.ID()
}
