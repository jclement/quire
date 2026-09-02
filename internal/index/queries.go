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
	Area        string
}

// AreaUnclassified is the filter value meaning "documents with no area".
// Daily notes are excluded from it: they belong to every area, not none.
const AreaUnclassified = "none"

// areaClause restricts documents alias `d` to an area. "" is no filter.
func areaClause(area string) (string, []any) {
	switch NormalizeArea(area) {
	case "":
		return "", nil
	case AreaUnclassified:
		return " AND d.area = '' AND d.type != 'daily'", nil
	default:
		return " AND d.area = ?", []any{NormalizeArea(area)}
	}
}

// AreaCount is one area with how many documents are filed under it.
type AreaCount struct {
	Area  string
	Count int
}

// Areas returns every area in use with its document count, most-used first.
func (ix *Index) Areas() ([]AreaCount, error) {
	rows, err := ix.DB.Query(`SELECT area, COUNT(*) FROM documents WHERE area != '' GROUP BY area ORDER BY 2 DESC, area`)
	if err != nil {
		return nil, fmt.Errorf("listing areas: %w", err)
	}
	defer rows.Close()
	var out []AreaCount
	for rows.Next() {
		var a AreaCount
		if err := rows.Scan(&a.Area, &a.Count); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
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
	       COALESCE((SELECT group_concat(t.tag) FROM tags t WHERE t.path = d.path), ''),
	       d.area
	FROM documents d`

func scanDocRow(rows interface{ Scan(...any) error }) (DocRow, error) {
	var d DocRow
	var mtime int64
	var fm, tags string
	if err := rows.Scan(&d.Path, &d.Type, &d.Title, &mtime, &d.SHA256, &fm, &tags, &d.Area); err != nil {
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
func (ix *Index) ListDocuments(docType, titleQuery, area string, limit int) ([]DocRow, error) {
	where := "WHERE 1=1"
	args := []any{}
	if clause, a := areaClause(area); clause != "" {
		where += clause
		args = append(args, a...)
	}
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
func (ix *Index) MeetingsOn(day, area string) ([]DocRow, error) {
	areaWhere, areaArgs := areaClause(area)
	rows, err := ix.DB.Query(docSelect+`
		WHERE d.type = 'meeting'
		  AND json_extract(d.frontmatter_json, '$.date') LIKE ?`+areaWhere+`
		ORDER BY json_extract(d.frontmatter_json, '$.date')`, append([]any{day + "%"}, areaArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("meetings on %s: %w", day, err)
	}
	defer rows.Close()
	return collectDocs(rows)
}

// PeopleWithBirthdays returns person documents that declare a birthday in
// frontmatter (any of YYYY-MM-DD or MM-DD; the service does the date math).
func (ix *Index) PeopleWithBirthdays() ([]DocRow, error) {
	rows, err := ix.DB.Query(docSelect + `
		WHERE d.type = 'person'
		  AND json_extract(d.frontmatter_json, '$.birthday') IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("people with birthdays: %w", err)
	}
	defer rows.Close()
	return collectDocs(rows)
}

// DocsModifiedBetween returns documents whose mtime falls in [from, to),
// oldest first — the input to the calendar month view.
func (ix *Index) DocsModifiedBetween(from, to time.Time) ([]DocRow, error) {
	rows, err := ix.DB.Query(docSelect+" WHERE d.mtime >= ? AND d.mtime < ? ORDER BY d.mtime",
		from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("documents modified between: %w", err)
	}
	defer rows.Close()
	return collectDocs(rows)
}

// MeetingsBetween returns meetings whose frontmatter date falls on a day in
// [fromDay, toDay] (inclusive, YYYY-MM-DD strings).
func (ix *Index) MeetingsBetween(fromDay, toDay string) ([]DocRow, error) {
	rows, err := ix.DB.Query(docSelect+`
		WHERE d.type = 'meeting'
		  AND substr(COALESCE(json_extract(d.frontmatter_json, '$.date'), ''), 1, 10) BETWEEN ? AND ?
		ORDER BY json_extract(d.frontmatter_json, '$.date')`, fromDay, toDay)
	if err != nil {
		return nil, fmt.Errorf("meetings between: %w", err)
	}
	defer rows.Close()
	return collectDocs(rows)
}

