package googleid

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// Compile-time proof the checker satisfies its port.
var _ ports.ProviderPasswordChecker = (*PasswordChecker)(nil)

// credentialRefusals are the provider's answers that mean "not this password
// for this account" (or "not now"). Everything else — a bad key, a disabled
// provider, an outage — is our fault, not the person's, and must not read as a
// wrong password.
var credentialRefusals = []string{
	"INVALID_PASSWORD", "EMAIL_NOT_FOUND", "INVALID_LOGIN_CREDENTIALS",
	"USER_DISABLED", "TOO_MANY_ATTEMPTS_TRY_LATER", "MISSING_PASSWORD", "INVALID_EMAIL",
}

// PasswordChecker re-checks a password where the provider owns credentials,
// through the same public REST endpoint the browser signs in with
// (accounts:signInWithPassword, authenticated by the WEB api key — public by
// design; see Inviter). It is the only way to ask Identity Platform "is this
// the password?" server-side: the Admin SDK can set a password but never check
// one.
//
// returnSecureToken is false: this is a question, not a sign-in, so the
// provider mints no session and nothing reusable ever reaches this process.
// The answer's email must be the address we asked about, so a key pointed at
// the wrong project cannot vouch for somebody else.
type PasswordChecker struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewPasswordChecker builds one. baseURL and client are parameters so the
// request shape can be tested against a stub instead of Google.
func NewPasswordChecker(apiKey, baseURL string, client *http.Client) *PasswordChecker {
	if baseURL == "" {
		baseURL = identityToolkitBase
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &PasswordChecker{apiKey: apiKey, baseURL: baseURL, client: client}
}

// CheckPassword answers nil only when the provider accepts password for email.
func (c *PasswordChecker) CheckPassword(ctx context.Context, email, password string) error {
	if c.apiKey == "" {
		return glerrors.New(glerrors.KindInternal,
			"no hay llave del proveedor configurada; no se puede verificar la contraseña")
	}
	body, err := json.Marshal(map[string]any{"email": email, "password": password, "returnSecureToken": false})
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "encoding the password check", err)
	}
	endpoint := c.baseURL + "/v1/accounts:signInWithPassword?key=" + url.QueryEscape(c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "building the password check", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "asking the provider", err)
	}
	defer func() { _ = res.Body.Close() }()

	var answer struct {
		Email string `json:"email"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(res.Body).Decode(&answer)

	if res.StatusCode == http.StatusOK {
		if !strings.EqualFold(strings.TrimSpace(answer.Email), strings.TrimSpace(email)) {
			return errUnauthorized()
		}
		return nil
	}
	if res.StatusCode == http.StatusBadRequest && isCredentialRefusal(answer.Error.Message) {
		return errUnauthorized()
	}
	return glerrors.New(glerrors.KindInternal, "el proveedor no pudo verificar la contraseña")
}

func isCredentialRefusal(msg string) bool {
	for _, code := range credentialRefusals {
		if strings.HasPrefix(msg, code) {
			return true
		}
	}
	return false
}
