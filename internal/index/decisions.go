// The decision log: what was decided, when, where, and about whom. Two
// sources feed one list — decision records (documents tagged `decision`,
// the templates/decision.md shape) and inline decisions (bullets under a
// "Decisions" heading, the meeting-note shape; see markdown.ScanDecisions).
// Both are written at index time into the decisions table so the log, the
// entity rail and `is:decision` search are single queries.
package index

import (
	"database/sql"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/jclement/quire/internal/markdown"
	"github.com/jclement/quire/internal/vault"
)

// Decision kinds.
const (
	DecisionInline = "inline" // a bullet under a Decisions heading
	DecisionRecord = "record" // a whole document tagged decision
)

// decisionTag marks a document as a decision record.
const decisionTag = "decision"

// decisionSource is what indexing one document knows that decisions need.
type decisionSource struct {
	path     string
	title    string
	docType  vault.DocType
	fm       map[string]any
	tags     map[string]struct{}
	links    []markdown.Link
	raw      []byte
	modified time.Time
}

// insertDecisions writes the document's decision rows: one for the whole
// document when it is a decision record, one per inline decision bullet.
// Templates are skeletons, not decisions, and are skipped.
func insertDecisions(tx *sql.Tx, src decisionSource) error {
	if src.docType == vault.TypeTemplate {
		return nil
	}
	date := decisionDate(src)
	inherited := docEntities(src.fm)

	if _, isRecord := src.tags[decisionTag]; isRecord {
		// A record is about everything it links to: it is one focused
		// document, so its body links are its subject, not passing mentions.
		var targets []string
		for _, l := range src.links {
			targets = append(targets, linkTarget(l.Raw))
		}
		if err := insertDecision(tx, src.path, DecisionRecord, 0, src.title, date, append(targets, inherited...)); err != nil {
			return err
		}
	}
	for _, d := range markdown.ScanDecisions(src.raw) {
		targets := make([]string, 0, len(d.Links)+len(inherited))
		for _, l := range d.Links {
			targets = append(targets, linkTarget(l.Raw))
		}
		if err := insertDecision(tx, src.path, DecisionInline, d.Line, d.Text, date, append(targets, inherited...)); err != nil {
			return err
		}
	}
	return nil
}

