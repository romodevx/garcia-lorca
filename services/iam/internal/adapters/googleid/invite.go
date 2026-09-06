package googleid

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// identityToolkitBase is the public endpoint that SENDS provider emails.
const identityToolkitBase = "https://identitytoolkit.googleapis.com"

// Inviter asks the provider to email an account's set-your-password link.
//
// It exists because the Admin SDK generates action links but does not deliver
// them, and this project has no mail sender of its own — comms does push, not
// email. The identity toolkit's sendOobCode endpoint does deliver, so Google
// composes and sends the message and we own no mail infrastructure at all.
//
// It authenticates with the WEB api key rather than the service account: this
// is the same public endpoint a browser would call, and the key is public by
// design (it ships in the client bundle). Nothing here is a credential — the
// endpoint only ever mails a link to the address that already owns the account.
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

// SendInvitation asks the provider to mail the set-your-password link.
//
// PASSWORD_RESET is deliberate: for an account created WITHOUT a password it is
// a "choose your first one" link, and for an existing account it is a genuine
// reset. One request type, so "resend the invitation" and "I forgot my
// password" are the same operation — which is the admin reset iam never had.
func (i *Inviter) SendInvitation(ctx context.Context, email string) error {
	if i.apiKey == "" {
		// Fail closed and say why. Without a key this endpoint answers 400 to
		// everything, and "the invitation was not sent" with no reason is a
		// support ticket nobody can close.
		return glerrors.New(glerrors.KindInternal,
			"no identity provider api key configured; cannot send invitations")
	}

	body, err := json.Marshal(map[string]string{"requestType": "PASSWORD_RESET", "email": email})
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "encoding the invitation request", err)
	}
	endpoint := i.baseURL + "/v1/accounts:sendOobCode?key=" + url.QueryEscape(i.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "building the invitation request", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := i.client.Do(req)
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "sending the invitation", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		// The director has to know the mail did not go out; a silent success
		// would leave them telling somebody to check an inbox nothing reached.
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		return glerrors.New(glerrors.KindInternal,
			"the provider refused to send the invitation: "+e.Error.Message)
	}
	return nil
}
