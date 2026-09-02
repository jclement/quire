// The audit log: what agents did to the vault. DESIGN.md has promised
// "every API/MCP write is audit-logged" since v0.1; this is where that
// finally becomes true.
//
// Scope is deliberate: actions by API tokens and OAuth clients are recorded,
// the owner's own browser session is not. The question the log answers is
// "what did the agents do?", and the owner's autosaves would drown it.
package auth

import (
	"fmt"
	"strings"
	"time"
)

// maxAuditRows bounds the table. Past it the oldest rows go; a single-user
// instance does not need an unbounded history of tool calls.
const maxAuditRows = 20_000

// AuditRecord is one row as stored.
type AuditRecord struct {
	ID        int64
	At        string
	Principal string
	Action    string
	Path      string
	Detail    string
	OK        bool
}

// Audited reports whether actions by this principal are recorded: every
// non-owner principal is. "owner" is the browser session and auth-none.
func Audited(p Principal) bool { return p.Name != "owner" }

// RecordAudit appends one entry. Failures are logged by callers, never
// surfaced to the caller of the audited action — an audit write must not
// be able to fail a document write.
func (s *Store) RecordAudit(rec AuditRecord) error {
	if len(rec.Detail) > 200 {
		rec.Detail = rec.Detail[:200] + "…"
	}
	_, err := s.DB.Exec(`INSERT INTO audit_log (at, principal, action, path, detail, ok) VALUES (?, ?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), rec.Principal, rec.Action, rec.Path, rec.Detail, rec.OK)
	if err != nil {
		return fmt.Errorf("recording audit: %w", err)
	}
	// Prune occasionally rather than on every insert.
	if rec.ID%100 == 0 {
		_, _ = s.DB.Exec(`DELETE FROM audit_log WHERE id NOT IN (SELECT id FROM audit_log ORDER BY id DESC LIMIT ?)`, maxAuditRows)
	}
	return nil
}

// ListAudit returns the newest entries first.
func (s *Store) ListAudit(limit int) ([]AuditRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.Query(`SELECT id, at, principal, action, path, detail, ok FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditRecord
	for rows.Next() {
		var r AuditRecord
		if err := rows.Scan(&r.ID, &r.At, &r.Principal, &r.Action, &r.Path, &r.Detail, &r.OK); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// summarize trims a value for the detail column.
func summarize(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}
