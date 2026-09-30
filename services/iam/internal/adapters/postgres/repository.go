package postgres

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/authctx"
	"github.com/garcia-lorca/pkg/authctx/gormtx"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// SQLSTATE codes the adapter maps to domain error Kinds (mirrors the config
// adapter): a unique violation is a Conflict; a foreign-key violation means a
// referenced row (e.g. a role id) is missing → Invalid.
const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
	checkViolation      = "23514"
	// emailNormalizedCheck is the CHECK the email migration adds. It is the one
	// CHECK an API client can trip (a raw email that bypassed the domain, e.g.
	// against the previous revision mid-deploy), so it alone maps to KindInvalid;
	// any other CHECK means the server is inconsistent and stays KindInternal.
	emailNormalizedCheck = "users_email_normalized_ck"
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

// withTenant runs fn inside a transaction that has bound app.current_tenant,
// delegating to the shared pkg/authctx RLS primitive so tenant binding lives in
// exactly one place across every service.
func (b base) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return authctx.WithTenantContext(ctx, gormtx.NewRunner(b.db), b.tenantID, func(tx authctx.Tx) error {
		return fn(gormtx.Gorm(tx))
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

// UpdatePassword sets a new hash, clears must_change_password, and stamps
// password_changed_at = now() in one write. The stamp is what revokes every
// refresh token issued before this rotation (RefreshToken rejects an earlier
// issued-at), so a stolen refresh token dies the moment the password changes.
func (r *UserRepo) UpdatePassword(ctx context.Context, userID, newHash string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&userModel{}).
			Where("id = ?", userID).
			Updates(map[string]any{"password_hash": newHash, "must_change_password": false, "password_changed_at": gorm.Expr("now()")})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating password", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// TouchPasswordChangedAt stamps the rotation instant without touching the hash.
//
// For an account whose credential lives at the identity provider there is no
// local hash to write, but the revocation floor still has to move: RefreshToken
// refuses anything minted before it, which is what makes a password change
// invalidate older sessions.
func (r *UserRepo) TouchPasswordChangedAt(ctx context.Context, userID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&userModel{}).
			Where("id = ?", userID).
			Update("password_changed_at", gorm.Expr("now()"))
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "stamping the password rotation", res.Error)
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

// CreateUser inserts a new principal together with its credential hash. A
// duplicate email (UNIQUE(tenant_id, email)) is KindConflict.
func (r *UserRepo) CreateUser(ctx context.Context, user domain.User, passwordHash string) error {
	model, err := fromDomainUser(user, passwordHash)
	if err != nil {
		return err
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		// Omit password_changed_at so the column's DB default (now()) fills it —
		// a freshly created domain user carries the zero instant.
		return translateCreateErr(tx.Omit("password_changed_at").Create(&model).Error, "creating user", "a user with this email already exists")
	})
}

