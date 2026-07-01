// Package postgres implements the iam repository ports with GORM. It is the ONLY
// place GORM appears in iam — the domain and use cases never import it. A double
// model (GORM structs here ↔ domain entities via mapper.go) keeps the domain
// framework-free. All access is tenant-scoped for RLS (see ADR 0001).
package postgres

import (
	"time"

	"github.com/lib/pq"
)

// userModel is the GORM row for `users`. `PasswordHash` is a credential that
// lives here, never on the domain User.
type userModel struct {
	ID           string `gorm:"column:id;primaryKey"`
	TenantID     string `gorm:"column:tenant_id"`
	Email        string `gorm:"column:email"`
	DisplayName  string `gorm:"column:display_name"`
	PasswordHash string `gorm:"column:password_hash"`
	Status       string `gorm:"column:status"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (userModel) TableName() string { return "users" }

// roleModel is the GORM row for `roles`. Permissions are a Postgres text[].
type roleModel struct {
	ID          string         `gorm:"column:id;primaryKey"`
	TenantID    string         `gorm:"column:tenant_id"`
	Name        string         `gorm:"column:name"`
	Permissions pq.StringArray `gorm:"column:permissions;type:text[]"`
}

func (roleModel) TableName() string { return "roles" }

// userRoleModel is the GORM row for the `user_roles` join table.
type userRoleModel struct {
	UserID   string `gorm:"column:user_id;primaryKey"`
	RoleID   string `gorm:"column:role_id;primaryKey"`
	TenantID string `gorm:"column:tenant_id"`
}

func (userRoleModel) TableName() string { return "user_roles" }
