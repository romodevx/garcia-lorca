package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// ISSUE-CC-115 — the `guardian` role could be handed out by hand from the staff
// screen (Invitar usuario → guardian → 201). An account in that role means
// nothing without the guardian↔student link students owns, so it is born ONLY
// from "Dar acceso a la app" on the pupil's record, which reaches iam through
// InviteGuardian. Every other road refuses it, in words the director can act on.

const guardianRefusal = "Dar acceso a la app"

func guardianRole(t *testing.T) domain.Role {
	return mustRoleUC(t, domain.GuardianRoleID, "guardian", []string{"family:read"})
}

// newGuardianService builds an admin service whose role catalog holds the
// teacher role and, when withGuardian, the seeded guardian role.
func newGuardianService(t *testing.T, users *fakeUsers, withGuardian bool) *usecase.AdminService {
	t.Helper()
	roles := &fakeRoles{byID: map[string]domain.Role{"r1": teacherRole(t)}}
	if withGuardian {
		roles.byID[domain.GuardianRoleID] = guardianRole(t)
	}
	return usecase.NewAdminService(
		users, roles, &fakeHasher{}, nil, &fakeInvitations{}, &fakeMailer{},
		"https://escuela.example/activar", "t1", seqIDs(), fixedClock(testNow),
		usecase.WithSleeper(func(time.Duration) {}),
	)
}

func TestManualRoadsRefuseTheGuardianRole(t *testing.T) {
	cases := []struct {
		name string
		call func(svc *usecase.AdminService) error
	}{
		{
			name: "inviting a user into guardian",
			call: func(svc *usecase.AdminService) error {
				_, err := svc.InviteUser(context.Background(), "mama@gl.mx", "Mamá", []string{"r1", domain.GuardianRoleID})
				return err
			},
		},
		{
			name: "creating a user with guardian",
			call: func(svc *usecase.AdminService) error {
				_, err := svc.CreateUser(context.Background(), "mama@gl.mx", "Mamá", "temporal-123", []string{domain.GuardianRoleID})
				return err
			},
		},
		{
			name: "the guardian id under another spelling",
			call: func(svc *usecase.AdminService) error {
				_, err := svc.InviteUser(context.Background(), "mama@gl.mx", "Mamá", []string{" " + strings.ToUpper(domain.GuardianRoleID) + " "})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			users := &fakeUsers{byID: map[string]domain.User{}}

			err := tc.call(newGuardianService(t, users, true))

			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("err = %v, want KindInvalid", err)
			}
			if !strings.Contains(glerrors.SafeMessage(err), guardianRefusal) {
				t.Errorf("message = %q, want it to point at %q", glerrors.SafeMessage(err), guardianRefusal)
			}
			if len(users.created) != 0 {
				t.Errorf("a refused request created %v", users.created)
			}
		})
	}
}

func TestAssignRoleRefusesTheGuardianRole(t *testing.T) {
	svc, users := fixtureService(t, domain.StatusActive)

	err := svc.AssignRole(context.Background(), "u1", domain.GuardianRoleID)

	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("err = %v, want KindInvalid", err)
	}
	if !strings.Contains(glerrors.SafeMessage(err), guardianRefusal) {
		t.Errorf("message = %q, want it to point at %q", glerrors.SafeMessage(err), guardianRefusal)
	}
	if len(users.added) != 0 {
		t.Errorf("a refused assignment wrote %v", users.added)
	}
}

func TestInviteGuardianCreatesAnInvitedGuardian(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{}}

	got, err := newGuardianService(t, users, true).InviteGuardian(context.Background(), "Mama@GL.mx", "Mamá de Ana")
	if err != nil {
		t.Fatalf("InviteGuardian: %v", err)
	}

	if got.User.Status() != domain.StatusInvited {
		t.Errorf("status = %v, want invited", got.User.Status())
	}
	if roles := users.rolesSet[got.User.ID()]; len(roles) != 1 || roles[0] != domain.GuardianRoleID {
		t.Errorf("roles = %v, want exactly the guardian role", roles)
	}
	if got.URL == "" {
		t.Error("no invitation link was issued")
	}
}

// A deployment missing migrations/iam/0010 has no guardian role. Inviting into
// it would create a login that can see nothing, so it is refused — as OUR
// fault, not the caller's.
func TestInviteGuardianWithoutTheSeededRoleIsAServerFault(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{}}

	_, err := newGuardianService(t, users, false).InviteGuardian(context.Background(), "mama@gl.mx", "Mamá")

	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("err = %v, want KindInternal", err)
	}
	if len(users.created) != 0 {
		t.Errorf("created %v without a guardian role", users.created)
	}
}
