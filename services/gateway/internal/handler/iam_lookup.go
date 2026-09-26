package handler

import (
	"context"

	"github.com/garcia-lorca/pkg/email"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// userDirectory is the iam read the "Dar acceso" actions match an existing
// account with. iam has no lookup by address, so the tenant's users are listed
// once per action — an administrator's click, not a hot path.
type userDirectory interface {
	ListUsers(ctx context.Context) ([]client.User, error)
}

// userByEmail returns the account registered under raw. Both sides go through
// pkg/email.Normalize — the one spelling every service stores an address by —
// so a record typed with capitals or stray spaces still finds its login. An
// empty address matches nothing.
func userByEmail(ctx context.Context, dir userDirectory, raw string) (client.User, bool, error) {
	want := email.Normalize(raw)
	if want == "" {
		return client.User{}, false, nil
	}
	users, err := dir.ListUsers(ctx)
	if err != nil {
		return client.User{}, false, err
	}
	for _, u := range users {
		if email.Normalize(u.Email) == want {
			return u, true, nil
		}
	}
	return client.User{}, false, nil
}

// findRoleByName resolves a seeded role by NAME rather than the uuid its
// migration happens to seed: a deployment that ever reseeds it would otherwise
// silently start inviting people into nothing.
func findRoleByName(roles []client.Role, name string) (client.Role, bool) {
	for _, role := range roles {
		if role.Name == name {
			return role, true
		}
	}
	return client.Role{}, false
}
