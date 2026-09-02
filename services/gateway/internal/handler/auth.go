package handler

import (
	"context"
	"encoding/json"
	"net/http"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// LoginService is the auth surface the proxy needs (consumer-defined, so the
// handler is testable with a fake). The Connect AuthClient implements it.
type LoginService interface {
	Login(ctx context.Context, email, password string) (client.LoginResult, error)
	LoginAs(ctx context.Context, email, password string, kind client.SessionKind) (client.LoginResult, error)
	Refresh(ctx context.Context, refreshToken string) (client.LoginResult, error)
	ChangePassword(ctx context.Context, accessToken, currentPassword, newPassword string) error
}

// MeEnricher is the iam directory surface the Me handler uses to decorate the
// response with the caller's real display name + role name. The Connect
// AuthClient implements it (it already backs the admin /users and /roles routes),
// so the same client is reused; it is consumer-defined here so the handler stays
// testable with a fake.
type MeEnricher interface {
	GetUser(ctx context.Context, id string) (client.User, error)
	ListRoles(ctx context.Context) ([]client.Role, error)
}

// AuthHandler exposes the front-facing (HTTP/JSON) auth endpoints, proxying to
// iam. It translates JSON ↔ the AuthClient and Connect error codes ↔ HTTP status.
type AuthHandler struct {
	auth LoginService
	dir  MeEnricher // optional; enriches GET /me. Nil ⇒ /me returns only the principal.
}

// AuthOption configures an AuthHandler at construction (functional options).
type AuthOption func(*AuthHandler)

// WithDirectory wires the iam directory surface used to enrich GET /me with the
// caller's real display name + role name. When unset, /me returns only the token
// principal (userId/email/permissions).
func WithDirectory(dir MeEnricher) AuthOption {
	return func(h *AuthHandler) { h.dir = dir }
}

// NewAuthHandler wires the handler to a login service, plus any options (e.g.
// WithDirectory to enable /me enrichment).
func NewAuthHandler(auth LoginService, opts ...AuthOption) *AuthHandler {
	h := &AuthHandler{auth: auth}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Auth cookie conventions. Both tokens live ONLY in HttpOnly cookies — they are
// never returned in a JSON body — so browser JS cannot read them (XSS-resistant).
const (
	accessCookie  = "gl_access"
	refreshCookie = "gl_refresh"
	refreshPath   = "/api/v1/auth" // refresh cookie only sent to auth endpoints
	// The panel's session closes after fifteen idle minutes — the school's own
	// rule, for a computer that sits on a shared desk. The cookie is re-set on
	// every refresh, and the panel refreshes while somebody is using it, so this
	// is a window of INACTIVITY and not a cap on how long a shift can last.
	//
	// It matches the refresh token's own lifetime for a staff session. The
	// browser dropping the cookie and iam refusing the token are two locks on
	// the same door: one keeps the next person at that desk out, the other keeps
	// out somebody who copied the cookie.
	refreshMaxAge = 15 * 60 // seconds; staff refresh cookie lifetime
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	ExpiresIn          int64 `json:"expiresIn"`
	MustChangePassword bool  `json:"mustChangePassword"`
}

// refreshResponse carries only the access-token lifetime; the rotated tokens
// themselves are delivered via Set-Cookie, never the body.
type refreshResponse struct {
	ExpiresIn int64 `json:"expiresIn"`
}

// setAuthCookies writes both HttpOnly session cookies from a login/refresh
// result: gl_access (Path /, lifetime = res.ExpiresIn) and gl_refresh
// (Path refreshPath so it is only sent to the auth endpoints). Both are
// Secure + SameSite=Strict.
func setAuthCookies(w http.ResponseWriter, res client.LoginResult) {
	http.SetCookie(w, &http.Cookie{
		Name:     accessCookie,
		Value:    res.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(res.ExpiresIn),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    res.RefreshToken,
		Path:     refreshPath,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   refreshMaxAge,
	})
}

// Login handles POST /api/v1/auth/login. On success it sets both auth cookies
// and returns only the access lifetime + forced-rotation flag (no tokens).
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		http.Error(w, http.StatusText(httpStatusFor(err)), httpStatusFor(err))
		return
	}
	setAuthCookies(w, res)
	writeJSON(w, http.StatusOK, loginResponse{
		ExpiresIn:          res.ExpiresIn,
		MustChangePassword: res.MustChangePassword,
	})
}