// TasksCompletedBetween counts completed tasks per day (YYYY-MM-DD → count)
// across the given inclusive day range.
func (ix *Index) TasksCompletedBetween(fromDay, toDay string) (map[string]int, error) {
	rows, err := ix.DB.Query(`SELECT completed_on, COUNT(*) FROM tasks
		WHERE done = 1 AND completed_on BETWEEN ? AND ? GROUP BY completed_on`, fromDay, toDay)
	if err != nil {
		return nil, fmt.Errorf("tasks completed between: %w", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var day string
		var n int
		if err := rows.Scan(&day, &n); err != nil {
			return nil, err
		}
		counts[day] = n
	}
	return counts, rows.Err()
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
func (ix *Index) Tasks(view TaskView, today, area string) ([]TaskRow, error) {
	var where, order string
	args := []any{}
	areaWhere, areaArgs := areaClause(area)
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
	rows, err := ix.DB.Query(taskSelect+" WHERE "+where+areaWhere+" ORDER BY "+order, append(args, areaArgs...)...)
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
func (ix *Index) OpenTasksDue(day, area string) ([]TaskRow, error) {
	areaWhere, areaArgs := areaClause(area)
	rows, err := ix.DB.Query(taskSelect+" WHERE t.done = 0 AND t.due != '' AND t.due <= ?"+areaWhere+" ORDER BY t.due, t.priority = 0, t.priority", append([]any{day}, areaArgs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectTasks(rows)
}

// ---- search ----

// Search runs the shared query grammar: bare words go to FTS (last word as a
// prefix); `type:x` and `tag:x` filter; `is:task` switches to task search
// with optional `due:today|overdue|week|YYYY-MM-DD`. today anchors the date
// filters. Returns snippeted hits.
func (ix *Index) Search(query string, limit int, today string) ([]SearchHit, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var terms []string
	var docType, tag, due, area string
	isTask := false
	for _, tok := range strings.Fields(query) {
		switch {
		case strings.HasPrefix(tok, "area:"):
			area = strings.TrimPrefix(tok, "area:")
		case tok == "is:task":
			isTask = true
		case strings.HasPrefix(tok, "due:"):
			isTask = true
			due = strings.TrimPrefix(tok, "due:")
		case strings.HasPrefix(tok, "type:"):
			docType = strings.TrimPrefix(tok, "type:")
		case strings.HasPrefix(tok, "tag:"):
			tag = strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(tok, "tag:"), "#"))
		default:
			terms = append(terms, tok)
		}
	}
	if isTask {
		return ix.searchTasks(terms, tag, due, area, today, limit)
	}

	where := "WHERE 1=1"
	args := []any{}
	if clause, a := areaClause(area); clause != "" {
		where += clause
		args = append(args, a...)
	}
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

// searchTasks answers `is:task` queries against the task index: substring
// terms over the display text, tag/due filters. Hits carry the task text as
// the title and the source document as the snippet.
func (ix *Index) searchTasks(terms []string, tag, due, area, today string, limit int) ([]SearchHit, error) {
	where := "t.done = 0"
	args := []any{}
	if clause, a := areaClause(area); clause != "" {
		where += clause
		args = append(args, a...)
	}
	for _, term := range terms {
		where += " AND t.text LIKE ? COLLATE NOCASE"
		args = append(args, "%"+term+"%")
	}
	if tag != "" {
		where += ` AND EXISTS (SELECT 1 FROM json_each(t.tags_json) WHERE json_each.value = ?)`
		args = append(args, tag)
	}
	switch due {
	case "":
	case "today":
		where += " AND t.due != '' AND t.due <= ?"
		args = append(args, today)
	case "overdue":
		where += " AND t.due != '' AND t.due < ?"
		args = append(args, today)
	case "week":
		where += " AND t.due != '' AND t.due <= date(?, '+7 days')"
		args = append(args, today)
	default:
		where += " AND t.due = ?"
		args = append(args, due)
	}

	rows, err := ix.DB.Query(fmt.Sprintf(`
		SELECT t.doc_path, t.text, COALESCE(d.title, t.doc_path)
		FROM tasks t LEFT JOIN documents d ON d.path = t.doc_path
		WHERE %s ORDER BY t.due = '', t.due LIMIT %d`, where, limit), args...)
	if err != nil {
		return nil, fmt.Errorf("task search: %w", err)
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		h.Type = "task"
		if err := rows.Scan(&h.Path, &h.Title, &h.Snippet); err != nil {
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

// TagCount is one tag with how many documents carry it.
type TagCount struct {
	Tag   string
	Count int
}

// Tags returns every tag in the vault with its document count, most-used
// first. Tags come from both body #tags and frontmatter tags:, already
// merged at index time.
func (ix *Index) Tags() ([]TagCount, error) {
	rows, err := ix.DB.Query(`SELECT tag, COUNT(DISTINCT path) AS n FROM tags GROUP BY tag ORDER BY n DESC, tag ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	defer rows.Close()
	var out []TagCount
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DailyNotesBefore returns existing daily notes dated strictly before
// `date` (YYYY-MM-DD), newest first — the journal view's page of history.
// Daily paths sort lexically as dates, which is what makes this one query.
func (ix *Index) DailyNotesBefore(date string, limit int) ([]DocRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	rows, err := ix.DB.Query(docSelect+` WHERE d.type = 'daily' AND d.path < ? ORDER BY d.path DESC LIMIT ?`,
		"daily/"+date+".md", limit)
	if err != nil {
		return nil, fmt.Errorf("listing daily notes: %w", err)
	}
	defer rows.Close()
	return collectDocs(rows)
}
