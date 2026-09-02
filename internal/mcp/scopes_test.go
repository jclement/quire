package mcp

import (
	"context"
	"path/filepath"
	"slices"
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
	readTools := []string{"search", "get_document", "list_tasks", "today", "person_context"}
	writeTools := []string{"create_document", "update_document", "append_to_document"}
	taskTools := []string{"create_task", "complete_task"}

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
			got := toolNames(t, newServer(newScopeTestService(t), "test", tc.allows))
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
