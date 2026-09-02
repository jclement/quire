// Package mcp exposes quire to AI agents over the Model Context Protocol
// (Streamable HTTP at /mcp). Every tool is a thin wrapper over the same
// service layer the REST API uses, so an agent can never do anything the API
// couldn't (DESIGN.md decision 5). No delete tool exists on purpose.
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

// Handler returns the Streamable HTTP handler for /mcp. A server is built
// per session rather than once at startup so that guidance the owner edits
// in Settings reaches the next client that connects, with no restart.
// Auditor records what a tool did. Nil disables auditing (tests).
type Auditor interface {
	RecordAudit(rec auth.AuditRecord) error
}

func Handler(svc *service.Service, version string, audit Auditor) http.Handler {
	return sdk.NewStreamableHTTPHandler(
		func(r *http.Request) *sdk.Server {
			principal, _ := auth.PrincipalFrom(r)
			return newServer(svc, version, scopesFor(r), principal.Name, audit)
		}, nil)
}

// scopesFor returns what the caller of r is allowed to do. A request that
// never passed the auth middleware carries no principal; that is a
// programming error rather than an anonymous caller (the middleware rejects
// those), so it yields no scopes at all — fail closed, never fail owner.
func scopesFor(r *http.Request) func(scope string) bool {
	principal, ok := auth.PrincipalFrom(r)
	if !ok {
		return func(string) bool { return false }
	}
	return principal.Allows
}

// baseInstructions are the cross-cutting rules every agent gets. The owner's
// own guidance (AGENTS.md in the vault, edited in Settings) is appended.
const baseInstructions = `quire is a personal knowledge and work vault: markdown files on disk with
first-class people, companies, projects, meetings, daily notes and tasks.

Working rules:
- Prefer append_to_document over update_document. update_document replaces the
  whole file, so it must carry the base_sha256 from a get_document you just
  made; a stale hash is rejected rather than clobbering concurrent edits.
- Relationships are wikilinks: [[Sarah Chen]] in prose, or frontmatter keys
  (company, people, project). Both are indexed, so either creates a backlink.
- Tasks are markdown checkboxes with an emoji grammar: 📅 due, 🛫 defer,
  ⏫/🔼/🔽 priority, ⏳ waiting, 🔁 recurrence, ✅ completion. Toggle them with
  complete_task rather than rewriting the line.
- Start with today for "what should I work on", and person_context before a
  meeting — each answers in one call what would otherwise take several.
- Never invent a document path; find it with search first.`

