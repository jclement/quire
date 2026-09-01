// API token lifecycle: create/list/revoke (driven by the `quire token` CLI)
// and bearer verification. Tokens are sk_ + 32 random bytes hex; only the
// SHA-256 and an 8-char display prefix are stored — the plaintext is shown
// exactly once at creation.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Token is a stored API token (never contains the secret).
type Token struct {
	ID         int64
	Name       string
	Prefix     string
	Scopes     []string
	CreatedAt  string
	ExpiresAt  string
	RevokedAt  string
	LastUsedAt string
}

// CreateToken mints a new token, returning the plaintext exactly once.
func (s *Store) CreateToken(name string, scopes []string, expiresIn time.Duration) (plaintext string, _ Token, err error) {
	if name == "" {
		return "", Token{}, fmt.Errorf("token name is required")
	}
	for _, sc := range scopes {
		if sc != ScopeRead && sc != ScopeWrite && sc != ScopeTasks {
			return "", Token{}, fmt.Errorf("unknown scope %q (want read|write|tasks)", sc)
		}
	}
	if len(scopes) == 0 {
		scopes = []string{ScopeRead}
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", Token{}, fmt.Errorf("generating token: %w", err)
	}
	plaintext = "sk_" + hex.EncodeToString(secret)
	sum := sha256.Sum256([]byte(plaintext))

	expires := ""
	if expiresIn > 0 {
		expires = time.Now().Add(expiresIn).UTC().Format(time.RFC3339)
	}
	t := Token{
		Name:      name,
		Prefix:    plaintext[3:11],
		Scopes:    scopes,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		ExpiresAt: expires,
	}
	res, err := s.DB.Exec(`INSERT INTO api_tokens (name, prefix, hash, scopes, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		t.Name, t.Prefix, hex.EncodeToString(sum[:]), strings.Join(scopes, ","), t.CreatedAt, t.ExpiresAt)
	if err != nil {
		return "", Token{}, fmt.Errorf("storing token: %w", err)
	}
	t.ID, _ = res.LastInsertId()
	return plaintext, t, nil
}

// ListTokens returns all tokens, including revoked ones (audit trail).
func (s *Store) ListTokens() ([]Token, error) {
	rows, err := s.DB.Query(`SELECT id, name, prefix, scopes, created_at, expires_at, revoked_at, last_used_at
		FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []Token
	for rows.Next() {
		var t Token
		var scopes string
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &scopes, &t.CreatedAt, &t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		t.Scopes = strings.Split(scopes, ",")
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// RevokeToken soft-revokes by display prefix (kept for the audit trail).
func (s *Store) RevokeToken(prefix string) error {
	res, err := s.DB.Exec(`UPDATE api_tokens SET revoked_at = ? WHERE prefix = ? AND revoked_at = ''`,
		time.Now().UTC().Format(time.RFC3339), prefix)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no active token with prefix %q", prefix)
	}
	return nil
}

// authenticateBearer resolves the Authorization header to a principal.
func (s *Store) authenticateBearer(r *http.Request) (Principal, error) {
	raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if raw == "" || !strings.HasPrefix(raw, "sk_") {
		return Principal{}, fmt.Errorf("missing bearer token")
	}
	sum := sha256.Sum256([]byte(raw))
	want := hex.EncodeToString(sum[:])

	var id int64
	var name, hash, scopes, expires, revoked string
	err := s.DB.QueryRow(`SELECT id, name, hash, scopes, expires_at, revoked_at FROM api_tokens WHERE hash = ?`, want).
		Scan(&id, &name, &hash, &scopes, &expires, &revoked)
	if err != nil {
		return Principal{}, fmt.Errorf("unknown token")
	}
	// The hash lookup already proves possession; the constant-time compare
	// is belt-and-braces against SQL-collation surprises.
	if subtle.ConstantTimeCompare([]byte(hash), []byte(want)) != 1 {
		return Principal{}, fmt.Errorf("unknown token")
	}
	if revoked != "" {
		return Principal{}, fmt.Errorf("token revoked")
	}
	if expires != "" {
		if t, err := time.Parse(time.RFC3339, expires); err != nil || time.Now().After(t) {
			return Principal{}, fmt.Errorf("token expired")
		}
	}

	// last_used_at is display metadata; failing to update it must not fail
	// the request.
	_, _ = s.DB.Exec(`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), id)

	scopeSet := map[string]bool{}
	for _, sc := range strings.Split(scopes, ",") {
		scopeSet[sc] = true
	}
	return Principal{Name: "token:" + name, Scopes: scopeSet}, nil
}
