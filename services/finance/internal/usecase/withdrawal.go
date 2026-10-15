package usecase

import (
	"context"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

const withdrawalCancelReason = "baja"

// PreviewWithdrawal projects a dated withdrawal using the same charge pricing
// rules and date as the family account. It does not write to the ledger.
func (s *FinanceService) PreviewWithdrawal(ctx context.Context, studentID, cycleID, withdrawnOn string) (domain.WithdrawalPlan, error) {
	on, err := withdrawalDate(withdrawnOn)
	if err != nil {
		return domain.WithdrawalPlan{}, err
	}
	charges, err := s.withdrawalCharges(ctx, studentID, cycleID, on)
	if err != nil {
		return domain.WithdrawalPlan{}, err
	}
	return planWithdrawal(charges, on), nil
}

// WithdrawStudent applies a dated withdrawal. Repeated delivery is safe: the
// guarded charge cancellation and activity end date both become no-ops once
// already applied.
func (s *FinanceService) WithdrawStudent(ctx context.Context, studentID, cycleID, withdrawnOn string) (domain.WithdrawalPlan, error) {
	on, err := withdrawalDate(withdrawnOn)
	if err != nil {
		return domain.WithdrawalPlan{}, err
	}
	charges, err := s.withdrawalCharges(ctx, studentID, cycleID, on)
	if err != nil {
		return domain.WithdrawalPlan{}, err
	}
	plan := planWithdrawal(charges, on)
	for _, charge := range plan.ToCancel {
		if _, err := s.repo.CancelChargeIfUnpaidWithReason(ctx, charge.ID(), withdrawalCancelReason); err != nil {
			// A payment or another withdrawal can win after the preview read. The
			// row lock is authoritative; either conflict means this row must stay.
			if glerrors.KindOf(err) == glerrors.KindConflict {
				continue
			}
			return domain.WithdrawalPlan{}, err
		}
	}
	if err := s.endStudentActivityEnrollments(ctx, studentID, on); err != nil {
		return domain.WithdrawalPlan{}, err
	}
	return plan, nil
}

func withdrawalDate(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "withdrawal date is required")
	}
	on, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, glerrors.Wrap(glerrors.KindInvalid, "withdrawal date must be YYYY-MM-DD", err)
	}
	return on, nil
}

func (s *FinanceService) withdrawalCharges(ctx context.Context, studentID, cycleID string, on time.Time) ([]ports.ChargeWithPaid, error) {
	if strings.TrimSpace(studentID) == "" || strings.TrimSpace(cycleID) == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "student and cycle are required")
	}
	charges, err := s.repo.ListCharges(ctx, studentID, cycleID)
	if err != nil {
		return nil, err
	}
	ids, err := s.conceptIDs(ctx)
	if err != nil {
		return nil, err
	}
	pricer, err := s.pricerFor(ctx, []string{studentID}, ids)
	if err != nil {
		return nil, err
	}
	return pricer.priceAll(charges, on)
}

func planWithdrawal(charges []ports.ChargeWithPaid, on time.Time) domain.WithdrawalPlan {
	rows := make([]domain.Charge, 0, len(charges))
	paid := make(map[string]int64, len(charges))
	for _, row := range charges {
		rows = append(rows, row.Charge)
		paid[row.Charge.ID()] = row.PaidCents
	}
	return domain.PlanWithdrawal(rows, paid, on)
}

func (s *FinanceService) endStudentActivityEnrollments(ctx context.Context, studentID string, on time.Time) error {
	if s.activities == nil {
		return nil
	}
	activities, err := s.activities.ListActivities(ctx, true)
	if err != nil {
		return err
	}
	for _, activity := range activities {
		enrollments, err := s.activities.ListActivityEnrollments(ctx, activity.ID())
		if err != nil {
			return err
		}
		for _, enrollment := range enrollments {
			if enrollment.StudentID() != studentID {
				continue
			}
			if _, err := s.activities.EndEnrollments(ctx, activity.ID(), []string{studentID}, on); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

// ApplyWithdrawal is the event-consumer seam; PreviewWithdrawal remains a
// read-only projection for the RPC.
func (s *FinanceService) ApplyWithdrawal(ctx context.Context, studentID, cycleID, withdrawnOn string) error {
	_, err := s.WithdrawStudent(ctx, studentID, cycleID, withdrawnOn)
	return err
}
