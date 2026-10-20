// Package financeclient implements the uniform shop's FinanceWriter port over a
// Connect client to the finance bounded context.
//
// This is a WRITE seam into another service, which is unusual in this codebase
// and deliberate. The alternative was a second till, and a school cannot
// reconcile two: the uniform a family buys has to land on the same account
// statement as their tuition, be taken by the same cashier, and appear in the
// same end-of-day count.
//
// Three calls carry the whole relationship:
//
//	EnsureConcept         the catalogue entry every uniform charge hangs off
//	CreateCharge          idempotent through external_ref (ADR 0008)
//	CancelChargeIfUnpaid  the guarded cancel, decided under the charge's row
//	                      lock, which is what makes the expiry sweep safe to run
//	                      while a cashier is collecting (ADR 0002's posture)
package financeclient

import (
	"context"
	"net/http"
	"strings"
	"sync"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/finance/v1/financev1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

// Compile-time proof the adapter satisfies the port.
var _ ports.FinanceWriter = (*Client)(nil)

// conceptName is the catalogue entry every uniform charge hangs off. One entry,
// not one per order: the concept is what a school reads in a dropdown, and three
// hundred rows a year would make that dropdown useless within a term.
const conceptName = "Uniformes"

// Client raises and settles the money side of the uniform shop.
type Client struct {
	rpc financev1connect.FinanceServiceClient

	// The concept id changes once a year at most, so it is resolved once and
	// kept. The mutex is not contention — it is one lookup on a cold start.
	mu        sync.Mutex
	conceptID string
	// refundCategoryID is the expense category refunds go under (C2-015),
	// resolved once like the concept.
	refundCategoryID string
}

// refundCategoryName is the expense category a later day's refund of a
// returned sale is recorded under (C2-015).
const refundCategoryName = "Devoluciones de uniformes"

// New builds a Client targeting the finance base URL over httpClient.
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: financev1connect.NewFinanceServiceClient(httpClient, baseURL)}
}

// EnsureConcept returns the id of the "Uniformes" charge concept, creating it if
// the school has not got one. It is idempotent by construction: finance reports a
// duplicate name as a conflict, and a conflict here means somebody else created
// it first, which is the outcome we wanted.
func (c *Client) EnsureConcept(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conceptID != "" {
		return c.conceptID, nil
	}

	if id, found, err := c.findConcept(ctx); err != nil {
		return "", err
	} else if found {
		c.conceptID = id
		return id, nil
	}

	res, err := c.rpc.CreateConcept(ctx, connect.NewRequest(&financev1.CreateConceptRequest{
		Name: conceptName,
		// No default amount: a uniform charge is always the sum of what was
		// actually sold, never a fixed figure somebody might edit by accident.
		DefaultAmountCents: 0,
		Recurrence:         "once",
	}))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeAlreadyExists {
			// Another instance won the race. Read back what it made.
			id, found, findErr := c.findConcept(ctx)
			if findErr != nil {
				return "", findErr
			}
			if found {
				c.conceptID = id
				return id, nil
			}
		}
		return "", mapErr(err, "creating the uniforms charge concept")
	}
	c.conceptID = res.Msg.GetConcept().GetId()
	return c.conceptID, nil
}

// findConcept looks for the uniforms concept in the school's catalogue,
// including retired ones: a school that deactivated it still has issued charges
// pointing at it, and minting a second entry with the same name would split the
// history in two.
func (c *Client) findConcept(ctx context.Context) (string, bool, error) {
	res, err := c.rpc.ListConcepts(ctx, connect.NewRequest(&financev1.ListConceptsRequest{
		IncludeInactive: true,
	}))
	if err != nil {
		return "", false, mapErr(err, "reading the charge catalogue")
	}
	for _, concept := range res.Msg.GetConcepts() {
		if strings.EqualFold(strings.TrimSpace(concept.GetName()), conceptName) {
			return concept.GetId(), true, nil
		}
	}
	return "", false, nil
}

// CreateCharge raises a charge against a family.
//
// ExternalRef is the delivery's own name, and it does two jobs at once: it
// exempts the charge from the ledger's "a student owes a concept once per cycle"
// rule — without which a family could never buy a uniform twice in a school year
// — and it deduplicates a retry whose response was lost, so the family is not
// billed twice for one delivery.
func (c *Client) CreateCharge(ctx context.Context, in ports.CreateChargeInput) (ports.ChargeRef, error) {
	res, err := c.rpc.CreateCharge(ctx, connect.NewRequest(&financev1.CreateChargeRequest{
		StudentId:           in.StudentID,
		CycleId:             in.CycleID,
		ConceptId:           in.ConceptID,
		AmountCentsOverride: in.AmountCents,
		DescriptionOverride: in.Description,
		DueDate:             in.DueDate,
		ExternalRef:         in.ExternalRef,
	}))
	if err != nil {
		return ports.ChargeRef{}, mapErr(err, "raising the charge")
	}
	charge := res.Msg.GetCharge()
	return ports.ChargeRef{
		ID:             charge.GetId(),
		PaidCents:      charge.GetPaidCents(),
		RemainingCents: charge.GetRemainingCents(),
		Status:         charge.GetStatus(),
	}, nil
}

