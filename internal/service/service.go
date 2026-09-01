// Package service is the one business-logic layer under every transport:
// REST handlers and MCP tools both call these methods and nothing else, so
// permissions and behavior cannot drift between them (DESIGN.md decision 5).
// The wire shapes those methods return live in apitypes.go, which
// generates the frontend's TypeScript types.
package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/markdown"
	"github.com/jclement/quire/internal/vault"
)

// Service wires the vault and index together.
type Service struct {
	Vault *vault.Vault
	Index *index.Index
	// Now allows tests to pin the clock.
	Now func() time.Time
}

// New returns a Service using the real clock.
func New(v *vault.Vault, ix *index.Index) *Service {
	return &Service{Vault: v, Index: ix, Now: time.Now}
}

func (s *Service) today() string { return s.Now().Format("2006-01-02") }

// ---- conversions ----

func metaFromRow(d index.DocRow) DocMeta {
	return DocMeta{
		Path:   d.Path,
		Type:   d.Type,
		Title:  d.Title,
		Mtime:  d.Mtime.Format(time.RFC3339),
		SHA256: d.SHA256,
		Tags:   d.Tags,
	}
}

func metasFromRows(rows []index.DocRow) []DocMeta {
	out := make([]DocMeta, 0, len(rows))
	for _, d := range rows {
		out = append(out, metaFromRow(d))
	}
	return out
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func taskFromRow(t index.TaskRow) Task {
	return Task{
		ID:          t.ID,
		DocPath:     t.DocPath,
		DocTitle:    t.DocTitle,
		Line:        t.Line,
		Text:        t.Text,
		Done:        t.Done,
		Due:         optStr(t.Due),
		Defer:       optStr(t.Defer),
		Priority:    t.Priority,
		Waiting:     t.Waiting,
		Recur:       optStr(t.Recur),
		Project:     optStr(t.ProjectPath),
		Tags:        t.Tags,
		CompletedOn: optStr(t.CompletedOn),
	}
}

func tasksFromRows(rows []index.TaskRow) []Task {
	out := make([]Task, 0, len(rows))
	for _, t := range rows {
		out = append(out, taskFromRow(t))
	}
	return out
}

// ---- documents ----

// ListDocuments lists document metadata, optionally filtered by type and a
// title substring.
func (s *Service) ListDocuments(docType, q string, limit int) ([]DocMeta, error) {
	rows, err := s.Index.ListDocuments(docType, q, limit)
	if err != nil {
		return nil, err
	}
	return metasFromRows(rows), nil
}

// GetDocument returns the full document at path: raw markdown plus parsed
// structure so clients (and agents) never re-parse markdown themselves.
func (s *Service) GetDocument(path string) (Document, error) {
	f, err := s.Vault.Read(path)
	if err != nil {
		return Document{}, err
	}
	return s.buildDocument(f)
}

func (s *Service) buildDocument(f vault.File) (Document, error) {
	scanned := markdown.Scan(f.Path, f.Raw)
	fm := vault.ParseFrontmatter(f.Raw)
	if fm == nil {
		fm = map[string]any{}
	}

	meta := DocMeta{Path: f.Path, SHA256: f.SHA256, Mtime: f.ModTime.Format(time.RFC3339)}
	// Prefer the indexed row for type/title/tags (it applies the same
	// inference rules); fall back to a direct scan for not-yet-indexed files.
	if row, err := s.Index.GetDocMeta(f.Path); err == nil {
		meta.Type = row.Type
		meta.Title = row.Title
		meta.Tags = row.Tags
	} else {
		meta.Type = string(vault.InferType(f.Path))
		meta.Title = scanned.Title
		meta.Tags = scanned.Tags
	}

	links := make([]Link, 0, len(scanned.Links))
	for _, l := range scanned.Links {
		links = append(links, Link{Target: optStr(s.Index.ResolveLink(l.Raw)), Raw: l.Raw, Display: l.Display})
	}
	backRows, err := s.Index.Backlinks(f.Path)
	if err != nil {
		return Document{}, err
	}

	tasks := make([]Task, 0, len(scanned.Tasks))
	for _, t := range scanned.Tasks {
		row, err := s.Index.TaskByID(t.ID)
		if err != nil {
			// Not yet indexed (fresh write); shape it directly.
			tasks = append(tasks, Task{
				ID: t.ID, DocPath: f.Path, Line: t.Line, Text: t.Text, Done: t.Done,
				Due: optStr(t.Due), Defer: optStr(t.Defer), Priority: t.Priority,
				Waiting: t.Waiting, Tags: t.Tags, CompletedOn: optStr(t.CompletedOn),
			})
			continue
		}
		tasks = append(tasks, taskFromRow(row))
	}

	return Document{
		DocMeta:     meta,
		Markdown:    string(f.Raw),
		Frontmatter: fm,
		Links:       links,
		Backlinks:   metasFromRows(backRows),
		Tasks:       tasks,
	}, nil
}

// UpdateDocument replaces a document's content, guarded by the caller's base
// hash (vault.ErrConflict when stale). The index is updated synchronously so
// the response reflects the write.
func (s *Service) UpdateDocument(path, content, baseSHA string) (Document, error) {
	f, err := s.Vault.Write(path, []byte(content), baseSHA)
	if err != nil {
		return Document{}, err
	}
	if _, err := s.Index.IndexFile(path); err != nil {
		return Document{}, fmt.Errorf("indexing after write: %w", err)
	}
	return s.buildDocument(f)
}

// CreateDocument creates a new typed document, picking its path and seeding
// type-appropriate frontmatter. Duplicate titles get a numeric suffix.
func (s *Service) CreateDocument(docType vault.DocType, title, body string) (Document, error) {
	if title == "" {
		return Document{}, fmt.Errorf("title is required")
	}
	path := vault.NewDocPath(docType, title, s.Now())
	for n := 2; s.Vault.Exists(path); n++ {
		base := strings.TrimSuffix(path, ".md")
		path = fmt.Sprintf("%s-%d.md", base, n)
		if n > 100 {
			return Document{}, fmt.Errorf("could not find a free path for %q", title)
		}
	}

	if body == "" {
		body = "# " + title + "\n\n"
	}
	content := s.seedFrontmatter(docType, body)

	f, err := s.Vault.Write(path, content, "")
	if err != nil {
		return Document{}, err
	}
	if _, err := s.Index.IndexFile(path); err != nil {
		return Document{}, fmt.Errorf("indexing new document: %w", err)
	}
	return s.buildDocument(f)
}

// seedFrontmatter gives new entity documents their minimal starting block.
// Notes get none — a plain file stays a plain file.
func (s *Service) seedFrontmatter(docType vault.DocType, body string) []byte {
	switch docType {
	case vault.TypeProject:
		return vault.BuildDoc([][2]string{{"status", "active"}}, body)
	case vault.TypeMeeting:
		return vault.BuildDoc([][2]string{{"date", s.Now().Format("2006-01-02T15:04")}, {"people", "[]"}}, body)
	default:
		return []byte(body)
	}
}

// AppendToDocument adds markdown to the end of a document, or to the end of
// a named section when given — the safe write for agents and quick capture:
// existing content is never rewritten, only added to.
func (s *Service) AppendToDocument(path, addition, section string) (Document, error) {
	f, err := s.Vault.Read(path)
	if err != nil {
		return Document{}, err
	}
	content := string(f.Raw)
	addition = strings.TrimRight(addition, "\n") + "\n"

	if section == "" {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return s.UpdateDocument(path, content+addition, f.SHA256)
	}

	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		heading := strings.TrimLeft(line, "# ")
		if strings.HasPrefix(line, "#") && strings.EqualFold(strings.TrimSpace(heading), strings.TrimSpace(section)) {
			start = i
			break
		}
	}
	if start < 0 {
		return Document{}, fmt.Errorf("section %q not found in %s", section, path)
	}
	// The section ends at the next heading of any level (or EOF).
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "#") {
			end = i
			break
		}
	}
	// Insert before trailing blank lines so the appended text sits inside
	// the section, not after its separating whitespace.
	insert := end
	for insert > start+1 && strings.TrimSpace(lines[insert-1]) == "" {
		insert--
	}
	updated := append(append(append([]string{}, lines[:insert]...), strings.Split(strings.TrimRight(addition, "\n"), "\n")...), lines[insert:]...)
	return s.UpdateDocument(path, strings.Join(updated, "\n"), f.SHA256)
}

