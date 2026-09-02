// OAuth 2.1 storage: dynamically-registered clients, single-use PKCE
// authorization codes, and rotating access/refresh token pairs. The HTTP
// endpoints live in internal/oauth; verification lives here because
// BearerPrincipal must accept OAuth access tokens wherever sk_ tokens work.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Token/credential prefixes. Distinct from sk_ so verification can dispatch
// without a table scan.
const (
	oauthAccessPrefix  = "oaq_"
	oauthRefreshPrefix = "orq_"
)

// Lifetimes per the house OAuth pattern: short codes, ~1h access tokens,
// ~30d rotating refresh tokens with a short reuse-grace window.
const (
	oauthCodeTTL     = 60 * time.Second
	oauthAccessTTL   = time.Hour
	oauthRefreshTTL  = 30 * 24 * time.Hour
	oauthRotateGrace = 60 * time.Second
	// maxUnconsentedClients caps DCR so anonymous registration can't fill
	// the table; consented clients don't count.
	maxUnconsentedClients = 20
)

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomHex(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// ---- clients ----

// OAuthClient is a dynamically-registered public client.
type OAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
	ConsentedAt  string
}

// RegisterOAuthClient stores a new public client (RFC 7591).
func (s *Store) RegisterOAuthClient(name string, redirectURIs []string) (OAuthClient, error) {
	if len(redirectURIs) == 0 {
		return OAuthClient{}, fmt.Errorf("redirect_uris is required")
	}
	var unconsented int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM oauth_clients WHERE consented_at = ''").Scan(&unconsented); err != nil {
		return OAuthClient{}, err
	}
	if unconsented >= maxUnconsentedClients {
		return OAuthClient{}, fmt.Errorf("too many pending client registrations")
	}

	id, err := randomHex(16)
	if err != nil {
		return OAuthClient{}, err
	}
	uris, err := json.Marshal(redirectURIs)
	if err != nil {
		return OAuthClient{}, err
	}
	_, err = s.DB.Exec(`INSERT INTO oauth_clients (id, name, redirect_uris, created_at) VALUES (?, ?, ?, ?)`,
		id, name, string(uris), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return OAuthClient{}, fmt.Errorf("storing client: %w", err)
	}
	return OAuthClient{ID: id, Name: name, RedirectURIs: redirectURIs}, nil
}

// GetOAuthClient looks a client up by id.
func (s *Store) GetOAuthClient(id string) (OAuthClient, error) {
	var c OAuthClient
	var uris string
	err := s.DB.QueryRow("SELECT id, name, redirect_uris, consented_at FROM oauth_clients WHERE id = ?", id).
		Scan(&c.ID, &c.Name, &uris, &c.ConsentedAt)
	if err != nil {
		return OAuthClient{}, fmt.Errorf("unknown client")
	}
	if err := json.Unmarshal([]byte(uris), &c.RedirectURIs); err != nil {
		return OAuthClient{}, fmt.Errorf("corrupt client record")
	}
	return c, nil
}

// AllowsRedirect reports whether uri exactly matches a registered redirect.
func (c OAuthClient) AllowsRedirect(uri string) bool {
	for _, registered := range c.RedirectURIs {
		if registered == uri {
			return true
		}
	}
	return false
}

// ---- authorization codes ----

