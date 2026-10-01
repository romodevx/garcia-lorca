# ADR 0024: El estado de cuenta del alumno es UN cálculo, en finance

- Status: Accepted
- Date: 2026-10-01
- Context: T7 of the review sprint (plan §1 A4). The office needed an account
  statement per pupil (from the student list, with a PDF), and the parents' app
  already had one — built on the phone by `toLedger` from raw charges and
  payments, with no surcharges accrued, no discounts as lines and no running
  balance. Two ledgers would disagree, and the figure a parent reads must be the
  one the counter quotes.

## Decision

- `finance.GetStatement(student, family_view)` is the only place the ledger is
  computed (`domain.BuildStatement` orders it and runs the balance; the use case
  turns charges, deposits and loose payments into movements). The gateway, the
  web drawer, the app and the PDF render it; none of them derives money.
- Movement kinds: `charge`, `surcharge`, `discount`, `scholarship` (reserved for
  T18 — `discountKind` is where its reason maps), `payment`, `credit_note`
  (reserved for T8). Amounts are positive; the kind says the direction.
- **Surcharges** use the counter's rule: `accruedSurcharges` was extracted from
  `expectedFrom`, and both call it. What the daily rule has earned up to today
  and nobody wrote down is an `accrued` surcharge line dated today. It counts.
  `dueTodayCents` is `expectedFrom`'s figure — exactly what Registrar pago asks.
- **Early payment** is contingent (it depends on paying today), so it is not a
  movement: it lives in `dueTodayCents`. An **unpaid inscription** is priced by
  the ladder on today's date (`inscriptionPaidOn`, the counter's function), so
  the balance carries the price the counter asks. A discount written on a
  charge is a `discount` line with its reason; a reason the office typed by
  hand reads "Descuento" to the family (it is an internal note).
- **Payments** are one line per deposit (money as it arrived: method, folio),
  plus payments made straight against a charge. A deposit's slices are not
  lines. Overpayment shows as a negative balance (the family's credit).
- **Voided deposits**: the office sees them struck through with their reason,
  with no effect on the balance; the family is not shown them (the reason is
  internal, and the payment never happened as far as the balance is concerned).
- **Charges not yet open (T5)**: hidden from the family (as `GetFamilyAccount`
  already did); shown to the office marked "aún no exigible", outside the
  balance. A charge with money on it is always in play. Hence the office and the
  family read the SAME balance.
- **Cancelled charges**, and surcharges of a cancelled month, are not movements
  — except a cancelled charge that had money collected on it, which keeps a
  line for what was collected so that money reads as spent, not as a false
  credit (the account counts it the same way).
- The old `/finance/accounts/{id}` and `/family/students/{id}/account` stay for
  the payment counter and app builds already installed; the app's own build
  reads `/statement` and `toLedger` is gone.
- PDF: rendered in the gateway with the receipts' engine (ADR 0017), paginated.

## Consequences

- T8 (credit notes) and T18 (scholarships) add lines by emitting the reserved
  kinds; every reader already renders them.
- `pendingBalance` (statement) ≠ `totalBalanceCents` (old account): the
  statement nets the family's credit and includes the accrued surcharge. The
  web drawer and the app show the statement's.
- Dates are UTC days (see school-day-is-utc-day), like every finance date.
