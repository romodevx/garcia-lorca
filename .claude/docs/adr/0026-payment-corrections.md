# ADR 0026: Corregir un pago — el original se sustituye, nunca se edita

- Status: Accepted
- Date: 2026-10-01
- Context: T9 of the review sprint (plan §T9, §7 answer 6). A cashier keys $500
  when the family handed over $1,000, the wrong day, cash for a transfer, or the
  wrong months. It must be correctable, with a mandatory reason and a history
  that loses nothing, by authorised roles only, recalculating what the payment
  affects, and the family must be told the right amount. `VoidDeposit` existed
  but loses the link between the wrong receipt and the right one.

## Decision

- **A correction is one transaction in finance** (`CorrectDeposit`), never an
  in-place edit:
  1. the original deposit is locked; a cancelled or already corrected one is a
     conflict (a second correction corrects its successor — the chain is a line);
  2. the family's charges are locked in the order every deposit locks them;
  3. the original is **released** exactly as a void releases it
     (`releaseDeposit`, shared with `VoidDeposit`): its payments go, the charges
     go back to the status their remaining payments imply, the
     price-at-payment-date discount and the unpaid daily surcharges it wrote go
     with it — **and** the early-payment reward the rule gave it for its date,
     when it was the last payment on the month (a reward the school granted by
     hand, `ReasonEarlyPaymentGranted`, is a person's decision and stays);
  4. the corrected deposit is planned and stored by **the same path as any
     deposit** (`prepareDeposit` + the planner, `storeDeposit`) on the
     corrected date, so the surcharge, the early reward and the inscription
     price are worked out again by the same arithmetic, not patched;
  5. the original is marked **superseded** (`superseded_at`, `superseded_by`):
     a distinct state from voided — its row, folio and tenders stay; it counts
     for nothing (statement, credit, cash-up, receipt credit);
  6. `deposit_corrections` records before/after (JSON snapshot: folio, amount,
     date, tenders, what it put on each charge), who (user id + name as it read
     then), when and the reason (required, ≤ 500).
- **Folios**: the corrected deposit gets **a new folio** (`P-2026-000124`) and
  `corrects_deposit_id` names the original; both receipts reprint — the new one
  says "Corrige a P-2026-000123", the original "SUSTITUIDO · Sustituido por
  P-2026-000124". A folio is never reused or rewritten: a printed receipt in a
  family's hands always matches the system.
- **Who received the money** stays the original cashier; who corrected it is in
  the history. The corrected deposit's `captured_at` is the correction's moment,
  so a cash-up shows it "capturado el …" when it is dated earlier.
- **Idempotency**: the request's key is the corrected deposit's idempotency key;
  a retry returns the stored correction (`replayed`) and changes nothing.
- **Permission** `payments:edit` (iam/0026): the owner's by default, delegable
  (not owner-only, owner answer 6). The gateway gates
  `POST /finance/deposits/{id}/correct` on it alone — not on `payments:write`
  nor `finance:write`; the history (`GET …/corrections`) on `payments:edit` or
  `finance:read`.
- **Event** `PaymentCorrected` (finance_outbox → `payment-events` → comms):
  one per correction, keyed on the corrected deposit, so comms' ledger dedupes
  redeliveries and never confuses it with the original `PaymentRegistered`.
  Push + notification "Pago actualizado — Tu pago fue actualizado: $1000.00
  MXN." (type `payment`). `ChargeSettled` only for charges the correction newly
  closed.
- **Cash-up**: the original day lists the superseded receipt apart
  ("Sustituidos · no cuentan"); the correction counts on the day it is dated.
  `superseded_at` is kept so T10 can say "modificado después del cierre".
- **Statement (ADR 0024)**: the office sees the original struck through with
  "Sustituido por P-…" and no balance effect, and the correction with "Corrige
  a P-…"; the family sees only the corrected payment.

## Consequences

- A correction that lowers a payment can leave an inscription unpaid again; no
  "un-settled" event exists (the same as a void today), so students keeps the
  pupil "inscrito". Follow-up if the school needs it.
- A superseded receipt reprints without concept lines (its payments moved to
  the correction); its history shows what it had paid.
- Loose payments (paid straight against a charge, e.g. a uniforms sale) are not
  deposits and are not corrected by this path.