// MintOAuthCode issues a single-use code bound to client, redirect URI, and
// PKCE challenge, and marks the client consented.
func (s *Store) MintOAuthCode(clientID, redirectURI, challenge, scopes string) (string, error) {
	code, err := randomHex(24)
	if err != nil {
		return "", err
	}
	_, err = s.DB.Exec(`INSERT INTO oauth_codes (code_hash, client_id, redirect_uri, challenge, scopes, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		hashToken(code), clientID, redirectURI, challenge, scopes,
		time.Now().Add(oauthCodeTTL).UTC().Format(time.RFC3339))
	if err != nil {
		return "", fmt.Errorf("storing code: %w", err)
	}
	_, _ = s.DB.Exec("UPDATE oauth_clients SET consented_at = ? WHERE id = ? AND consented_at = ''",
		time.Now().UTC().Format(time.RFC3339), clientID)
	return code, nil
}

// RedeemOAuthCode consumes a code after verifying the PKCE verifier and
// binding parameters, returning the granted scopes.
func (s *Store) RedeemOAuthCode(code, clientID, redirectURI, verifier string) (scopes string, _ error) {
	var challenge, storedClient, storedRedirect, expires, used string
	err := s.DB.QueryRow(`SELECT challenge, client_id, redirect_uri, scopes, expires_at, used_at
		FROM oauth_codes WHERE code_hash = ?`, hashToken(code)).
		Scan(&challenge, &storedClient, &storedRedirect, &scopes, &expires, &used)
	if err != nil {
		return "", fmt.Errorf("invalid code")
	}
	// Single-use: burn before verifying so a race cannot redeem twice.
	if _, err := s.DB.Exec("UPDATE oauth_codes SET used_at = ? WHERE code_hash = ? AND used_at = ''",
		time.Now().UTC().Format(time.RFC3339), hashToken(code)); err != nil {
		return "", err
	}
	if used != "" {
		return "", fmt.Errorf("code already used")
	}
	if t, err := time.Parse(time.RFC3339, expires); err != nil || time.Now().After(t) {
		return "", fmt.Errorf("code expired")
	}
	if storedClient != clientID || storedRedirect != redirectURI {
		return "", fmt.Errorf("code binding mismatch")
	}
	sum := sha256.Sum256([]byte(verifier))
	if base64URLNoPad(sum[:]) != challenge {
		return "", fmt.Errorf("PKCE verification failed")
	}
	return scopes, nil
}

// ---- access / refresh tokens ----

// OAuthTokenPair is a freshly-minted credential pair (plaintext, shown once).
type OAuthTokenPair struct {
	AccessToken      string
	RefreshToken     string
	Scopes           string
	AccessExpiresIn  int64 // seconds
	RefreshExpiresAt time.Time
}

// MintOAuthTokens issues a new access/refresh pair for a client.
func (s *Store) MintOAuthTokens(clientID, scopes string) (OAuthTokenPair, error) {
	access, err := randomHex(32)
	if err != nil {
		return OAuthTokenPair{}, err
	}
	refresh, err := randomHex(32)
	if err != nil {
		return OAuthTokenPair{}, err
	}
	pair := OAuthTokenPair{
		AccessToken:      oauthAccessPrefix + access,
		RefreshToken:     oauthRefreshPrefix + refresh,
		Scopes:           scopes,
		AccessExpiresIn:  int64(oauthAccessTTL / time.Second),
		RefreshExpiresAt: time.Now().Add(oauthRefreshTTL).UTC(),
	}
	_, err = s.DB.Exec(`INSERT INTO oauth_tokens
		(client_id, access_hash, refresh_hash, scopes, access_expires_at, refresh_expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		clientID, hashToken(pair.AccessToken), hashToken(pair.RefreshToken), scopes,
		time.Now().Add(oauthAccessTTL).UTC().Format(time.RFC3339),
		pair.RefreshExpiresAt.Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return OAuthTokenPair{}, fmt.Errorf("storing tokens: %w", err)
	}
	return pair, nil
}

// RotateOAuthTokens exchanges a refresh token for a new pair. The previous
// refresh hash stays valid for a short grace window (network retries), then
// dies; reuse outside the window revokes the grant (theft signal).
func (s *Store) RotateOAuthTokens(refreshToken string) (OAuthTokenPair, error) {
	h := hashToken(refreshToken)
	var id int64
	var clientID, scopes, refreshExpires, revoked, rotatedAt string
	var isPrev bool
	err := s.DB.QueryRow(`SELECT id, client_id, scopes, refresh_expires_at, revoked_at, rotated_at, refresh_hash != ?
		FROM oauth_tokens WHERE refresh_hash = ? OR prev_refresh_hash = ?`, h, h, h).
		Scan(&id, &clientID, &scopes, &refreshExpires, &revoked, &rotatedAt, &isPrev)
	if err != nil {
		return OAuthTokenPair{}, fmt.Errorf("invalid refresh token")
	}
	if revoked != "" {
		return OAuthTokenPair{}, fmt.Errorf("grant revoked")
	}
	if t, err := time.Parse(time.RFC3339, refreshExpires); err != nil || time.Now().After(t) {
		return OAuthTokenPair{}, fmt.Errorf("refresh token expired")
	}
	if isPrev {
		// The already-rotated token: honored only inside the grace window.
		t, err := time.Parse(time.RFC3339, rotatedAt)
		if err != nil || time.Since(t) > oauthRotateGrace {
			_, _ = s.DB.Exec("UPDATE oauth_tokens SET revoked_at = ? WHERE id = ?",
				time.Now().UTC().Format(time.RFC3339), id)
			return OAuthTokenPair{}, fmt.Errorf("refresh token reuse detected; grant revoked")
		}
	}

	access, err := randomHex(32)
	if err != nil {
		return OAuthTokenPair{}, err
	}
	refresh, err := randomHex(32)
	if err != nil {
		return OAuthTokenPair{}, err
	}
	pair := OAuthTokenPair{
		AccessToken:      oauthAccessPrefix + access,
		RefreshToken:     oauthRefreshPrefix + refresh,
		Scopes:           scopes,
		AccessExpiresIn:  int64(oauthAccessTTL / time.Second),
		RefreshExpiresAt: time.Now().Add(oauthRefreshTTL).UTC(),
	}
	_, err = s.DB.Exec(`UPDATE oauth_tokens SET
			access_hash = ?, refresh_hash = ?, prev_refresh_hash = ?, rotated_at = ?,
			access_expires_at = ?, refresh_expires_at = ?
		WHERE id = ?`,
		hashToken(pair.AccessToken), hashToken(pair.RefreshToken), h,
		time.Now().UTC().Format(time.RFC3339),
		time.Now().Add(oauthAccessTTL).UTC().Format(time.RFC3339),
		pair.RefreshExpiresAt.Format(time.RFC3339), id)
	if err != nil {
		return OAuthTokenPair{}, fmt.Errorf("rotating tokens: %w", err)
	}
	return pair, nil
}

// RevokeOAuthToken revokes the grant holding the given access or refresh
// token (RFC 7009 semantics: unknown tokens succeed silently).
func (s *Store) RevokeOAuthToken(token string) {
	h := hashToken(token)
	_, _ = s.DB.Exec(`UPDATE oauth_tokens SET revoked_at = ?
		WHERE revoked_at = '' AND (access_hash = ? OR refresh_hash = ? OR prev_refresh_hash = ?)`,
		time.Now().UTC().Format(time.RFC3339), h, h, h)
}

// oauthAccessPrincipal resolves an oaq_ access token.
func (s *Store) oauthAccessPrincipal(token string) (Principal, error) {
	var clientID, scopes, expires, revoked string
	var id int64
	err := s.DB.QueryRow(`SELECT id, client_id, scopes, access_expires_at, revoked_at
		FROM oauth_tokens WHERE access_hash = ?`, hashToken(token)).
		Scan(&id, &clientID, &scopes, &expires, &revoked)
	if err != nil {
		return Principal{}, fmt.Errorf("unknown token")
	}
	if revoked != "" {
		return Principal{}, fmt.Errorf("token revoked")
	}
	if t, err := time.Parse(time.RFC3339, expires); err != nil || time.Now().After(t) {
		return Principal{}, fmt.Errorf("token expired")
	}
	_, _ = s.DB.Exec("UPDATE oauth_tokens SET last_used_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339), id)

	scopeSet := map[string]bool{}
	for _, sc := range strings.Split(scopes, " ") {
		if sc != "" {
			scopeSet[sc] = true
		}
	}
	return Principal{Name: "oauth:" + clientID, Scopes: scopeSet}, nil
}

// base64URLNoPad matches the PKCE S256 encoding (RFC 7636).
func base64URLNoPad(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// ---- connected apps (management) ----

// ConnectedApp is a consented OAuth client as the management UI sees it:
// who is attached to this vault, with what access, and when it was last
// used. Unconsented registrations are not apps — anyone can create those
// via DCR — so they are excluded.
type ConnectedApp struct {
	ClientID    string   `json:"client_id"`
	Name        string   `json:"name"`
	Scopes      []string `json:"scopes"`
	ConsentedAt string   `json:"consented_at"`
	LastUsedAt  string   `json:"last_used_at"`
	ActiveGrant bool     `json:"active_grant"`
}

// ListConnectedApps returns every consented client with the state of its
// live grant, newest consent first.
func (s *Store) ListConnectedApps() ([]ConnectedApp, error) {
	rows, err := s.DB.Query(`
		SELECT c.id, c.name, c.consented_at,
		       COALESCE(MAX(t.scopes), ''),
		       COALESCE(MAX(t.last_used_at), ''),
		       COALESCE(MAX(CASE WHEN t.revoked_at = '' AND t.refresh_expires_at > ?
		                         THEN 1 ELSE 0 END), 0)
		FROM oauth_clients c
		LEFT JOIN oauth_tokens t ON t.client_id = c.id
		WHERE c.consented_at != ''
		GROUP BY c.id
		ORDER BY c.consented_at DESC`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []ConnectedApp
	for rows.Next() {
		var app ConnectedApp
		var scopes string
		var active int
		if err := rows.Scan(&app.ClientID, &app.Name, &app.ConsentedAt, &scopes, &app.LastUsedAt, &active); err != nil {
			return nil, err
		}
		app.ActiveGrant = active == 1
		app.Scopes = strings.Fields(scopes)
		if app.Scopes == nil {
			app.Scopes = []string{}
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

// DisconnectApp revokes every token the client holds and forgets the client
// itself, so a reconnect has to pass consent again. Returns an error only if
// the client does not exist — revoking an app with no live tokens is a
// perfectly reasonable thing to ask for.
func (s *Store) DisconnectApp(clientID string) error {
	res, err := s.DB.Exec("DELETE FROM oauth_clients WHERE id = ?", clientID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no connected app %q", clientID)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.DB.Exec("UPDATE oauth_tokens SET revoked_at = ? WHERE client_id = ? AND revoked_at = ''", now, clientID); err != nil {
		return fmt.Errorf("revoking tokens for %s: %w", clientID, err)
	}
	// Outstanding authorization codes for a disconnected client are dead too.
	_, _ = s.DB.Exec("DELETE FROM oauth_codes WHERE client_id = ?", clientID)
	return nil
}