// TasksFromRows converts index rows to the API shape (exported for the MCP
// layer's rollup tools).
func TasksFromRows(rows []index.TaskRow) []Task { return tasksFromRows(rows) }

// DeleteDocument removes a document and its index rows.
func (s *Service) DeleteDocument(path string) error {
	if err := s.Vault.Delete(path); err != nil {
		return err
	}
	return s.Index.Remove(path)
}

// Search runs the shared query grammar.
func (s *Service) Search(q string, limit int) ([]SearchResult, error) {
	hits, err := s.Index.Search(q, limit, s.today())
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, SearchResult{Path: h.Path, Type: h.Type, Title: h.Title, Snippet: h.Snippet})
	}
	return out, nil
}

// ---- daily & today ----

// GetDaily returns the daily note for date (vault.ErrNotFound if absent).
func (s *Service) GetDaily(date string) (Document, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return Document{}, fmt.Errorf("invalid date %q", date)
	}
	return s.GetDocument("daily/" + date + ".md")
}

// EnsureDaily returns the daily note for date, creating it from the template
// if missing.
func (s *Service) EnsureDaily(date string) (Document, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return Document{}, fmt.Errorf("invalid date %q", date)
	}
	path := "daily/" + date + ".md"
	if s.Vault.Exists(path) {
		return s.GetDocument(path)
	}
	f, err := s.Vault.Write(path, []byte("# "+date+"\n\n"), "")
	if err != nil {
		return Document{}, err
	}
	if _, err := s.Index.IndexFile(path); err != nil {
		return Document{}, err
	}
	return s.buildDocument(f)
}

