package usecase

import (
	"context"
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
	inPlay := inPlayOn(now)
	shown := func(ports.ChargeWithPaid) bool { return true }
	if family {
		shown = inPlay
	}
	account, terms, err := s.accountWithTerms(ctx, studentID, shown)
	if err != nil {
		return Statement{}, err
	}
	deposits, err := s.repo.ListDeposits(ctx, studentID)
	if err != nil {
		return Statement{}, err
	}

	accrued := accruedSurcharges(account.Charges, terms.plan.Policy(), terms.calendar, terms.lateFeeID, now)
	asOf := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	movements := chargeMovements(account.Charges, terms.lateFeeID, accrued, inPlay, asOf)
	movements = append(movements, depositMovements(deposits, family)...)
	movements = append(movements, loosePaymentMovements(account.Payments, account.Charges)...)

	ledger, totals := domain.BuildStatement(movements)
	return Statement{
		Movements:          ledger,
		Totals:             totals,
		DueTodayCents:      account.DueTodayCents,
		SurchargeCents:     account.SurchargeCents,
		EarlyDiscountCents: account.EarlyDiscountCents,
		UnallocatedCents:   account.UnallocatedCents,
		AsOf:               account.AsOf,
	}, nil
}

// chargeMovements turns the charges into ledger lines: the charge (or written
// surcharge), its discount, and the surcharge the daily rule has earned on it.
//
// Skipped, as the counter skips them: cancelled charges, and surcharges whose
// month was cancelled — nobody owes those.
func chargeMovements(
	charges []ports.ChargeWithPaid, lateFeeID string, accrued map[string]int64,
	inPlay func(ports.ChargeWithPaid) bool, asOf time.Time,
) []domain.Movement {
	cancelled := map[string]bool{}
	for _, c := range charges {
		if c.Charge.Status() == domain.StatusCancelled {
			cancelled[c.Charge.ID()] = true
		}
	}
	out := make([]domain.Movement, 0, len(charges))
	for _, c := range charges {
		ch := c.Charge
		if cancelled[ch.ID()] {
			continue
		}
		kind := domain.MovementCharge
		if ch.ConceptID() == lateFeeID {
			if parent, ok := feeParent(ch.ExternalRef()); ok && cancelled[parent] {
				continue
			}
			kind = domain.MovementSurcharge
		}
		notYetDue := !inPlay(c)
		out = append(out, domain.Movement{
			Kind: kind, Date: ch.DueDate(), Description: ch.Description(),
			AmountCents: ch.Amount().Int64(), ChargeID: ch.ID(), NotYetDue: notYetDue,
		})
		if d := ch.Discount().Int64(); d > 0 {
			out = append(out, domain.Movement{
				Kind: discountKind(ch.DiscountReason()), Date: ch.DueDate(),
				Description: discountLabel(ch.DiscountReason()) + " · " + ch.Description(),
				AmountCents: d, ChargeID: ch.ID(), NotYetDue: notYetDue,
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
	return out
}

// depositMovements is the money as it arrived: one line per deposit, whatever
// it paid. A voided deposit is shown to the office with no effect on the
// balance; the family is not shown it.
func depositMovements(deposits []domain.Deposit, family bool) []domain.Movement {
	out := make([]domain.Movement, 0, len(deposits))
	for _, d := range deposits {
		if d.Voided() && family {
			continue
		}
		out = append(out, domain.Movement{
			Kind: domain.MovementPayment, Date: d.PaidAt(), Description: "Pago",
			AmountCents: d.Amount().Int64(), DepositID: d.ID(),
			Method: d.Method(), CardType: d.CardType(), Reference: d.Reference(), Folio: d.Folio(),
			Voided: d.Voided(), VoidReason: d.VoidReason(),
		})
	}
	return out
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
		description := "Pago"
		if d := descriptions[p.ChargeID()]; d != "" {
			description += " · " + d
		}
		out = append(out, domain.Movement{
			Kind: domain.MovementPayment, Date: p.PaidAt(), Description: description,
			AmountCents: p.Amount().Int64(), ChargeID: p.ChargeID(),
			Method: p.Method(), Reference: p.Reference(),
		})
	}
	return out
}

// discountKind says which ledger line a discount is. Every reason is a
// discount today; T18 maps the scholarship's reason to MovementScholarship
// here, and every reader already renders that kind.
func discountKind(string) domain.MovementKind {
	return domain.MovementDiscount
}

// discountLabel names a discount reason the way the office says it. A reason
// the school typed itself is shown as typed.
func discountLabel(reason string) string {
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
		return reason
	}
}
