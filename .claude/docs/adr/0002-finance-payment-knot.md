# ADR 0002 — the finance payment knot: a derive-callback seam over a row lock

- Status: Accepted
- Date: 2026-08-06
- Context: M05 (finance), Phase 7. Registering a payment against a `charges` row
  must (a) never let the collected total exceed the charge's effective amount,
  (b) never pay a cancelled charge, and (c) re-derive and persist the charge's
  status — all atomically, safe against two cashiers taking the last peso at
  once. The status column is a **cache** of a projection over `payments`, not an
  independently mutable field.

## Decision

The postgres adapter owns the **transaction and the row lock**; the domain owns
the **rules**. `FinanceRepository.RegisterPayment` takes the payment plus a
callback:

```go
RegisterPayment(ctx, p domain.Payment,
    derive func(charge domain.Charge, newPaidCents int64) (status string, err error),
) (domain.Payment, ChargeWithPaid, error)
```

Inside one `withTenant` transaction the adapter: `SELECT … FOR UPDATE` the
charge (→ `ErrNotFound` if absent), `SUM(amount_cents)` its existing payments to
`currentPaid`, calls `derive(charge, currentPaid + p.Amount())`, and only if
`derive` returns no error inserts the payment and updates the charge's status.
`derive` is `usecase.derivePaidStatus`, which enforces the two vetoes
(overpay / cancelled → `KindInvalid`) and otherwise returns
`charge.DeriveStatus(newPaid)`. The `derive` error aborts the transaction, so a
vetoed payment writes **zero** rows.

The status **column** is written from the derived value; on read, the mapper
reconstructs the domain status from the row + its paid total (`Cancel()` for the
one non-derivable status, `WithStatus(paid)` otherwise), which is provably equal
to the stored value because a discount can only shrink `Effective()`.

## Why the callback-in-the-port shape

- **A func value is the smallest possible interface.** The adapter contributes
  exactly the lock and the transaction; it never reimplements a rule. Business
  logic does not leak into the adapter — it *calls* domain code.
- **The rules run under the row lock**, not in a read-then-write race in the use
  case. Concurrent payments serialize on the charge row (verified against real
  Postgres: an overpay-by-one-cent rolls back with the row count unchanged; a
  payment on a cancelled charge writes nothing).
- Alternatives considered: a full **Unit-of-Work port** (heavier; leaks
  transaction lifecycle into the use case) and an **optimistic version column**
  (retriable but adds a column + retry loop for a low-contention cashier window).
  The callback won on simplicity while keeping the veto atomic.

## Consequences / limits

- **Money is bounded.** `domain.NewCents` caps at `MaxCents` (1e15 centavos) so
  `currentPaid + amount` cannot overflow int64 and slip the overpay veto (the
  bug this pattern would otherwise hide). `NewPayment` routes its amount through
  `NewCents`.
- **The sibling mutation path (ApplyDiscount / CancelCharge) does NOT yet use
  this seam** — it is a two-transaction read-modify-write (`GetCharge` then
  `SaveCharge`) with no row lock, so a payment landing between the two can
  persist a stale status column (reads self-heal via the mapper; `ListOverdue`'s
  SQL predicate does not). Closing this means generalising the seam to a
  `MutateCharge(ctx, id, mutate func(ChargeWithPaid) (domain.Charge, error))`
  doing `FOR UPDATE` + fresh paid-total + apply + write in one transaction.
  Tracked as a Phase 7b follow-up (finance I3).
- **No idempotency key on payments yet** — a retried POST posts the money twice
  when the doubled total still fits the balance (partial payments duplicate
  silently). Tracked as Phase 7b (finance I1): an `idempotency_key` on the
  request + `UNIQUE (tenant_id, idempotency_key)` returning the existing payment
  on conflict.
- attendance and comms will copy this seam; the invariant "status is a
  re-derived projection, the column is a cache" travels with it. A follow-up
  (finance I2) moves `derivePaidStatus` onto the entity
  (`Charge.StatusAfterPayment`) so the use case and the integration test bind the
  **same** domain function instead of a copy.
