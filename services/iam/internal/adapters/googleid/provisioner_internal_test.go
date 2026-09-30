package googleid

import (
	"context"
	"errors"
	"reflect"
	"testing"

	firebaseauth "firebase.google.com/go/v4/auth"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Internal test: the SDK's "user not found" error cannot be built outside the
// SDK, so the predicate is swapped here and the mapping is what gets proved.

var errNoSuchUser = errors.New("no such user")

type fakeSDK struct {
	byEmail   map[string]string // email -> uid
	updated   map[string]*firebaseauth.UserToUpdate
	updateErr error
	revoked   []string // uids whose provider refresh tokens were revoked
}

func (f *fakeSDK) RevokeRefreshTokens(_ context.Context, uid string) error {
	f.revoked = append(f.revoked, uid)
	return nil
}

func (f *fakeSDK) CreateUser(context.Context, *firebaseauth.UserToCreate) (*firebaseauth.UserRecord, error) {
	return nil, errors.New("not used")
}

func (f *fakeSDK) GetUserByEmail(_ context.Context, email string) (*firebaseauth.UserRecord, error) {
	uid, ok := f.byEmail[email]
	if !ok {
		return nil, errNoSuchUser
	}
	return &firebaseauth.UserRecord{UserInfo: &firebaseauth.UserInfo{UID: uid, Email: email}}, nil
}

func (f *fakeSDK) UpdateUser(_ context.Context, uid string, user *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	if f.updated == nil {
		f.updated = map[string]*firebaseauth.UserToUpdate{}
	}
	f.updated[uid] = user
	return &firebaseauth.UserRecord{}, nil
}

func newTestProvisioner(sdk *fakeSDK) *Provisioner {
	p := NewProvisioner(sdk, nil)
	p.userNotFound = func(err error) bool { return errors.Is(err, errNoSuchUser) }
	return p
}

func TestSetDisabledTogglesOnlyTheDisabledFlag(t *testing.T) {
	for _, disabled := range []bool{true, false} {
		sdk := &fakeSDK{byEmail: map[string]string{"ana@gl.mx": "uid-ana"}}

		if err := newTestProvisioner(sdk).SetDisabled(context.Background(), "ana@gl.mx", disabled); err != nil {
			t.Fatalf("SetDisabled(%v): %v", disabled, err)
		}
		want := (&firebaseauth.UserToUpdate{}).Disabled(disabled)
		if got := sdk.updated["uid-ana"]; !reflect.DeepEqual(got, want) {
			t.Errorf("SetDisabled(%v) sent %+v, want only Disabled(%v) — nothing else about the account changes", disabled, got, disabled)
		}
	}
}

func TestSetDisabledErrors(t *testing.T) {
	cases := []struct {
		name     string
		sdk      *fakeSDK
		wantKind glerrors.Kind
	}{
		{"no account at the provider", &fakeSDK{}, glerrors.KindNotFound},
		{"provider refuses the update", &fakeSDK{byEmail: map[string]string{"ana@gl.mx": "uid"}, updateErr: errors.New("503")}, glerrors.KindInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := newTestProvisioner(tc.sdk).SetDisabled(context.Background(), "ana@gl.mx", true)
			if glerrors.KindOf(err) != tc.wantKind {
				t.Errorf("err = %v, want kind %v", err, tc.wantKind)
			}
		})
	}
}

// Re-enabling an account that was deleted must not bring its old password
// back: the credential is replaced with a random one nobody is told, so the
// account opens only from the new invitation.
func TestReenableTurnsSignInOnWithAFreshUnknownPassword(t *testing.T) {
	sdk := &fakeSDK{byEmail: map[string]string{"ana@gl.mx": "uid-ana"}}
	p := newTestProvisioner(sdk)
	p.randomPassword = func() (string, error) { return "random-credential", nil }

	if err := p.Reenable(context.Background(), "ana@gl.mx"); err != nil {
		t.Fatalf("Reenable: %v", err)
	}
	want := (&firebaseauth.UserToUpdate{}).Disabled(false).Password("random-credential")
	if got := sdk.updated["uid-ana"]; !reflect.DeepEqual(got, want) {
		t.Errorf("Reenable sent %+v, want Disabled(false) and a fresh password", got)
	}
	if err := newTestProvisioner(&fakeSDK{}).Reenable(context.Background(), "x@gl.mx"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("no provider account = %v, want KindNotFound", err)
	}
}

func TestRandomPasswordIsLongAndUnique(t *testing.T) {
	a, err := randomPassword()
	if err != nil {
		t.Fatalf("randomPassword: %v", err)
	}
	b, _ := randomPassword()
	if len(a) < 32 || a == b {
		t.Errorf("randomPassword gave %q and %q, want two different values of at least 32 chars", a, b)
	}
}

// Switching an account off or back on also ends every provider session it
// had: a provider refresh token kept minting ID tokens otherwise, and an ID
// token is enough to activate an invited account.
func TestDisableAndReenableRevokeProviderSessions(t *testing.T) {
	sdk := &fakeSDK{byEmail: map[string]string{"ana@gl.mx": "uid-ana"}}
	p := newTestProvisioner(sdk)

	if err := p.SetDisabled(context.Background(), "ana@gl.mx", true); err != nil {
		t.Fatalf("SetDisabled: %v", err)
	}
	if err := p.Reenable(context.Background(), "ana@gl.mx"); err != nil {
		t.Fatalf("Reenable: %v", err)
	}
	if len(sdk.revoked) != 2 {
		t.Errorf("revoked sessions %v, want once on disable and once on re-enable", sdk.revoked)
	}
}
