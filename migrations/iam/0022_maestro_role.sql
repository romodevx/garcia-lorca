-- iam — the Maestro role (T2, "Rol Profesor y acceso restringido").
--
-- A teacher reaches exactly two things: Calificaciones and Asistencias, and in
-- both only for their OWN assignments (teacher + subject + group) of the active
-- cycle. The permissions below open the self-scoped /api/v1/teaching/* surface;
-- the scoping itself is the gateway's principal→teacher→assignments rule, not
-- the permission (ADR 0006).
--
--   * teaching:read / teaching:write             — their groups' grades (0012).
--   * attendance_own:read / attendance_own:write — their groups' pass list (0013).
--
-- Nothing else: no students:read, no grades:*, no attendance:* (the admin-wide
-- surfaces), no config:read. None of the four is owner-only.
--
-- The owner already holds all four (0012, 0013) so it can delegate them — the
-- caller-subset guard lets an admin grant only what it holds. The UPDATEs below
-- restate that idempotently, by the owner's fixed id (domain.OwnerRoleID), so a
-- renamed owner role still gets them.
--
-- The role gets a fixed id, like guardian (0010, …a2) and cobranza (0021, …a3).
-- Idempotent. RLS: roles is FORCE ROW LEVEL SECURITY, so SET LOCAL binds the
-- tenant.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['teaching:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('teaching:read' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['teaching:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('teaching:write' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['attendance_own:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('attendance_own:read' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['attendance_own:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('attendance_own:write' = ANY (permissions));

INSERT INTO roles (id, tenant_id, name, permissions)
VALUES (
    '00000000-0000-0000-0000-0000000000a4',
    '00000000-0000-0000-0000-000000000001',
    'maestro',
    ARRAY['teaching:read', 'teaching:write', 'attendance_own:read', 'attendance_own:write']
)
-- A role the school already named "maestro" (the dev seed made one by hand) is
-- brought to exactly these four rather than skipped: left as it was, it could
-- keep broader access (grades:write reaches every group) and nothing would say
-- so. It keeps its own id.
ON CONFLICT (tenant_id, name) DO UPDATE SET permissions = EXCLUDED.permissions;

COMMIT;