// Logout expires BOTH auth cookies. It requires no valid token so a stale
// session can always be cleared; it only overwrites the cookies, no iam call.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     accessCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    "",
		Path:     refreshPath,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

// Refresh handles POST /api/v1/auth/refresh: it reads the refresh token from the
// gl_refresh cookie, exchanges it for a fresh rotated pair via iam, and rotates
// BOTH cookies. A missing refresh cookie is 401. Tokens never touch the body.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil || c.Value == "" {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	res, err := h.auth.Refresh(r.Context(), c.Value)
	if err != nil {
		http.Error(w, http.StatusText(httpStatusFor(err)), httpStatusFor(err))
		return
	}
	setAuthCookies(w, res)
	writeJSON(w, http.StatusOK, refreshResponse{ExpiresIn: res.ExpiresIn})
}

// --- mobile credential posture ---------------------------------------------
//
// The gateway serves TWO credential postures from the same iam auth surface:
//
//   - WEB (Login/Refresh above): both tokens live ONLY in HttpOnly, Secure,
//     SameSite=Strict cookies and never appear in a response body, so browser JS
//     cannot read them (XSS-resistant). The browser replays them automatically.
//   - MOBILE (MobileLogin/MobileRefresh below): both tokens are returned in the
//     JSON body and NO cookie is set. A React Native client has no cookie jar; it
//     stores the pair in the OS keychain (expo-secure-store) and sends the access
//     token as "Authorization: Bearer …" — which middleware.TokenFrom already
//     accepts, so the authenticated routes need no change.
//
// The postures must not bleed into each other: a token in the web body would
// defeat HttpOnly, and a cookie on the mobile response would be dead weight the
// client cannot manage. Each is pinned by a test.

// mobileLoginResponse carries the token pair IN THE BODY — the mobile posture.
type mobileLoginResponse struct {
	AccessToken        string `json:"accessToken"`
	RefreshToken       string `json:"refreshToken"`
	ExpiresIn          int64  `json:"expiresIn"`
	MustChangePassword bool   `json:"mustChangePassword"`
}

// mobileRefreshResponse carries the rotated pair. It omits mustChangePassword:
// iam's RefreshToken does not report it, and an always-false field would lie.
type mobileRefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
}

// mobileRefreshRequest carries the refresh token in the BODY: a mobile client has
// no cookie jar, so the body is the only credential source. The token itself is
// what authorizes the call, which is why the route needs no access token.
type mobileRefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// MobileLogin handles POST /api/v1/auth/mobile/login. It calls the same iam Login
// as the web handler but returns both tokens in the body and sets NO cookies (see
// the posture note above). It is unauthenticated, so it MUST stay behind the same
// per-IP login rate limiter as the web login.
func (h *AuthHandler) MobileLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// The family app: a parent's own phone, so the session lasts a day rather
	// than closing the moment they put the phone down.
	res, err := h.auth.LoginAs(r.Context(), req.Email, req.Password, client.SessionFamily)
	if err != nil {
		http.Error(w, http.StatusText(httpStatusFor(err)), httpStatusFor(err))
		return
	}
	writeJSON(w, http.StatusOK, mobileLoginResponse{
		AccessToken:        res.AccessToken,
		RefreshToken:       res.RefreshToken,
		ExpiresIn:          res.ExpiresIn,
		MustChangePassword: res.MustChangePassword,
	})
}

// MobileRefresh handles POST /api/v1/auth/mobile/refresh. Unlike the web Refresh
// it reads the refresh token from the BODY (never a cookie) and returns the
// rotated pair in the body, setting no cookies. A missing token is 401, exactly
// like a missing refresh cookie on the web route.
func (h *AuthHandler) MobileRefresh(w http.ResponseWriter, r *http.Request) {
	var req mobileRefreshRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.RefreshToken == "" {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	res, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		http.Error(w, http.StatusText(httpStatusFor(err)), httpStatusFor(err))
		return
	}
	writeJSON(w, http.StatusOK, mobileRefreshResponse{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		ExpiresIn:    res.ExpiresIn,
	})
}

