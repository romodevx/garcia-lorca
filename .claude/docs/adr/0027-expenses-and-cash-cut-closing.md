# ADR 0027: Egresos y cierre del corte — en finance, con un motor de reportes reutilizable

- Status: Accepted
- Date: 2026-10-01
- Context: T10 of the review sprint (plan §T10, §7 answers 7 and 14). The school
  records what comes in but not what goes out; the day's cut is a figure nobody
  counts against the drawer, and nothing says when yesterday's figures moved.
  Reports per day/week/month/year were asked for in PDF and Excel.

## Decision

- **Expenses live in finance** (finance/0024): `expense_categories` (the school's
  own list, unique by name ignoring case, deactivated never deleted),
  `expenses` (day, category, amount in cents, method cash|card|transfer,
  description, optional receipt), `expense_events` (one row per create / edit /
  cancel / receipt change: before and after as JSON, who, when, optional
  reason). Cancelled is a state with its reason — never a DELETE. All four
  tables (with `cash_cut_closures`) are RLS ENABLE + FORCE + tenant_isolation.
  An edit locks the row and writes the event in the same transaction, so each
  history entry's "before" is what that edit replaced. A credit note is not a
  way to pay a supplier: expenses refuse it.
- **Receipts use the pupils' photos mechanism**: a private GCS bucket
  (`expense_receipts`, Terraform; fake-gcs locally), the service holds only the
  object key, type and size; JPEG/PNG/PDF judged by magic bytes, ≤ 5 MB; the
  gateway serves the bytes after `expenses:read`, `nosniff`, `no-store` and a
  `sandbox` CSP (it is an upload served from the app's origin). Unset
  `EXPENSE_RECEIPTS_BUCKET` ⇒ receipts off, expenses on.
- **Money by method** (`FinanceSummary`): income is money received — tenders of
  deposits that count (not voided, not superseded) and payments taken outside a
  deposit — on the day it changed hands: the same money the cash-up counts.
  Credit notes are reported apart (ADR 0025). Balance = income − expenses, per
  method and in total. The report's "income by concept" is the existing
  `IncomeByConcept` (what payments settled), labelled "Total aplicado": it can
  differ from money received by what a family left as credit.
- **Cash cut**: the existing `DailyCashUp` is extended, not duplicated. With
  expenses wired it adds the day's expenses (cancelled listed, not counted),
  expenses by method, the **expected cash** (cash in − cash out; card and
  transfer never touch the drawer), the live closure and the reopened ones.
  **Closing** stores a snapshot (income/expenses by method, credit notes,
  expected, counted, difference = counted − expected, who, when, note); one
  live closure per day (partial unique index). **Modified after close** is
  asked of the database, not inferred from totals: a deposit dated that day
  captured, voided or superseded (T9) after the close, a counter payment
  captured after it, or an expense event after it that dates the expense on
  that day or moved it off it — counting only events that move money (a
  record, a cancel, an edit of amount, method or day; a receipt attached the
  next morning or a reworded description does not flag the cut). The closure
  is stamped with the app's clock before the cut is read, so a movement
  committed in between flags the cut (a false "modified" is preferred to a
  missed one). The web closes the day the cut names (`day` in the response),
  never "today" at the moment of the POST. **Reopen** (with reason) stamps the closure; the
  day may be closed again and keeps its history.
- **Permissions** (iam/0027): `expenses:read` (list, history, receipts, the
  summary and reports — aggregates only, no family named, which is why it may
  be delegated while `finance:read` stays owner-only) and `expenses:write`
  (record, edit, cancel, categories, receipts, **close** the cut). Both the
  owner's by default, delegable (owner answer 7). **Reopening** a closed cut is
  the director's: `finance:write`.
- **Visibility**: the cash-up GET (finance:read OR collections:read) carries
  the T10 fields only to a caller with `expenses:read` while module_expenses
  is on; the gateway empties them for everyone else (the cashier).
- **Flag** `module_expenses` (default OFF). Every T10 route needs
  `module_finance` AND `module_expenses`. The cash-up GET keeps answering to
  `module_finance` and carries the new fields (empty when unused).
- **Report engine** (`services/gateway/internal/report`): a tabular `Document`
  (title, header lines, tables of text / money / counts), `Period` (daily,
  weekly Monday–Sunday, monthly, yearly; UTC days, Spanish labels) and two
  renderers — a letter PDF with the receipts' engine and faces (gopdf +
  Poppins, ADR 0017) that paginates repeating column heads, and an `.xlsx`
  (**excelize v2.11.0**, BSD-3, pinned; govulncheck clean) with one sheet per
  table, money as numbers with a currency format and text never written as a
  formula. It knows nothing of finance: `internal/expensereport` arranges
  finance's figures into it; T12's attendance reports reuse the engine.

## Consequences

- Dates are UTC days (see school-day-is-utc-day): an expense keyed after 18:00
  local on its day is still its day only if the office types the date; the
  default "today" is finance's UTC day, like every finance date.
- The cut's "modified after close" is a flag, not a diff: the snapshot keeps
  what was closed, the screen shows today's figures beside it.
- A receipt replaced or removed is recoverable for 7 days (bucket versioning).