func newServer(svc *service.Service, version string, allows func(string) bool, principal string, audit Auditor) *sdk.Server {
	instructions := baseInstructions
	if guidance := svc.AgentGuidance(); guidance != "" {
		instructions += "\n\n---\n\nThe vault owner's own guidance (authoritative where it conflicts\nwith the above):\n\n" + guidance
	}
	s := sdk.NewServer(&sdk.Implementation{Name: "quire", Version: version},
		&sdk.ServerOptions{Instructions: instructions})
	t := &tools{svc: svc, principal: principal, audit: audit}

	// Tools are registered per request against the caller's scopes, so a
	// token sees exactly the tools it may use: tools/list is honest, and an
	// agent is never handed a tool that will refuse it mid-task. Before
	// this, passing the /mcp gate granted every tool — a token scoped only
	// to "tasks" could create and rewrite documents, which REST refused.

	// Reading the vault.
	if allows(auth.ScopeRead) {
		sdk.AddTool(s, &sdk.Tool{Name: "search", Annotations: readOnly,
			Description: "Search the vault. Bare words are full-text (title and body, ranked); filters combine with them: type:<note|person|company|project|meeting|daily>, tag:<tag>, area:<work|personal|none>, is:task (search tasks instead of documents), due:today | due:overdue | due:week | due:YYYY-MM-DD. Returns paths — use get_document for content. Always search before guessing a path."},
			t.search)
		if svc.SemanticEnabled() {
			sdk.AddTool(s, &sdk.Tool{Name: "semantic_search", Annotations: readOnly,
				Description: "Search by meaning rather than exact words: the query and every note are compared as embeddings, so 'what did we decide about pricing' finds the note titled 'Rate card discussion'. Use it when the owner's wording is uncertain or the question is conceptual; use search for names, tags, exact phrases, task filters and dates (it is exact and free). Returns paths with a similarity score and the best-matching section heading — use get_document for content."},
				t.semanticSearch)
			sdk.AddTool(s, &sdk.Tool{Name: "related_documents", Annotations: readOnly,
				Description: "Documents closest in meaning to a given one, from stored embeddings (no external call). Good for 'what else touches this project' or surfacing an older note that covers the same ground before creating a duplicate."},
				t.relatedDocuments)
		}
		sdk.AddTool(s, &sdk.Tool{Name: "list_documents", Annotations: readOnly,
			Description: "Browse documents by type, most recently modified first, optionally filtered by a title substring. Use this to see what exists (all people, recent meetings, every project) when you have no search term; use search when you do."},
			t.listDocuments)
		sdk.AddTool(s, &sdk.Tool{Name: "get_document", Annotations: readOnly,
			Description: "Fetch a document by vault path: raw markdown plus parsed structure (frontmatter, links with resolution, backlinks, tasks with ids) and the sha256 that update_document needs. Use search or list_documents to find paths."},
			t.getDocument)
		sdk.AddTool(s, &sdk.Tool{Name: "get_daily", Annotations: readOnly,
			Description: "Today's daily note (or a given date's). The daily note is the capture spine: quick thoughts and captured tasks land here. Returns not-found rather than creating one; create_task or append_to_document will create it on write."},
			t.getDaily)
		sdk.AddTool(s, &sdk.Tool{Name: "list_tasks", Annotations: readOnly,
			Description: "List tasks by view: inbox (no date, unprocessed), today (due, overdue, or available now), upcoming (deferred or due later), waiting (delegated ⏳), logbook (completed). Each task carries the id complete_task and edit_task take."},
			t.listTasks)
		sdk.AddTool(s, &sdk.Tool{Name: "list_areas", Annotations: readOnly,
			Description: "The areas documents are filed under (work, personal, and any the owner has added) with counts. Areas partition everything except daily notes; pass one to search, list_documents, list_tasks or today to narrow to it, and to create_document to file under it."},
			t.listAreas)
		sdk.AddTool(s, &sdk.Tool{Name: "list_tags", Annotations: readOnly,
			Description: "Every tag in the vault with how many documents carry it, most-used first. Use it to pick an existing tag rather than inventing a near-duplicate."},
			t.listTags)
		sdk.AddTool(s, &sdk.Tool{Name: "today", Annotations: readOnly,
			Description: "The composed 'what matters right now' payload: today's meetings, overdue and due tasks, available and waiting tasks, birthdays, recent documents, and the daily note. Start here for 'what should I work on' — it answers in one call what would take six."},
			t.today)
		sdk.AddTool(s, &sdk.Tool{Name: "person_context", Annotations: readOnly,
			Description: "Everything about a person, project or company in one call: the document, its backlinks (every meeting and note that mentions it), and open tasks involving it. The right first call before a meeting or a 1:1. Accepts a name or a vault path."},
			t.personContext)
	}

	// Creating and rewriting documents.
	if allows(auth.ScopeWrite) {
		sdk.AddTool(s, &sdk.Tool{Name: "create_document", Annotations: additive,
			Description: "Create a new document of a given type (note, person, company, project, meeting). The server picks the path from the title and seeds type-appropriate frontmatter. With no body, the type's default template (templates/<type>.md) applies if one exists, or name one with template; list_documents type=template shows what is available. Body is optional markdown; wikilinks like [[Sarah Chen]] create relationships. Search first — a duplicate title gets a numeric suffix, not an error."},
			t.createDocument)
		sdk.AddTool(s, &sdk.Tool{Name: "update_document", Annotations: destructive,
			Description: "Replace a document's full markdown. Requires base_sha256 from a get_document made just before; a stale hash is rejected so you can never clobber a concurrent edit — on that error, re-read and reapply. Prefer append_to_document for additions and set_frontmatter/link_entity for metadata; use this only for real rewrites."},
			t.updateDocument)
		sdk.AddTool(s, &sdk.Tool{Name: "append_to_document", Annotations: additive,
			Description: "Append markdown to the end of a document, or to the end of a named section (heading text). The safe way to add notes, decisions or action items without touching anything else. Creates the daily note if the path is today's and it does not exist."},
			t.appendToDocument)
		sdk.AddTool(s, &sdk.Tool{Name: "link_entity", Annotations: additive,
			Description: "Relate one document to another through frontmatter — company on a person, people or project on a meeting — without rewriting the body. key is the frontmatter field (company, people, project, attendees); target is a document name or path. Idempotent."},
			t.linkEntity)
		sdk.AddTool(s, &sdk.Tool{Name: "set_frontmatter", Annotations: destructive,
			Description: "Set frontmatter fields on a document (status on a project, role or email on a person, tags on anything) surgically, leaving the body untouched. Pass a JSON object of key → value; a null value removes the key."},
			t.setFrontmatter)
	}

	// Task management — the "agent runs my todos" token stops here.
	if allows(auth.ScopeTasks) {
		sdk.AddTool(s, &sdk.Tool{Name: "create_task", Annotations: additive,
			Description: "Create a task. It lands in today's daily note with full provenance. due = deadline, defer = hide until this date; both accept YYYY-MM-DD or a natural form (today, tomorrow, fri, +3d) and are resolved server-side. An unparseable date is an error, never a guess. The text may carry #tags and [[wikilinks]] to relate it to people or projects."},
			t.createTask)
		sdk.AddTool(s, &sdk.Tool{Name: "complete_task", Annotations: idempotent,
			Description: "Mark a task complete by id (from list_tasks, today, or get_document). Edits the source checkbox surgically; a recurring task mints its next occurrence. Completing an already-complete task is an error, not a reopen."},
			t.completeTask)
		sdk.AddTool(s, &sdk.Tool{Name: "edit_task", Annotations: idempotent,
			Description: "Reschedule or reprioritise a task by id: set due and/or defer (natural dates accepted; empty string clears), and priority (0 none, 1 high, 2 medium, 3 low). Only the fields you pass change. This is how to snooze."},
			t.editTask)
	}

	return s
}

