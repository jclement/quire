// Read-side queries over index.db: document lists, backlinks, task views,
// search, and the Today rollup inputs. The service layer shapes these rows
// into API responses; no HTTP or JSON-envelope concerns live here.
package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DocRow is a document's indexed metadata.
type DocRow struct {
	Path        string
	Type        string
	Title       string
	Mtime       time.Time
	SHA256      string
	Tags        []string
	Frontmatter json.RawMessage
}

// TaskRow is a task with its rollup joins resolved.
type TaskRow struct {
	ID          string
	DocPath     string
	DocTitle    string
	Line        int
	Text        string
	Done        bool
	Due         string
	Defer       string
	CompletedOn string
	Priority    int
	Waiting     bool
	Recur       string
	RawText     string
	ProjectPath string
	Tags        []string
}

// SearchHit is one FTS result.
type SearchHit struct {
	Path    string
	Type    string
	Title   string
	Snippet string
}

const docSelect = `
	SELECT d.path, d.type, d.title, d.mtime, d.sha256, d.frontmatter_json,
	       COALESCE((SELECT group_concat(t.tag) FROM tags t WHERE t.path = d.path), '')
	FROM documents d`

func scanDocRow(rows interface{ Scan(...any) error }) (DocRow, error) {
	var d DocRow
	var mtime int64
	var fm, tags string
	if err := rows.Scan(&d.Path, &d.Type, &d.Title, &mtime, &d.SHA256, &fm, &tags); err != nil {
		return DocRow{}, err
	}
	d.Mtime = time.Unix(mtime, 0)
	d.Frontmatter = json.RawMessage(fm)
	if tags != "" {
		d.Tags = strings.Split(tags, ",")
	} else {
		d.Tags = []string{}
	}
	return d, nil
}

