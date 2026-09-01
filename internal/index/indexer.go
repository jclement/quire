// The indexer: parse one file → replace its rows, plus the full-scan and
// removal paths. All writes to index.db funnel through these functions on
// whatever goroutine calls them; the watcher serializes its calls, and the
// single-connection pool serializes the rest.
package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"strings"

	"github.com/jclement/quire/internal/markdown"
	"github.com/jclement/quire/internal/vault"
)

// Event describes an index change, published to the UI over SSE.
type Event struct {
	Path   string `json:"path"`
	Action string `json:"action"` // "upsert" | "delete"
}

// Index owns index.db and knows how to (re)build it from a Vault.
type Index struct {
	DB    *sql.DB
	Vault *vault.Vault
	// Notify, when set, receives an Event after each applied change.
	Notify func(Event)
}

func (ix *Index) notify(ev Event) {
	if ix.Notify != nil {
		ix.Notify(ev)
	}
}

// IndexFile parses the file at rel and replaces all of its index rows.
// Returns false when the file was already indexed at the same content hash
// (the mechanism that makes the watcher ignore quire's own writes).
func (ix *Index) IndexFile(rel string) (bool, error) {
	f, err := ix.Vault.Read(rel)
	if err != nil {
		return false, err
	}

	var existing string
	err = ix.DB.QueryRow("SELECT sha256 FROM documents WHERE path = ?", rel).Scan(&existing)
	if err == nil && existing == f.SHA256 {
		return false, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking %s: %w", rel, err)
	}

	doc := markdown.Scan(rel, f.Raw)
	fm := vault.ParseFrontmatter(f.Raw)
	docType := effectiveType(rel, fm)
	title := effectiveTitle(rel, doc.Title, fm)

	fmJSON, err := json.Marshal(orEmptyMap(fm))
	if err != nil {
		// Frontmatter values that don't marshal (shouldn't happen with YAML
		// input) must not block indexing the document itself.
		fmJSON = []byte("{}")
	}

	tx, err := ix.DB.Begin()
	if err != nil {
		return false, fmt.Errorf("indexing %s: %w", rel, err)
	}
	defer tx.Rollback()

	if err := deleteDocRows(tx, rel); err != nil {
		return false, err
	}

	_, err = tx.Exec(`INSERT INTO documents (path, type, title, mtime, size, sha256, frontmatter_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rel, string(docType), title, f.ModTime.Unix(), len(f.Raw), f.SHA256, string(fmJSON))
	if err != nil {
		return false, fmt.Errorf("inserting document %s: %w", rel, err)
	}

	if err := insertDocNames(tx, rel, title, fm); err != nil {
		return false, err
	}
	for _, l := range doc.Links {
		if _, err := tx.Exec(`INSERT INTO links (src_path, target_norm, target_raw, display, line) VALUES (?, ?, ?, ?, ?)`,
			rel, normalizeName(l.Raw), l.Raw, l.Display, l.Line); err != nil {
			return false, fmt.Errorf("inserting link in %s: %w", rel, err)
		}
	}
	for _, tag := range doc.Tags {
		if _, err := tx.Exec(`INSERT INTO tags (path, tag) VALUES (?, ?)`, rel, tag); err != nil {
			return false, fmt.Errorf("inserting tag in %s: %w", rel, err)
		}
	}
	if err := insertTasks(tx, rel, fm, doc.Tasks); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`INSERT INTO fts (path, title, body, tags) VALUES (?, ?, ?, ?)`,
		rel, title, doc.Body, strings.Join(doc.Tags, " ")); err != nil {
		return false, fmt.Errorf("inserting fts row for %s: %w", rel, err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("committing index of %s: %w", rel, err)
	}
	ix.notify(Event{Path: rel, Action: "upsert"})
	return true, nil
}

// Remove drops all index rows for rel (the file is gone from disk).
func (ix *Index) Remove(rel string) error {
	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := deleteDocRows(tx, rel); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM documents WHERE path = ?", rel); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	ix.notify(Event{Path: rel, Action: "delete"})
	return nil
}

// FullScan walks the vault, indexing new/changed files and removing rows for
// files that no longer exist. Cheap when nothing changed: it compares
// (mtime, size) before re-reading content.
func (ix *Index) FullScan() error {
	known := map[string][2]int64{} // path → (mtime, size)
	rows, err := ix.DB.Query("SELECT path, mtime, size FROM documents")
	if err != nil {
		return fmt.Errorf("full scan: %w", err)
	}
	for rows.Next() {
		var p string
		var mtime, size int64
		if err := rows.Scan(&p, &mtime, &size); err != nil {
			return err
		}
		known[p] = [2]int64{mtime, size}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	seen := map[string]bool{}
	err = ix.Vault.WalkMarkdown(func(rel string, info fs.FileInfo) error {
		seen[rel] = true
		if k, ok := known[rel]; ok && k[0] == info.ModTime().Unix() && k[1] == info.Size() {
			return nil
		}
		if _, err := ix.IndexFile(rel); err != nil {
			// One broken file must not abort the scan.
			slog.Warn("indexing failed", "path", rel, "err", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking vault: %w", err)
	}

	for p := range known {
		if !seen[p] {
			if err := ix.Remove(p); err != nil {
				slog.Warn("removing stale index rows", "path", p, "err", err)
			}
		}
	}
	return nil
}

// ---- row helpers ----

func deleteDocRows(tx *sql.Tx, rel string) error {
	stmts := []string{
		"DELETE FROM docnames WHERE path = ?",
		"DELETE FROM links WHERE src_path = ?",
		"DELETE FROM tags WHERE path = ?",
		"DELETE FROM task_links WHERE task_id IN (SELECT id FROM tasks WHERE doc_path = ?)",
		"DELETE FROM tasks WHERE doc_path = ?",
		"DELETE FROM fts WHERE path = ?",
		"DELETE FROM documents WHERE path = ?",
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s, rel); err != nil {
			return fmt.Errorf("clearing rows for %s: %w", rel, err)
		}
	}
	return nil
}

// insertDocNames records every name this document answers to for wikilink
// resolution: path, path-sans-extension, filename stem, lowered title, slug
// of title, and any frontmatter aliases.
func insertDocNames(tx *sql.Tx, rel, title string, fm map[string]any) error {
	stem := strings.TrimSuffix(path.Base(rel), ".md")
	names := map[string]struct{}{
		normalizeName(rel): {},
		normalizeName(strings.TrimSuffix(rel, ".md")): {},
		normalizeName(stem):                           {},
		normalizeName(title):                          {},
		vault.Slugify(title):                          {},
	}
	for _, a := range stringList(fm["aliases"]) {
		names[normalizeName(a)] = struct{}{}
	}
	delete(names, "")
	for n := range names {
		if _, err := tx.Exec("INSERT INTO docnames (path, name) VALUES (?, ?)", rel, n); err != nil {
			return fmt.Errorf("inserting docname for %s: %w", rel, err)
		}
	}
	return nil
}

func insertTasks(tx *sql.Tx, rel string, fm map[string]any, tasks []markdown.Task) error {
	// The document's own project association is the fallback for tasks that
	// don't name one: a `project:` frontmatter link, or the document itself
	// when it *is* a project.
	docProject := normalizeName(stripWikilink(stringValue(fm["project"])))
	if vault.InferType(rel) == vault.TypeProject {
		docProject = normalizeName(strings.TrimSuffix(rel, ".md"))
	}

	seen := map[string]int{}
	for _, t := range tasks {
		id := t.ID
		// Identical text twice in one file: suffix with the occurrence
		// ordinal (accepted edge; see DESIGN.md).
		if n := seen[t.ID]; n > 0 {
			id = fmt.Sprintf("%s-%d", t.ID, n)
		}
		seen[t.ID]++

		tagsJSON, _ := json.Marshal(orEmptyList(t.Tags))
		_, err := tx.Exec(`INSERT INTO tasks
			(id, doc_path, line, text, raw_text, done, due, defer_date, completed_on, priority, waiting, project_norm, tags_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, rel, t.Line, t.Text, t.RawText, t.Done, t.Due, t.Defer, t.CompletedOn,
			t.Priority, t.Waiting, docProject, string(tagsJSON))
		if err != nil {
			return fmt.Errorf("inserting task in %s: %w", rel, err)
		}
		for _, l := range t.Links {
			if _, err := tx.Exec("INSERT INTO task_links (task_id, target_norm) VALUES (?, ?)", id, normalizeName(l.Raw)); err != nil {
				return fmt.Errorf("inserting task link in %s: %w", rel, err)
			}
		}
	}
	return nil
}

