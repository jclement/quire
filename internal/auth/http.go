// HTTP surface for passkey auth: registration/login ceremonies, recovery,
// logout, and passkey management. Lives here rather than internal/api
// because every byte of it is auth-sensitive and should be reviewed as one
// unit. Handlers gate themselves: register requires bootstrap (zero
// passkeys) or a session; login/recover are open; management needs a session.
package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SessionCookie is the session cookie name.
const SessionCookie = "quire_session"

// HTTPConfig configures the auth endpoints.
type HTTPConfig struct {
	Passkeys *Passkeys
	// SecureCookies marks cookies Secure — true whenever the deployment is
	// reached over HTTPS (base URL scheme).
	SecureCookies bool

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
	count, err := h.store().PasskeyCount()
	if err != nil {
		authError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	// Bootstrap: the very first passkey needs no session (there is no way to
	// have one). After that, adding passkeys requires being logged in.
	if count > 0 && !h.hasSession(r) {
		authError(w, http.StatusUnauthorized, "UNAUTHORIZED", "log in before adding another passkey")
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
	count, err := h.store().PasskeyCount()
	if err != nil {
		authError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if count > 0 && !h.hasSession(r) {
		authError(w, http.StatusUnauthorized, "UNAUTHORIZED", "log in before adding another passkey")
		return
	}
	codes, err := h.Passkeys.FinishRegistration(r, r.URL.Query().Get("name"))
	if err != nil {
		authError(w, http.StatusBadRequest, "VALIDATION_ERROR", "passkey registration failed — try again")
		return
	}
	// Bootstrap registration logs the new passkey straight in.
	if count == 0 {
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
