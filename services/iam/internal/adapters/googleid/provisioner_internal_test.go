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
