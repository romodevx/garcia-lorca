package gcp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/garcia-lorca/pkg/pubsub"
)

// bearerPrefix is the scheme of the Authorization header Cloud Run forwards from
// a Pub/Sub push request: "Bearer <oidc-jwt>".
const bearerPrefix = "Bearer "

// errUnverifiedCaller marks a request whose caller identity could not be
// confirmed as the expected push service account — a missing or malformed token,
// or one whose email claim does not match. It is answered 403 and never
// dispatched.
var errUnverifiedCaller = errors.New("push caller identity not verified")

// maxPushBodyBytes caps the request body a push delivery may carry. A Cloud
// Pub/Sub message is at most 10 MiB; base64 inflates the payload by ~⅓ and the
// JSON envelope adds a little more, so 16 MiB leaves headroom while refusing a
// body no legitimate delivery would send.
const maxPushBodyBytes = 16 << 20

// pushEnvelope is the JSON body Cloud Pub/Sub POSTs to a push endpoint. Data is
// the message payload base64-encoded (standard encoding), which encoding/json
// decodes into the []byte for us — so an invalid base64 payload surfaces as a
// decode error and is answered 400, never dispatched. Attributes carries the
// same routing headers the pull path receives (the "type" attribute among them).
// The remaining fields are delivery metadata this consumer does not need.
type pushEnvelope struct {
	Message struct {
		Data        []byte            `json:"data"`
		Attributes  map[string]string `json:"attributes"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// NewPushHandler adapts a Cloud Pub/Sub PUSH subscription to a pubsub.Handler,
// so a service consumes the very same events through the same handler whether
// they arrive by streaming pull or by push. This is what lets comms run under
// Cloud Run scale-to-zero: with no process calling Receive, Pub/Sub delivers by
// POSTing here instead, and the platform scales the container up to serve it.
//
// It decodes the push envelope, hands the payload and attributes to h exactly as
// the pull adapter does, and maps the outcome onto the ack/nack the platform
// reads from the HTTP status:
//
//   - decoded and h returns nil  -> 204: Pub/Sub acks, the message is done.
//   - h returns an error         -> 500: Pub/Sub nacks, redelivers, and routes
//     to the DLQ after the subscription's max attempts. h must be idempotent
//     (comms' is, via its processed-events ledger), so redelivery is safe.
//   - unverified caller          -> 403: never dispatched (see AUTH below).
//   - malformed body / bad base64 -> 400: never dispatched. Cloud Run push treats
//     EVERY non-2xx as a nack, so a 400 is redelivered and eventually DLQ'd like
//     any other failure; it is used here only to distinguish a body this consumer
//     could never parse — a sender bug — from a transient processing failure.
//
// AUTH: on Cloud Run the frontend verifies the OIDC token minted by the push
// subscription's service account BEFORE the request reaches this container. That
// gate is run.invoker, which is coarse: the shared runtime SA holds it on comms
// too, so any internal workload could POST a forged envelope. When expectedSA is
// non-empty, the handler adds defense-in-depth by pinning the caller to that
// exact identity — it reads the email claim of the forwarded OIDC token (the
// signature is already verified upstream, so only the claim is decoded) and
// requires it to equal expectedSA. When expectedSA is empty (local docker-compose
// / emulator, where no token is forwarded) the check is skipped entirely.
func NewPushHandler(h pubsub.Handler, expectedSA string, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Method-aware muxes route only POST here, but the handler stays correct
		// when mounted without one.
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if expectedSA != "" {
			if err := verifyCaller(r.Header.Get("Authorization"), expectedSA); err != nil {
				log.Warn("comms rejected a push delivery from an unverified caller", slog.Any("error", err))
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		var env pushEnvelope
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPushBodyBytes))
		if err := dec.Decode(&env); err != nil {
			log.Warn("comms rejected a malformed push delivery", slog.Any("error", err))
			http.Error(w, "malformed push envelope", http.StatusBadRequest)
			return
		}

		msg := pubsub.Message{Data: env.Message.Data, Attributes: env.Message.Attributes}
		if err := h(r.Context(), msg); err != nil {
			// A retryable failure (e.g. the ledger unreachable): nack so Pub/Sub
			// redelivers. The handler swallows and acks the messages it can never
			// process, so an error here genuinely wants a retry.
			log.Warn("comms failed to handle a pushed event; asking Pub/Sub to redeliver",
				slog.String("message_id", env.Message.MessageID),
				slog.Any("error", err))
			http.Error(w, "processing failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// verifyCaller confirms the Authorization header carries an OIDC token whose
// email claim is expectedSA. It does NOT validate the signature: on Cloud Run
// the frontend already did, so this only reads the identity to pin the caller to
// the push subscription's service account. Any deviation — no bearer token, a
// token that is not three dot-separated segments, an undecodable or non-JSON
// payload, or a mismatched email — is errUnverifiedCaller.
func verifyCaller(authHeader, expectedSA string) error {
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return fmt.Errorf("%w: no bearer token", errUnverifiedCaller)
	}
	token := strings.TrimPrefix(authHeader, bearerPrefix)

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("%w: token is not a JWT", errUnverifiedCaller)
	}
	// JWT segments are base64url without padding.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("%w: undecodable token payload: %w", errUnverifiedCaller, err)
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return fmt.Errorf("%w: unreadable token claims: %w", errUnverifiedCaller, err)
	}
	if claims.Email != expectedSA {
		return fmt.Errorf("%w: caller %q is not the push service account", errUnverifiedCaller, claims.Email)
	}
	return nil
}
