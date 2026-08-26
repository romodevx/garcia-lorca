-- iam (M12 uniforms) — the uniform shop's permissions.
--
-- The read permission is SPLIT, and the split is the whole point of this file.
--
--   uniforms:stock   the catalogue, the inventory, goods receipts, adjustments
--                    and the counter sale. It resolves NO student data at all —
--                    garments, sizes and counts. GRANTABLE without reservation:
--                    this is the "Caja/Almacen" role a school needs, and it
--                    covers the great majority of the shop's daily work.
--
--   uniforms:orders  the order list, which carries every student's name and
--                    purchase history for the whole school with no per-object
--                    scoping. That is the literal criterion ownerOnlyPermissions
--                    states (services/iam/internal/domain/permission.go), the
--                    same one that keeps finance:read and attendance:read owner
--                    -only. It is therefore OWNER-ONLY until per-object scoping
--                    exists for it.
--
-- A single uniforms:read would have forced the choice between a storeroom role
-- that cannot work and handing the school roll to whoever holds it. It is a
-- false choice: the storeroom does not need names.
--
-- This matters more than the list above suggests while the grant-subset gap is
-- open: anybody holding roles:write can mint a role with any permission that is
-- not owner-only and assign it to themselves. "Grantable" therefore reads, in
-- practice, as "legible to anyone who can edit roles".
--
-- uniforms:write gates every mutation of the shop (receipts, adjustments,
-- picking, dispatch, cancellation). The counter sale additionally requires
-- finance:write, because it takes money; that check lives at the gateway, since
-- it is a composition of two modules' rights rather than a permission of its own.
--
-- The family side is uniforms_own:read / uniforms_own:write, held by the
-- `guardian` role. Ownership is enforced by the guardian->student link at the
-- gateway, exactly as for /api/v1/family/*; the permission only says "this
-- principal may act on its OWN children's orders", never whose.
--
-- uniforms_own:write is the FIRST write permission ever granted to a guardian on
-- this platform, and it creates a financial obligation. That is a deliberate
-- decision recorded in ADR 0010, not a row quietly added to a table.
--
-- Plain SQL only (no psql variables): this file must apply BOTH via the psql
-- runner AND via the Go integration tests, which exec the SQL through GORM.
--
-- RLS: roles is FORCE ROW LEVEL SECURITY, so SET LOCAL binds app.current_tenant
-- for the UPDATE's USING/WITH CHECK under a non-superuser owner.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

-- --- staff surface: the owner holds all four, so it can delegate ------------
-- The admin subset-guard lets an admin grant only permissions it already holds,
-- so the owner must carry uniforms:stock and uniforms:write to mint a
-- "Caja/Almacen" role. Appended idempotently; a re-run is a no-op.

UPDATE roles
   SET permissions = permissions || ARRAY['uniforms:stock', 'uniforms:orders', 'uniforms:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['uniforms:stock', 'uniforms:orders', 'uniforms:write']);

-- --- family surface: the guardian role gains its own-orders rights ----------

UPDATE roles
   SET permissions = permissions || ARRAY['uniforms_own:read', 'uniforms_own:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'guardian'
   AND NOT (permissions @> ARRAY['uniforms_own:read', 'uniforms_own:write']);

-- The owner must hold them too, or it cannot delegate them when a school runs
-- more than one guardian-shaped role.
UPDATE roles
   SET permissions = permissions || ARRAY['uniforms_own:read', 'uniforms_own:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['uniforms_own:read', 'uniforms_own:write']);

COMMIT;
