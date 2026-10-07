package mcp

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestCancelDeleteAndRemoveLines drives the three tools that take things
// out of a note, end to end through the tool schemas.
func TestCancelDeleteAndRemoveLines(t *testing.T) {
	s, svc := connectWithService(t)
	original := "# Chores\n\n- [ ] sell the bike\n- [ ] sell the bike twice\n- a stale bullet\n- keep this\n"
	if _, err := svc.UpdateDocument("notes/chores.md", original, ""); err != nil {
		t.Fatal(err)
	}
	doc, err := svc.GetDocument("notes/chores.md")
	if err != nil {
		t.Fatal(err)
	}

	cancelled := call(t, s, "cancel_task", map[string]any{"id": doc.Tasks[0].ID})
	if cancelled["line"] != "- [-] sell the bike ❌ 2026-09-01" {
		t.Errorf("cancel_task = %v", cancelled)
	}
	deleted := call(t, s, "delete_task", map[string]any{"id": doc.Tasks[1].ID})
	if deleted["text"] != "sell the bike twice" {
		t.Errorf("delete_task = %v", deleted)
	}
	removed := call(t, s, "remove_lines", map[string]any{"path": "notes/chores.md", "lines": []string{"- a stale bullet"}})
	want := "# Chores\n\n- [-] sell the bike ❌ 2026-09-01\n- keep this\n"
	if removed["markdown"] != want {
		t.Errorf("after all three:\ngot  %q\nwant %q", removed["markdown"], want)
	}
	if tasks := call(t, s, "list_tasks", map[string]any{"view": "inbox"})["tasks"]; tasks != nil && len(tasks.([]any)) != 0 {
		t.Errorf("inbox should be empty, got %v", tasks)
	}

	// A line that is not there is a tool error the agent can read, not a
	// silent success.
	res, err := s.CallTool(context.Background(), &sdk.CallToolParams{Name: "remove_lines",
		Arguments: map[string]any{"path": "notes/chores.md", "lines": []string{"- never written"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].(*sdk.TextContent).Text, "nothing was removed") {
		t.Errorf("remove_lines of a missing line = %+v", res.Content)
	}
}
