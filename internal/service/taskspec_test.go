// Creating and editing tasks through the full grammar, which is what the
// agent surface needs: any marker, any target document.
package service

import (
	"strings"
	"testing"
)

func TestCreateTaskWithEveryMarker(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"projects/apollo.md": "# Apollo\n\n## Next actions\n\n- [ ] existing\n\n## Notes\n\n- context\n",
	})

	task, err := svc.CreateTaskWith(TaskSpec{
		Path:     "projects/apollo.md",
		Text:     "book the launch review",
		Due:      "2026-10-01",
		Defer:    "2026-09-20",
		Priority: 1,
		Waiting:  true,
		Recur:    "every 3 months",
		Section:  "Next actions",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Text != "book the launch review" || task.DocPath != "projects/apollo.md" {
		t.Errorf("task = %+v", task)
	}
	if task.Due == nil || *task.Due != "2026-10-01" || task.Priority != 1 || !task.Waiting {
		t.Errorf("markers lost: %+v", task)
	}
	if task.Recur == nil || *task.Recur != "every 3 months" {
		t.Errorf("recurrence = %v", task.Recur)
	}
	f, _ := svc.Vault.Read("projects/apollo.md")
	body := string(f.Raw)
	// It lands in the named section, above what follows it.
	if strings.Index(body, "book the launch review") > strings.Index(body, "## Notes") {
		t.Errorf("task should sit in Next actions:\n%s", body)
	}

	// A task with no path lands in today's note, under Captured.
	day, err := svc.CreateTaskWith(TaskSpec{Text: "a plain one"})
	if err != nil {
		t.Fatal(err)
	}
	if day.DocPath != "daily/"+svc.today()+".md" {
		t.Errorf("default target = %s", day.DocPath)
	}

	// Bad input is refused rather than written as words no view can match.
	for _, bad := range []TaskSpec{
		{Text: ""},
		{Text: "x", Recur: "every fortnight"},
		{Text: "x", Priority: 9},
		{Text: "x", Due: "whenever"},
	} {
		if _, err := svc.CreateTaskWith(bad); err == nil {
			t.Errorf("CreateTaskWith(%+v) should fail", bad)
		}
	}
}

func TestEditTaskAcrossTheGrammar(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"notes/edits.md": "# Edits\n\n- [ ] draft the memo 📅 2026-09-10 ⏫\n",
	})
	doc, err := svc.GetDocument("notes/edits.md")
	if err != nil {
		t.Fatal(err)
	}
	id := doc.Tasks[0].ID

	yes := true
	spec := "every week"
	edited, err := svc.EditTask(id, TaskEdit{Waiting: &yes, Recur: &spec})
	if err != nil {
		t.Fatal(err)
	}
	if !edited.Waiting || edited.Recur == nil || *edited.Recur != "every week" {
		t.Errorf("edited = %+v", edited)
	}
	if edited.Due == nil || *edited.Due != "2026-09-10" || edited.Priority != 1 {
		t.Errorf("untouched markers were disturbed: %+v", edited)
	}

	// Renaming keeps every marker and gives the task a new identity.
	text := "draft the board memo"
	renamed, err := svc.EditTask(edited.ID, TaskEdit{Text: &text})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Text != text || renamed.ID == edited.ID {
		t.Errorf("renamed = %+v (old id %s)", renamed, edited.ID)
	}
	if renamed.Due == nil || *renamed.Due != "2026-09-10" || !renamed.Waiting || renamed.Priority != 1 {
		t.Errorf("rename lost markers: %+v", renamed)
	}

	// Clearing works too.
	no, none := false, ""
	cleared, err := svc.EditTask(renamed.ID, TaskEdit{Waiting: &no, Recur: &none})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Waiting || cleared.Recur != nil {
		t.Errorf("cleared = %+v", cleared)
	}
	f, _ := svc.Vault.Read("notes/edits.md")
	if strings.Contains(string(f.Raw), "🔁") || strings.Contains(string(f.Raw), "⏳") {
		t.Errorf("markers should be gone:\n%s", f.Raw)
	}
	if empty := ""; func() bool { _, err := svc.EditTask(cleared.ID, TaskEdit{Text: &empty}); return err == nil }() {
		t.Error("empty text should be refused")
	}
}
