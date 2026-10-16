package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Statement is a student's ledger (T7 · A4): every movement in date order with
// the running balance, its totals, and the counter's figures for today.
//
// The ONE computation behind the office's drawer, the parents' app and the PDF.
// The balance is what is in play: opened charges and their surcharges, the
// daily surcharge earned up to today, less discounts and the money received.
// DueTodayCents is the counter's quote (expectedFrom), the same function
// Registrar Pago reads — the two never disagree.
type Statement struct {
	Movements []domain.Movement
	Totals    domain.StatementTotals
	// The counter's figures, as on AccountStatement.
	DueTodayCents      int64
	SurchargeCents     int64
	EarlyDiscountCents int64
	UnallocatedCents   int64
	AsOf               string
}

// GetStatement is the office's ledger: every charge, those not yet opened
// marked (T5) and kept out of the balance, and voided deposits struck through.
func (s *FinanceService) GetStatement(ctx context.Context, studentID string) (Statement, error) {
	return s.statement(ctx, studentID, false)
}

// GetFamilyStatement is the family's ledger: the same balance, without the
// charges that have not opened and without voided deposits (a cancelled
// receipt is the office's correction, and its reason is internal).
func (s *FinanceService) GetFamilyStatement(ctx context.Context, studentID string) (Statement, error) {
	return s.statement(ctx, studentID, true)
}

func (s *FinanceService) statement(ctx context.Context, studentID string, family bool) (Statement, error) {
	now := s.now()
	shown := func(ports.ChargeWithPaid) bool { return true }
	if family {
		shown = inPlayOn(now)
	}
	// One instant for the whole read: the accrued line, the ladder price and
	// the counter's figures are all worked out for the same day.
	account, terms, err := s.accountWithTerms(ctx, studentID, shown, now)
	if err != nil {
		return Statement{}, err
	}

	// The ledger prices each charge itself (ladder, then scholarship), from
	// the charges as stored. Names only label lines: the balance is the
	// account's, worked out by the same ledger.
	if err := s.learnNames(ctx, terms.scholarships, account.raw); err != nil {
		return Statement{}, err
	}
	movements, totals, err := ledger(account.raw, account.Payments, account.deposits, terms, now, family)
	if err != nil {
		return Statement{}, err
	}
	return Statement{
		Movements:          movements,
		Totals:             totals,
		DueTodayCents:      account.DueTodayCents,
		SurchargeCents:     account.SurchargeCents,
		EarlyDiscountCents: account.EarlyDiscountCents,
		UnallocatedCents:   account.UnallocatedCents,
		AsOf:               account.AsOf,
	}, nil
}

// ledger is THE balance (T5 · ISSUE-006): the statement's lines and totals,
// worked out on `now`. The account (office and family), the statement and the
// counter's "del año debe" all read totals.BalanceCents from here, so the five
// surfaces cannot disagree.
//
// charges are as stored (the ledger prices each one); payments are the
// student's, of which only those outside a deposit become lines of their own.
// `family` changes labels and which voided deposits are shown — never the
// balance: a line not counted for one reader is not counted for the other.
func ledger(
	charges []ports.ChargeWithPaid, payments []domain.Payment, deposits []domain.Deposit,
	terms collectionTerms, now time.Time, family bool,
) ([]domain.Movement, domain.StatementTotals, error) {
	movements, err := chargeMovements(charges, terms, inPlayOn(now), now, family)
	if err != nil {
		return nil, domain.StatementTotals{}, err
	}
	movements = append(movements, depositMovements(deposits, family)...)
	movements = append(movements, loosePaymentMovements(payments, charges)...)
	lines, totals := domain.BuildStatement(movements)
	return lines, totals, nil
}

// chargeMovements turns the charges into ledger lines: the charge (or written
// surcharge), its discount, and the surcharge the daily rule has earned on it.
//
// The counter's rules, applied the counter's way:
//   - surcharges of a cancelled month owe nothing and are not lines;
//   - a cancelled charge with nothing collected is not a line; one that had
//     money taken against it keeps a line for what was collected, so that
//     money reads as spent and not as a credit (as the account counts it);
//   - an unpaid inscription is priced by the ladder on `now`
//     (inscriptionPaidOn), so the balance carries the counter's price.
func chargeMovements(
	charges []ports.ChargeWithPaid, terms collectionTerms,
	inPlay func(ports.ChargeWithPaid) bool, now time.Time, family bool,
) ([]domain.Movement, error) {
	cancelled := map[string]bool{}
	for _, c := range charges {
		if c.Charge.Status() == domain.StatusCancelled {
			cancelled[c.Charge.ID()] = true
		}
	}
	priced, err := terms.scholarships.priceAll(charges, now)
	if err != nil {
		return nil, err
	}
	accrued := accruedSurcharges(priced, terms.plan.Policy(), terms.calendar, terms.lateFeeID, now)
	asOf := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]domain.Movement, 0, len(charges))
	for _, c := range charges {
		ch := c.Charge
		if cancelled[ch.ID()] {
			if c.PaidCents > 0 {
				out = append(out, domain.Movement{
					Kind: domain.MovementCharge, Date: ch.DueDate(),
					Description: ch.Description() + " · cancelado (se conserva lo cobrado)",
					AmountCents: c.PaidCents, ChargeID: ch.ID(),
				})
			}
			continue
		}
		kind := domain.MovementCharge
		if ch.ConceptID() == terms.lateFeeID {
			if parent, ok := feeParent(ch.ExternalRef()); ok && cancelled[parent] {
				continue
			}
			kind = domain.MovementSurcharge
		}
		if owed(c) > 0 {
			laddered, change, err := inscriptionPaidOn(c, terms.plan, terms.enrolmentID, now, now)
			if err != nil {
				return nil, err
			}
			if change != priceUnchanged {
				ch = laddered
			}
		}
		// The scholarship comes off the price the ladder left (T18).
		withBeca, err := terms.scholarships.price(ports.ChargeWithPaid{
			Charge: ch, PaidCents: c.PaidCents, PaidByDueCents: c.PaidByDueCents}, now)
		if err != nil {
			return nil, err
		}
		ch = withBeca.Charge
		notYetDue := !inPlay(c)
		out = append(out, domain.Movement{
			Kind: kind, Date: ch.DueDate(), Description: ch.Description(),
			AmountCents: ch.Amount().Int64(), ChargeID: ch.ID(), NotYetDue: notYetDue,
		})
		if d := ch.Discount().Int64(); d > 0 {
			out = append(out, domain.Movement{
				Kind: domain.MovementDiscount, Date: ch.DueDate(),
				Description: discountLabel(ch.DiscountReason(), family) + " · " + ch.Description(),
				AmountCents: d, ChargeID: ch.ID(), NotYetDue: notYetDue,
			})
		}
		if b := ch.Scholarship().Int64(); b > 0 {
			out = append(out, domain.Movement{
				Kind: domain.MovementScholarship, Date: ch.DueDate(),
				Description: scholarshipLabel(terms.scholarships.name(ch.ScholarshipID())) + " · " + ch.Description(),
				AmountCents: b, ChargeID: ch.ID(), NotYetDue: notYetDue,
			})
		}
		if pending := accrued[ch.ID()]; pending > 0 {
			out = append(out, domain.Movement{
				Kind: domain.MovementSurcharge, Date: asOf,
				Description: "Recargo por atraso · " + ch.Description(),
				AmountCents: pending, ChargeID: ch.ID(), Accrued: true,
			})
		}
	}
	return out, nil
}