// RegisterPayment records money taken at the counter. The idempotency key is the
// delivery's, so a retried call returns the original payment rather than
// collecting from the family a second time.
//
// The tenders travel as given (T8); a single one also fills method and
// reference, which a finance from before T8 understands.
func (c *Client) RegisterPayment(ctx context.Context, chargeID string, amountCents int64, tenders []ports.Tender, idempotencyKey string) error {
	req := &financev1.RegisterPaymentRequest{
		ChargeId:       chargeID,
		AmountCents:    amountCents,
		IdempotencyKey: idempotencyKey,
		Tenders:        make([]*financev1.Tender, 0, len(tenders)),
	}
	for _, t := range tenders {
		req.Tenders = append(req.Tenders, &financev1.Tender{
			Method: t.Method, AmountCents: t.AmountCents, Reference: t.Reference, CardType: t.CardType,
		})
	}
	if len(tenders) == 1 {
		req.Method, req.Reference = tenders[0].Method, tenders[0].Reference
	}
	_, err := c.rpc.RegisterPayment(ctx, connect.NewRequest(req))
	if err != nil {
		return mapErr(err, "recording the payment")
	}
	return nil
}

// CancelChargeIfUnpaid voids a charge only if nothing was collected against it.
//
// A refusal is NOT a failure here, and the shop reads it as an answer: it means
// the cashier got there first, so the goods must stay where they are. Reporting
// it as a conflict rather than an error is what turns a race between the expiry
// sweep and the till into something observable instead of something that quietly
// voids a charge with money behind it.
func (c *Client) CancelChargeIfUnpaid(ctx context.Context, chargeID string) error {
	_, err := c.rpc.CancelChargeIfUnpaid(ctx, connect.NewRequest(&financev1.CancelChargeIfUnpaidRequest{
		ChargeId: chargeID,
	}))
	if err != nil {
		return mapErr(err, "cancelling the charge")
	}
	return nil
}

// VoidSalePayments voids a returned sale's payments and cancels its charge
// (C2-015). Finance answers a done void with what it voided, so a retry is safe.
func (c *Client) VoidSalePayments(ctx context.Context, chargeID, reason, actorUserID string) error {
	_, err := c.rpc.VoidSalePayments(ctx, connect.NewRequest(&financev1.VoidSalePaymentsRequest{
		ChargeId: chargeID, Reason: reason, ActorUserId: actorUserID,
	}))
	if err != nil {
		return mapErr(err, "voiding the sale's payments")
	}
	return nil
}

// RecordRefund records the refund of a sale returned on a later day as an
// expense of that day, under its own category (C2-015).
func (c *Client) RecordRefund(ctx context.Context, in ports.RefundInput) (string, error) {
	categoryID, err := c.refundCategory(ctx)
	if err != nil {
		return "", err
	}
	res, err := c.rpc.CreateExpense(ctx, connect.NewRequest(&financev1.CreateExpenseRequest{
		SpentOn: in.SpentOn, CategoryId: categoryID, AmountCents: in.AmountCents, Method: in.Method,
		Description: in.Description, ActorId: in.ActorUserID, ActorName: in.ActorName,
	}))
	if err != nil {
		return "", mapErr(err, "recording the refund")
	}
	return res.Msg.GetExpense().GetId(), nil
}

// refundCategory returns the refunds category, creating it the first time.
// A category with the same name made by the office is reused, not duplicated.
func (c *Client) refundCategory(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refundCategoryID != "" {
		return c.refundCategoryID, nil
	}
	if id, found, err := c.findRefundCategory(ctx); err != nil || found {
		c.refundCategoryID = id
		return id, err
	}
	res, err := c.rpc.CreateExpenseCategory(ctx, connect.NewRequest(&financev1.CreateExpenseCategoryRequest{Name: refundCategoryName}))
	if err != nil {
		if connect.CodeOf(err) != connect.CodeAlreadyExists {
			return "", mapErr(err, "creating the refunds category")
		}
		id, found, findErr := c.findRefundCategory(ctx)
		if findErr != nil || !found {
			return "", mapErr(err, "creating the refunds category")
		}
		c.refundCategoryID = id
		return id, nil
	}
	c.refundCategoryID = res.Msg.GetCategory().GetId()
	return c.refundCategoryID, nil
}

func (c *Client) findRefundCategory(ctx context.Context) (string, bool, error) {
	res, err := c.rpc.ListExpenseCategories(ctx, connect.NewRequest(&financev1.ListExpenseCategoriesRequest{}))
	if err != nil {
		return "", false, mapErr(err, "listing expense categories")
	}
	for _, cat := range res.Msg.GetCategories() {
		if strings.EqualFold(strings.TrimSpace(cat.GetName()), refundCategoryName) {
			return cat.GetId(), true, nil
		}
	}
	return "", false, nil
}

// mapErr translates a Connect code back into the shared kinds the use case
// branches on. AlreadyExists becomes KindConflict specifically so the caller can
// tell "somebody beat me to it" — a normal outcome — from a real failure.
func mapErr(err error, what string) error {
	switch connect.CodeOf(err) {
	case connect.CodeNotFound:
		return glerrors.ErrNotFound
	case connect.CodeAlreadyExists:
		return glerrors.Wrap(glerrors.KindConflict, what, err)
	case connect.CodeInvalidArgument:
		return glerrors.Wrap(glerrors.KindInvalid, what, err)
	default:
		return glerrors.Wrap(glerrors.KindInternal, "finance: "+what, err)
	}
}
