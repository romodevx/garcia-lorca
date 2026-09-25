# ADR 0016: A counter sale to a pupil with no cycle yet

- Status: Accepted
- Date: 2026-09-25
- Context: Task 10 of the September adjustments. The school sells uniforms to
  pupils registered but not yet placed in a group, and wants the sale in
  Finanzas under the pupil's name (owner's decision: enrolled pupils and pupils
  without a group; not people who are not pupils).

## Problem

Every finance charge named a student AND a cycle (`charges.cycle_id NOT NULL`),
and every uniforms order for a pupil named a cycle. A pupil with no group has no
cycle, so the counter refused the sale ("aún no está inscrito en un ciclo").

## Decision

- **finance/0017:** `charges.cycle_id` is nullable, under
  `CHECK (cycle_id IS NOT NULL OR external_ref <> '')`. A charge with no cycle
  is only ever another module's sale, and answers to its ref (the uniforms
  delivery). The school's own charges — tuition, inscription, hand-typed — still
  name their cycle. Domain: `NewSaleCharge` (no cycle, ref required); `NewCharge`
  still requires a cycle; `CreateCharge` takes the cycle-free path only when the
  cycle is empty and a ref is given.
- **uniforms/0009:** an order may name a pupil and no cycle when it comes from
  the counter. Family and staff orders for a child still name their cycle.

A NULL cycle matches no `cycle_id = ?` read: billing runs, cycle ledgers and
overdue-by-cycle never see these charges; the pupil's account (by student) and
the income report (by payment) do.

## Found on the way

The uniforms finance client created its concept with recurrence `one_off`, which
finance does not know (`once` | `monthly`). On a school with no "Uniformes"
concept yet, no counter sale could ever reach finance: every one was flagged
"no se pudo preparar el concepto de cobro en Finanzas". Fixed (`once`); the test
stub now refuses an unknown recurrence as the ledger does. Sales flagged before
the fix stay flagged for a person to reconcile.

## Consequences

- An anonymous counter sale ("Venta sin alumno", with a note) still cannot be
  charged in finance — it names no student — and stays flagged. Out of scope by
  the owner's answer (no customers who are not pupils).
