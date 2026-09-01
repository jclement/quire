package service

import (
	"strings"
	"testing"
)

func TestParseRecur(t *testing.T) {
	cases := []struct {
		spec     string
		n        int
		unit     string
		whenDone bool
		ok       bool
	}{
		{"every year", 1, "year", false, true},
		{"every 3 months", 3, "month", false, true},
		{"every 2 weeks when done", 2, "week", true, true},
		{"every day", 1, "day", false, true},
		{"whenever", 0, "", false, false},
		{"every 3 fortnights", 0, "", false, false},
	}
	for _, tc := range cases {
		n, unit, whenDone, err := parseRecur(tc.spec)
		if tc.ok != (err == nil) {
			t.Errorf("%q: err = %v", tc.spec, err)
			continue
		}
		if tc.ok && (n != tc.n || unit != tc.unit || whenDone != tc.whenDone) {
			t.Errorf("%q = (%d, %s, %v)", tc.spec, n, unit, whenDone)
		}
	}
}

// The annual-renewal case: due date advances a year, and the three-week lead
// time (due − defer) is preserved on the next occurrence.
func TestRecurringTaskWithLeadTime(t *testing.T) {
	s := newTestService(t) // clock pinned to 2026-09-01
	original := "# Life admin\n\n- [ ] Renew car registration 🔁 every year 🛫 2026-08-10 📅 2026-08-31\n"
	if _, err := s.UpdateDocument("notes/admin.md", original, ""); err != nil {
		t.Fatal(err)
	}
	doc, err := s.GetDocument("notes/admin.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Tasks) != 1 || doc.Tasks[0].Recur == nil || *doc.Tasks[0].Recur != "every year" {
		t.Fatalf("tasks = %+v", doc.Tasks)
	}

	if _, err := s.ToggleTask(doc.Tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetDocument("notes/admin.md")
	if err != nil {
		t.Fatal(err)
	}
	wantCompleted := "- [x] Renew car registration 🔁 every year 🛫 2026-08-10 📅 2026-08-31 ✅ 2026-09-01"
	wantNext := "- [ ] Renew car registration 🔁 every year 🛫 2027-08-10 📅 2027-08-31"
	if !strings.Contains(after.Markdown, wantCompleted) {
		t.Errorf("completed line missing:\n%s", after.Markdown)
	}
	if !strings.Contains(after.Markdown, wantNext) {
		t.Errorf("next occurrence wrong (lead time must be preserved):\n%s", after.Markdown)
	}
}

// The furnace-filter case: "when done" repeats from the completion day.
func TestRecurWhenDone(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("notes/house.md", "- [ ] Change furnace filter 🔁 every 3 months when done 📅 2026-05-01\n", ""); err != nil {
		t.Fatal(err)
	}
	doc, _ := s.GetDocument("notes/house.md")
	if _, err := s.ToggleTask(doc.Tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetDocument("notes/house.md")
	// Completed 2026-09-01 (late) → next is Dec 1, not Aug 1.
	if !strings.Contains(after.Markdown, "- [ ] Change furnace filter 🔁 every 3 months when done 📅 2026-12-01") {
		t.Errorf("when-done recurrence:\n%s", after.Markdown)
	}
}

func TestRecurWithoutDueGetsOne(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("notes/x.md", "- [ ] Water plants 🔁 every week\n", ""); err != nil {
		t.Fatal(err)
	}
	doc, _ := s.GetDocument("notes/x.md")
	if _, err := s.ToggleTask(doc.Tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetDocument("notes/x.md")
	if !strings.Contains(after.Markdown, "- [ ] Water plants 🔁 every week 📅 2026-09-08") {
		t.Errorf("undated recurrence:\n%s", after.Markdown)
	}
}

func TestEditTaskSnoozeAndClear(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("notes/t.md", "prose\n\n- [ ] Call plumber 📅 2026-09-01 ⏫\n", ""); err != nil {
		t.Fatal(err)
	}
	doc, _ := s.GetDocument("notes/t.md")
	id := doc.Tasks[0].ID

	// Snooze to Friday.
	newDue := "2026-09-04"
	task, err := s.EditTask(id, TaskEdit{Due: &newDue})
	if err != nil {
		t.Fatal(err)
	}
	if task.Due == nil || *task.Due != newDue {
		t.Errorf("snoozed = %+v", task)
	}
	after, _ := s.GetDocument("notes/t.md")
	if !strings.Contains(after.Markdown, "- [ ] Call plumber 📅 2026-09-04 ⏫\n") {
		t.Errorf("snooze rewrite:\n%q", after.Markdown)
	}

	// Clear the due date and priority; add a defer.
	empty, deferDate, none := "", "2026-09-10", 0
	task, err = s.EditTask(task.ID, TaskEdit{Due: &empty, Defer: &deferDate, Priority: &none})
	if err != nil {
		t.Fatal(err)
	}
	if task.Due != nil || task.Defer == nil || *task.Defer != deferDate || task.Priority != 0 {
		t.Errorf("cleared = %+v", task)
	}
	after, _ = s.GetDocument("notes/t.md")
	if !strings.Contains(after.Markdown, "- [ ] Call plumber 🛫 2026-09-10\n") {
		t.Errorf("clear rewrite:\n%q", after.Markdown)
	}

	// Bad date rejected.
	bad := "friday"
	if _, err := s.EditTask(task.ID, TaskEdit{Due: &bad}); err == nil {
		t.Errorf("invalid date accepted")
	}
}

func TestRenameRewritesLinks(t *testing.T) {
	s := newTestService(t)
	// A file whose name doesn't match its title (external creation): links
	// by the filename stem die on rename; links by title keep resolving.
	if _, err := s.UpdateDocument("projects/plan.md", "# Reporting Plan\n\nThe plan.\n", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDocument("notes/refs.md", "See [[plan]] and [[plan|the project]].\n", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDocument("notes/bytitle.md", "See [[Reporting Plan]].\n", ""); err != nil {
		t.Fatal(err)
	}

	result, err := s.RenameDocument("projects/plan.md", "projects/plan-2026.md", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Document.Path != "projects/plan-2026.md" {
		t.Errorf("moved doc = %+v", result.Document.DocMeta)
	}
	if s.Vault.Exists("projects/plan.md") {
		t.Errorf("old file still exists")
	}
	if len(result.Rewritten) != 1 || result.Rewritten[0] != "notes/refs.md" {
		t.Errorf("rewritten list = %v", result.Rewritten)
	}

	refs, _ := s.GetDocument("notes/refs.md")
	want := "See [[plan-2026|plan]] and [[plan-2026|the project]].\n"
	if refs.Markdown != want {
		t.Errorf("rewritten:\ngot  %q\nwant %q", refs.Markdown, want)
	}

	// Title-based link was never broken, so it must be untouched — and must
	// still resolve to the moved document. Rewriting it would be churn.
	bytitle, _ := s.GetDocument("notes/bytitle.md")
	if bytitle.Markdown != "See [[Reporting Plan]].\n" {
		t.Errorf("title link rewritten unnecessarily: %q", bytitle.Markdown)
	}
	if got := s.Index.ResolveLink("Reporting Plan"); got != "projects/plan-2026.md" {
		t.Errorf("title resolution after move = %q", got)
	}
}
