package usecase_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// mustRole is a test helper that builds a role or fails the test.
func mustRole(t *testing.T, id, name string, perms []string) domain.Role {
	t.Helper()
	r, err := domain.NewRole(id, name, perms)
	if err != nil {
		t.Fatalf("NewRole(%q): %v", name, err)
	}
	return r
}

func newAdmin(users *fakeUsers, roles *fakeRoles, newID func() string) *usecase.AdminService {
	return usecase.NewAdminService(users, roles, fakeHasher{}, nil, testTenant, newID)
}

func TestAdminCreateUserHashesTempPasswordAndAssignsRoles(t *testing.T) {
	users := &fakeUsers{}
	roles := &fakeRoles{byID: map[string]domain.Role{
		"r1": mustRole(t, "r1", "Docente", []string{"students:read"}),
	}}
	svc := newAdmin(users, roles, func() string { return "u-new" })

	user, err := svc.CreateUser(context.Background(), "Nueva@garcialorca.MX", "Nueva Docente", "init-pw-123", []string{"r1"})
	if err != nil {
		t.Fatalf("CreateUser error: %v", err)
	}

	if user.ID() != "u-new" {
		t.Errorf("user id = %q, want %q", user.ID(), "u-new")
	}
	if user.Email().String() != "nueva@garcialorca.mx" {
		t.Errorf("email = %q, want normalized", user.Email().String())
	}
	if user.Status() != domain.StatusActive {
		t.Errorf("status = %v, want StatusActive", user.Status())
	}
	if !user.MustChangePassword() {
		t.Error("MustChangePassword() = false, want true (temp password)")
	}
	read, _ := domain.NewPermission("students:read")
	if !user.HasPermission(read) {
		t.Error("created user should carry the assigned role's permissions")
	}

	if len(users.created) != 1 {
		t.Fatalf("CreateUser persisted %d users, want 1", len(users.created))
	}
	if got := users.created[0].hash; got != "h:init-pw-123" {
		t.Errorf("persisted hash = %q, want the hashed temp password (never plaintext)", got)
	}
	if users.created[0].user.MustChangePassword() != true {
		t.Error("persisted user must_change_password = false, want true")
	}
	if got := users.rolesSet["u-new"]; len(got) != 1 || got[0] != "r1" {
		t.Errorf("SetUserRoles recorded %v, want [r1]", got)
	}
}

func TestAdminCreateUserEmptyTempPasswordIsInvalid(t *testing.T) {
	users := &fakeUsers{}
	roles := &fakeRoles{}
	svc := newAdmin(users, roles, func() string { return "u-new" })

	_, err := svc.CreateUser(context.Background(), "a@garcialorca.mx", "A", "  ", nil)
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
	if len(users.created) != 0 {
		t.Error("no user should be persisted when the temp password is empty")
	}
}

func TestAdminCreateUserDuplicateEmailIsConflict(t *testing.T) {
	dup, _ := domain.NewEmail("taken@garcialorca.mx")
	users := &fakeUsers{byEmail: map[domain.Email]struct {
		user domain.User
		hash string
	}{dup: {}}}
	roles := &fakeRoles{}
	svc := newAdmin(users, roles, func() string { return "u-new" })

	_, err := svc.CreateUser(context.Background(), "taken@garcialorca.mx", "Dup", "init-pw-123", nil)
	if err == nil || glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("want KindConflict, got %v", err)
	}
}

func TestAdminCreateUserUnknownRoleIsInvalidAndDoesNotCreate(t *testing.T) {
	users := &fakeUsers{}
	roles := &fakeRoles{byID: map[string]domain.Role{}}
	svc := newAdmin(users, roles, func() string { return "u-new" })

	_, err := svc.CreateUser(context.Background(), "a@garcialorca.mx", "A", "init-pw-123", []string{"ghost"})
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid for a missing role, got %v", err)
	}
	if len(users.created) != 0 {
		t.Error("user must not be persisted when a requested role does not exist (no role-less inconsistency)")
	}
	if len(users.rolesSet) != 0 {
		t.Error("SetUserRoles must not be called when role validation fails")
	}
}

