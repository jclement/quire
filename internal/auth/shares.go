// Share links live in auth.db (they are grants, not derivable from the
// vault): revocable tokens mapping a public URL to one document. Sharing
// defaults to off; a share exposes exactly one document read-only, plus the
// attachments that document references.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

// Share is one sharing grant.
type Share struct {
	Token        string `json:"token"`
	DocPath      string `json:"doc_path"`
	CreatedAt    string `json:"created_at"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	RevokedAt    string `json:"revoked_at,omitempty"`
	ViewCount    int64  `json:"view_count"`
	LastViewedAt string `json:"last_viewed_at,omitempty"`
}

// CreateShare mints a share for docPath. expiresIn <= 0 means no expiry.
func (s *Store) CreateShare(docPath string, expiresIn time.Duration) (Share, error) {
	if docPath == "" {
		return Share{}, fmt.Errorf("document path is required")
	}
	// 12 random bytes → 16 URL-safe chars: unguessable, short enough to read
	// aloud from a phone.
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return Share{}, fmt.Errorf("generating share token: %w", err)
	}
	share := Share{
		Token:     base64.RawURLEncoding.EncodeToString(raw),
		DocPath:   docPath,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if expiresIn > 0 {
		share.ExpiresAt = time.Now().Add(expiresIn).UTC().Format(time.RFC3339)
	}
	_, err := s.DB.Exec(`INSERT INTO shares (token, doc_path, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		share.Token, share.DocPath, share.CreatedAt, share.ExpiresAt)
	if err != nil {
		return Share{}, fmt.Errorf("storing share: %w", err)
	}
	return share, nil
}

// ListShares returns all shares, newest first (revoked included — audit
// trail, same as tokens).
func (s *Store) ListShares() ([]Share, error) {
	rows, err := s.DB.Query(`SELECT token, doc_path, created_at, expires_at, revoked_at, view_count, last_viewed_at
		FROM shares ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var shares []Share
	for rows.Next() {
		var sh Share
		if err := rows.Scan(&sh.Token, &sh.DocPath, &sh.CreatedAt, &sh.ExpiresAt, &sh.RevokedAt, &sh.ViewCount, &sh.LastViewedAt); err != nil {
			return nil, err
		}
		shares = append(shares, sh)
	}
	return shares, rows.Err()
}

// RevokeShare soft-revokes a share token.
func (s *Store) RevokeShare(token string) error {
	res, err := s.DB.Exec(`UPDATE shares SET revoked_at = ? WHERE token = ? AND revoked_at = ''`,
		time.Now().UTC().Format(time.RFC3339), token)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no active share %q", token)
	}
	return nil
}

// ResolveShare returns the active share for token (revoked/expired shares
// resolve as not-found, indistinguishable from never-existed) and bumps its
// view counter.
func (s *Store) ResolveShare(token string) (Share, error) {
	var sh Share
	err := s.DB.QueryRow(`SELECT token, doc_path, created_at, expires_at, revoked_at, view_count, last_viewed_at
		FROM shares WHERE token = ?`, token).
		Scan(&sh.Token, &sh.DocPath, &sh.CreatedAt, &sh.ExpiresAt, &sh.RevokedAt, &sh.ViewCount, &sh.LastViewedAt)
	if err != nil {
		return Share{}, fmt.Errorf("share not found")
	}
	if sh.RevokedAt != "" {
		return Share{}, fmt.Errorf("share not found")
	}
	if sh.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, sh.ExpiresAt); err != nil || time.Now().After(t) {
			return Share{}, fmt.Errorf("share not found")
		}
	}
	_, _ = s.DB.Exec(`UPDATE shares SET view_count = view_count + 1, last_viewed_at = ? WHERE token = ?`,
		time.Now().UTC().Format(time.RFC3339), token)
	sh.ViewCount++
	return sh, nil
}