type meResponse struct {
	UserID      string   `json:"userId"`
	TenantID    string   `json:"tenantId"`
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
	DisplayName string   `json:"displayName,omitempty"`
	Role        string   `json:"role,omitempty"`
}

// Me handles GET /api/v1/me: returns the authenticated principal, enriched (when
// a directory is wired) with the caller's real display name + role name. It must
// run behind middleware.RequireAuth, which injects the principal.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Never emit a null permissions array: a principal with no permissions must
	// serialize as [] so JSON clients can always iterate it (same contract as the
	// roles/users responses).
	perms := p.Permissions
	if perms == nil {
		perms = []string{}
	}
	resp := meResponse{
		UserID:      p.UserID,
		TenantID:    p.TenantID,
		Email:       p.Email,
		Permissions: perms,
	}
	// Best-effort enrichment: /me drives session restore, so a directory outage
	// must never fail it — the response degrades to the token principal.
	h.enrichMe(r.Context(), p.UserID, &resp)
	writeJSON(w, http.StatusOK, resp)
}

// enrichMe fills resp.DisplayName and resp.Role from the iam directory. Every
// failure path — no directory wired, GetUser error, ListRoles error, no roles —
// leaves the base response untouched, so enrichment can never break /me.
func (h *AuthHandler) enrichMe(ctx context.Context, userID string, resp *meResponse) {
	if h.dir == nil {
		return
	}
	u, err := h.dir.GetUser(ctx, userID)
	if err != nil {
		return
	}
	resp.DisplayName = u.DisplayName
	if len(u.RoleIDs) == 0 {
		return
	}
	roles, err := h.dir.ListRoles(ctx)
	if err != nil {
		return
	}
	nameByID := make(map[string]string, len(roles))
	for _, role := range roles {
		nameByID[role.ID] = role.Name
	}
	// The user's primary/first role names the caller in the chrome.
	resp.Role = nameByID[u.RoleIDs[0]]
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// minNewPasswordLen is the edge floor for a new password. It mirrors iam's
// server-side policy so an obviously too-short password is rejected at the
// gateway (defense-in-depth) without a needless round-trip; iam remains the
// authoritative validator.
const minNewPasswordLen = 8

// ChangePassword handles POST /api/v1/auth/change-password for the authenticated
// principal. RequireAuth injects the principal; iam derives the subject from the
// forwarded access token, so the gateway forwards the caller's token — never a
// user id from the request body or principal.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req changePasswordRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.NewPassword) < minNewPasswordLen {
		http.Error(w, "new password is too short", http.StatusBadRequest)
		return
	}
	if err := h.auth.ChangePassword(r.Context(), middleware.TokenFrom(r), req.CurrentPassword, req.NewPassword); err != nil {
		http.Error(w, http.StatusText(httpStatusFor(err)), httpStatusFor(err))
		return
	}
	// The change revoked every refresh token issued before it — including the
	// caller's current one. Re-issue a fresh session immediately (login with the
	// new password) so the user is not bounced to the login screen right after
	// changing their password. The principal's email is the verified subject, not
	// a request-supplied value. A rare re-login failure leaves the password
	// changed but no fresh cookies (204); the client falls back to login.
	res, err := h.auth.Login(r.Context(), p.Email, req.NewPassword)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	setAuthCookies(w, res)
	writeJSON(w, http.StatusOK, loginResponse{ExpiresIn: res.ExpiresIn, MustChangePassword: res.MustChangePassword})
}

// httpStatusFor maps a Connect error code (from iam) to an HTTP status.
func httpStatusFor(err error) int {
	switch connect.CodeOf(err) {
	case connect.CodeInvalidArgument:
		return http.StatusBadRequest
	case connect.CodeUnauthenticated:
		return http.StatusUnauthorized
	case connect.CodePermissionDenied:
		return http.StatusForbidden
	case connect.CodeNotFound:
		return http.StatusNotFound
	case connect.CodeAlreadyExists, connect.CodeFailedPrecondition:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// maxRequestBytes caps a decoded JSON request body (1 MiB) so an authenticated
// caller cannot exhaust gateway memory with an oversized payload.
const maxRequestBytes = 1 << 20

// decodeJSONBody bounds r.Body and decodes it into dst. A body over the cap or
// malformed JSON returns an error the caller maps to 400.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	return json.NewDecoder(r.Body).Decode(dst)
}