func TestAdminSetUserStatusSuspends(t *testing.T) {
	users := &fakeUsers{}
	roles := &fakeRoles{}
	svc := newAdmin(users, roles, func() string { return "u-new" })

	if err := svc.SetUserStatus(context.Background(), "u1", domain.StatusSuspended); err != nil {
		t.Fatalf("SetUserStatus error: %v", err)
	}
	if got, ok := users.statusSet["u1"]; !ok || got != domain.StatusSuspended {
		t.Errorf("status for u1 = %v (ok=%v), want StatusSuspended", got, ok)
	}
	// Note: the existing domain User.CanAuthenticate() already blocks login for
	// a suspended account, so status is the single enforcement point.
}

func TestAdminListUsers(t *testing.T) {
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	u, _ := domain.NewUser("u1", testTenant, email, "Ana", domain.StatusActive, false, nil)
	users := &fakeUsers{listOut: []domain.User{u}}
	svc := newAdmin(users, &fakeRoles{}, func() string { return "x" })

	got, err := svc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers error: %v", err)
	}
	if len(got) != 1 || got[0].ID() != "u1" {
		t.Errorf("ListUsers = %v, want [u1]", got)
	}
}

func TestAdminGetUser(t *testing.T) {
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	u, _ := domain.NewUser("u1", testTenant, email, "Ana", domain.StatusActive, false, nil)
	users := &fakeUsers{byID: map[string]domain.User{"u1": u}}
	svc := newAdmin(users, &fakeRoles{}, func() string { return "x" })

	got, err := svc.GetUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetUser error: %v", err)
	}
	if got.ID() != "u1" {
		t.Errorf("GetUser = %q, want u1", got.ID())
	}

	if _, err := svc.GetUser(context.Background(), "ghost"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetUser(ghost): want KindNotFound, got %v", err)
	}
}

func TestAdminGetRole(t *testing.T) {
	roles := &fakeRoles{byID: map[string]domain.Role{
		"r1": mustRole(t, "r1", "Docente", []string{"students:read"}),
	}}
	svc := newAdmin(&fakeUsers{}, roles, func() string { return "x" })

	got, err := svc.GetRole(context.Background(), "r1")
	if err != nil {
		t.Fatalf("GetRole error: %v", err)
	}
	if got.ID() != "r1" || got.Name() != "Docente" {
		t.Errorf("GetRole = {%q,%q}, want {r1,Docente}", got.ID(), got.Name())
	}

	if _, err := svc.GetRole(context.Background(), "ghost"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("GetRole(ghost): want KindNotFound, got %v", err)
	}
}

func TestAdminCreateRole(t *testing.T) {
	roles := &fakeRoles{}
	svc := newAdmin(&fakeUsers{}, roles, func() string { return "role-new" })

	role, err := svc.CreateRole(context.Background(), "Coordinador", []string{"users:read", "roles:read"})
	if err != nil {
		t.Fatalf("CreateRole error: %v", err)
	}
	if role.ID() != "role-new" || role.Name() != "Coordinador" {
		t.Errorf("role = {%q,%q}, want {role-new,Coordinador}", role.ID(), role.Name())
	}
	if len(role.Permissions()) != 2 {
		t.Errorf("role permissions = %d, want 2", len(role.Permissions()))
	}
	if len(roles.created) != 1 {
		t.Errorf("CreateRole persisted %d roles, want 1", len(roles.created))
	}
}

func TestAdminCreateRoleRejectsBadPermission(t *testing.T) {
	for _, bad := range []string{"Students:Create", "foo"} {
		roles := &fakeRoles{}
		svc := newAdmin(&fakeUsers{}, roles, func() string { return "role-new" })
		_, err := svc.CreateRole(context.Background(), "R", []string{bad})
		if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("CreateRole(perm=%q): want KindInvalid, got %v", bad, err)
		}
		if len(roles.created) != 0 {
			t.Errorf("CreateRole(perm=%q): must not persist on bad permission", bad)
		}
	}
}

