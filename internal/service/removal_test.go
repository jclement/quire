// Deleting and cancelling tasks, and removing lines from a note.
package service

import (
	"errors"
	"strings"
	"testing"
)

func TestDeleteTaskRemovesOnlyItsLine(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"notes/list.md": "# List\n\n- [ ] keep me\n  * [ ] drop me 📅 2026-09-10\n    - a nested thought\n- [x] done one ✅ 2026-08-30\n",
	})
	doc, _ := svc.GetDocument("notes/list.md")

	gone, err := svc.DeleteTask(doc.Tasks[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if gone.Text != "drop me" || gone.DocPath != "notes/list.md" {
		t.Errorf("deleted task = %+v", gone)
	}
	after, _ := svc.GetDocument("notes/list.md")
	want := "# List\n\n- [ ] keep me\n    - a nested thought\n- [x] done one ✅ 2026-08-30\n"
	if after.Markdown != want {
		t.Errorf("after delete:\ngot  %q\nwant %q", after.Markdown, want)
	}
	if len(after.Tasks) != 2 {
		t.Errorf("tasks left = %+v", after.Tasks)
	}
	// A completed task can be deleted too; the id of a deleted one is gone.
	if _, err := svc.DeleteTask(doc.Tasks[2].ID); err != nil {
		t.Errorf("deleting a completed task: %v", err)
	}
	if _, err := svc.DeleteTask(doc.Tasks[1].ID); err == nil {
		t.Error("deleting a task twice should not find it the second time")
	}
}

func TestCancelTask(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"notes/list.md": "# List\n\n  * [ ] renew the domain 📅 2026-09-10 🔁 every year\n- [ ] keep me\n- [x] done one ✅ 2026-08-30\n",
	})
	doc, _ := svc.GetDocument("notes/list.md")

	was, line, err := svc.CancelTask(doc.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	wantLine := "  * [-] renew the domain 📅 2026-09-10 🔁 every year ❌ " + svc.today()
	if line != wantLine || was.Text != "renew the domain" {
		t.Errorf("cancel = %+v, line %q, want %q", was, line, wantLine)
	}
	after, _ := svc.GetDocument("notes/list.md")
	want := "# List\n\n" + wantLine + "\n- [ ] keep me\n- [x] done one ✅ 2026-08-30\n"
	if after.Markdown != want {
		t.Errorf("after cancel:\ngot  %q\nwant %q", after.Markdown, want)
	}
	// It is no longer a task anywhere, and a repeat minted nothing.
	if len(after.Tasks) != 2 {
		t.Errorf("a cancelled task should leave the task list: %+v", after.Tasks)
	}
	for _, view := range []string{"inbox", "today", "upcoming", "logbook"} {
		tasks, err := svc.Tasks(view)
		if err != nil {
			t.Fatal(err)
		}
		for _, task := range tasks {
			if strings.Contains(task.Text, "renew the domain") {
				t.Errorf("cancelled task still in %s", view)
			}
		}
	}

	// A completed task is not cancellable, and the file is left alone.
	if _, _, err := svc.CancelTask(doc.Tasks[2].ID); !errors.Is(err, ErrValidation) {
		t.Errorf("cancelling a done task = %v, want a validation error", err)
	}
	if again, _ := svc.GetDocument("notes/list.md"); again.Markdown != want {
		t.Errorf("refused cancel changed the file: %q", again.Markdown)
	}
}

func TestRemoveLines(t *testing.T) {
	svc := newTestService(t)
	original := "---\narea: work\n---\n# Plan\n\n- alpha\n- beta   \n\n## Old\n\n- alpha\n  - nested\n\ntail\n"
	writeVault(t, svc, map[string]string{"notes/plan.md": original})

	// One line by its text (trailing space ignored), and a block that pins
	// down a line which on its own occurs twice.
	doc, err := svc.RemoveLines("notes/plan.md", []string{"- beta", "## Old\n\n- alpha"})
	if err != nil {
		t.Fatal(err)
	}
	want := "---\narea: work\n---\n# Plan\n\n- alpha\n\n  - nested\n\ntail\n"
	if doc.Markdown != want {
		t.Errorf("after remove:\ngot  %q\nwant %q", doc.Markdown, want)
	}

	// Everything below is refused, and refused whole.
	for name, blocks := range map[string][]string{
		"missing line":        {"tail", "- gamma"},
		"wrong indentation":   {"- nested"},
		"blank entry":         {"tail", "  "},
		"frontmatter":         {"area: work"},
		"frontmatter fence":   {"---"},
		"nothing asked":       {},
		"block not adjoining": {"# Plan\n- alpha"},
	} {
		if _, err := svc.RemoveLines("notes/plan.md", blocks); !errors.Is(err, ErrValidation) {
			t.Errorf("%s: err = %v, want a validation error", name, err)
		}
		if now, _ := svc.GetDocument("notes/plan.md"); now.Markdown != want {
			t.Fatalf("%s: a refused removal changed the file: %q", name, now.Markdown)
		}
	}
}

func TestRemoveLinesRefusesAnAmbiguousLine(t *testing.T) {
	svc := newTestService(t)
	original := "# Log\n\n- call back\n- call back\n"
	writeVault(t, svc, map[string]string{"notes/log.md": original})
	_, err := svc.RemoveLines("notes/log.md", []string{"- call back"})
	if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), "2 times") {
		t.Errorf("err = %v, want an ambiguity refusal", err)
	}
	if now, _ := svc.GetDocument("notes/log.md"); now.Markdown != original {
		t.Errorf("file changed: %q", now.Markdown)
	}
}
