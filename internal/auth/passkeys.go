// Passkey (WebAuthn) authentication for the single-user deployment, plus
// server-side sessions and single-use recovery codes. Design per DESIGN.md
// "Auth modes": passwordless only; recovery is explicit (argon2id-hashed
// codes shown once at bootstrap), never a quiet password fallback.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"golang.org/x/crypto/argon2"
)

// ownerID is the WebAuthn user handle. quire is single-user: there is
// exactly one account, and this constant is it. Random-looking but fixed so
// re-registration always targets the same account.
var ownerID = []byte("quire-owner-account-v1")

// webauthnUser adapts the singleton owner to the library's User interface.
type webauthnUser struct {
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte                         { return ownerID }
func (u *webauthnUser) WebAuthnName() string                       { return "owner" }
func (u *webauthnUser) WebAuthnDisplayName() string                { return "quire owner" }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// Passkeys bundles the WebAuthn handler state: library config plus in-flight
// ceremony data (in memory — ceremonies live for minutes on one instance).
type Passkeys struct {
	Store *Store
	wa    *webauthn.WebAuthn

	mu        sync.Mutex
	ceremony  *webauthn.SessionData // one user → at most one in-flight ceremony
	ceremonyT time.Time
}

// NewPasskeys builds the WebAuthn state for a deployment. rpID must be the
// hostname users visit (passkeys bind to it — a passkey registered at one
// hostname will not work at another).
func NewPasskeys(store *Store, displayName, rpID string, origins []string) (*Passkeys, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: displayName,
		RPID:          rpID,
		RPOrigins:     origins,
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn config: %w", err)
	}
	return &Passkeys{Store: store, wa: wa}, nil
}

// ---- stored credentials ----

