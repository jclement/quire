// Package service is the one business-logic layer under every transport:
// REST handlers and MCP tools both call these methods and nothing else, so
// permissions and behavior cannot drift between them (DESIGN.md decision 5).
// The wire shapes those methods return live in apitypes.go, which
// generates the frontend's TypeScript types.
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/markdown"
	"github.com/jclement/quire/internal/semantic"
	"github.com/jclement/quire/internal/settings"
	"github.com/jclement/quire/internal/vault"
)

// ErrValidation marks a caller mistake — a missing field, a date that
// cannot be resolved — as distinct from a server fault, so the HTTP layer
// can answer 400 instead of 500. Without it, `{"due":"someday"}` looked to
// the client like an internal error.
var ErrValidation = errors.New("validation")

// Service wires the vault and index together.
type Service struct {
	Vault *vault.Vault
	Index *index.Index
	// Settings holds the owner's app-level configuration (areas and their
	// colours). Nil means defaults, which is what tests get.
	Settings *settings.Store
	// Semantic is the embedding pipeline; nil unless an API key is set.
	Semantic *semantic.Embedder
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
		Path:     d.Path,
		Type:     d.Type,
		Title:    d.Title,
		Mtime:    d.Mtime.Format(time.RFC3339),
		SHA256:   d.SHA256,
		Tags:     d.Tags,
		Area:     d.Area,
		AreaFrom: d.AreaFrom,
	}
}

// definedAreas is the Settings list; none when there is no store.
func (s *Service) definedAreas() ([]settings.AreaDef, error) {
	if s.Settings == nil {
		return nil, nil
	}
	cfg, err := s.Settings.Load()
	if err != nil {
		return nil, err
	}
	return cfg.Areas, nil
}

// Areas returns the defined areas in their configured order with colours
// and counts, followed by any area found only in frontmatter (neutral, so a
// document filed under a typo'd area still has somewhere to be found).
func (s *Service) Areas() ([]AreaCount, error) {
	defined, err := s.definedAreas()
	if err != nil {
		return nil, err
	}
	rows, err := s.Index.Areas()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Area] = r.Count
	}
	out := make([]AreaCount, 0, len(defined)+len(rows))
	seen := map[string]bool{}
	for _, d := range defined {
		seen[d.Name] = true
		out = append(out, AreaCount{Area: d.Name, Count: counts[d.Name], Color: d.Color, Defined: true})
	}
	for _, r := range rows {
		if !seen[r.Area] {
			out = append(out, AreaCount{Area: r.Area, Count: r.Count, Color: "slate"})
		}
	}
	return out, nil
}

