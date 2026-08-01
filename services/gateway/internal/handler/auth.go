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
	Refresh(ctx context.Context, refreshToken string) (client.LoginResult, error)
	ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error
}

// AuthHandler exposes the front-facing (HTTP/JSON) auth endpoints, proxying to
// iam. It translates JSON ↔ the AuthClient and Connect error codes ↔ HTTP status.
type AuthHandler struct {
	auth LoginService
}

// NewAuthHandler wires the handler to a login service.
func NewAuthHandler(auth LoginService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken        string `json:"accessToken"`
	RefreshToken       string `json:"refreshToken"`
	ExpiresIn          int64  `json:"expiresIn"`
	MustChangePassword bool   `json:"mustChangePassword"`
}

// Login handles POST /api/v1/auth/login.
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
	http.SetCookie(w, &http.Cookie{
		Name:     "gl_access",
		Value:    res.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(res.ExpiresIn),
	})
	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken:        res.AccessToken,
		RefreshToken:       res.RefreshToken,
		ExpiresIn:          res.ExpiresIn,
		MustChangePassword: res.MustChangePassword,
	})
}

// Logout expires the gl_access cookie. It requires no valid token so a stale
// session can always be cleared; it only overwrites the cookie, no iam call.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "gl_access",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// Refresh handles POST /api/v1/auth/refresh: exchanges a refresh token for a
// fresh, rotated pair via iam.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		http.Error(w, http.StatusText(httpStatusFor(err)), httpStatusFor(err))
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{
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
}

// Me handles GET /api/v1/me: returns the authenticated principal. It must run
// behind middleware.RequireAuth, which injects the principal.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, meResponse{
		UserID:      p.UserID,
		TenantID:    p.TenantID,
		Email:       p.Email,
		Permissions: p.Permissions,
	})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// ChangePassword handles POST /api/v1/auth/change-password for the authenticated
// principal. RequireAuth injects the principal; the user id is never taken from
// the request body.
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
	if err := h.auth.ChangePassword(r.Context(), p.UserID, req.CurrentPassword, req.NewPassword); err != nil {
		http.Error(w, http.StatusText(httpStatusFor(err)), httpStatusFor(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
