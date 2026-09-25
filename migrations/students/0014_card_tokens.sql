-- students — the codes of the pupils' virtual cards (task 13).
--
-- The card's QR carries GLCARD:<token> and nothing else: 128 random bits,
-- meaningless to anybody without a session. Staff verify it (students:read) to
-- see whose card it is and whether it still counts. One active code per pupil.
-- Revoking one keeps its row, so the old code verifies as "no longer valid"
-- rather than as unknown, and SUSPENDS the card (the owner's decision): no new
-- code is issued to the family's session — a stolen phone keeps its session —
-- until the office reissues it.
BEGIN;

CREATE TABLE student_card_tokens (
    id          uuid PRIMARY KEY,
    tenant_id   uuid NOT NULL,
    student_id  uuid NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    token       text NOT NULL CHECK (token ~ '^[A-Za-z0-9_-]{22}$'),
    issued_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz,
    UNIQUE (tenant_id, token)
);

-- One active code per pupil: two cards for one child would each verify.
CREATE UNIQUE INDEX uq_student_card_tokens_active
    ON student_card_tokens (tenant_id, student_id)
    WHERE revoked_at IS NULL;

ALTER TABLE student_card_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE student_card_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON student_card_tokens
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