// Annotation presets. Clients use these to decide what needs confirmation:
// a read-only tool can run freely, an additive one is safe to retry, a
// destructive one (full-document replace) deserves a look first.
var (
	readOnly    = &sdk.ToolAnnotations{ReadOnlyHint: true}
	additive    = &sdk.ToolAnnotations{DestructiveHint: boolPtr(false)}
	idempotent  = &sdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true}
	destructive = &sdk.ToolAnnotations{DestructiveHint: boolPtr(true)}
)

func boolPtr(b bool) *bool { return &b }

type tools struct {
	svc       *service.Service
	principal string
	audit     Auditor
}

// record writes an audit row for a mutating tool. Never fails the call.
func (t *tools) record(tool, path, detail string, err error) {
	if t.audit == nil || t.principal == "" || t.principal == "owner" {
		return
	}
	if len(detail) > 120 {
		detail = detail[:120] + "…"
	}
	if aerr := t.audit.RecordAudit(auth.AuditRecord{
		Principal: t.principal, Action: "mcp:" + tool, Path: path, Detail: detail, OK: err == nil,
	}); aerr != nil {
		slog.Warn("audit", "tool", tool, "err", aerr)
	}
}

// ---- inputs ----

type searchIn struct {
	Query string `json:"query" jsonschema:"the search query, e.g. 'type:meeting acme' or 'reporting decisions'"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results (default 20)"`
}

type pathIn struct {
	Path string `json:"path" jsonschema:"vault-relative document path, e.g. people/sarah-chen.md"`
}

type createDocIn struct {
	Type     string `json:"type" jsonschema:"one of: note, person, company, project, meeting"`
	Title    string `json:"title" jsonschema:"document title, e.g. 'Sarah Chen'"`
	Body     string `json:"body,omitempty" jsonschema:"optional markdown body; defaults to a heading"`
	Area     string `json:"area,omitempty" jsonschema:"area to file it under (work, personal, …); omit for unclassified"`
	Template string `json:"template,omitempty" jsonschema:"template name to start from (see list_documents type=template); omit to use the type's default template if one exists"`
}

