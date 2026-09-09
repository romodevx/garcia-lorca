-- iam — invitations with a lifetime of our own.
--
-- WHY THIS TABLE EXISTS: the identity provider's reset links expire in one hour
-- and that is not configurable. The school asked for twenty-four. A promise we
-- make to a person cannot be a value somebody else picks, so the token becomes
-- ours.
--
-- WHY IT ALSO FIXES TWO OTHER THINGS: it is the first place in this system that
-- records the difference between "an account was created" and "a person has
-- used it". That difference is what the resend button keys on, and what the
-- status pill shows.
--
-- RLS: tenant-scoped like every other iam table (ADR 0001), ENABLE + FORCE so
-- the policy applies even to the owner. The redeem endpoints are anonymous, and
-- that is safe here only because the tenant is bound by the adapter from
-- configuration (pkg/authctx) and never travels in the request.

BEGIN;

-- --- users.status gains a third value ------------------------------------
-- The CHECK in 0001_init.sql is inline, so Postgres named it users_status_check.
-- It has to be dropped by that exact name. The replacement is a strict SUPERSET
-- of the old one, so a plain ADD CONSTRAINT is right: NOT VALID exists for
-- constraints that might reject existing rows, and none of these can.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check
    CHECK (status IN ('active', 'suspended', 'invited'));

-- --- the invitations ------------------------------------------------------

CREATE TABLE user_invitations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- The SHA-256 of the token, never the token. The clear value exists in
    -- exactly one place — the message that carries it to its owner — so a
    -- database dump leaks nothing anybody can use.
    token_hash  text NOT NULL,

    -- Which of the two jobs this token was minted for. They share a table and
    -- nothing else: consuming an 'invitation' activates the account, consuming
    -- a 'reset' must leave the status untouched. Folded together, a suspended
    -- person could reset their password and walk back in through the front door.
    purpose     text NOT NULL CHECK (purpose IN ('invitation', 'reset')),

    expires_at  timestamptz NOT NULL,
    -- NULL means "not spent yet". Named consumed_at rather than accepted_at on
    -- purpose: it says the TOKEN was used, which is not the same claim as "this
    -- person is activated". Activation lives on users.status, and conflating
    -- the two is how a reset would silently un-invite somebody.
    consumed_at timestamptz,

    -- What happened when we tried to deliver it. The original bug survived a
    -- week because nothing anywhere recorded that a message had not gone out.
    sent_at     timestamptz,
    sent_count  integer NOT NULL DEFAULT 0,
    last_error  text NOT NULL DEFAULT '',

    created_at  timestamptz NOT NULL DEFAULT now()
);

-- The redemption lookup. UNIQUE because a hash collision would be two people
-- sharing one door, and the index is what makes the lookup a comparison against
-- an index entry rather than against a secret.
CREATE UNIQUE INDEX idx_user_invitations_token ON user_invitations (token_hash);

-- At most ONE live token per user per purpose. This is what makes "resend" mean
-- "the old link stops working": without it, every resend would leave another
-- valid link in another inbox for its full lifetime.
CREATE UNIQUE INDEX idx_user_invitations_live
    ON user_invitations (user_id, purpose)
    WHERE consumed_at IS NULL;

CREATE INDEX idx_user_invitations_tenant ON user_invitations (tenant_id);

-- --- Row Level Security ---------------------------------------------------
-- current_setting('app.current_tenant', true) is NULL when unset, so an
-- unscoped connection sees zero rows (fail-closed).

ALTER TABLE user_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_invitations
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
