package domain

import (
	"regexp"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// permissionFormat is the strict admin-surface grammar for a permission string:
// lowercase ASCII letters/underscores on each side of a single ':'.
var permissionFormat = regexp.MustCompile(`^[a-z_]+:[a-z_]+$`)

// IsValidPermission reports whether s is already a well-formed "resource:action"
// permission in the strict catalog grammar (no normalization). It is stricter
// than NewPermission — which trims and lowercases and tolerates dotted resources
// — because the admin role editor accepts only canonical catalog entries.
func IsValidPermission(s string) bool {
	return permissionFormat.MatchString(s)
}

// ownerOnlyPermissions are the tenant-wide sensitive reads: they resolve data
// for ANY student in the tenant, with no per-object scoping (a teacher holding
// attendance:read reads every group; a holder of finance:read reads every
// family's ledger; a holder of comms:read reads every student's notifications,
// payment amounts included). They are safe only on the single owner role, so the
// admin surface refuses to grant them to any other role — see the security
// carry-forward in deploy/README.
//
// guardians:read/write are here for a different reason: they don't read student
// data directly, but they COMPOSE into it — a holder can link itself to every
// student and then read every family's notifications through /api/v1/family/*.
// Keeping the guard closed under composition (not just over a hand-picked list of
// direct reads) means guardian management stays owner-only until the grant-subset
// gap ("grant only permissions you hold") is closed.
//
// TEMPORARY: lift the direct reads once per-object scoping (guardian sees only
// their child, teacher only their assigned groups) is enforced on those admin
// endpoints; the family path is already self-scoped (gateway /api/v1/family/*).
var ownerOnlyPermissions = map[string]struct{}{
	"finance:read":    {},
	"attendance:read": {},
	"comms:read":      {},
	"guardians:read":  {},
	"guardians:write": {},
}

// IsOwnerOnly reports whether raw is a tenant-wide sensitive read that only the
// owner role may hold. The input is normalized as NewPermission would normalize
// it, so a differently-cased or padded string cannot slip past the guard.
func IsOwnerOnly(raw string) bool {
	_, restricted := ownerOnlyPermissions[strings.ToLower(strings.TrimSpace(raw))]
	return restricted
}

// Permission is a "resource:action" capability, e.g. "students:read" or
// "finance.payment:create". Authorization is permission-based: a principal's
// effective permissions are the union across its roles, and gates check a
// specific Permission — never a role name.
//
//	"students:read" ──> Permission{resource:"students", action:"read"}
type Permission struct {
	resource string
	action   string
}

// NewPermission parses and validates a "resource:action" string (trimmed and
// lowercased). Exactly one ':' with non-empty sides is required.
func NewPermission(raw string) (Permission, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	resource, action, found := strings.Cut(value, ":")
	if !found {
		return Permission{}, glerrors.New(glerrors.KindInvalid, "permission must be 'resource:action'")
	}
	resource = strings.TrimSpace(resource)
	action = strings.TrimSpace(action)
	if resource == "" || action == "" || strings.Contains(action, ":") {
		return Permission{}, glerrors.New(glerrors.KindInvalid, "permission must be 'resource:action' with non-empty parts")
	}
	return Permission{resource: resource, action: action}, nil
}

// Resource is the object the permission applies to (e.g. "students").
func (p Permission) Resource() string { return p.resource }

// Action is the operation permitted (e.g. "read").
func (p Permission) Action() string { return p.action }

// String renders the canonical "resource:action" form.
func (p Permission) String() string { return p.resource + ":" + p.action }
