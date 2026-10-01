# ADR 0025: Nota de crédito y pago combinado — los métodos son `tenders`

- Status: Accepted
- Date: 2026-10-01
- Context: T8 of the review sprint (plan §T8, §7 answer 5). Some families hold a
  credit note the school issued by hand. The school does **not** automate credit
  balances: the cashier picks "Nota de crédito", types its amount (and an
  optional free-text reference), and completes the rest with another method.
  A deposit and a payment had room for exactly one method.

## Decision

- **Tender** (`domain.Tender`): one method and its amount — `cash | card |
  transfer | credit_note`, a free-text reference, and the card type (debit /
  credit) on a card. A payment combines **1 to 4** tenders that must add up
  **exactly** to the amount to pay (`domain.ValidateTenders`); otherwise it is
  refused with KindInvalid and nothing is written. The same rule runs in the
  web (on submit), at the gateway (shape: ≤4, method, amount > 0, reference ≤
  120) and in finance/uniforms (the sum).
- **Partial payments (abonos) stay**: the cashier sets "Monto a pagar" (≤ what is
  owed, default the full amount) and the tenders must add up to it.
- **Storage** (finance/0021): `deposit_tenders(deposit_id, position, method,
  amount_cents, reference, card_type)` with RLS, the source of truth for how a
  deposit was paid. `deposits.method` and `deposits.card_type` are **kept as a
  derived summary** written with the deposit: the one tender's method/card type,
  or `mixed` / `''` when several — so every reader and report that read them keeps
  working. Existing deposits were backfilled 1:1 (additive; reversible by
  dropping the table). New code reads `Deposit.Tenders()`.
- **Payments carry one method each.** A deposit's allocations are cut where one
  tender ends and the next begins (`domain.SplitByTender`, allocations oldest
  debt first; **credit notes are spent first**, then money in the cashier's
  order — so what a deposit leaves as the family's credit is money, and the
  cash-up and income report agree on what the note settled). Each payment row
  keeps its card type (`payments.card_type`, finance/0022). A combined
  payment against one charge (Registrar pago, uniforms) is one payment row per
  tender in one transaction under the charge lock. So "how much of this concept a
  credit note settled" is a SUM, never an apportionment.
- **Idempotency** holds for the whole set: a deposit by its key as before; a
  combined payment stores the caller's key on the first row and `key#2`,
  `key#3`… on the rest (`domain.TenderKey`), and a replay looks the set up by
  those exact keys (`IN`, on the unique index) and returns every row.
- **A credit note is not money.** `DailyCashUp` puts it in `credit_note_cents`,
  outside the per-method figures and the total; `IncomeByConcept` likewise
  (`credit_note_cents`, outside the lines and the total). The statement emits a
  `credit_note` movement per credit-note tender (it lowers the balance like a
  payment does), a `payment` movement per money tender. One `PaymentRegistered`
  per payment (method `mixed` when combined) — a family is told once.
- Receipts (ticket + PDF) print one line per method when there are several;
  the uniforms sale receipt reads every payment on the sale's charge.
- Uniforms checks the tenders (methods, card type, and that they add up to
  the total it prices) **before** the garments leave the shelf — after its
  replay check, so a retried sale still replays — then hands them to finance.
  A sale with no student cannot use a credit note (a note is a family's).

## Consequences

- T9 (editing payments) corrects tenders, not a single method.
- A deposit's "method" in old reports may read `mixed`; the breakdown is in
  `deposit_tenders`.
- No credit balance is tracked (school decision): the credit note's amount is
  whatever the cashier types; its folio is free text.
