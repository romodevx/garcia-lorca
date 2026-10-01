-- finance (T10) — expenses (egresos) and the closing of the daily cash cut.
--
-- THE PROBLEM. The school records what comes in (deposits, counter sales) but
-- not what goes out, so nobody can say what the month left, and the day's cut
-- is a figure on a screen: nobody counts the drawer against it, and nothing
-- stops yesterday's numbers moving after the office went home.
--
-- THE SHAPE.
--
--   * expense_categories: the school's own list (Papelería, Mantenimiento…).
--     Deactivated, never deleted — recorded expenses keep pointing at them.
--   * expenses: money paid out, dated the day it was paid, with its method,
--     description and an optional receipt (comprobante) kept in a PRIVATE
--     bucket — this row holds only the object key, the type the bytes proved
--     to be and the size. Cancelled is a state, never a DELETE.
--   * expense_events: the history. Every create / edit / cancel / receipt
--     change writes one row with the expense before and after (JSON), who and
--     when. spent_on and previous_spent_on are the days the event touches, so
--     a closed cut can ask "did anything of MY day change after I closed?"
--     even when an edit moved the expense to another day.
--   * cash_cut_closures: one closure of one day — the snapshot then (expected
--     cash, counted cash, difference, income and expenses by method), who and
--     when. Reopening stamps reopened_* on the row; it is not deleted, and the
--     day may be closed again. At most one LIVE closure per day (partial
--     unique index).
--
-- Money is integer cents. Schema only, additive; reversible by dropping the
-- four tables. RLS: ENABLE + FORCE + tenant_isolation on every table.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE expense_categories (
    id         uuid PRIMARY KEY,
    tenant_id  uuid NOT NULL,
    name       text NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 80),
    active     boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- One name per school, whatever its case: "Papelería" and "papelería" are the
-- same drawer of the filing cabinet.
CREATE UNIQUE INDEX uq_expense_categories_name ON expense_categories (tenant_id, lower(btrim(name)));

CREATE TABLE expenses (
    id                   uuid PRIMARY KEY,
    tenant_id            uuid NOT NULL,
    spent_on             date NOT NULL,
    category_id          uuid NOT NULL REFERENCES expense_categories (id) ON DELETE RESTRICT,
    amount_cents         bigint NOT NULL CHECK (amount_cents > 0 AND amount_cents <= 1000000000000000),
    method               text NOT NULL CHECK (method IN ('cash', 'card', 'transfer')),
    description          text NOT NULL DEFAULT '' CHECK (char_length(description) <= 500),
    status               text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled')),
    cancel_reason        text NOT NULL DEFAULT '',
    cancelled_at         timestamptz NULL,
    receipt_key          text NULL CHECK (receipt_key IS NULL OR receipt_key <> ''),
    receipt_content_type text NULL CHECK (receipt_content_type IS NULL
                                          OR receipt_content_type IN ('image/jpeg', 'image/png', 'application/pdf')),
    receipt_size_bytes   integer NULL CHECK (receipt_size_bytes IS NULL
                                             OR (receipt_size_bytes > 0 AND receipt_size_bytes <= 5242880)),
    created_by           uuid NULL,
    created_by_name      text NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),

    -- A cancelled expense says why and when; a live one carries neither.
    CONSTRAINT expenses_cancel_shape_ck CHECK (
        (status = 'cancelled' AND cancelled_at IS NOT NULL AND btrim(cancel_reason) <> '')
        OR (status = 'active' AND cancelled_at IS NULL AND cancel_reason = '')
    ),
    -- The receipt's three columns are set together or not at all.
    CONSTRAINT expenses_receipt_shape_ck CHECK (
        (receipt_key IS NULL) = (receipt_content_type IS NULL)
        AND (receipt_key IS NULL) = (receipt_size_bytes IS NULL)
    )
);

-- Every read is a date range (a day's cut, a month's report).
CREATE INDEX idx_expenses_spent_on ON expenses (tenant_id, spent_on);

CREATE TABLE expense_events (
    id                uuid PRIMARY KEY,
    tenant_id         uuid NOT NULL,
    expense_id        uuid NOT NULL REFERENCES expenses (id) ON DELETE RESTRICT,
    kind              text NOT NULL CHECK (kind IN ('created', 'edited', 'cancelled', 'receipt_attached', 'receipt_removed')),
    before            jsonb NOT NULL DEFAULT '{}'::jsonb,
    after             jsonb NOT NULL,
    reason            text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 500),
    -- The day the expense is dated after the event, and before it (the same
    -- unless an edit moved it).
    spent_on          date NOT NULL,
    previous_spent_on date NOT NULL,
    actor_id          uuid NULL,
    actor_name        text NOT NULL DEFAULT '',
    at                timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_expense_events_expense ON expense_events (tenant_id, expense_id, at);
-- "Did anything dated this day change after it was closed?"
CREATE INDEX idx_expense_events_days ON expense_events (tenant_id, spent_on, previous_spent_on, at);

CREATE TABLE cash_cut_closures (
    id                  uuid PRIMARY KEY,
    tenant_id           uuid NOT NULL,
    day                 date NOT NULL,
    expected_cash_cents bigint NOT NULL,
    counted_cash_cents  bigint NOT NULL CHECK (counted_cash_cents >= 0),
    difference_cents    bigint NOT NULL,
    -- {income:{cash,card,transfer,total}, expenses:{…}, creditNoteCents}
    snapshot            jsonb NOT NULL,
    note                text NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    closed_by           uuid NULL,
    closed_by_name      text NOT NULL DEFAULT '',
    closed_at           timestamptz NOT NULL DEFAULT now(),
    reopened_at         timestamptz NULL,
    reopened_by         uuid NULL,
    reopened_by_name    text NOT NULL DEFAULT '',
    reopen_reason       text NOT NULL DEFAULT '' CHECK (char_length(reopen_reason) <= 500),

    CONSTRAINT cash_cut_closures_difference_ck CHECK (difference_cents = counted_cash_cents - expected_cash_cents),
    CONSTRAINT cash_cut_closures_reopen_shape_ck CHECK (
        (reopened_at IS NULL AND reopen_reason = '')
        OR (reopened_at IS NOT NULL AND btrim(reopen_reason) <> '')
    )
);

-- One live closure per day; reopened ones stay as history.
CREATE UNIQUE INDEX uq_cash_cut_closures_live ON cash_cut_closures (tenant_id, day) WHERE reopened_at IS NULL;
CREATE INDEX idx_cash_cut_closures_day ON cash_cut_closures (tenant_id, day, closed_at);

ALTER TABLE expense_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE expense_categories FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON expense_categories
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE expenses ENABLE ROW LEVEL SECURITY;
ALTER TABLE expenses FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON expenses
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE expense_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE expense_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON expense_events
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE cash_cut_closures ENABLE ROW LEVEL SECURITY;
ALTER TABLE cash_cut_closures FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cash_cut_closures
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