// ListDocuments returns documents, optionally filtered by type and a
// case-insensitive title substring, newest first.
func (ix *Index) ListDocuments(docType, titleQuery string, limit int) ([]DocRow, error) {
	where := "WHERE 1=1"
	args := []any{}
	if docType != "" {
		where += " AND d.type = ?"
		args = append(args, docType)
	}
	if titleQuery != "" {
		where += " AND d.title LIKE ? COLLATE NOCASE"
		args = append(args, "%"+titleQuery+"%")
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	args = append(args, limit)

	rows, err := ix.DB.Query(docSelect+" "+where+" ORDER BY d.mtime DESC LIMIT ?", args...)
	if err != nil {
		return nil, fmt.Errorf("listing documents: %w", err)
	}
	defer rows.Close()
	return collectDocs(rows)
}

// GetDocMeta returns one document's indexed row, or sql.ErrNoRows.
func (ix *Index) GetDocMeta(path string) (DocRow, error) {
	row := ix.DB.QueryRow(docSelect+" WHERE d.path = ?", path)
	return scanDocRow(row)
}

// Backlinks lists documents that link to the document at path, by any of the
// names it answers to.
func (ix *Index) Backlinks(path string) ([]DocRow, error) {
	rows, err := ix.DB.Query(docSelect+`
		WHERE d.path IN (
			SELECT DISTINCT l.src_path FROM links l
			JOIN docnames n ON n.name = l.target_norm
			WHERE n.path = ? AND l.src_path != ?
		) ORDER BY d.mtime DESC`, path, path)
	if err != nil {
		return nil, fmt.Errorf("backlinks of %s: %w", path, err)
	}
	defer rows.Close()
	return collectDocs(rows)
}

// ResolveLink maps a raw wikilink target to a document path ("" when
// dangling). Ties break lexicographically for determinism.
func (ix *Index) ResolveLink(raw string) string {
	var path string
	err := ix.DB.QueryRow("SELECT MIN(path) FROM docnames WHERE name = ?", normalizeName(raw)).Scan(&path)
	if err != nil {
		return ""
	}
	return path
}

// MeetingsOn returns meeting documents whose frontmatter date falls on day
// (YYYY-MM-DD), ordered by that date (so times sort naturally).
func (ix *Index) MeetingsOn(day string) ([]DocRow, error) {
	rows, err := ix.DB.Query(docSelect+`
		WHERE d.type = 'meeting'
		  AND json_extract(d.frontmatter_json, '$.date') LIKE ?
		ORDER BY json_extract(d.frontmatter_json, '$.date')`, day+"%")
	if err != nil {
		return nil, fmt.Errorf("meetings on %s: %w", day, err)
	}
	defer rows.Close()
	return collectDocs(rows)
}

// ---- tasks ----

const taskSelect = `
	SELECT t.id, t.doc_path, COALESCE(d.title, t.doc_path), t.line, t.text, t.done,
	       t.due, t.defer_date, t.completed_on, t.priority, t.waiting, t.recur, t.raw_text, t.tags_json,
	       COALESCE((
	           SELECT MIN(n.path) FROM docnames n
	           WHERE n.name IN (SELECT tl.target_norm FROM task_links tl WHERE tl.task_id = t.id UNION SELECT t.project_norm)
	             AND n.path IN (SELECT path FROM documents WHERE type = 'project')
	       ), '')
	FROM tasks t LEFT JOIN documents d ON d.path = t.doc_path`

// TaskView is a named task list. Semantics per DESIGN.md "Tasks".
type TaskView string

const (
	ViewInbox    TaskView = "inbox"    // open, undated, unfiled — needs processing
	ViewToday    TaskView = "today"    // open, overdue / due today / deferred-to-now
	ViewUpcoming TaskView = "upcoming" // open, dated in the future
	ViewWaiting  TaskView = "waiting"  // open, delegated
	ViewLogbook  TaskView = "logbook"  // completed, newest first
)

// Tasks returns the tasks for a view; today is the local YYYY-MM-DD used for
// all date comparisons (passed in for testability).
func (ix *Index) Tasks(view TaskView, today string) ([]TaskRow, error) {
	var where, order string
	args := []any{}
	switch view {
	case ViewInbox:
		where = "t.done = 0 AND t.due = '' AND t.defer_date = '' AND t.waiting = 0 AND t.project_norm = ''"
		order = "t.doc_path, t.line"
	case ViewToday:
		where = `t.done = 0 AND t.waiting = 0 AND (
			(t.due != '' AND t.due <= ?) OR
			(t.defer_date != '' AND t.defer_date <= ? AND (t.due = '' OR t.due <= ?)))`
		args = append(args, today, today, today)
		order = "t.due != '', t.due, t.priority = 0, t.priority"
	case ViewUpcoming:
		where = "t.done = 0 AND ((t.due > ?) OR (t.due = '' AND t.defer_date > ?))"
		args = append(args, today, today)
		order = "CASE WHEN t.due != '' THEN t.due ELSE t.defer_date END"
	case ViewWaiting:
		where = "t.done = 0 AND t.waiting = 1"
		order = "t.due != '', t.due, t.doc_path"
	case ViewLogbook:
		where = "t.done = 1"
		order = "t.completed_on DESC, t.doc_path LIMIT 200"
	default:
		return nil, fmt.Errorf("unknown task view %q", view)
	}
	rows, err := ix.DB.Query(taskSelect+" WHERE "+where+" ORDER BY "+order, args...)
	if err != nil {
		return nil, fmt.Errorf("task view %s: %w", view, err)
	}
	defer rows.Close()
	return collectTasks(rows)
}

// TaskByID fetches a single task row.
func (ix *Index) TaskByID(id string) (TaskRow, error) {
	rows, err := ix.DB.Query(taskSelect+" WHERE t.id = ?", id)
	if err != nil {
		return TaskRow{}, err
	}
	defer rows.Close()
	tasks, err := collectTasks(rows)
	if err != nil {
		return TaskRow{}, err
	}
	if len(tasks) == 0 {
		return TaskRow{}, sql.ErrNoRows
	}
	return tasks[0], nil
}

// TasksMentioning returns open tasks whose text links the document at path
// (by any name it answers to) — the person/project page rollup.
func (ix *Index) TasksMentioning(path string) ([]TaskRow, error) {
	rows, err := ix.DB.Query(taskSelect+`
		WHERE t.done = 0 AND t.id IN (
			SELECT tl.task_id FROM task_links tl
			JOIN docnames n ON n.name = tl.target_norm
			WHERE n.path = ?
		) ORDER BY t.due = '', t.due`, path)
	if err != nil {
		return nil, fmt.Errorf("tasks mentioning %s: %w", path, err)
	}
	defer rows.Close()
	return collectTasks(rows)
}

// OpenTasksDue returns open tasks with due <= day (the Today screen's
// overdue + due-today sections split by the caller).
func (ix *Index) OpenTasksDue(day string) ([]TaskRow, error) {
	rows, err := ix.DB.Query(taskSelect+" WHERE t.done = 0 AND t.due != '' AND t.due <= ? ORDER BY t.due, t.priority = 0, t.priority", day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectTasks(rows)
}

// ---- search ----

// Search runs the shared query grammar: bare words go to FTS (last word as a
// prefix); `type:x` and `tag:x` filter. Returns snippeted hits.
func (ix *Index) Search(query string, limit int) ([]SearchHit, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var terms []string
	var docType, tag string
	for _, tok := range strings.Fields(query) {
		switch {
		case strings.HasPrefix(tok, "type:"):
			docType = strings.TrimPrefix(tok, "type:")
		case strings.HasPrefix(tok, "tag:"):
			tag = strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(tok, "tag:"), "#"))
		default:
			terms = append(terms, tok)
		}
	}

	where := "WHERE 1=1"
	args := []any{}
	join := ""
	if len(terms) > 0 {
		join = "JOIN fts ON fts.path = d.path"
		where += " AND fts MATCH ?"
		args = append(args, ftsQuery(terms))
	}
	if docType != "" {
		where += " AND d.type = ?"
		args = append(args, docType)
	}
	if tag != "" {
		where += " AND d.path IN (SELECT path FROM tags WHERE tag = ?)"
		args = append(args, tag)
	}

	snippet := "''"
	order := "d.mtime DESC"
	if len(terms) > 0 {
		snippet = "snippet(fts, 2, '<mark>', '</mark>', '…', 16)"
		order = "fts.rank"
	}

	rows, err := ix.DB.Query(fmt.Sprintf(
		"SELECT d.path, d.type, d.title, %s FROM documents d %s %s ORDER BY %s LIMIT %d",
		snippet, join, where, order, limit), args...)
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", query, err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.Path, &h.Type, &h.Title, &h.Snippet); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// ftsQuery builds a safe FTS5 MATCH expression: each term double-quoted (so
// user punctuation can't inject FTS syntax), the final term as a prefix so
// search-as-you-type works.
func ftsQuery(terms []string) string {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	quoted[len(quoted)-1] += "*"
	return strings.Join(quoted, " ")
}

// ---- collectors ----

func collectDocs(rows *sql.Rows) ([]DocRow, error) {
	var docs []DocRow
	for rows.Next() {
		d, err := scanDocRow(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func collectTasks(rows *sql.Rows) ([]TaskRow, error) {
	var tasks []TaskRow
	for rows.Next() {
		var t TaskRow
		var tagsJSON string
		if err := rows.Scan(&t.ID, &t.DocPath, &t.DocTitle, &t.Line, &t.Text, &t.Done,
			&t.Due, &t.Defer, &t.CompletedOn, &t.Priority, &t.Waiting, &t.Recur, &t.RawText, &tagsJSON, &t.ProjectPath); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(tagsJSON), &t.Tags); err != nil {
			t.Tags = []string{}
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