type updateDocIn struct {
	Path       string `json:"path" jsonschema:"vault-relative document path"`
	Markdown   string `json:"markdown" jsonschema:"the complete new markdown content"`
	BaseSHA256 string `json:"base_sha256" jsonschema:"sha256 from the get_document you based this edit on"`
}

type appendIn struct {
	Path     string `json:"path" jsonschema:"vault-relative document path"`
	Markdown string `json:"markdown" jsonschema:"markdown to append"`
	Section  string `json:"section,omitempty" jsonschema:"optional heading text — append at the end of this section instead of the file end"`
}

type listTasksIn struct {
	View string `json:"view" jsonschema:"one of: inbox, today, upcoming, waiting, logbook"`
	Area string `json:"area,omitempty" jsonschema:"area to narrow to (e.g. work, personal), or none for unclassified; comma-separate several (work,personal); omit for all"`
}

type areasOut struct {
	Areas []service.AreaCount `json:"areas"`
}

type createTaskIn struct {
	Text  string `json:"text" jsonschema:"the task text; may include #tags and [[wikilinks]]"`
	Due   string `json:"due,omitempty" jsonschema:"due date: YYYY-MM-DD or today, tomorrow, fri, +3d"`
	Defer string `json:"defer,omitempty" jsonschema:"defer/start date (hidden until then): YYYY-MM-DD or a natural form"`
}

type editTaskIn struct {
	ID       string  `json:"id" jsonschema:"task id from list_tasks, today, or get_document"`
	Due      *string `json:"due,omitempty" jsonschema:"new due date (YYYY-MM-DD or natural); empty string clears; omit to leave unchanged"`
	Defer    *string `json:"defer,omitempty" jsonschema:"new defer date; empty string clears; omit to leave unchanged"`
	Priority *int    `json:"priority,omitempty" jsonschema:"0 none, 1 high, 2 medium, 3 low; omit to leave unchanged"`
}

// areaDoc is the shared description of the area parameter.
const areaDoc = "area to narrow to (e.g. work, personal), or none for unclassified; omit for all"

type areaIn struct {
	Area string `json:"area,omitempty" jsonschema:"area to narrow to (e.g. work, personal), or none for unclassified; comma-separate several (work,personal); omit for all"`
}

type listDocsIn struct {
	Area  string `json:"area,omitempty" jsonschema:"area to narrow to (e.g. work, personal), or none for unclassified; comma-separate several (work,personal); omit for all"`
	Type  string `json:"type,omitempty" jsonschema:"filter by type: note, person, company, project, meeting, daily, template; omit for all (templates are only listed when asked for by type)"`
	Title string `json:"title,omitempty" jsonschema:"optional title substring, case-insensitive"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results (default 50)"`
}

type getDailyIn struct {
	Date string `json:"date,omitempty" jsonschema:"YYYY-MM-DD; omit for today"`
}

type linkEntityIn struct {
	Path   string `json:"path" jsonschema:"vault-relative path of the document to add the relationship to"`
	Key    string `json:"key" jsonschema:"frontmatter key: company, people, project, attendees, or any list/single field"`
	Target string `json:"target" jsonschema:"the related document, by name (Sarah Chen) or path (people/sarah-chen.md)"`
}

type setFrontmatterIn struct {
	Path   string         `json:"path" jsonschema:"vault-relative document path"`
	Values map[string]any `json:"values" jsonschema:"fields to set; a null value removes that key"`
}

type docListOut struct {
	Documents []service.DocMeta `json:"documents"`
}

type tagsOut struct {
	Tags []service.TagCount `json:"tags"`
}

type taskIDIn struct {
	ID string `json:"id" jsonschema:"task id from list_tasks or get_document"`
}

type personContextIn struct {
	Name string `json:"name" jsonschema:"person/project/company name or vault path, e.g. 'Sarah Chen'"`
}

// ---- outputs ----

type taskListOut struct {
	Tasks []service.Task `json:"tasks"`
}

type searchOut struct {
	Results []service.SearchResult `json:"results"`
}

type personContextOut struct {
	Document  service.Document `json:"document"`
	OpenTasks []service.Task   `json:"open_tasks"`
}

// ---- handlers ----

