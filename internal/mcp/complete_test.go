package mcp

import "testing"

// TestCompleteTaskOnDoneTaskIsANoOp: agents retry, so complete_task on a
// task that is already done must succeed and leave the file alone. The tool
// used to reopen it and toggle it back, restamping ✅ with today and giving
// a repeating task a second next occurrence.
func TestCompleteTaskOnDoneTaskIsANoOp(t *testing.T) {
	s, svc := connectWithService(t)
	original := "# Chores\n\n- [x] change the filter 📅 2026-08-01 🔁 every month ✅ 2026-08-01\n" +
		"- [ ] change the filter 📅 2026-09-01 🔁 every month\n"
	if _, err := svc.UpdateDocument("notes/chores.md", original, ""); err != nil {
		t.Fatal(err)
	}
	doc, err := svc.GetDocument("notes/chores.md")
	if err != nil {
		t.Fatal(err)
	}

	done := call(t, s, "complete_task", map[string]any{"id": doc.Tasks[0].ID})
	if done["done"] != true || done["id"] != doc.Tasks[0].ID {
		t.Errorf("complete_task on a done task = %v", done)
	}
	after, err := svc.GetDocument("notes/chores.md")
	if err != nil {
		t.Fatal(err)
	}
	if after.Markdown != original {
		t.Errorf("complete_task rewrote a done task's file:\ngot  %q\nwant %q", after.Markdown, original)
	}
}
