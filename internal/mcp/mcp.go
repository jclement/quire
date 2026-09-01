// Package mcp exposes quire to AI agents over the Model Context Protocol
// (Streamable HTTP at /mcp). Every tool is a thin wrapper over the same
// service layer the REST API uses, so an agent can never do anything the API
// couldn't (DESIGN.md decision 5). No delete tool exists on purpose.
package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

// Handler returns the Streamable HTTP handler for /mcp. A server is built
// per session rather than once at startup so that guidance the owner edits
// in Settings reaches the next client that connects, with no restart.
func Handler(svc *service.Service, version string) http.Handler {
	return sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return newServer(svc, version) }, nil)
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

func newServer(svc *service.Service, version string) *sdk.Server {
	instructions := baseInstructions
	if guidance := svc.AgentGuidance(); guidance != "" {
		instructions += "\n\n---\n\nThe vault owner's own guidance (authoritative where it conflicts\nwith the above):\n\n" + guidance
	}
	s := sdk.NewServer(&sdk.Implementation{Name: "quire", Version: version},
		&sdk.ServerOptions{Instructions: instructions})
	t := &tools{svc: svc}

	sdk.AddTool(s, &sdk.Tool{Name: "search",
		Description: "Search the knowledge base. Supports the shared filter grammar: bare words full-text search; type:<note|person|company|project|meeting|daily> and tag:<tag> filter."},
		t.search)
	sdk.AddTool(s, &sdk.Tool{Name: "get_document",
		Description: "Fetch a document by vault path: raw markdown plus parsed structure (frontmatter, links with resolution, backlinks, tasks). Use search to find paths."},
		t.getDocument)
	sdk.AddTool(s, &sdk.Tool{Name: "create_document",
		Description: "Create a new document of a given type (note, person, company, project, meeting). The server picks the path from the title. Body is optional markdown; wikilinks like [[Sarah Chen]] create relationships."},
		t.createDocument)
	sdk.AddTool(s, &sdk.Tool{Name: "update_document",
		Description: "Replace a document's full markdown. Requires base_sha256 from a prior get_document — a stale hash is rejected so agents can never clobber concurrent edits. Prefer append_to_document for additions."},
		t.updateDocument)
	sdk.AddTool(s, &sdk.Tool{Name: "append_to_document",
		Description: "Append markdown to the end of a document (or under a named section heading if given). The safe way to add notes, decisions, or action items without rewriting the file."},
		t.appendToDocument)
	sdk.AddTool(s, &sdk.Tool{Name: "list_tasks",
		Description: "List tasks by view: inbox (unprocessed), today (due/overdue/available now), upcoming, waiting (delegated), logbook (completed)."},
		t.listTasks)
	sdk.AddTool(s, &sdk.Tool{Name: "create_task",
		Description: "Create a task (lands in today's daily note with full provenance). Dates are YYYY-MM-DD: due = deadline, defer = hide until this date."},
		t.createTask)
	sdk.AddTool(s, &sdk.Tool{Name: "complete_task",
		Description: "Mark a task complete by id (from list_tasks/get_document). Edits the source markdown checkbox surgically."},
		t.completeTask)
	sdk.AddTool(s, &sdk.Tool{Name: "today",
		Description: "The composed 'what matters right now' payload: today's meetings, overdue and due tasks, available and waiting tasks, recent documents, and the daily note. Start here for 'what should I work on'."},
		t.today)
	sdk.AddTool(s, &sdk.Tool{Name: "person_context",
		Description: "Everything about a person (or project/company) in one call: their document, backlinks (meetings and notes mentioning them), and open tasks involving them. Ideal for meeting prep."},
		t.personContext)

	return s
}

type tools struct{ svc *service.Service }

// ---- inputs ----

type searchIn struct {
	Query string `json:"query" jsonschema:"the search query, e.g. 'type:meeting acme' or 'reporting decisions'"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results (default 20)"`
}

type pathIn struct {
	Path string `json:"path" jsonschema:"vault-relative document path, e.g. people/sarah-chen.md"`
}

type createDocIn struct {
	Type  string `json:"type" jsonschema:"one of: note, person, company, project, meeting"`
	Title string `json:"title" jsonschema:"document title, e.g. 'Sarah Chen'"`
	Body  string `json:"body,omitempty" jsonschema:"optional markdown body; defaults to a heading"`
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
}

type createTaskIn struct {
	Text  string `json:"text" jsonschema:"the task text; may include #tags and [[wikilinks]]"`
	Due   string `json:"due,omitempty" jsonschema:"due date YYYY-MM-DD"`
	Defer string `json:"defer,omitempty" jsonschema:"defer/start date YYYY-MM-DD (hidden until then)"`
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
	doc, err := t.svc.CreateDocument(docType, in.Title, in.Body)
	return nil, doc, err
}

func (t *tools) updateDocument(_ context.Context, _ *sdk.CallToolRequest, in updateDocIn) (*sdk.CallToolResult, service.Document, error) {
	doc, err := t.svc.UpdateDocument(in.Path, in.Markdown, in.BaseSHA256)
	return nil, doc, err
}

func (t *tools) appendToDocument(_ context.Context, _ *sdk.CallToolRequest, in appendIn) (*sdk.CallToolResult, service.Document, error) {
	doc, err := t.svc.AppendToDocument(in.Path, in.Markdown, in.Section)
	return nil, doc, err
}

func (t *tools) listTasks(_ context.Context, _ *sdk.CallToolRequest, in listTasksIn) (*sdk.CallToolResult, taskListOut, error) {
	tasks, err := t.svc.Tasks(in.View)
	if err != nil {
		return nil, taskListOut{}, err
	}
	return nil, taskListOut{Tasks: tasks}, nil
}

func (t *tools) createTask(_ context.Context, _ *sdk.CallToolRequest, in createTaskIn) (*sdk.CallToolResult, service.Task, error) {
	task, err := t.svc.CreateTask(in.Text, in.Due, in.Defer)
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
	return nil, task, nil
}

func (t *tools) today(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, service.TodayPayload, error) {
	payload, err := t.svc.Today()
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