func (t *tools) search(_ context.Context, _ *sdk.CallToolRequest, in searchIn) (*sdk.CallToolResult, searchOut, error) {
	hits, err := t.svc.Search(in.Query, in.Limit)
	if err != nil {
		return nil, searchOut{}, err
	}
	return nil, searchOut{Results: hits}, nil
}

type semanticIn struct {
	Query string `json:"query" jsonschema:"what you are looking for, in plain language"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results (default 20)"`
	Area  string `json:"area,omitempty" jsonschema:"restrict to an area (see list_areas); 'none' for unclassified; comma-separate several"`
}

type relatedIn struct {
	Path  string `json:"path" jsonschema:"vault path of the document"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results (default 5)"`
}

func (t *tools) semanticSearch(ctx context.Context, _ *sdk.CallToolRequest, in semanticIn) (*sdk.CallToolResult, searchOut, error) {
	hits, err := t.svc.SemanticSearch(ctx, in.Query, in.Limit, in.Area)
	if err != nil {
		return nil, searchOut{}, err
	}
	return nil, searchOut{Results: hits}, nil
}

func (t *tools) relatedDocuments(_ context.Context, _ *sdk.CallToolRequest, in relatedIn) (*sdk.CallToolResult, searchOut, error) {
	hits, err := t.svc.RelatedDocuments(in.Path, in.Limit)
	if err != nil {
		return nil, searchOut{}, err
	}
	return nil, searchOut{Results: hits}, nil
}

func (t *tools) getDocument(_ context.Context, _ *sdk.CallToolRequest, in pathIn) (*sdk.CallToolResult, service.Document, error) {
	doc, err := t.svc.GetDocument(in.Path)
	return nil, doc, err
}

func (t *tools) createDocument(_ context.Context, _ *sdk.CallToolRequest, in createDocIn) (*sdk.CallToolResult, service.Document, error) {
	docType := vault.DocType(in.Type)
	switch docType {
	case vault.TypeNote, vault.TypePerson, vault.TypeCompany, vault.TypeProject, vault.TypeMeeting:
	default:
		return nil, service.Document{}, fmt.Errorf("invalid type %q (want note|person|company|project|meeting)", in.Type)
	}
	doc, err := t.svc.CreateDocumentWith(docType, in.Title, in.Body, service.CreateOptions{Area: in.Area, Template: in.Template})
	t.record("create_document", doc.Path, in.Type+": "+in.Title, err)
	return nil, doc, err
}

func (t *tools) updateDocument(_ context.Context, _ *sdk.CallToolRequest, in updateDocIn) (*sdk.CallToolResult, service.Document, error) {
	doc, err := t.svc.UpdateDocument(in.Path, in.Markdown, in.BaseSHA256)
	t.record("update_document", in.Path, fmt.Sprintf("%d bytes", len(in.Markdown)), err)
	return nil, doc, err
}

func (t *tools) appendToDocument(_ context.Context, _ *sdk.CallToolRequest, in appendIn) (*sdk.CallToolResult, service.Document, error) {
	doc, err := t.svc.AppendToDocument(in.Path, in.Markdown, in.Section)
	t.record("append_to_document", in.Path, firstLine(in.Markdown), err)
	return nil, doc, err
}

func (t *tools) linkEntity(_ context.Context, _ *sdk.CallToolRequest, in linkEntityIn) (*sdk.CallToolResult, service.Document, error) {
	doc, err := t.svc.LinkEntity(in.Path, in.Key, in.Target)
	t.record("link_entity", in.Path, in.Key+" → "+in.Target, err)
	return nil, doc, err
}

func (t *tools) setFrontmatter(_ context.Context, _ *sdk.CallToolRequest, in setFrontmatterIn) (*sdk.CallToolResult, service.Document, error) {
	if len(in.Values) == 0 {
		return nil, service.Document{}, fmt.Errorf("values is required")
	}
	// Frontmatter edits are surgical and base themselves on the current
	// file, so there is no sha to pass: the write is CAS'd internally.
	current, err := t.svc.GetDocument(in.Path)
	if err != nil {
		return nil, service.Document{}, err
	}
	doc, err := t.svc.SetFrontmatter(in.Path, in.Values, current.SHA256)
	keys := make([]string, 0, len(in.Values))
	for k := range in.Values {
		keys = append(keys, k)
	}
	t.record("set_frontmatter", in.Path, strings.Join(keys, ", "), err)
	return nil, doc, err
}