func TestAdminCreateRoleDuplicateNameIsConflict(t *testing.T) {
	roles := &fakeRoles{}
	svc := newAdmin(&fakeUsers{}, roles, func() string { return "role-new" })

	if _, err := svc.CreateRole(context.Background(), "Coordinador", []string{"users:read"}); err != nil {
		t.Fatalf("first CreateRole error: %v", err)
	}
	_, err := svc.CreateRole(context.Background(), "Coordinador", []string{"users:read"})
	if err == nil || glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("duplicate role name: want KindConflict, got %v", err)
	}
}

func TestAdminListRoles(t *testing.T) {
	roles := &fakeRoles{listOut: []domain.Role{mustRole(t, "r1", "Docente", nil)}}
	svc := newAdmin(&fakeUsers{}, roles, func() string { return "x" })

	got, err := svc.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("ListRoles error: %v", err)
	}
	if len(got) != 1 || got[0].ID() != "r1" {
		t.Errorf("ListRoles = %v, want [r1]", got)
	}
}

func TestAdminSetRolePermissions(t *testing.T) {
	roles := &fakeRoles{}
	svc := newAdmin(&fakeUsers{}, roles, func() string { return "x" })

	if err := svc.SetRolePermissions(context.Background(), "r1", []string{"users:read", "users:write"}); err != nil {
		t.Fatalf("SetRolePermissions error: %v", err)
	}
	if got := roles.permsSet["r1"]; len(got) != 2 {
		t.Errorf("permissions for r1 = %v, want 2 entries", got)
	}

	if err := svc.SetRolePermissions(context.Background(), "r1", []string{"bad perm"}); err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("bad permission: want KindInvalid, got %v", err)
	}
}

// The tenant-wide sensitive reads (finance:read, attendance:read, comms:read)
// return ANY student's data — there is no per-object scoping yet, so only the
// single owner role may hold them. The admin API must refuse to build the
// vulnerable config.

func TestAdminSetRolePermissionsRejectsOwnerOnlyPermissionOnNonOwnerRole(t *testing.T) {
	for _, restricted := range []string{"finance:read", "attendance:read", "comms:read"} {
		roles := &fakeRoles{byID: map[string]domain.Role{
			"r1": mustRole(t, "r1", "Docente", []string{"students:read"}),
		}}
		svc := newAdmin(&fakeUsers{}, roles, func() string { return "x" })

		err := svc.SetRolePermissions(context.Background(), "r1", []string{"students:read", restricted})
		if err == nil || glerrors.KindOf(err) != glerrors.KindForbidden {
			t.Errorf("SetRolePermissions(Docente, %q): want KindForbidden, got %v", restricted, err)
		}
		if _, persisted := roles.permsSet["r1"]; persisted {
			t.Errorf("SetRolePermissions(Docente, %q): must not persist the rejected set", restricted)
		}
	}
}

func TestAdminSetRolePermissionsAllowsUnrestrictedOnNonOwnerRole(t *testing.T) {
	roles := &fakeRoles{byID: map[string]domain.Role{
		"r1": mustRole(t, "r1", "Docente", nil),
	}}
	svc := newAdmin(&fakeUsers{}, roles, func() string { return "x" })

	if err := svc.SetRolePermissions(context.Background(), "r1", []string{"students:read", "finance:write"}); err != nil {
		t.Fatalf("SetRolePermissions with unrestricted permissions: %v", err)
	}
	if got := roles.permsSet["r1"]; len(got) != 2 {
		t.Errorf("permissions for r1 = %v, want 2 entries", got)
	}
}

func TestAdminSetRolePermissionsAllowsOwnerOnlyPermissionOnOwnerRole(t *testing.T) {
	// The exemption is keyed on the seeded owner role's fixed id, never its name.
	roles := &fakeRoles{byID: map[string]domain.Role{
		domain.OwnerRoleID: mustRole(t, domain.OwnerRoleID, "owner", nil),
	}}
	svc := newAdmin(&fakeUsers{}, roles, func() string { return "x" })

	perms := []string{"finance:read", "attendance:read", "comms:read", "students:read"}
	if err := svc.SetRolePermissions(context.Background(), domain.OwnerRoleID, perms); err != nil {
		t.Fatalf("SetRolePermissions on the owner role: %v", err)
	}
	if got := roles.permsSet[domain.OwnerRoleID]; len(got) != 4 {
		t.Errorf("permissions for owner role = %v, want 4 entries", got)
	}
}

