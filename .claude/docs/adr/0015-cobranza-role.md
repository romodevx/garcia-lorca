# ADR 0015: The Cobranza role and the counter's own permissions

- Status: Accepted
- Date: 2026-09-24
- Context: Task 9 of the September adjustments. The owner asked for a role that
  collects payments without seeing or changing the rest of finance, and chose
  new permissions over reusing `finance:*`.

## Problem

Every finance route answered to `finance:read` or `finance:write`, and iam's
owner-only guard keeps `finance:read` on the owner role alone — it resolves any
family's ledger with no per-object scoping. So a cashier could not be given the
counter without also being given tuition tables, billing runs, discounts,
cancellations, voids and the income report; and, because of the guard, could not
be given it at all.

## Decision

Two new permissions describe the counter, and the gateway accepts either the
finance permission or the counter's on the counter's routes only:

| Permission | Routes (gateway) |
|---|---|
| `collections:read` | `GET /finance/accounts/{id}`, `GET /finance/expected`, `GET /finance/overdue`, `GET /finance/charges?studentId=` (one pupil only — a whole cycle's ledger stays `finance:read`), `GET /finance/cash-up`, `GET /finance/deposits`, `POST /finance/deposits/preview` |
| `payments:write` | `POST /finance/payments`, `POST /finance/deposits` |

Everything else in finance — tuition tables, concepts, billing runs, charges
created by hand, discounts, cancellations, voiding a deposit, the income report —
still answers to `finance:read`/`finance:write`, owner-only.

iam migration 0021 gives the owner both permissions (so it can delegate them:
the caller-subset guard lets an admin grant only what it holds) and seeds the
role `cobranza` (fixed id `…00a3`): `students:read`, `students:list`,
`collections:read`, `payments:write`.

Neither permission is owner-only. Reading every family's balance is precisely
this role's job; that is the difference with `finance:read`, which also reaches
the school's income and the tables that set prices.

## Consequences

- The counter (web "Registro de pagos") gates on `payments:write`; Finanzas
  keeps gating on `finance:read`.
- A cashier keys money under their own name (task 7's `received_by`), and the
  owner sees it, flagged when captured late, in the cash-up.
- Out of scope, by the owner's decision: voiding one's own deposit, and selling
  uniforms at the counter (`uniforms:write`). Either can be granted later as its
  own permission without touching this one.