func (t *tools) listDocuments(_ context.Context, _ *sdk.CallToolRequest, in listDocsIn) (*sdk.CallToolResult, docListOut, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	docs, err := t.svc.ListDocuments(in.Type, in.Title, in.Area, limit)
	if err != nil {
		return nil, docListOut{}, err
	}
	if docs == nil {
		docs = []service.DocMeta{}
	}
	return nil, docListOut{Documents: docs}, nil
}

func (t *tools) listAreas(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, areasOut, error) {
	areas, err := t.svc.Areas()
	if err != nil {
		return nil, areasOut{}, err
	}
	return nil, areasOut{Areas: areas}, nil
}

func (t *tools) listTags(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, tagsOut, error) {
	tags, err := t.svc.Tags()
	if err != nil {
		return nil, tagsOut{}, err
	}
	return nil, tagsOut{Tags: tags}, nil
}

func (t *tools) getDaily(_ context.Context, _ *sdk.CallToolRequest, in getDailyIn) (*sdk.CallToolResult, service.Document, error) {
	date := in.Date
	if date == "" {
		date = t.svc.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, service.Document{}, fmt.Errorf("date must be YYYY-MM-DD, got %q", in.Date)
	}
	doc, err := t.svc.GetDaily(date)
	return nil, doc, err
}

func (t *tools) editTask(_ context.Context, _ *sdk.CallToolRequest, in editTaskIn) (*sdk.CallToolResult, service.Task, error) {
	task, err := t.svc.EditTask(in.ID, service.TaskEdit{Due: in.Due, Defer: in.Defer, Priority: in.Priority})
	t.record("edit_task", task.DocPath, task.Text, err)
	return nil, task, err
}

// firstLine is a short, human-readable summary for the audit row.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func (t *tools) listTasks(_ context.Context, _ *sdk.CallToolRequest, in listTasksIn) (*sdk.CallToolResult, taskListOut, error) {
	tasks, err := t.svc.TasksIn(in.View, in.Area)
	if err != nil {
		return nil, taskListOut{}, err
	}
	return nil, taskListOut{Tasks: tasks}, nil
}

func (t *tools) createTask(_ context.Context, _ *sdk.CallToolRequest, in createTaskIn) (*sdk.CallToolResult, service.Task, error) {
	task, err := t.svc.CreateTask(in.Text, in.Due, in.Defer)
	t.record("create_task", task.DocPath, in.Text, err)
	return nil, task, err
}

func (t *tools) completeTask(_ context.Context, _ *sdk.CallToolRequest, in taskIDIn) (*sdk.CallToolResult, service.Task, error) {
	task, err := t.svc.ToggleTask(in.ID)
	if err != nil {
		return nil, service.Task{}, err
	}
	if !task.Done {
		// The task was already complete and toggle reopened it: put it back
		// and tell the agent instead of guessing intent.
		if _, reErr := t.svc.ToggleTask(task.ID); reErr == nil {
			return nil, service.Task{}, fmt.Errorf("task was already complete")
		}
	}
	t.record("complete_task", task.DocPath, task.Text, nil)
	return nil, task, nil
}

func (t *tools) today(_ context.Context, _ *sdk.CallToolRequest, in areaIn) (*sdk.CallToolResult, service.TodayPayload, error) {
	payload, err := t.svc.TodayIn(in.Area)
	return nil, payload, err
}

func (t *tools) personContext(_ context.Context, _ *sdk.CallToolRequest, in personContextIn) (*sdk.CallToolResult, personContextOut, error) {
	path := in.Name
	if !strings.HasSuffix(path, ".md") {
		resolved := t.svc.Index.ResolveLink(in.Name)
		if resolved == "" {
			return nil, personContextOut{}, fmt.Errorf("no document found for %q — try search first", in.Name)
		}
		path = resolved
	}
	doc, err := t.svc.GetDocument(path)
	if err != nil {
		return nil, personContextOut{}, err
	}
	rows, err := t.svc.Index.TasksMentioning(path)
	if err != nil {
		return nil, personContextOut{}, err
	}
	return nil, personContextOut{Document: doc, OpenTasks: service.TasksFromRows(rows)}, nil
}