func (s *Store) loadCredentials() ([]webauthn.Credential, error) {
	rows, err := s.DB.Query("SELECT credential_json FROM passkeys")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []webauthn.Credential
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			continue // one corrupt row must not lock the user out entirely
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// PasskeyCount reports how many passkeys are registered — zero means the
// instance is in bootstrap mode.
func (s *Store) PasskeyCount() (int, error) {
	var n int
	err := s.DB.QueryRow("SELECT COUNT(*) FROM passkeys").Scan(&n)
	return n, err
}

func (s *Store) savePasskey(name string, cred *webauthn.Credential) error {
	raw, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO passkeys (id, name, credential_json, created_at) VALUES (?, ?, ?, ?)`,
		hex.EncodeToString(cred.ID), name, string(raw), time.Now().UTC().Format(time.RFC3339))
	return err
}

// PasskeyInfo is the management view of a stored passkey.
type PasskeyInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// ListPasskeys returns registered passkeys for the settings UI.
func (s *Store) ListPasskeys() ([]PasskeyInfo, error) {
	rows, err := s.DB.Query("SELECT id, name, created_at FROM passkeys ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PasskeyInfo
	for rows.Next() {
		var p PasskeyInfo
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePasskey removes a passkey — refused for the last one so the user
// cannot lock themselves out (recovery codes notwithstanding).
func (s *Store) DeletePasskey(id string) error {
	n, err := s.PasskeyCount()
	if err != nil {
		return err
	}
	if n <= 1 {
		return fmt.Errorf("cannot delete the last passkey — register another first")
	}
	res, err := s.DB.Exec("DELETE FROM passkeys WHERE id = ?", id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return fmt.Errorf("no passkey %q", id)
	}
	return nil
}

// ---- ceremonies ----

const ceremonyTTL = 5 * time.Minute

func (p *Passkeys) storeCeremony(sd *webauthn.SessionData) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ceremony, p.ceremonyT = sd, time.Now()
}

func (p *Passkeys) takeCeremony() (*webauthn.SessionData, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sd := p.ceremony
	p.ceremony = nil
	if sd == nil || time.Since(p.ceremonyT) > ceremonyTTL {
		return nil, fmt.Errorf("no ceremony in progress (it may have expired — try again)")
	}
	return sd, nil
}

// BeginRegistration starts adding a passkey. The caller enforces authorization
// (bootstrap when zero passkeys exist; a valid session afterwards).
func (p *Passkeys) BeginRegistration() (any, error) {
	creds, err := p.Store.loadCredentials()
	if err != nil {
		return nil, err
	}
	options, sd, err := p.wa.BeginRegistration(&webauthnUser{credentials: creds})
	if err != nil {
		return nil, fmt.Errorf("beginning registration: %w", err)
	}
	p.storeCeremony(sd)
	return options, nil
}

// FinishRegistration validates the browser's response and stores the new
// passkey. Returns recovery codes exactly once: on the very first passkey.
func (p *Passkeys) FinishRegistration(r *http.Request, name string) (recoveryCodes []string, _ error) {
	sd, err := p.takeCeremony()
	if err != nil {
		return nil, err
	}
	creds, err := p.Store.loadCredentials()
	if err != nil {
		return nil, err
	}
	cred, err := p.wa.FinishRegistration(&webauthnUser{credentials: creds}, *sd, r)
	if err != nil {
		return nil, fmt.Errorf("finishing registration: %w", err)
	}
	first := len(creds) == 0
	if name == "" {
		name = "passkey " + time.Now().Format("2006-01-02")
	}
	if err := p.Store.savePasskey(name, cred); err != nil {
		return nil, err
	}
	if first {
		return p.Store.generateRecoveryCodes()
	}
	return nil, nil
}

// BeginLogin starts an assertion ceremony.
func (p *Passkeys) BeginLogin() (any, error) {
	creds, err := p.Store.loadCredentials()
	if err != nil {
		return nil, err
	}
	if len(creds) == 0 {
		return nil, fmt.Errorf("no passkeys registered")
	}
	options, sd, err := p.wa.BeginLogin(&webauthnUser{credentials: creds})
	if err != nil {
		return nil, fmt.Errorf("beginning login: %w", err)
	}
	p.storeCeremony(sd)
	return options, nil
}

// FinishLogin validates the assertion and returns a new session token.
func (p *Passkeys) FinishLogin(r *http.Request) (sessionToken string, _ error) {
	sd, err := p.takeCeremony()
	if err != nil {
		return "", err
	}
	creds, err := p.Store.loadCredentials()
	if err != nil {
		return "", err
	}
	if _, err := p.wa.FinishLogin(&webauthnUser{credentials: creds}, *sd, r); err != nil {
		return "", fmt.Errorf("finishing login: %w", err)
	}
	return p.Store.CreateSession()
}

// ---- recovery codes ----

// argon2id parameters: the defaults recommended for interactive use.
const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
)

func hashRecoveryCode(code string) string {
	// A fixed salt is acceptable here: codes are 50-bit random values, not
	// human-chosen passwords, so rainbow tables don't apply — argon2 just
	// slows any offline brute force of a stolen auth.db.
	sum := argon2.IDKey([]byte(code), []byte("quire-recovery-v1"), argonTime, argonMemory, argonThreads, argonKeyLen)
	return hex.EncodeToString(sum)
}

// generateRecoveryCodes mints 8 single-use codes, replacing any previous set.
func (s *Store) generateRecoveryCodes() ([]string, error) {
	if _, err := s.DB.Exec("DELETE FROM recovery_codes"); err != nil {
		return nil, err
	}
	codes := make([]string, 8)
	for i := range codes {
		raw := make([]byte, 5)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		code := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))
		codes[i] = code[:4] + "-" + code[4:]
		if _, err := s.DB.Exec("INSERT INTO recovery_codes (hash) VALUES (?)", hashRecoveryCode(codes[i])); err != nil {
			return nil, err
		}
	}
	return codes, nil
}

// RedeemRecoveryCode burns a code and returns a session token. The UI should
// push the user straight into registering a replacement passkey.
func (s *Store) RedeemRecoveryCode(code string) (string, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	res, err := s.DB.Exec("UPDATE recovery_codes SET used_at = ? WHERE hash = ? AND used_at = ''",
		time.Now().UTC().Format(time.RFC3339), hashRecoveryCode(code))
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", fmt.Errorf("invalid or already-used recovery code")
	}
	return s.CreateSession()
}

// ---- sessions ----

// sessionTTL is a 30-day sliding window, refreshed on activity.
const sessionTTL = 30 * 24 * time.Hour

// CreateSession mints a server-side session, returning the bearer value for
// the cookie. Only the SHA-256 lands in auth.db.
func (s *Store) CreateSession() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	_, err := s.DB.Exec(`INSERT INTO sessions (token_hash, created_at, expires_at, last_seen_at) VALUES (?, ?, ?, ?)`,
		hex.EncodeToString(sum[:]), now.Format(time.RFC3339), now.Add(sessionTTL).Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return token, nil
}

// SessionPrincipal resolves a session cookie value to the owner principal,
// sliding the expiry forward.
func (s *Store) SessionPrincipal(token string) (Principal, error) {
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	var expires string
	if err := s.DB.QueryRow("SELECT expires_at FROM sessions WHERE token_hash = ?", hash).Scan(&expires); err != nil {
		return Principal{}, fmt.Errorf("unknown session")
	}
	t, err := time.Parse(time.RFC3339, expires)
	if err != nil || time.Now().After(t) {
		_, _ = s.DB.Exec("DELETE FROM sessions WHERE token_hash = ?", hash)
		return Principal{}, fmt.Errorf("session expired")
	}
	now := time.Now().UTC()
	_, _ = s.DB.Exec("UPDATE sessions SET expires_at = ?, last_seen_at = ? WHERE token_hash = ?",
		now.Add(sessionTTL).Format(time.RFC3339), now.Format(time.RFC3339), hash)
	return OwnerPrincipal(), nil
}

// DeleteSession revokes one session (logout).
func (s *Store) DeleteSession(token string) {
	sum := sha256.Sum256([]byte(token))
	_, _ = s.DB.Exec("DELETE FROM sessions WHERE token_hash = ?", hex.EncodeToString(sum[:]))
}
