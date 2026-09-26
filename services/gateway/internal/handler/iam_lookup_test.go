package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

type stubUserDirectory []client.User

func (s stubUserDirectory) ListUsers(context.Context) ([]client.User, error) { return s, nil }

// userByEmail is the one lookup both "Dar acceso" actions (family, teacher)
// match an existing account with: both sides normalized by pkg/email, so a
// record typed " Ana@Colegio.MX " finds iam's "ana@colegio.mx".
func TestUserByEmailNormalizesBothSides(t *testing.T) {
	dir := stubUserDirectory{
		{ID: "usr-1", Email: "otra@colegio.mx"},
		{ID: "usr-2", Email: "Ana@Colegio.mx "},
	}
	tests := []struct {
		name   string
		raw    string
		wantID string
		found  bool
	}{
		{"same spelling", "Ana@Colegio.mx ", "usr-2", true},
		{"other case and whitespace", "  ana@COLEGIO.MX\t", "usr-2", true},
		{"absent", "nadie@colegio.mx", "", false},
		{"empty never matches", "  ", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user, found, err := userByEmail(context.Background(), dir, tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.found, found)
			assert.Equal(t, tc.wantID, user.ID)
		})
	}
}

func TestFindRoleByName(t *testing.T) {
	roles := []client.Role{{ID: "r1", Name: "owner"}, {ID: "r2", Name: "guardian"}}
	role, ok := findRoleByName(roles, "guardian")
	assert.True(t, ok)
	assert.Equal(t, "r2", role.ID)
	_, ok = findRoleByName(roles, "maestro")
	assert.False(t, ok)
}