// ListUsers returns every principal in the tenant with roles resolved. It costs
// exactly two queries regardless of user count — one for the users, one join for
// all of their roles — then stitches roles onto users in memory (no N+1).
func (r *UserRepo) ListUsers(ctx context.Context) ([]domain.User, error) {
	var out []domain.User
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var ums []userModel
		// A deleted account leaves every listing (T2); its row stays for the
		// history that references it, reachable by id.
		if err := tx.Where("status <> ?", statusDeleted).Order("email").Find(&ums).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing users", err)
		}
		ids := make([]string, 0, len(ums))
		for _, um := range ums {
			ids = append(ids, um.ID)
		}
		rolesByUser, err := loadRolesForUsers(tx, ids)
		if err != nil {
			return err
		}
		out = make([]domain.User, 0, len(ums))
		for _, um := range ums {
			user, _, derr := toDomainUser(um, rolesByUser[um.ID])
			if derr != nil {
				return derr
			}
			out = append(out, user)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetStatus updates a user's lifecycle status. An id that matches no row is
// KindNotFound.
func (r *UserRepo) SetStatus(ctx context.Context, userID string, status domain.Status) error {
	text, err := statusColumn(status)
	if err != nil {
		return err
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&userModel{}).Where("id = ? AND status <> ?", userID, statusDeleted).Update("status", text)
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating user status", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// statusDeleted is the status column's value for a soft-deleted account.
const statusDeleted = "deleted"

// SoftDelete marks the account deleted and stamps who and when, in ONE
// tenant-bound transaction with the last-holder rule.
//
// The rule is checked under row locks, not just read: every ACTIVE holder of
// keepActiveHolderOf is locked FOR UPDATE before counting. Two directors
// deleting each other at the same instant then serialize — the second one's
// lock waits, re-reads the first one's row as deleted, counts itself alone and
// is refused. A plain read would let both pass and leave the school with no
// director at all.
//
// password_changed_at moves with it: RefreshToken refuses any token minted
// before that instant (ADR 0005), so every session the account held dies here
// rather than when its tokens happen to expire.
func (r *UserRepo) SoftDelete(ctx context.Context, userID, deletedBy, keepActiveHolderOf string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := guardLastActiveHolder(tx, userID, keepActiveHolderOf); err != nil {
			return err
		}
		res := tx.Model(&userModel{}).
			Where("id = ? AND status <> ?", userID, statusDeleted).
			Updates(map[string]any{
				"status":              statusDeleted,
				"deleted_at":          gorm.Expr("now()"),
				"deleted_by":          deletedBy,
				"password_changed_at": gorm.Expr("now()"),
			})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting user", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// guardLastActiveHolder refuses when userID is the only ACTIVE holder of
// roleID. The holders are locked first (see SoftDelete); a target that is not
// an active holder has nothing to guard.
func guardLastActiveHolder(tx *gorm.DB, userID, roleID string) error {
	var holders []string
	err := tx.Raw(`SELECT u.id::text FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		WHERE ur.role_id = ? AND u.status = 'active'
		ORDER BY u.id
		FOR UPDATE OF u`, roleID).Scan(&holders).Error
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "locking the role's holders", err)
	}
	if slices.Contains(holders, userID) && len(holders) == 1 {
		return domain.ErrLastOwner
	}
	return nil
}

// Restore brings a deleted account back as `invited` under displayName,
// clears its deletion stamp and replaces its roles with roleIDs — all in ONE
// transaction, so a failure anywhere leaves it deleted rather than restored
// with the roles it held before (a former owner would come back an owner).
// Anything but a deleted id is KindNotFound.
func (r *UserRepo) Restore(ctx context.Context, userID, displayName string, roleIDs []string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&userModel{}).
			Where("id = ? AND status = ?", userID, statusDeleted).
			Updates(map[string]any{
				"status":       "invited",
				"display_name": displayName,
				"deleted_at":   nil,
				"deleted_by":   nil,
				// No old credential survives: the account opens only from
				// the new link, which writes the password its owner chooses.
				"password_hash":        "",
				"must_change_password": false,
			})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "restoring user", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return replaceRoles(tx, r.tenantID, userID, roleIDs)
	})
}

// SetUserRoles replaces the user's entire role set with roleIDs. The delete and
// the inserts run in ONE tenant-bound transaction so the swap is atomic (a
// failure rolls both back). A role id that violates the FK is KindInvalid.
func (r *UserRepo) SetUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return replaceRoles(tx, r.tenantID, userID, roleIDs)
	})
}

// replaceRoles swaps the user's whole role set inside the caller's transaction.
func replaceRoles(tx *gorm.DB, tenantID, userID string, roleIDs []string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&userRoleModel{}).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "clearing user roles", err)
	}
	if len(roleIDs) == 0 {
		return nil
	}
	rows := make([]userRoleModel, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		rows = append(rows, userRoleModel{UserID: userID, RoleID: roleID, TenantID: tenantID})
	}
	return translateCreateErr(tx.Create(&rows).Error, "assigning user roles", "role already assigned")
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