// SECURITY regression: the owner-only guard must key off the seeded owner role's
// fixed id, NOT its name. Role-name uniqueness in Postgres is case-sensitive, so
// a case-variant like "Owner" is a distinct, insertable role; if it inherited
// the owner exemption a delegated non-owner admin (roles:write + users:write)
// could mint itself a tenant-wide sensitive read and see every family's ledger.
func TestAdminSetRolePermissionsCaseVariantOwnerNameCannotHoldOwnerOnly(t *testing.T) {
	for _, name := range []string{"Owner", "OWNER", "owner "} {
		roles := &fakeRoles{byID: map[string]domain.Role{
			"r1": mustRole(t, "r1", name, []string{"students:read"}),
		}}
		svc := newAdmin(&fakeUsers{}, roles, func() string { return "x" })

		err := svc.SetRolePermissions(context.Background(), "r1", []string{"finance:read"})
		if err == nil || glerrors.KindOf(err) != glerrors.KindForbidden {
			t.Errorf("SetRolePermissions(name=%q, finance:read): want KindForbidden, got %v", name, err)
		}
		if _, persisted := roles.permsSet["r1"]; persisted {
			t.Errorf("SetRolePermissions(name=%q): must not persist an escalated set on a non-owner id", name)
		}
	}
}

func TestAdminSetRolePermissionsUnknownRoleWithOwnerOnlyPermissionFailsClosed(t *testing.T) {
	roles := &fakeRoles{byID: map[string]domain.Role{}}
	svc := newAdmin(&fakeUsers{}, roles, func() string { return "x" })

	err := svc.SetRolePermissions(context.Background(), "ghost", []string{"finance:read"})
	if err == nil {
		t.Fatal("SetRolePermissions on an unresolvable role with a restricted permission must fail")
	}
	if _, persisted := roles.permsSet["ghost"]; persisted {
		t.Error("must not persist when the target role cannot be confirmed to be the owner")
	}
}

func TestAdminCreateRoleRejectsOwnerOnlyPermissionOnNonOwnerRole(t *testing.T) {
	for _, restricted := range []string{"finance:read", "attendance:read", "comms:read"} {
		roles := &fakeRoles{}
		svc := newAdmin(&fakeUsers{}, roles, func() string { return "role-new" })

		_, err := svc.CreateRole(context.Background(), "Coordinador", []string{"users:read", restricted})
		if err == nil || glerrors.KindOf(err) != glerrors.KindForbidden {
			t.Errorf("CreateRole(Coordinador, %q): want KindForbidden, got %v", restricted, err)
		}
		if len(roles.created) != 0 {
			t.Errorf("CreateRole(Coordinador, %q): must not persist the rejected role", restricted)
		}
	}
}

// CreateRole can never mint the seeded owner role — that role is created by
// migration with a fixed id (ON CONFLICT DO NOTHING). A freshly generated role
// id is never the owner id, so an owner-only permission on CreateRole is
// forbidden regardless of the requested name (even "owner"): the API cannot
// bootstrap a second, name-colliding super-role.
func TestAdminCreateRoleRejectsOwnerOnlyEvenWhenNamedOwner(t *testing.T) {
	for _, name := range []string{"owner", "Owner", "OWNER"} {
		roles := &fakeRoles{}
		svc := newAdmin(&fakeUsers{}, roles, func() string { return "role-new" })

		_, err := svc.CreateRole(context.Background(), name, []string{"finance:read"})
		if err == nil || glerrors.KindOf(err) != glerrors.KindForbidden {
			t.Errorf("CreateRole(%q, finance:read): want KindForbidden, got %v", name, err)
		}
		if len(roles.created) != 0 {
			t.Errorf("CreateRole(%q): must not persist an escalated role", name)
		}
	}
}