// ---- small conversions ----

// normalizeName is the one canonicalization used on both sides of the
// docnames join.
func normalizeName(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// stripWikilink unwraps "[[Target|Alias]]" → "Target"; plain strings pass
// through.
func stripWikilink(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[[")
	s = strings.TrimSuffix(s, "]]")
	if target, _, found := strings.Cut(s, "|"); found {
		return target
	}
	return s
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

// stringList coerces a frontmatter value (string or []any) to []string.
func stringList(v any) []string {
	switch vv := v.(type) {
	case string:
		return []string{vv}
	case []any:
		var out []string
		for _, item := range vv {
			if s, ok := item.(string); ok {
				out = append(out, stripWikilink(s))
			}
		}
		return out
	}
	return nil
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func orEmptyList(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func effectiveType(rel string, fm map[string]any) vault.DocType {
	if t := stringValue(fm["type"]); t != "" {
		switch vault.DocType(t) {
		case vault.TypeNote, vault.TypePerson, vault.TypeCompany, vault.TypeProject, vault.TypeMeeting, vault.TypeDaily:
			return vault.DocType(t)
		}
	}
	return vault.InferType(rel)
}

func effectiveTitle(rel, scannedTitle string, fm map[string]any) string {
	if t := stringValue(fm["title"]); t != "" {
		return t
	}
	if scannedTitle != "" {
		return scannedTitle
	}
	// Filename fallback: "sarah-chen" → "Sarah Chen" is too presumptuous;
	// just use the stem verbatim.
	return strings.TrimSuffix(path.Base(rel), ".md")
}