// Today composes the home-screen payload: meetings, task buckets, recent
// documents, and the daily note if it exists (nil, not auto-created — files
// appear when the user writes, DESIGN.md decision 9).
func (s *Service) Today() (TodayPayload, error) {
	day := s.today()
	payload := TodayPayload{Date: day}

	if doc, err := s.GetDaily(day); err == nil {
		payload.Daily = &doc
	}

	meetings, err := s.Index.MeetingsOn(day)
	if err != nil {
		return payload, err
	}
	payload.Meetings = metasFromRows(meetings)

	dueRows, err := s.Index.OpenTasksDue(day)
	if err != nil {
		return payload, err
	}
	payload.Overdue, payload.DueToday = []Task{}, []Task{}
	for _, t := range dueRows {
		if t.Due < day {
			payload.Overdue = append(payload.Overdue, taskFromRow(t))
		} else {
			payload.DueToday = append(payload.DueToday, taskFromRow(t))
		}
	}

	todayRows, err := s.Index.Tasks(index.ViewToday, day)
	if err != nil {
		return payload, err
	}
	payload.Available = []Task{}
	for _, t := range todayRows {
		// The today view includes due tasks; Available is only the
		// deferred-to-now remainder so sections don't repeat rows.
		if t.Due == "" {
			payload.Available = append(payload.Available, taskFromRow(t))
		}
	}

	waitingRows, err := s.Index.Tasks(index.ViewWaiting, day)
	if err != nil {
		return payload, err
	}
	payload.Waiting = tasksFromRows(waitingRows)

	payload.Birthdays, err = s.upcomingBirthdays(7)
	if err != nil {
		return payload, err
	}

	recent, err := s.Index.ListDocuments("", "", 8)
	if err != nil {
		return payload, err
	}
	payload.Recent = metasFromRows(recent)

	return payload, nil
}

// upcomingBirthdays scans person frontmatter for birthdays falling within
// the next `days` days. Accepts YYYY-MM-DD (age computable) or MM-DD.
func (s *Service) upcomingBirthdays(days int) ([]Birthday, error) {
	people, err := s.Index.PeopleWithBirthdays()
	if err != nil {
		return nil, err
	}
	now := s.Now()
	out := []Birthday{}
	for _, p := range people {
		var fm struct {
			Birthday any `json:"birthday"`
		}
		if err := json.Unmarshal(p.Frontmatter, &fm); err != nil {
			continue
		}
		raw, _ := fm.Birthday.(string)
		month, day, birthYear, ok := parseBirthday(raw)
		if !ok {
			continue
		}
		next := time.Date(now.Year(), month, day, 0, 0, 0, 0, now.Location())
		if next.Before(now.Truncate(24 * time.Hour)) {
			next = next.AddDate(1, 0, 0)
		}
		until := int(next.Sub(now.Truncate(24*time.Hour)).Hours() / 24)
		if until > days {
			continue
		}
		b := Birthday{Path: p.Path, Title: p.Title, Date: next.Format("2006-01-02"), DaysUntil: until}
		if birthYear > 0 {
			age := next.Year() - birthYear
			b.Age = &age
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DaysUntil < out[j].DaysUntil })
	return out, nil
}

// parseBirthday accepts "YYYY-MM-DD" or "MM-DD".
func parseBirthday(raw string) (time.Month, int, int, bool) {
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.Month(), t.Day(), t.Year(), true
	}
	if t, err := time.Parse("01-02", raw); err == nil {
		return t.Month(), t.Day(), 0, true
	}
	return 0, 0, 0, false
}
