-- grades — period closing and report cards (T7, cierre de periodos y boletas).
--
-- A period is one config term: exactly one level of one cycle, covering a set
-- of months (config.term_months). The director closes it; closing freezes a
-- report card per active enrollment of the level IN THE SAME TRANSACTION as
-- the closure (both tables live here, so there is no cross-service commit).
-- Once closed, nobody writes a monthly grade of the term's months until it is
-- reopened; closing again issues the next version, which supersedes the
-- previous one (parents see the latest).
--
--   period_closures     one row per term that was ever closed. version counts
--                       the closes (= the version of the latest report cards);
--                       a reopen keeps it. No row = open, version 0.
--   report_cards        the frozen header: ids plus the names AS THEY READ at
--                       issue time (a later rename never changes an issued
--                       card), the term's months, the general average and the
--                       level's scale then. UNIQUE (enrollment, term, version).
--   report_card_lines   one per subject: month_values is a JSON object of exact
--                       decimal strings ({"2026-09":"9.5"}), term_average the
--                       half-up one-decimal mean (NULL = no grade in the term).
--
-- Averages are numeric(5,2) holding one decimal; NULL means "no average", never
-- zero. References to other services (term, cycle, level, group, enrollment,
-- student, subject) are cross-service UUIDs — NO foreign keys, same posture as
-- 0001/0002. report_card_lines → report_cards is local, ON DELETE CASCADE.
--
-- Writers and the closer serialise on a transaction-scoped advisory lock keyed
-- by the term (see adapters/postgres): a grade write takes it SHARED and reads
-- the closure; a close takes it EXCLUSIVE, so no write slips between the
-- snapshot and the closure.
--
-- RLS is tenant-scoped exactly like monthly_grades: ENABLE + FORCE, policy on
-- app.current_tenant; the application role is neither owner nor BYPASSRLS.
-- Additive only.

BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE period_closures (
    tenant_id       uuid NOT NULL,
    term_id         uuid NOT NULL,   -- config.terms (RPC-validated)
    cycle_id        uuid NOT NULL,   -- config.cycles, denormalised from the term
    level_id        uuid NOT NULL,   -- config.levels, denormalised from the term
    status          text NOT NULL CHECK (status IN ('open', 'closed')),
    version         int  NOT NULL DEFAULT 0 CHECK (version >= 0),
    closed_at       timestamptz NULL,
    closed_by       uuid NULL,       -- iam user who last closed it
    closed_by_name  text NULL,
    reopened_at     timestamptz NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, term_id),
    -- A closed period always says when and by whom.
    CONSTRAINT period_closures_closed_ck CHECK (status = 'open' OR (closed_at IS NOT NULL AND version >= 1))
);

-- The periods screen lists a cycle's closures.
CREATE INDEX idx_period_closures_cycle ON period_closures (tenant_id, cycle_id);

CREATE TABLE report_cards (
    id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                 uuid NOT NULL,
    term_id                   uuid NOT NULL,
    cycle_id                  uuid NOT NULL,
    level_id                  uuid NOT NULL,
    group_id                  uuid NOT NULL,
    enrollment_id             uuid NOT NULL,
    student_id                uuid NOT NULL,
    version                   int  NOT NULL CHECK (version >= 1),
    status                    text NOT NULL CHECK (status IN ('published', 'superseded')),
    issued_at                 timestamptz NOT NULL,
    issued_by                 uuid NOT NULL,
    student_name              text NOT NULL,
    enrollment_number         text NOT NULL DEFAULT '',
    group_label               text NOT NULL DEFAULT '',
    level_name                text NOT NULL DEFAULT '',
    cycle_name                text NOT NULL DEFAULT '',
    term_name                 text NOT NULL DEFAULT '',
    months                    text[] NOT NULL,
    general_average           numeric(5,2) NULL CHECK (general_average >= 0 AND general_average <= 100),
    scale_min_hundredths      int NOT NULL,
    scale_max_hundredths      int NOT NULL,
    scale_decimals            int NOT NULL,
    scale_passing_hundredths  int NOT NULL,
    created_at                timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, enrollment_id, term_id, version)
);

-- At most ONE published report card per enrollment and term: the one parents see.
CREATE UNIQUE INDEX uq_report_cards_published
    ON report_cards (tenant_id, enrollment_id, term_id) WHERE status = 'published';
-- The director lists a term's (and a group's) cards; the family lists a pupil's.
CREATE INDEX idx_report_cards_term_group ON report_cards (tenant_id, term_id, group_id);
CREATE INDEX idx_report_cards_student ON report_cards (tenant_id, student_id);

CREATE TABLE report_card_lines (
    report_card_id  uuid NOT NULL REFERENCES report_cards (id) ON DELETE CASCADE,
    tenant_id       uuid NOT NULL,
    subject_id      uuid NOT NULL,   -- config.subjects
    subject_name    text NOT NULL,
    position        int  NOT NULL CHECK (position >= 1),
    month_values    jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(month_values) = 'object'),
    term_average    numeric(5,2) NULL CHECK (term_average >= 0 AND term_average <= 100),
    PRIMARY KEY (report_card_id, subject_id)
);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE period_closures ENABLE ROW LEVEL SECURITY;
ALTER TABLE period_closures FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON period_closures
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE report_cards ENABLE ROW LEVEL SECURITY;
ALTER TABLE report_cards FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON report_cards
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE report_card_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE report_card_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON report_card_lines
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
