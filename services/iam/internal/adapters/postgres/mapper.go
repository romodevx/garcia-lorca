package postgres

import (
	"github.com/lib/pq"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// mapper.go translates GORM rows ↔ domain entities. It is the seam that keeps
// GORM out of the domain: everything above the adapter speaks domain types only.

var statusToDomain = map[string]domain.Status{
	"active":    domain.StatusActive,
	"suspended": domain.StatusSuspended,
}

// statusToDB is the inverse of statusToDomain: it renders a domain Status onto
// the `status` text column ("active"/"suspended").
var statusToDB = map[domain.Status]string{
	domain.StatusActive:    "active",
	domain.StatusSuspended: "suspended",
}

// statusColumn renders a domain Status onto its text column, rejecting a status
// with no column representation (e.g. StatusUnspecified) as KindInvalid so a
// half-specified write never reaches the database.
func statusColumn(s domain.Status) (string, error) {
	text, ok := statusToDB[s]
	if !ok {
		return "", glerrors.New(glerrors.KindInvalid, "unsupported user status")
	}
	return text, nil
}

// fromDomainUser maps a domain User + its credential hash onto the users row.
// The status must have a column representation (KindInvalid otherwise).
func fromDomainUser(u domain.User, passwordHash string) (userModel, error) {
	status, err := statusColumn(u.Status())
	if err != nil {
		return userModel{}, err
	}
	return userModel{
		ID:                 u.ID(),
		TenantID:           u.TenantID(),
		Email:              u.Email().String(),
		DisplayName:        u.DisplayName(),
		PasswordHash:       passwordHash,
		Status:             status,
		MustChangePassword: u.MustChangePassword(),
	}, nil
}

// fromDomainRole maps a domain Role onto its row, projecting the validated
// permission value objects back to their canonical "resource:action" strings.
func fromDomainRole(r domain.Role, tenantID string) roleModel {
	perms := r.Permissions()
	raw := make([]string, 0, len(perms))
	for _, p := range perms {
		raw = append(raw, p.String())
	}
	return roleModel{
		ID:          r.ID(),
		TenantID:    tenantID,
		Name:        r.Name(),
		Permissions: pq.StringArray(raw),
	}
}

// toDomainRole builds a domain Role from its row.
func toDomainRole(m roleModel) (domain.Role, error) {
	return domain.NewRole(m.ID, m.Name, []string(m.Permissions))
}

// toDomainUser builds a domain User from its row + resolved roles, and returns
// the stored password hash separately (a credential the domain never holds).
// The must-change-password flag is carried on the User itself via NewUser, so it
// has a single source of truth.
func toDomainUser(m userModel, roleRows []roleModel) (domain.User, string, error) {
	email, err := domain.NewEmail(m.Email)
	if err != nil {
		return domain.User{}, "", err
	}
	roles := make([]domain.Role, 0, len(roleRows))
	for _, rr := range roleRows {
		role, err := toDomainRole(rr)
		if err != nil {
			return domain.User{}, "", err
		}
		roles = append(roles, role)
	}
	status, ok := statusToDomain[m.Status]
	if !ok {
		return domain.User{}, "", glerrors.New(glerrors.KindInternal, "unknown user status: "+m.Status)
	}
	user, err := domain.NewUser(m.ID, m.TenantID, email, m.DisplayName, status, m.MustChangePassword, roles)
	if err != nil {
		return domain.User{}, "", err
	}
	// Hydrate the credential-rotation instant so RefreshToken can revoke tokens
	// minted before the last password change.
	user = user.WithPasswordChangedAt(m.PasswordChangedAt)
	return user, m.PasswordHash, nil
}
