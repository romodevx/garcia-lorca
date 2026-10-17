# ADR 0028: Felicitaciones de cumpleaños — un job diario en comms, en la hora del colegio

- Status: Accepted
- Date: 2026-10-01
- Context: T11 of the review sprint (plan §T11). The school wants every pupil
  greeted in the parents' app on their birthday, automatically, with a text it
  writes. comms had no scheduler, and the system's only date logic (finance)
  reads the UTC day (memory `school-day-is-utc-day`).

## Decision

- **Cloud Scheduler → comms, the billing-run pattern.** A daily job at 07:00
  America/Mexico_City posts `CommsService/RunBirthdays` with an OIDC token from
  its own service account (`birthday-scheduler`, `run.invoker` on comms only).
  The gateway does not expose the run: no session can trigger a send. Repo
  config only (`birthday_schedule_enabled`, default false); not applied in T11.
- **"Today" is the school's local date, decided in comms.** comms is the first
  service to carry the school's time zone (`SCHOOL_TIMEZONE`, default
  America/Mexico_City, `time/tzdata` embedded; an unknown zone fails startup).
  An empty request date means today in that zone; a date can be passed to
  replay a day. This does not fix finance's UTC day — that is still a single
  fix to make where finance turns `now()` into a date.
- **Students answers "who was born on this day".** `ListStudents` gains a
  `BirthdayFilter{month, day, include_feb_29}`, paginated and tenant-scoped like
  every list. comms asks for pupils with an ACTIVE ENROLLMENT, whatever their
  admission status, and walks the pages (bounded). *(Amended 2026-10-16, owner
  decision D4 / plan T11: it used to ask for admission "Inscrito" only, so a
  pupil "en trámite" who already attends got no greeting.)*
- **February 29:** in a non-leap year those pupils are greeted on February 28
  (owner decision: still in their own month). The rule lives in comms; students
  only answers the question it is asked.
- **Idempotency through the existing ledger.** Each greeting is claimed in
  `processed_events` as `birthday:<student>:<year>` in the same transaction as
  its notification. A scheduler retry or a manual re-run greets nobody twice.
- **No family, no greeting — and no claim.** A pupil without a linked guardian
  account is counted (`without_guardians`) and skipped; the key is not claimed,
  so linking the family later that day and re-running still reaches them.
- **One failing pupil does not stop the rest**; the run then returns an error so
  the scheduler retries, and the ledger skips everybody already greeted.
- **The text is the school's.** `birthday_templates` (comms 0002, one row per
  tenant, RLS ENABLE + FORCE) with a default text while none is saved;
  `{nombre}` becomes the pupil's first name. Edited in the web (Avisos) via
  `GET/PUT /api/v1/comms/birthday-template`, permission **`comms:write`** — the
  one that already sends announcements (owner-held, delegable), so no new
  permission or iam migration.
- **Intake captures the birth date.** `CreateApplicant` accepts an optional
  `birth_date` (same rule as the profile: not after today). The quick-add
  drawer already had eight fields, so the date sits under a collapsed
  "Más datos (opcional)".

## Consequences

- Turning `module_comms` off does NOT stop the greetings (flags only gate the
  gateway; memory `feature-flags-env-not-db`). Stopping them is
  `birthday_schedule_enabled = false` + apply, or pausing the job.
- Locally, `make birthdays [DATE=aaaa-mm-dd]` does what the scheduler does.
- The birthday scan has no index: once a day over one school's enrolled pupils.
  An expression index on (month, day) is the move if that ever shows.
