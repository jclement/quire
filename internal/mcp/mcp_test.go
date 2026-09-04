package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

// connect spins up an in-memory MCP client/server pair over the real server
// wiring, so the tool schemas and handlers are exercised end to end.
func connect(t *testing.T) *sdk.ClientSession {
	t.Helper()
	v, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := service.New(v, &index.Index{DB: db, Vault: v})
	svc.Now = func() time.Time { return time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local) }

	server := newServer(svc, "test", allowAll, "owner", nil)
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func call(t *testing.T, s *sdk.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	res, err := s.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if res.IsError {
		t.Fatalf("%s returned tool error: %+v", tool, res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAgentWorkflow(t *testing.T) {
	s := connect(t)

	// The flagship loop: create entities, capture a task, ask "today".
	call(t, s, "create_document", map[string]any{"type": "person", "title": "Sarah Chen"})
	call(t, s, "create_document", map[string]any{
		"type": "meeting", "title": "Acme Sync",
		"body": "# Acme Sync\n\nWith [[Sarah Chen]].\n\n## Action Items\n\n- [ ] Send [[Sarah Chen]] the diagram 📅 2026-09-01\n",
	})
	call(t, s, "create_task", map[string]any{"text": "Review budget", "due": "2026-09-01"})

	today := call(t, s, "today", nil)
	if len(today["due_today"].([]any)) != 2 {
		t.Errorf("due_today = %v", today["due_today"])
	}
	if len(today["meetings"].([]any)) != 1 {
		t.Errorf("meetings = %v", today["meetings"])
	}

	// person_context resolves by name and rolls up tasks + backlinks.
	pc := call(t, s, "person_context", map[string]any{"name": "Sarah Chen"})
	doc := pc["document"].(map[string]any)
	if doc["path"] != "people/sarah-chen.md" {
		t.Errorf("resolved doc = %v", doc["path"])
	}
	if len(pc["open_tasks"].([]any)) != 1 {
		t.Errorf("open_tasks = %v", pc["open_tasks"])
	}
	if len(doc["backlinks"].([]any)) != 1 {
		t.Errorf("backlinks = %v", doc["backlinks"])
	}

	// Append into a section, then complete the meeting task.
	call(t, s, "append_to_document", map[string]any{
		"path": "meetings/2026-09-01-acme-sync.md", "markdown": "- [ ] Book follow-up", "section": "Action Items",
	})
	got := call(t, s, "get_document", map[string]any{"path": "meetings/2026-09-01-acme-sync.md"})
	if !strings.Contains(got["markdown"].(string), "- [ ] Send [[Sarah Chen]] the diagram 📅 2026-09-01\n- [ ] Book follow-up") {
		t.Errorf("append result:\n%s", got["markdown"])
	}

	tasks := call(t, s, "list_tasks", map[string]any{"view": "today"})
	first := tasks["tasks"].([]any)[0].(map[string]any)
	done := call(t, s, "complete_task", map[string]any{"id": first["id"]})
	if done["done"] != true {
		t.Errorf("complete = %v", done)
	}

	// Search finds the meeting via FTS.
	hits := call(t, s, "search", map[string]any{"query": "type:meeting diagram"})
	if len(hits["results"].([]any)) != 1 {
		t.Errorf("search = %v", hits["results"])
	}
}

func TestStaleWriteRejected(t *testing.T) {
	s := connect(t)
	call(t, s, "create_document", map[string]any{"type": "note", "title": "Precious"})

	res, err := s.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "update_document",
		Arguments: map[string]any{
			"path": "notes/precious.md", "markdown": "clobbered", "base_sha256": "stale",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("stale agent write should be a tool error")
	}
}

// The owner's guidance (AGENTS.md in the vault, edited in Settings) must
// reach agents through the server's MCP instructions — that is the whole
// point of the feature.
func TestOwnerGuidanceReachesInstructions(t *testing.T) {
	v, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := service.New(v, &index.Index{DB: db, Vault: v})

	connect := func() *sdk.InitializeResult {
		t.Helper()
		server := newServer(svc, "test", allowAll, "owner", nil)
		clientTransport, serverTransport := sdk.NewInMemoryTransports()
		ctx := context.Background()
		if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
			t.Fatal(err)
		}
		client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
		session, err := client.Connect(ctx, clientTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { session.Close() })
		return session.InitializeResult()
	}

	// With no guidance written, agents still get the built-in rules.
	before := connect()
	if !strings.Contains(before.Instructions, "append_to_document") {
		t.Fatalf("base instructions missing: %q", before.Instructions)
	}
	if strings.Contains(before.Instructions, "owner's own guidance") {
		t.Errorf("guidance section present with no guidance written")
	}

	// Writing guidance reaches the NEXT session without a restart.
	if _, err := svc.SetAgentGuidance("Always file client work under projects/.\nUse British spelling."); err != nil {
		t.Fatal(err)
	}
	after := connect()
	if !strings.Contains(after.Instructions, "Always file client work under projects/.") ||
		!strings.Contains(after.Instructions, "British spelling") {
		t.Errorf("guidance not delivered:\n%s", after.Instructions)
	}
	if !strings.Contains(after.Instructions, "append_to_document") {
		t.Errorf("guidance replaced the base instructions instead of extending them")
	}

	// Clearing it removes the section (and the file).
	if _, err := svc.SetAgentGuidance("  "); err != nil {
		t.Fatal(err)
	}
	if got := svc.AgentGuidance(); got != "" {
		t.Errorf("guidance after clear = %q", got)
	}
	if strings.Contains(connect().Instructions, "British spelling") {
		t.Errorf("cleared guidance still delivered")
	}
}

// TestAgentCoversTheWholeVault walks the surface the way an agent actually
// would across a working day: file a thought, put a task on a project with
// the full grammar, reshape it, tidy a name, and repair a recurrence that
// was ticked off in an editor. Anything an agent cannot do here, a person
// has to do by hand.
func TestAgentCoversTheWholeVault(t *testing.T) {
	s := connect(t)

	// A thought that is not an action goes into the day as prose.
	day := call(t, s, "capture_note", map[string]any{
		"text": "the solver slows above 800 wells",
	})
	if !strings.Contains(day["markdown"].(string), "- the solver slows above 800 wells") {
		t.Errorf("capture_note did not file prose: %v", day["markdown"])
	}
	if strings.Contains(day["markdown"].(string), "- [ ] the solver") {
		t.Error("a captured thought must not become a checkbox")
	}

	// Today's note exists on demand, without writing to it.
	ensured := call(t, s, "ensure_daily", map[string]any{})
	if ensured["path"] != day["path"] {
		t.Errorf("ensure_daily = %v, want today's note %v", ensured["path"], day["path"])
	}

	// A project, and a task filed on it with every marker.
	call(t, s, "create_document", map[string]any{"type": "project", "title": "Apollo"})
	task := call(t, s, "create_task", map[string]any{
		"text": "size the rollout", "path": "projects/apollo.md",
		"due": "2026-10-01", "priority": 1, "waiting": true, "recur": "every month",
	})
	if task["doc_path"] != "projects/apollo.md" {
		t.Errorf("task landed in %v, not the project", task["doc_path"])
	}
	if task["priority"].(float64) != 1 || task["waiting"] != true || task["recur"] != "every month" {
		t.Errorf("markers lost: %+v", task)
	}

	// Reshaping it: rename, delegate, stop repeating — one call each field.
	edited := call(t, s, "edit_task", map[string]any{
		"id": task["id"], "text": "size the staged rollout", "recur": "", "waiting": false,
	})
	if edited["text"] != "size the staged rollout" || edited["recur"] != nil || edited["waiting"] != false {
		t.Errorf("edit_task = %+v", edited)
	}
	if edited["due"] != "2026-10-01" || edited["priority"].(float64) != 1 {
		t.Errorf("edit_task disturbed untouched markers: %+v", edited)
	}

	// Relationships go both ways.
	call(t, s, "create_document", map[string]any{"type": "person", "title": "Sarah Chen"})
	linked := call(t, s, "link_entity", map[string]any{
		"path": "projects/apollo.md", "key": "people", "target": "Sarah Chen",
	})
	if !strings.Contains(linked["markdown"].(string), "Sarah Chen") {
		t.Errorf("link_entity did not write the link: %v", linked["markdown"])
	}
	unlinked := call(t, s, "unlink_entity", map[string]any{
		"path": "projects/apollo.md", "key": "people", "target": "Sarah Chen",
	})
	if strings.Contains(unlinked["markdown"].(string), "[[Sarah Chen]]") {
		t.Errorf("unlink_entity left the link: %v", unlinked["markdown"])
	}

	// A note that turned out to be a person moves, and its links follow.
	call(t, s, "create_document", map[string]any{
		"type": "note", "title": "Bo Vance", "body": "# Bo Vance\n\nCTO somewhere.\n",
	})
	call(t, s, "append_to_document", map[string]any{
		"path": "projects/apollo.md", "markdown": "Spoke to [[Bo Vance]] about staging.",
	})
	renamed := call(t, s, "rename_document", map[string]any{
		"path": "notes/bo-vance.md", "new_path": "people/bo-vance.md",
	})
	moved := renamed["document"].(map[string]any)
	if moved["path"] != "people/bo-vance.md" {
		t.Errorf("rename_document left it at %v", moved["path"])
	}
	if moved["type"] != "person" {
		t.Errorf("moving to people/ should retype the document, got %v", moved["type"])
	}
	apollo := call(t, s, "get_document", map[string]any{"path": "projects/apollo.md"})
	if !strings.Contains(apollo["markdown"].(string), "Bo Vance") {
		t.Errorf("the reference should survive the rename: %v", apollo["markdown"])
	}
	// Templates are discoverable rather than guessed at.
	templates := call(t, s, "list_templates", map[string]any{})
	if templates["templates"] == nil {
		t.Error("list_templates returned nothing at all")
	}

	// A recurrence ticked off in an editor is spotted and repaired.
	call(t, s, "create_document", map[string]any{
		"type": "note", "title": "Vehicle",
		"body": "# Vehicle\n\n- [x] renew rego 📅 2026-08-01 🛫 2026-07-11 🔁 every year ✅ 2026-08-02\n",
	})
	week := call(t, s, "week_review", map[string]any{})
	problems, _ := week["recurrence"].([]any)
	var stoppedID string
	for _, p := range problems {
		row := p.(map[string]any)
		if row["reason"] == "stopped" {
			stoppedID = row["task"].(map[string]any)["id"].(string)
		}
	}
	if stoppedID == "" {
		t.Fatalf("week_review should report the stopped recurrence: %+v", week["recurrence"])
	}
	restored := call(t, s, "restore_recurrence", map[string]any{"id": stoppedID})
	if restored["due"] != "2027-08-01" || restored["defer"] != "2027-07-11" {
		t.Errorf("restore_recurrence = %+v, want next year with the lead time kept", restored)
	}
}
