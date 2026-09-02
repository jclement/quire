// HTTP surface for passkey auth: registration/login ceremonies, recovery,
// logout, and passkey management. Lives here rather than internal/api
// because every byte of it is auth-sensitive and should be reviewed as one
// unit. Handlers gate themselves: register requires bootstrap (zero
// passkeys) or a session; login/recover are open; management needs a session.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SessionCookie is the session cookie name.
const SessionCookie = "quire_session"

// NewEnrollCode returns the one-time code that authorizes claiming an
// un-bootstrapped instance. Without it, the first anonymous visitor to a
// publicly-reachable quire could register the owner passkey and take the
// vault — /api/v1/auth/status even advertises that the window is open. The
// code is minted at startup, printed to the server log, and never stored,
// so a restart invalidates it and a claimed instance stops accepting one.
func NewEnrollCode() (string, error) {
	raw := make([]byte, 10) // 80 bits
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating enrollment code: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// HTTPConfig configures the auth endpoints.
type HTTPConfig struct {
	Passkeys *Passkeys
	// SecureCookies marks cookies Secure — true whenever the deployment is
	// reached over HTTPS (base URL scheme).
	SecureCookies bool

	// EnrollCode gates bootstrap registration (see NewEnrollCode). Empty
	// disables the gate, which is only correct on a loopback-only listener.
	EnrollCode string

	limiter *rateLimiter
}

// Routes registers the /api/v1/auth endpoints. The credential-guessing
// surfaces (finish/recover) are rate limited; begin endpoints are not (they
// only mint challenges).
func (h *HTTPConfig) Routes(mux *http.ServeMux) {
	if h.limiter == nil {
		h.limiter = newRateLimiter()
	}
	mux.HandleFunc("GET /api/v1/auth/status", h.handleStatus)
	mux.HandleFunc("POST /api/v1/auth/register/begin", h.handleRegisterBegin)
	mux.HandleFunc("POST /api/v1/auth/register/finish", h.limited(h.handleRegisterFinish))
	mux.HandleFunc("POST /api/v1/auth/login/begin", h.handleLoginBegin)
	mux.HandleFunc("POST /api/v1/auth/login/finish", h.limited(h.handleLoginFinish))
	mux.HandleFunc("POST /api/v1/auth/recover", h.limited(h.handleRecover))
	mux.HandleFunc("POST /api/v1/auth/logout", h.handleLogout)
	mux.HandleFunc("GET /api/v1/auth/passkeys", h.handleListPasskeys)
	mux.HandleFunc("DELETE /api/v1/auth/passkeys/{id}", h.handleDeletePasskey)
}

func (h *HTTPConfig) store() *Store { return h.Passkeys.Store }

// allowRegister reports whether this request may register a passkey, and
// writes the refusal if not. Two ways in: an existing session (adding a
// second passkey), or — only while no passkey exists at all — the startup
// enrollment code.
func (h *HTTPConfig) allowRegister(w http.ResponseWriter, r *http.Request) bool {
	count, err := h.store().PasskeyCount()
	if err != nil {
		authError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return false
	}
	if count > 0 {
		if !h.hasSession(r) {
			authError(w, http.StatusUnauthorized, "UNAUTHORIZED", "log in before adding another passkey")
			return false
		}
		return true
	}
	if h.EnrollCode == "" {
		return true // loopback-only deployment; nothing remote can reach this
	}
	supplied := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("enroll_code")))
	if subtle.ConstantTimeCompare([]byte(supplied), []byte(h.EnrollCode)) != 1 {
		authError(w, http.StatusUnauthorized, "UNAUTHORIZED",
			"enrollment code required — it is printed in the server log at startup")
		return false
	}
	return true
}

// bootstrapping reports whether no passkey exists yet (errors count as
// "not bootstrapping", so a database problem cannot mint a session).
func (h *HTTPConfig) bootstrapping() bool {
	count, err := h.store().PasskeyCount()
	return err == nil && count == 0
}

// hasSession reports whether the request carries a valid session.
func (h *HTTPConfig) hasSession(r *http.Request) bool {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		return false
	}
	_, err = h.store().SessionPrincipal(cookie.Value)
	return err == nil
}

func (h *HTTPConfig) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.SecureCookies,
		MaxAge:   int(sessionTTL / time.Second),
	})
}

func (h *HTTPConfig) handleStatus(w http.ResponseWriter, r *http.Request) {
	count, err := h.store().PasskeyCount()
	if err != nil {
		authError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	authData(w, http.StatusOK, map[string]any{
		"registered":    count > 0,
		"authenticated": h.hasSession(r),
	})
}

func (h *HTTPConfig) handleRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if !h.allowRegister(w, r) {
		return
	}
	options, err := h.Passkeys.BeginRegistration()
	if err != nil {
		authError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "could not start registration")
		return
	}
	authData(w, http.StatusOK, options)
}

func (h *HTTPConfig) handleRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if !h.allowRegister(w, r) {
		return
	}
	// Read before the write: FinishRegistration is what makes count non-zero.
	bootstrap := h.bootstrapping()
	codes, err := h.Passkeys.FinishRegistration(r, r.URL.Query().Get("name"))
	if err != nil {
		authError(w, http.StatusBadRequest, "VALIDATION_ERROR", "passkey registration failed — try again")
		return
	}
	// Bootstrap registration logs the new passkey straight in.
	if bootstrap {
		token, err := h.store().CreateSession()
		if err != nil {
			authError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
			return
		}
		h.setSessionCookie(w, token)
	}
	authData(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func (h *HTTPConfig) handleLoginBegin(w http.ResponseWriter, r *http.Request) {
	options, err := h.Passkeys.BeginLogin()
	if err != nil {
		authError(w, http.StatusBadRequest, "VALIDATION_ERROR", "no passkeys registered here")
		return
	}
	authData(w, http.StatusOK, options)
}

func (h *HTTPConfig) handleLoginFinish(w http.ResponseWriter, r *http.Request) {
	token, err := h.Passkeys.FinishLogin(r)
	if err != nil {
		authError(w, http.StatusUnauthorized, "UNAUTHORIZED", "passkey login failed")
		return
	}
	h.setSessionCookie(w, token)
	authData(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPConfig) handleRecover(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	token, err := h.store().RedeemRecoveryCode(body.Code)
	if err != nil {
		authError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or already-used recovery code")
		return
	}
	h.setSessionCookie(w, token)
	// The client should route the user to passkey registration immediately —
	// the code that got them in is now burned.
	authData(w, http.StatusOK, map[string]any{"ok": true, "register_passkey": true})
}

func (h *HTTPConfig) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookie); err == nil {
		h.store().DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	authData(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPConfig) handleListPasskeys(w http.ResponseWriter, r *http.Request) {
	if !h.hasSession(r) {
		authError(w, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
		return
	}
	keys, err := h.store().ListPasskeys()
	if err != nil {
		authError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if keys == nil {
		keys = []PasskeyInfo{}
	}
	authData(w, http.StatusOK, keys)
}

func (h *HTTPConfig) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	if !h.hasSession(r) {
		authError(w, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
		return
	}
	if err := h.store().DeletePasskey(r.PathValue("id")); err != nil {
		authError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- envelope helpers (auth cannot import internal/api) ----

func authData(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func authError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":{"code":%q,"message":%q}}`, code, message)
}