// SetAreas replaces the defined areas. Renaming here does not rewrite
// documents — the frontmatter is the truth, and a document filed under the
// old name simply becomes a discovered area until it is re-filed.
func (s *Service) SetAreas(areas []AreaDef) error {
	if s.Settings == nil {
		return fmt.Errorf("settings are not available on this instance")
	}
	defs := make([]settings.AreaDef, 0, len(areas))
	for _, a := range areas {
		defs = append(defs, settings.AreaDef{Name: a.Name, Color: a.Color})
	}
	if err := settings.ValidateAreas(defs); err != nil {
		return fmt.Errorf("%w: %s", ErrValidation, err)
	}
	cfg, err := s.Settings.Load()
	if err != nil {
		return err
	}
	cfg.Areas = defs
	return s.Settings.Save(cfg)
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
// ListDocuments lists by type and title substring within an area ("" = all,
// "none" = unclassified), most recently modified first.
func (s *Service) ListDocuments(docType, q, area string, limit int) ([]DocMeta, error) {
	rows, err := s.Index.ListDocuments(docType, q, area, limit)
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
	// The area comes from the file, exactly as the indexer reads it, so a
	// document returned straight after a write agrees with the index.
	if area, ok := fm["area"].(string); ok && vault.InferType(f.Path) != vault.TypeDaily {
		meta.Area = index.NormalizeArea(area)
	}
	// Prefer the indexed row for type/title/tags (it applies the same
	// inference rules); fall back to a direct scan for not-yet-indexed files.
	if row, err := s.Index.GetDocMeta(f.Path); err == nil {
		meta.Type = row.Type
		meta.Title = row.Title
		meta.Tags = row.Tags
		// The effective area (explicit or inherited) is the index's answer;
		// the frontmatter value above only stands in for an unindexed file.
		meta.Area = row.Area
		meta.AreaFrom = row.AreaFrom
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
	return s.CreateDocumentIn(docType, title, body, "")
}

// CreateDocumentIn creates a document filed under an area ("" for none).
func (s *Service) CreateDocumentIn(docType vault.DocType, title, body, area string) (Document, error) {
	return s.CreateDocumentWith(docType, title, body, CreateOptions{Area: area})
}

// CreateOptions are the optional parts of creating a document.
type CreateOptions struct {
	// Area files the document (frontmatter area:). "" or "none" for none.
	Area string
	// Template names a template (path or bare name) to start from. When
	// empty and Body is empty, the type's default template applies if one
	// exists. Ignored when a body is supplied — the caller knows better.
	Template string
}

// CreateDocumentWith creates a document from a body, a template, or the
// type's default template, in that order of precedence.
func (s *Service) CreateDocumentWith(docType vault.DocType, title, body string, opts CreateOptions) (Document, error) {
	var templateSeed [][2]string
	if body == "" {
		if path, ok := s.resolveTemplate(docType, opts.Template); ok {
			seed, rendered, err := s.renderTemplate(path, title, s.Now())
			if err != nil {
				return Document{}, err
			}
			templateSeed, body = seed, rendered
		} else if opts.Template != "" {
			return Document{}, fmt.Errorf("%w: no template named %q", ErrValidation, opts.Template)
		}
	}
	area := opts.Area
	_ = templateSeed // applied below once the path is known
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
	content := s.seedFrontmatter(docType, body, area)
	for _, kv := range templateSeed {
		content = vault.SetFrontmatterKey(content, kv[0], kv[1])
	}

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
//
// When the caller supplied their own frontmatter (agents do this constantly,
// since create_document invites passing markdown), that block is authoritative:
// we only fill in seed keys it doesn't already set. Prepending a second block
// produced a file with two `---` fences, the second of which rendered as body
// text.
func (s *Service) seedFrontmatter(docType vault.DocType, body, area string) []byte {
	var seed [][2]string
	switch docType {
	case vault.TypeProject:
		seed = [][2]string{{"status", "active"}}
	case vault.TypeMeeting:
		seed = [][2]string{{"date", s.Now().Format("2006-01-02T15:04")}, {"people", "[]"}}
	default:
		seed = nil
	}
	// The area a document is created in is the one it files under; daily
	// notes never carry one.
	if area = index.NormalizeArea(area); area != "" && area != index.AreaUnclassified && docType != vault.TypeDaily {
		seed = append(seed, [2]string{"area", area})
	}
	if seed == nil {
		return []byte(body)
	}

	raw := []byte(body)
	if _, _, hasFrontmatter := vault.SplitFrontmatter(raw); !hasFrontmatter {
		return vault.BuildDoc(seed, body)
	}
	existing := vault.ParseFrontmatter(raw)
	for _, kv := range seed {
		if _, present := existing[kv[0]]; !present {
			raw = vault.SetFrontmatterKey(raw, kv[0], kv[1])
		}
	}
	return raw
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
	// templates/daily.md, if present, shapes every new day.
	content := []byte("# " + date + "\n\n")
	if tpl, ok := s.resolveTemplate(vault.TypeDaily, ""); ok {
		// Placeholders describe the note's day, at this instant's time.
		day, _ := time.ParseInLocation("2006-01-02", date, s.Now().Location())
		at := time.Date(day.Year(), day.Month(), day.Day(), s.Now().Hour(), s.Now().Minute(), 0, 0, s.Now().Location())
		if seed, body, err := s.renderTemplate(tpl, date, at); err == nil {
			content = []byte(body)
			for _, kv := range seed {
				content = vault.SetFrontmatterKey(content, kv[0], kv[1])
			}
		}
	}
	f, err := s.Vault.Write(path, content, "")
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
// Today composes the "what matters now" payload. An area narrows the tasks,
// meetings and recent documents; the daily note and birthdays are shared.
func (s *Service) Today() (TodayPayload, error) { return s.TodayIn("") }

func (s *Service) TodayIn(area string) (TodayPayload, error) {
	day := s.today()
	payload := TodayPayload{Date: day}

	if doc, err := s.GetDaily(day); err == nil {
		payload.Daily = &doc
	}

	meetings, err := s.Index.MeetingsOn(day, area)
	if err != nil {
		return payload, err
	}
	payload.Meetings = metasFromRows(meetings)

	dueRows, err := s.Index.OpenTasksDue(day, area)
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

	todayRows, err := s.Index.Tasks(index.ViewToday, day, area)
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

	waitingRows, err := s.Index.Tasks(index.ViewWaiting, day, area)
	if err != nil {
		return payload, err
	}
	payload.Waiting = tasksFromRows(waitingRows)

	payload.Birthdays, err = s.upcomingBirthdays(7)
	if err != nil {
		return payload, err
	}

	recent, err := s.Index.ListDocuments("", "", area, 8) // "" excludes templates
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

// Tags lists every tag with its document count, most-used first.
func (s *Service) Tags() ([]TagCount, error) {
	rows, err := s.Index.Tags()
	if err != nil {
		return nil, err
	}
	out := make([]TagCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, TagCount{Tag: r.Tag, Count: r.Count})
	}
	return out, nil
}

// DailyNotesBefore is the journal's page of history: existing daily notes
// dated before `date`, newest first, as full documents so they render
// without a second round trip each.
func (s *Service) DailyNotesBefore(date string, limit int) ([]Document, error) {
	rows, err := s.Index.DailyNotesBefore(date, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Document, 0, len(rows))
	for _, r := range rows {
		doc, err := s.GetDocument(r.Path)
		if err != nil {
			continue // deleted between index and read; the journal just skips it
		}
		out = append(out, doc)
	}
	return out, nil
}
