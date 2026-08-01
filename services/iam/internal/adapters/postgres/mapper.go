package postgres

import (
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// mapper.go translates GORM rows ↔ domain entities. It is the seam that keeps
// GORM out of the domain: everything above the adapter speaks domain types only.

var statusToDomain = map[string]domain.Status{
	"active":    domain.StatusActive,
	"suspended": domain.StatusSuspended,
}

// toDomainRole builds a domain Role from its row.
func toDomainRole(m roleModel) (domain.Role, error) {
	return domain.NewRole(m.ID, m.Name, []string(m.Permissions))
}

// toDomainUser builds a domain User from its row + resolved roles, and returns
// the stored password hash and the must-change-password flag separately (a
// credential and a policy flag the domain either never holds or carries by value).
func toDomainUser(m userModel, roleRows []roleModel) (domain.User, string, bool, error) {
	email, err := domain.NewEmail(m.Email)
	if err != nil {
		return domain.User{}, "", false, err
	}
	roles := make([]domain.Role, 0, len(roleRows))
	for _, rr := range roleRows {
		role, err := toDomainRole(rr)
		if err != nil {
			return domain.User{}, "", false, err
		}
		roles = append(roles, role)
	}
	status, ok := statusToDomain[m.Status]
	if !ok {
		return domain.User{}, "", false, glerrors.New(glerrors.KindInternal, "unknown user status: "+m.Status)
	}
	user, err := domain.NewUser(m.ID, m.TenantID, email, m.DisplayName, status, m.MustChangePassword, roles)
	if err != nil {
		return domain.User{}, "", false, err
	}
	return user, m.PasswordHash, m.MustChangePassword, nil
}
