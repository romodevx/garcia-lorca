package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// Compile-time proof the adapters satisfy their ports. UserRepository and
// RoleRepository both declare a FindByID with different return types, so they
// are separate structs over a shared base rather than one struct.
var (
	_ ports.UserRepository = (*UserRepo)(nil)
	_ ports.RoleRepository = (*RoleRepo)(nil)
)

// base holds the shared connection + tenant scope and the RLS binding helper.
// Every operation runs in a transaction that has bound app.current_tenant so
// Row Level Security (tenant isolation) applies — see ADR 0001. In M01 the
// tenant is fixed at construction (single-tenant).
type base struct {
	db       *gorm.DB
	tenantID string
}

// withTenant runs fn inside a transaction that has bound app.current_tenant.
// Mirrors pkg/authctx's SET LOCAL pattern (set_config(..., true)).
func (b base) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", b.tenantID).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "binding RLS tenant", err)
		}
		return fn(tx)
	})
}

// --- users ----------------------------------------------------------------

// UserRepo is the GORM-backed ports.UserRepository.
type UserRepo struct{ base }

// NewUserRepo builds a user repository scoped to tenantID.
func NewUserRepo(db *gorm.DB, tenantID string) *UserRepo {
	return &UserRepo{base{db: db, tenantID: tenantID}}
}

// FindAuthByEmail returns the user (with roles) and their password hash for
// login. A missing user is KindNotFound; the use case maps that to a generic
// auth failure so email existence is never leaked.
func (r *UserRepo) FindAuthByEmail(ctx context.Context, _ string, email domain.Email) (domain.User, string, error) {
	var (
		user domain.User
		hash string
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var um userModel
		if err := tx.Where("email = ?", email.String()).First(&um).Error; err != nil {
			return translateFindErr(err)
		}
		roleRows, err := loadRoles(tx, um.ID)
		if err != nil {
			return err
		}
		user, hash, err = toDomainUser(um, roleRows)
		return err
	})
	if err != nil {
		return domain.User{}, "", err
	}
	return user, hash, nil
}

// FindByID returns a user with roles resolved, or KindNotFound.
func (r *UserRepo) FindByID(ctx context.Context, userID string) (domain.User, error) {
	var user domain.User
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var um userModel
		if err := tx.Where("id = ?", userID).First(&um).Error; err != nil {
			return translateFindErr(err)
		}
		roleRows, err := loadRoles(tx, um.ID)
		if err != nil {
			return err
		}
		user, _, err = toDomainUser(um, roleRows)
		return err
	})
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}

// FindAuthByID returns a user and their password hash by id (for ChangePassword).
func (r *UserRepo) FindAuthByID(ctx context.Context, userID string) (domain.User, string, error) {
	var (
		user domain.User
		hash string
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var um userModel
		if err := tx.Where("id = ?", userID).First(&um).Error; err != nil {
			return translateFindErr(err)
		}
		roleRows, err := loadRoles(tx, um.ID)
		if err != nil {
			return err
		}
		user, hash, err = toDomainUser(um, roleRows)
		return err
	})
	if err != nil {
		return domain.User{}, "", err
	}
	return user, hash, nil
}

// UpdatePassword sets a new hash and clears must_change_password in one write.
func (r *UserRepo) UpdatePassword(ctx context.Context, userID, newHash string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&userModel{}).
			Where("id = ?", userID).
			Updates(map[string]any{"password_hash": newHash, "must_change_password": false})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating password", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// AddRole assigns a role to a user (idempotent: a duplicate assignment is a no-op).
func (r *UserRepo) AddRole(ctx context.Context, userID, roleID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		row := userRoleModel{UserID: userID, RoleID: roleID, TenantID: r.tenantID}
		if err := tx.Where("user_id = ? AND role_id = ?", userID, roleID).
			FirstOrCreate(&row).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "assigning role", err)
		}
		return nil
	})
}

// RemoveRole revokes a role from a user.
func (r *UserRepo) RemoveRole(ctx context.Context, userID, roleID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND role_id = ?", userID, roleID).
			Delete(&userRoleModel{}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "revoking role", err)
		}
		return nil
	})
}

// --- roles ----------------------------------------------------------------

// RoleRepo is the GORM-backed ports.RoleRepository.
type RoleRepo struct{ base }

// NewRoleRepo builds a role repository scoped to tenantID.
func NewRoleRepo(db *gorm.DB, tenantID string) *RoleRepo {
	return &RoleRepo{base{db: db, tenantID: tenantID}}
}

// FindByID returns a role, or KindNotFound.
func (r *RoleRepo) FindByID(ctx context.Context, roleID string) (domain.Role, error) {
	var role domain.Role
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rm roleModel
		if err := tx.Where("id = ?", roleID).First(&rm).Error; err != nil {
			return translateFindErr(err)
		}
		var err error
		role, err = toDomainRole(rm)
		return err
	})
	if err != nil {
		return domain.Role{}, err
	}
	return role, nil
}

// --- shared helpers -------------------------------------------------------

// loadRoles resolves a user's roles via the join table (explicit; no N+1).
func loadRoles(tx *gorm.DB, userID string) ([]roleModel, error) {
	var roles []roleModel
	err := tx.Table("roles").
		Joins("JOIN user_roles ur ON ur.role_id = roles.id").
		Where("ur.user_id = ?", userID).
		Find(&roles).Error
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "loading roles", err)
	}
	return roles, nil
}

func translateFindErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "query failed", err)
}