// depositMovements is the money as it arrived: one line per tender of each
// deposit (T8) — a deposit paid one way is one line, as before; a credit note
// completed with cash is a credit_note line beside a payment line, with the
// deposit's folio on both. A voided deposit, or one superseded by its
// correction (T9), is shown to the office with no effect on the balance; the
// family is not shown it.
func depositMovements(deposits []domain.Deposit, family bool) []domain.Movement {
	out := make([]domain.Movement, 0, len(deposits))
	for _, d := range deposits {
		// The family reads the money as it stands: neither a cancelled receipt
		// nor one replaced by its correction (T9).
		if !d.Counts() && family {
			continue
		}
		corrects := d.CorrectsFolio()
		if family {
			// The family never sees the receipt it replaced; naming it would
			// point at a folio missing from their own ledger.
			corrects = ""
		}
		for _, t := range d.Tenders() {
			out = append(out, domain.Movement{
				Kind: tenderKind(t.Method()), Date: d.PaidAt(), Description: tenderDescription(t.Method()),
				AmountCents: t.Amount().Int64(), DepositID: d.ID(),
				Method: t.Method(), CardType: t.CardType(), Reference: t.Reference(), Folio: d.Folio(),
				Voided: d.Voided(), VoidReason: d.VoidReason(),
				Superseded: d.Superseded(), SupersededByFolio: d.SupersededByFolio(),
				CorrectsFolio: corrects,
			})
		}
	}
	return out
}

// tenderKind: a credit note lowers the balance like money does, but it is its
// own kind of line — the family and the office read it as what it was.
func tenderKind(method string) domain.MovementKind {
	if method == domain.MethodCreditNote {
		return domain.MovementCreditNote
	}
	return domain.MovementPayment
}

func tenderDescription(method string) string {
	if method == domain.MethodCreditNote {
		return "Nota de crédito"
	}
	return "Pago"
}

// loosePaymentMovements is the money paid straight against a charge, outside
// any deposit (a counter sale, the original payment API). A deposit's slices
// are not lines of their own: the deposit is.
func loosePaymentMovements(payments []domain.Payment, charges []ports.ChargeWithPaid) []domain.Movement {
	descriptions := make(map[string]string, len(charges))
	for _, c := range charges {
		descriptions[c.Charge.ID()] = c.Charge.Description()
	}
	var out []domain.Movement
	for _, p := range payments {
		if p.DepositID() != "" {
			continue
		}
		description := tenderDescription(p.Method())
		if d := descriptions[p.ChargeID()]; d != "" {
			description += " · " + d
		}
		out = append(out, domain.Movement{
			Kind: tenderKind(p.Method()), Date: p.PaidAt(), Description: description,
			AmountCents: p.Amount().Int64(), ChargeID: p.ChargeID(),
			Method: p.Method(), CardType: p.CardType(), Reference: p.Reference(),
		})
	}
	return out
}

// scholarshipLabel is how a scholarship's line reads: "Beca <name>".
// A name that already says so ("Beca hermanos") is not said twice.
func scholarshipLabel(name string) string {
	switch {
	case name == "":
		return "Beca"
	case strings.HasPrefix(strings.ToLower(name), "beca"):
		return name
	default:
		return "Beca " + name
	}
}

// discountLabel names a discount reason the way the office says it. A reason
// the office typed itself is shown as typed to the office and as "Descuento"
// to the family: it is an internal note.
func discountLabel(reason string, family bool) string {
	switch reason {
	case "":
		return "Descuento"
	case domain.ReasonEarlyPayment:
		return "Pronto pago"
	case domain.ReasonEarlyPaymentGranted:
		return "Pronto pago autorizado"
	case domain.ReasonLevelDiscount:
		return "Descuento de nivel"
	case domain.ReasonPriceAtPaymentDate:
		return "Precio a la fecha de pago"
	default:
		if family {
			return "Descuento"
		}
		return reason
	}
}
