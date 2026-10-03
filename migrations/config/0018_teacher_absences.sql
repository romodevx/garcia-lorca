-- config — teacher absences in the timetable (T19, ADR 0032).
--
-- WHY. A teacher who is sick, on vacation or at a medical appointment leaves
-- their classes without anybody in front of the group. The school wants to
-- register the absence once (who, what kind, which days, optionally which hours)
-- and see, on every date it covers, which classes are left "Sin cubrir" until a
-- substitute is assigned or the class is cancelled for that date (the T16
-- date-specific changes). This is NOT teacher attendance: nobody takes a roll
-- of teachers; an absence is planned or reported by the school.
--
-- WHAT.
--   teacher_absences (tenant, cycle, teacher, kind, starts_on, ends_on,
--                     start_minute, end_minute, note, created_by…, updated…,
--                     cancelled_at/by…)
--     kind: sick (Incapacidad) | vacation (Vacaciones) | personal (Asunto
--     personal) | other (Otro). Both dates inclusive and inside the cycle
--     (checked by config). start_minute/end_minute are both NULL for a full
--     day, or a half-open range [start, end) of minutes since midnight in
--     5-minute steps applying to EVERY date of the span (a medical appointment
--     08:00–10:00). Never deleted: a cancellation stamps cancelled_at/by so the
--     record and its history stay.
--     Overlapping active absences of one teacher are refused by config under
--     the cycle's timetable lock (the same lock every timetable write takes),
--     not by an EXCLUDE: "overlap" depends on the full-day/partial rule.
--   schedule_events gains absence_id and three actions (absence_created,
--     absence_updated, absence_cancelled). Its summary never carries the kind
--     or the note: the history is schedule:read, the kind and note are only
--     for schedule:write.
--
-- RLS: ENABLE + FORCE + tenant_isolation, like every config table.
-- Additive only: no data is created or transformed.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

CREATE TABLE teacher_absences (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL,
    cycle_id          uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    teacher_id        uuid NOT NULL REFERENCES teachers (id) ON DELETE RESTRICT,
    kind              text NOT NULL,
    starts_on         date NOT NULL,
    ends_on           date NOT NULL,
    start_minute      integer NULL,
    end_minute        integer NULL,
    note              text NOT NULL DEFAULT '',
    created_by        text NOT NULL,
    created_by_name   text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    cancelled_at      timestamptz NULL,
    cancelled_by      text NULL,
    cancelled_by_name text NULL,

    CONSTRAINT teacher_absences_kind_ck CHECK (kind IN ('sick', 'vacation', 'personal', 'other')),
    CONSTRAINT teacher_absences_dates_ck CHECK (ends_on >= starts_on),
    CONSTRAINT teacher_absences_time_ck CHECK (
        (start_minute IS NULL AND end_minute IS NULL)
        OR (start_minute IS NOT NULL AND end_minute IS NOT NULL
            AND start_minute >= 0 AND end_minute <= 1440 AND start_minute < end_minute
            AND start_minute % 5 = 0 AND end_minute % 5 = 0)),
    CONSTRAINT teacher_absences_note_ck CHECK (char_length(note) <= 500),
    CONSTRAINT teacher_absences_cancel_ck CHECK (
        (cancelled_at IS NULL AND cancelled_by IS NULL AND cancelled_by_name IS NULL)
        OR (cancelled_at IS NOT NULL AND cancelled_by IS NOT NULL AND cancelled_by_name IS NOT NULL))
);
-- Every read is "the absences of this cycle that touch these dates".
CREATE INDEX idx_teacher_absences_cycle_dates ON teacher_absences (tenant_id, cycle_id, starts_on, ends_on);
CREATE INDEX idx_teacher_absences_teacher ON teacher_absences (teacher_id);

ALTER TABLE teacher_absences ENABLE ROW LEVEL SECURITY;
ALTER TABLE teacher_absences FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON teacher_absences
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE schedule_events ADD COLUMN absence_id uuid NULL;
ALTER TABLE schedule_events DROP CONSTRAINT schedule_events_action_check;
ALTER TABLE schedule_events ADD CONSTRAINT schedule_events_action_check CHECK (action IN (
    'block_created', 'block_updated', 'block_deleted', 'override_created', 'override_deleted',
    'absence_created', 'absence_updated', 'absence_cancelled'));

COMMIT;
