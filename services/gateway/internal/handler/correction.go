package handler

import (
	"net/http"
	"strings"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// maxCorrectionReason is the contract's bound on a correction's reason.
const maxCorrectionReason = 500

// correctDepositRequest is a payment correction as the office sends it (T9).
// Who corrected it is NOT a field: it is the signed-in caller.
type correctDepositRequest struct {
	AmountCents    int64           `json:"amountCents"`
	PaidAt         string          `json:"paidAt"`
	Tenders        []client.Tender `json:"tenders"`
	ChargeIDs      []string        `json:"chargeIds"`
	Reason         string          `json:"reason"`
	IdempotencyKey string          `json:"idempotencyKey"`
}

// invalid checks the request against the contract at the edge. Whether the
// tenders add up, and every money rule, is finance's.
func (req correctDepositRequest) invalid() string {
	reason := strings.TrimSpace(req.Reason)
	switch {
	case req.AmountCents <= 0:
		return "amountCents must be greater than zero"
	case reason == "":
		return "reason is required to correct a payment"
	case len([]rune(reason)) > maxCorrectionReason:
		return "reason is at most 500 characters"
	case len(req.Tenders) == 0:
		return "tenders: at least one method"
	case len(req.IdempotencyKey) > maxIdempotencyKey:
		return "idempotencyKey is at most 120 characters"
	}
	if msg := invalidTenders(req.Tenders); msg != "" {
		return msg
	}
	if untypedCard("", "", req.Tenders) {
		return msgCardWithoutType
	}
	return invalidChoice(req.ChargeIDs)
}

// maxIdempotencyKey bounds the caller's handle on the attempt.
const maxIdempotencyKey = 120

// CorrectDeposit handles POST /api/v1/finance/deposits/{id}/correct (T9,
// payments:edit). The original receipt is kept, superseded; finance creates
// the corrected one and the history row, and tells the family.
func (h *FinanceHandler) CorrectDeposit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "deposit not found", http.StatusNotFound)
		return
	}
	var req correctDepositRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if msg := req.invalid(); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	paidAt, err := parseCounterDate(req.PaidAt)
	if err != nil {
		http.Error(w, "paidAt must be a date like 2026-09-04", http.StatusBadRequest)
		return
	}
	got, cerr := h.svc.CorrectDeposit(r.Context(), client.CorrectionInput{
		DepositID: id, AmountCents: req.AmountCents, PaidAt: paidAt, Tenders: req.Tenders,
		ChargeIDs: req.ChargeIDs, Reason: strings.TrimSpace(req.Reason), IdempotencyKey: req.IdempotencyKey,
		By: h.receiverOf(r),
	})
	if cerr != nil {
		writeConnectError(w, cerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"original":         got.Original,
		"deposit":          got.Deposit,
		"allocations":      toPaymentResponses(got.Allocations),
		"surcharges":       toChargeResponses(got.Surcharges),
		"unallocatedCents": got.UnallocatedCents,
		"replayed":         got.Replayed,
		"correction":       got.Correction,
	})
}

// DepositCorrections handles GET /api/v1/finance/deposits/{id}/corrections:
// the receipt's correction history, its chain whole.
func (h *FinanceHandler) DepositCorrections(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "deposit not found", http.StatusNotFound)
		return
	}
	history, err := h.svc.DepositCorrections(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"corrections": history})
}