// CreateRole inserts a new role. A duplicate name (UNIQUE(tenant_id, name)) is
// KindConflict.
func (r *RoleRepo) CreateRole(ctx context.Context, role domain.Role) error {
	model := fromDomainRole(role, r.tenantID)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return translateCreateErr(tx.Create(&model).Error, "creating role", "a role with this name already exists")
	})
}

// ListRoles returns every role in the tenant, ordered by name.
func (r *RoleRepo) ListRoles(ctx context.Context) ([]domain.Role, error) {
	var out []domain.Role
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []roleModel
		if err := tx.Order("name").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing roles", err)
		}
		out = make([]domain.Role, 0, len(rows))
		for _, rm := range rows {
			role, derr := toDomainRole(rm)
			if derr != nil {
				return derr
			}
			out = append(out, role)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetPermissions replaces a role's permission set. An id that matches no row is
// KindNotFound.
func (r *RoleRepo) SetPermissions(ctx context.Context, roleID string, permissions []string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&roleModel{}).
			Where("id = ?", roleID).
			Update("permissions", pq.StringArray(permissions))
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating role permissions", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// --- shared helpers -------------------------------------------------------

// loadRolesForUsers resolves the roles of many users in a single join query,
// grouping them by user id in memory. Called once per ListUsers so the whole
// listing costs two queries, never one-per-user (no N+1).
func loadRolesForUsers(tx *gorm.DB, userIDs []string) (map[string][]roleModel, error) {
	if len(userIDs) == 0 {
		return map[string][]roleModel{}, nil
	}
	type joinRow struct {
		UserID      string         `gorm:"column:user_id"`
		ID          string         `gorm:"column:id"`
		TenantID    string         `gorm:"column:tenant_id"`
		Name        string         `gorm:"column:name"`
		Permissions pq.StringArray `gorm:"column:permissions;type:text[]"`
	}
	var rows []joinRow
	err := tx.Table("roles").
		Select("ur.user_id AS user_id, roles.id AS id, roles.tenant_id AS tenant_id, roles.name AS name, roles.permissions AS permissions").
		Joins("JOIN user_roles ur ON ur.role_id = roles.id").
		Where("ur.user_id IN ?", userIDs).
		Find(&rows).Error
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "loading roles", err)
	}
	out := make(map[string][]roleModel, len(userIDs))
	for _, row := range rows {
		out[row.UserID] = append(out[row.UserID], roleModel{
			ID:          row.ID,
			TenantID:    row.TenantID,
			Name:        row.Name,
			Permissions: row.Permissions,
		})
	}
	return out, nil
}

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

// translateCreateErr maps a GORM insert error to the port's error contract: a
// unique violation is KindConflict (conflictMsg), a foreign-key violation is
// KindInvalid (a referenced row — e.g. a role id — is missing), anything else is
// KindInternal.
func translateCreateErr(err error, internalMsg, conflictMsg string) error {
	if err == nil {
		return nil
	}
	switch pgCode(err) {
	case uniqueViolation:
		return glerrors.Wrap(glerrors.KindConflict, conflictMsg, err)
	case foreignKeyViolation:
		return glerrors.Wrap(glerrors.KindInvalid, "referenced row does not exist", err)
	case checkViolation:
		if isEmailCheck(err) {
			return glerrors.Wrap(glerrors.KindInvalid, "email is not in its normalized form", err)
		}
		return glerrors.Wrap(glerrors.KindInternal, internalMsg, err)
	default:
		return glerrors.Wrap(glerrors.KindInternal, internalMsg, err)
	}
}

// isEmailCheck reports whether err is the email-normalization CHECK violation.
func isEmailCheck(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == emailNormalizedCheck
}

// pgCode extracts the SQLSTATE code from a driver error, or "" if it is not one.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
