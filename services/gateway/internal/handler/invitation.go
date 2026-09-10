package handler

import (
	"context"
	"net/http"
)

// The two anonymous doors this feature needs, and the reasoning that keeps them
// safe without a session behind them.
//
// Both are reachable by anyone with the URL, which is unavoidable: the person
// walking through them has no account yet, or cannot get into the one they
// have. What makes that acceptable is the token — 256 bits, single use, and
// dead after twenty-four hours — and the fact that neither route will ever tell
// a caller something they did not already know.

// InvitationService is the slice of iam these two routes need, declared here so
// the handler is testable without a running service.
type InvitationService interface {
	AcceptInvitation(ctx context.Context, token, password string) error
	RequestPasswordReset(ctx context.Context, email string) error
}

// InvitationHandler serves the anonymous half of the invitation flow.
type InvitationHandler struct{ svc InvitationService }

// NewInvitationHandler wires the handler to iam.
func NewInvitationHandler(svc InvitationService) *InvitationHandler {
	return &InvitationHandler{svc: svc}
}

type acceptInvitationRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// Accept handles POST /api/v1/auth/invitations/accept.
//
// The token travels in the BODY, not the path, and that is deliberate: a secret
// in a URL ends up in browser history, in access logs, and in the Referer
// header sent to every third-party resource the page loads. The activation page
// reads it from its own query string — where the mail had to put it — and posts
// it from there, so it never reaches our logs.
func (h *InvitationHandler) Accept(w http.ResponseWriter, r *http.Request) {
	var req acceptInvitationRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Token == "" {
		// Refused before hashing. An empty token hashes to a perfectly valid
		// digest, and a lookup on it is a lookup nobody should be able to make.
		http.Error(w, "el enlace está incompleto", http.StatusBadRequest)
		return
	}
	if err := h.svc.AcceptInvitation(r.Context(), req.Token, req.Password); err != nil {
		// Unknown, expired and already-used all arrive here as the same
		// NotFound, carrying iam's single message. Keeping them apart at this
		// layer would undo the work done to merge them below it.
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type passwordResetRequest struct {
	Email string `json:"email"`
}

// RequestReset handles POST /api/v1/auth/password-reset.
//
// It answers 204 no matter what: whether the address has an account, whether
// the mail went out, whether iam is even reachable. Anything else turns an
// unauthenticated endpoint into a way to read the school's staff and guardian
// directory one request at a time — and this system holds minors' records.
//
// The caller is told the same sentence either way, on the screen: "if that
// address has an account, you will get a message".
func (h *InvitationHandler) RequestReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		// Even a malformed body gets the same 204. A 400 here would say "your
		// JSON was wrong" and a 204 "your JSON was fine", which is one bit more
		// than this endpoint should ever emit.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// The error is deliberately dropped. iam already swallows everything for
	// the same reason; this is the second half of the same promise.
	_ = h.svc.RequestPasswordReset(r.Context(), req.Email)
	w.WriteHeader(http.StatusNoContent)
}
