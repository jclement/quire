package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/jclement/quire/internal/vault"
)

// TestCompleteTaskIsIdempotent: completing a done task must not touch the
// file. The MCP tool used to toggle it open and toggle it back, which
// restamped ✅ with today and, for a repeating task, spawned a second
// next occurrence.
func TestCompleteTaskIsIdempotent(t *testing.T) {
	s := newTestService(t)
	original := "# Chores\n\n- [x] change the filter 📅 2026-08-01 🔁 every month ✅ 2026-08-01\n" +
		"- [ ] change the filter 📅 2026-09-01 🔁 every month\n"
	if _, err := s.UpdateDocument("notes/chores.md", original, ""); err != nil {
		t.Fatal(err)
	}
	doc, _ := s.GetDocument("notes/chores.md")
	done := doc.Tasks[0]

	got, err := s.CompleteTask(done.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Done || got.ID != done.ID {
		t.Errorf("completed task = %+v, want the done task unchanged", got)
	}
	after, _ := s.GetDocument("notes/chores.md")
	if after.Markdown != original {
		t.Errorf("completing a done task rewrote the file:\ngot  %q\nwant %q", after.Markdown, original)
	}

	// An open task still completes.
	opened, err := s.CompleteTask(doc.Tasks[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !opened.Done {
		t.Errorf("open task not completed: %+v", opened)
	}
}

// TestToggleRefusesAmbiguousDuplicate: when the indexed line hint is stale
// and two identical tasks remain, guessing the first one ticked a task the
// user never touched.
func TestToggleRefusesAmbiguousDuplicate(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("notes/t.md", "- [ ] call mum\n- [ ] call mum\n", ""); err != nil {
		t.Fatal(err)
	}
	doc, _ := s.GetDocument("notes/t.md")
	second := doc.Tasks[1]

	// An outside edit moves both lines down without reindexing, so the
	// hint for the second task now points at the first.
	f, _ := s.Vault.Read("notes/t.md")
	shifted := "# Calls\n- [ ] call mum\n- [ ] call mum\n"
	if _, err := s.Vault.Write("notes/t.md", []byte(shifted), f.SHA256); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ToggleTask(second.ID); !errors.Is(err, vault.ErrConflict) {
		t.Errorf("toggle with two candidates: got %v, want ErrConflict", err)
	}
	after, _ := s.Vault.Read("notes/t.md")
	if string(after.Raw) != shifted {
		t.Errorf("ambiguous toggle wrote anyway: %q", after.Raw)
	}
}

// TestCreateTaskRejectsMarkerOnlyText: text the scanner reads entirely as
// markers left a line on disk and then an error, since the task it wrote
// had no words to find it by.
func TestCreateTaskRejectsMarkerOnlyText(t *testing.T) {
	s := newTestService(t)
	for _, text := range []string{"", "   ", "⏫", "⏳"} {
		_, err := s.CreateTaskWith(TaskSpec{Text: text})
		if !errors.Is(err, ErrValidation) {
			t.Errorf("CreateTaskWith(%q): got %v, want ErrValidation", text, err)
		}
		_, err = s.CreateTaskWithAttachment(text, "", "", Attachment{})
		if !errors.Is(err, ErrValidation) {
			t.Errorf("CreateTaskWithAttachment(%q): got %v, want ErrValidation", text, err)
		}
	}
	if doc, err := s.GetDaily("2026-09-01"); err == nil && strings.Contains(doc.Markdown, "- [ ]") {
		t.Errorf("a rejected task was written: %q", doc.Markdown)
	}
}

// TestAttachmentTaskUsesTheSharedLine: the attachment path now builds its
// line with the same code as every other task, markers included.
func TestAttachmentTaskUsesTheSharedLine(t *testing.T) {
	s := newTestService(t)
	task, err := s.CreateTaskWithAttachment("", "2026-09-03", "", Attachment{
		Path: "attachments/permission-slip.jpg", Markdown: "![](attachments/permission-slip.jpg)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Due == nil || *task.Due != "2026-09-03" || !strings.Contains(task.Text, "permission-slip") {
		t.Errorf("task = %+v", task)
	}
	doc, _ := s.GetDaily("2026-09-01")
	if !strings.Contains(doc.Markdown, "- [ ] permission-slip ![](attachments/permission-slip.jpg) 📅 2026-09-03\n") {
		t.Errorf("daily note = %q", doc.Markdown)
	}
}
