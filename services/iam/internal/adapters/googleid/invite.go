package googleid

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// identityToolkitBase is the public endpoint that SENDS provider emails.
const identityToolkitBase = "https://identitytoolkit.googleapis.com"

// Inviter asks the provider to mail its own set-a-password link.
//
// TEMPORARY BY DESIGN, and it is worth being honest about what it is: a way to
// have mail going out today without waiting on an SMTP credential from the
// school. Everything about the message belongs to Google — the link, the page it
// lands on, the one-hour lifetime, and a shared sender address that Gmail files
// as spam. Our own twenty-four-hour token cannot travel this way; that link
// carries Google's `oobCode`, not ours.
//
// It authenticates with the WEB api key rather than the service account: this is
// the same public endpoint a browser would call, and the key is public by design
// (it ships in the client bundle). Nothing here is a credential — the endpoint
// only ever mails a link to the address that already owns the account.
//
// The reply is nearly meaningless. With email-enumeration protection on, this
// endpoint answers 200 for an address with no account at all, so a success says
// "accepted", never "delivered". Real errors — a bad key, an exhausted quota, a
// disabled provider — do come back non-200, and those are worth surfacing.
type Inviter struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewInviter builds one. baseURL and client are parameters so the request shape
// can be tested against a stub instead of Google.
func NewInviter(apiKey, baseURL string, client *http.Client) *Inviter {
	if baseURL == "" {
		baseURL = identityToolkitBase
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Inviter{apiKey: apiKey, baseURL: baseURL, client: client}
}

// SendPasswordSetupEmail asks the provider to mail the link.
//
// PASSWORD_RESET is deliberate: for an account created WITHOUT a password it is
// a "choose your first one" link, and for an existing account it is a genuine
// reset. One request type covers both.
func (i *Inviter) SendPasswordSetupEmail(ctx context.Context, email string) error {
	if i.apiKey == "" {
		// Fail closed and say why. Without a key this endpoint answers 400 to
		// everything, and "the invitation was not sent" with no reason is a
		// support ticket nobody can close.
		return glerrors.New(glerrors.KindInternal,
			"no hay llave del proveedor configurada; no se puede enviar por Firebase")
	}

	body, err := json.Marshal(map[string]string{"requestType": "PASSWORD_RESET", "email": email})
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "encoding the invitation request", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, i.baseURL+"/v1/accounts:sendOobCode", bytes.NewReader(body))
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "building the invitation request", withoutURL(err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(apiKeyHeader, i.apiKey)

	res, err := i.client.Do(req)
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "sending the invitation", withoutURL(err))
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		return glerrors.New(glerrors.KindInternal,
			"el proveedor rechazó el envío: "+e.Error.Message)
	}
	return nil
}
