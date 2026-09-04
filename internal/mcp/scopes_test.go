package mcp

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

// toolNames asks a server what it exposes, the way a client would.
func toolNames(t *testing.T, server *sdk.Server) []string {
	t.Helper()
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "scope-probe", Version: "0"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func newScopeTestService(t *testing.T) *service.Service {
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
	return svc
}

// allowAll is the permissive scope check used by tests that are not about
// scopes; it stands in for an owner principal.
func allowAll(string) bool { return true }

// principal builds the scope check a token with these scopes would produce,
// including the real implication rules (write implies tasks).
func principal(scopes ...string) func(string) bool {
	set := map[string]bool{}
	for _, s := range scopes {
		set[s] = true
	}
	return auth.Principal{Name: "test", Scopes: set}.Allows
}

// TestToolsAreScoped is the regression test for a real privilege escalation:
// a token scoped only to "tasks" was refused a document write over REST (403)
// but could create the same document through MCP's create_document, because
// passing the /mcp gate registered every tool regardless of scope.
func TestToolsAreScoped(t *testing.T) {
	const (
		read  = auth.ScopeRead
		write = auth.ScopeWrite
		tasks = auth.ScopeTasks
	)
	readTools := []string{
		"search", "list_documents", "get_document", "get_daily", "get_weekly",
		"list_daily", "list_tasks", "list_tags", "list_areas", "list_templates",
		"list_unwritten", "today", "week_review", "calendar", "person_context",
	}
	writeTools := []string{
		"create_document", "update_document", "append_to_document",
		"link_entity", "unlink_entity", "rename_document", "set_frontmatter",
		"capture_note", "ensure_daily", "ensure_weekly",
	}
	taskTools := []string{"create_task", "complete_task", "edit_task", "restore_recurrence"}

	for _, tc := range []struct {
		name    string
		allows  func(string) bool
		want    []string
		notWant []string
	}{
		{"read-only agent", principal(read), readTools, append(slices.Clone(writeTools), taskTools...)},
		{"todo agent (tasks only)", principal(tasks), taskTools, append(slices.Clone(writeTools), readTools...)},
		{"read+tasks", principal(read, tasks), append(slices.Clone(readTools), taskTools...), writeTools},
		{"full agent", principal(read, write, tasks), append(append(slices.Clone(readTools), writeTools...), taskTools...), nil},
		// write implies tasks, so a write token keeps the task tools.
		{"write-only", principal(write), append(slices.Clone(writeTools), taskTools...), readTools},
		// No principal at all (never passed the middleware) gets nothing.
		{"no principal", func(string) bool { return false }, nil, append(append(slices.Clone(readTools), writeTools...), taskTools...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := toolNames(t, newServer(newScopeTestService(t), "test", tc.allows, "test-token", nil))
			for _, name := range tc.want {
				if !slices.Contains(got, name) {
					t.Errorf("%s should expose %s, got %v", tc.name, name, got)
				}
			}
			for _, name := range tc.notWant {
				if slices.Contains(got, name) {
					t.Errorf("ESCALATION: %s must not expose %s", tc.name, name)
				}
			}
		})
	}
}

type recordingAuditor struct{ rows []auth.AuditRecord }

func (r *recordingAuditor) RecordAudit(rec auth.AuditRecord) error {
	r.rows = append(r.rows, rec)
	return nil
}

// TestMutatingToolsAreAudited: an agent's writes are recorded with who, what
// and where; reads are not; and the owner's own session is never audited.
func TestMutatingToolsAreAudited(t *testing.T) {
	svc := newScopeTestService(t)
	rec := &recordingAuditor{}
	server := newServer(svc, "test", allowAll, "token:claude", rec)

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "audit-probe", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	call := func(name string, args map[string]any) {
		t.Helper()
		if _, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	call("create_document", map[string]any{"type": "note", "title": "Audited Note"})
	call("append_to_document", map[string]any{"path": "notes/audited-note.md", "markdown": "more"})
	call("create_task", map[string]any{"text": "Audited task", "due": "today"})
	call("search", map[string]any{"query": "audited"}) // a read: not recorded
	call("list_tags", map[string]any{})

	if len(rec.rows) != 3 {
		t.Fatalf("expected 3 audit rows (writes only), got %d: %+v", len(rec.rows), rec.rows)
	}
	if rec.rows[0].Principal != "token:claude" || rec.rows[0].Action != "mcp:create_document" || rec.rows[0].Path != "notes/audited-note.md" || !rec.rows[0].OK {
		t.Errorf("first row = %+v", rec.rows[0])
	}
	if rec.rows[2].Action != "mcp:create_task" || rec.rows[2].Detail != "Audited task" {
		t.Errorf("task row = %+v", rec.rows[2])
	}

	// The owner's own session is not an agent.
	ownerRec := &recordingAuditor{}
	ownerServer := newServer(newScopeTestService(t), "test", allowAll, "owner", ownerRec)
	ct2, st2 := sdk.NewInMemoryTransports()
	if _, err := ownerServer.Connect(ctx, st2, nil); err != nil {
		t.Fatal(err)
	}
	s2, err := sdk.NewClient(&sdk.Implementation{Name: "owner-probe", Version: "0"}, nil).Connect(ctx, ct2, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s2.Close() })
	if _, err := s2.CallTool(ctx, &sdk.CallToolParams{Name: "create_task", Arguments: map[string]any{"text": "owner task"}}); err != nil {
		t.Fatal(err)
	}
	if len(ownerRec.rows) != 0 {
		t.Errorf("owner's calls must not be audited: %+v", ownerRec.rows)
	}
}

// TestSurfaceIsCompleteAndDocumented pins the whole agent surface: nothing
// exists that these lists do not name, and every tool carries a description
// that says when to reach for it rather than restating its own name. The
// tool list is the agent's entire documentation — a tool added without one
// is a tool that will be used wrongly or not at all.
func TestSurfaceIsCompleteAndDocumented(t *testing.T) {
	server := newServer(newScopeTestService(t), "test", allowAll, "owner", nil)
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "surface-probe", Version: "0"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Semantic tools register only when an embeddings key is configured, so
	// they are absent here by construction.
	want := map[string]bool{
		"search": true, "list_documents": true, "get_document": true,
		"get_daily": true, "get_weekly": true, "list_daily": true,
		"list_tasks": true, "list_tags": true, "list_areas": true,
		"list_templates": true, "list_unwritten": true, "today": true,
		"week_review": true, "calendar": true, "person_context": true,
		"create_document": true, "update_document": true,
		"append_to_document": true, "link_entity": true, "unlink_entity": true,
		"rename_document": true, "set_frontmatter": true, "capture_note": true,
		"ensure_daily": true, "ensure_weekly": true,
		"create_task": true, "complete_task": true, "edit_task": true,
		"restore_recurrence": true,
	}
	for _, tool := range res.Tools {
		if !want[tool.Name] {
			t.Errorf("tool %q is not in the documented surface — add it to the lists above and to README", tool.Name)
		}
		delete(want, tool.Name)
		if len(tool.Description) < 80 {
			t.Errorf("tool %q needs a description that teaches when to use it, got %q", tool.Name, tool.Description)
		}
		if tool.Annotations == nil {
			t.Errorf("tool %q has no annotations, so clients cannot tell whether it writes", tool.Name)
		}
	}
	for name := range want {
		t.Errorf("tool %q is missing from the surface", name)
	}
}

// TestDeleteStaysAbsent: no tool may delete a document. Losing work to an
// agent is the one failure a notes vault cannot come back from, and REST
// plus git are the deliberate way out.
func TestDeleteStaysAbsent(t *testing.T) {
	for _, name := range toolNames(t, newServer(newScopeTestService(t), "test", allowAll, "owner", nil)) {
		if strings.Contains(strings.ToLower(name), "delete") ||
			strings.Contains(strings.ToLower(name), "remove_document") {
			t.Errorf("a destructive tool appeared on the agent surface: %q", name)
		}
	}
}