func insertDecision(tx *sql.Tx, docPath, kind string, line int, text, date string, targets []string) error {
	res, err := tx.Exec(`INSERT INTO decisions (doc_path, kind, line, text, date) VALUES (?, ?, ?, ?, ?)`,
		docPath, kind, line, text, date)
	if err != nil {
		return fmt.Errorf("inserting decision in %s: %w", docPath, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("inserting decision in %s: %w", docPath, err)
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		if _, err := tx.Exec(`INSERT INTO decision_links (decision_id, target_norm) VALUES (?, ?)`, id, target); err != nil {
			return fmt.Errorf("inserting decision link in %s: %w", docPath, err)
		}
	}
	return nil
}

// decisionDate is when the decision was made, as best the file says: its
// frontmatter date: (a meeting's, a record's), a daily note's own day, and
// only failing both the day the file last changed — which drifts with every
// edit, so it is the last resort rather than the rule.
func decisionDate(src decisionSource) string {
	switch v := src.fm["date"].(type) {
	case string:
		if day, ok := isoDay(v); ok {
			return day
		}
	case time.Time:
		return v.Format("2006-01-02")
	}
	if src.docType == vault.TypeDaily {
		if day, ok := isoDay(strings.TrimSuffix(path.Base(src.path), ".md")); ok {
			return day
		}
	}
	return src.modified.UTC().Format("2006-01-02")
}

// isoDay reads the YYYY-MM-DD prefix of a date or datetime string.
func isoDay(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 10 {
		return "", false
	}
	if _, err := time.Parse("2006-01-02", s[:10]); err != nil {
		return "", false
	}
	return s[:10], true
}

// ---- queries ----

// DecisionFilter narrows the decision log. Zero values mean no filter.
type DecisionFilter struct {
	// Text: every word must appear in the decision or its source's title.
	Text string
	// Entity is a vault path: decisions linking it, or made in it.
	Entity string
	Area   string
	// After and Before bound the decision's date, inclusive (YYYY-MM-DD).
	After  string
	Before string
	Limit  int
}

// EntityRef is a person, company or project a decision is about.
type EntityRef struct {
	Path  string
	Title string
	Type  string
}

// DecisionRow is one decision with its source and subjects resolved.
type DecisionRow struct {
	ID       int64
	Kind     string
	DocPath  string
	DocTitle string
	DocType  string
	Line     int
	Text     string
	Date     string
	Area     string
	Entities []EntityRef
}

// maxDecisions bounds one page of the log.
const maxDecisions = 500

// Decisions returns decisions newest first, filtered.
func (ix *Index) Decisions(f DecisionFilter) ([]DecisionRow, error) {
	where := "WHERE 1=1"
	var args []any
	for _, term := range strings.Fields(f.Text) {
		where += " AND (dc.text LIKE ? COLLATE NOCASE OR d.title LIKE ? COLLATE NOCASE)"
		args = append(args, "%"+term+"%", "%"+term+"%")
	}
	if f.Entity != "" {
		where += ` AND (dc.doc_path = ? OR dc.id IN (
			SELECT dl.decision_id FROM decision_links dl
			JOIN docnames n ON n.name = dl.target_norm
			WHERE n.path = ?))`
		args = append(args, f.Entity, f.Entity)
	}
	if clause, a := areaClause(f.Area); clause != "" {
		where += clause
		args = append(args, a...)
	}
	if f.After != "" {
		where += " AND dc.date >= ?"
		args = append(args, f.After)
	}
	if f.Before != "" {
		where += " AND dc.date <= ?"
		args = append(args, f.Before)
	}
	limit := f.Limit
	if limit <= 0 || limit > maxDecisions {
		limit = 100
	}
	args = append(args, limit)

	rows, err := ix.DB.Query(`
		SELECT dc.id, dc.kind, dc.doc_path, d.title, d.type, dc.line, dc.text, dc.date, d.area
		FROM decisions dc JOIN documents d ON d.path = dc.doc_path
		`+where+`
		ORDER BY dc.date DESC, dc.doc_path, dc.line
		LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing decisions: %w", err)
	}
	var out []DecisionRow
	for rows.Next() {
		var r DecisionRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.DocPath, &r.DocTitle, &r.DocType, &r.Line, &r.Text, &r.Date, &r.Area); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := ix.attachDecisionEntities(out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachDecisionEntities resolves each decision's links to the people,
// companies and projects they name — the chips on a decision row. A name
// shared by two documents resolves as ResolveLink does, to the lowest path.
func (ix *Index) attachDecisionEntities(decisions []DecisionRow) error {
	if len(decisions) == 0 {
		return nil
	}
	byID := make(map[int64]int, len(decisions))
	ids := make([]any, 0, len(decisions))
	for i, d := range decisions {
		byID[d.ID] = i
		ids = append(ids, d.ID)
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := ix.DB.Query(`
		SELECT r.decision_id, d.path, d.title, d.type FROM (
			SELECT dl.decision_id, MIN(n.path) AS path
			FROM decision_links dl JOIN docnames n ON n.name = dl.target_norm
			WHERE dl.decision_id IN (`+marks+`)
			GROUP BY dl.decision_id, dl.target_norm
		) r JOIN documents d ON d.path = r.path
		WHERE d.type IN ('person', 'company', 'project')
		ORDER BY d.type = 'project' DESC, d.title`, ids...)
	if err != nil {
		return fmt.Errorf("resolving decision links: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var ref EntityRef
		if err := rows.Scan(&id, &ref.Path, &ref.Title, &ref.Type); err != nil {
			return err
		}
		d := &decisions[byID[id]]
		if !containsEntity(d.Entities, ref.Path) {
			d.Entities = append(d.Entities, ref)
		}
	}
	return rows.Err()
}

// searchDecisions answers `is:decision` in the shared search grammar: the
// terms filter the log, and each hit is the decision (as the title) in its
// source document (as the snippet, with the date).
func (ix *Index) searchDecisions(terms []string, area, after, before string, limit int) ([]SearchHit, error) {
	rows, err := ix.Decisions(DecisionFilter{
		Text: strings.Join(terms, " "), Area: area, After: after, Before: before, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	hits := make([]SearchHit, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, SearchHit{Path: r.DocPath, Type: "decision", Title: r.Text, Snippet: r.Date + " · " + r.DocTitle})
	}
	return hits, nil
}

func containsEntity(refs []EntityRef, path string) bool {
	for _, r := range refs {
		if r.Path == path {
			return true
		}
	}
	return false
}
