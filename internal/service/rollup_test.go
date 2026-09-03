// The entity rollup: "what am I still owed about this person" has to work
// when the action items were written the way the templates write them —
// under a heading, with the names only in frontmatter.
package service

import (
	"strings"
	"testing"
)

func taskTextsOf(tasks []Task) string {
	var out []string
	for _, t := range tasks {
		out = append(out, t.Text)
	}
	return strings.Join(out, ",")
}

func TestTasksInheritTheirDocumentsEntities(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"people/sarah-chen.md": "---\ncompany: \"[[Acme]]\"\n---\n# Sarah Chen\n\n- [ ] her own page task\n",
		"companies/acme.md":    "# Acme\n",
		"meetings/2026-09-01-sync.md": "---\npeople: [\"[[Sarah Chen]]\"]\ncompany: \"[[Acme]]\"\n" +
			"date: 2026-09-01T09:00\n---\n# Sync\n\n## Action items\n\n- [ ] send the deck\n- [x] already done\n",
		"notes/unrelated.md": "# Unrelated\n\n- [ ] nothing to do with anyone\n",
	})

	sarah, err := svc.GetDocument("people/sarah-chen.md")
	if err != nil {
		t.Fatal(err)
	}
	// The action item never names Sarah; the meeting's frontmatter does.
	if s := taskTextsOf(sarah.OpenTasks); !strings.Contains(s, "send the deck") {
		t.Errorf("open tasks for Sarah = %q, want the meeting's action item", s)
	}
	// Completed work and unrelated tasks stay out...
	if s := taskTextsOf(sarah.OpenTasks); strings.Contains(s, "already done") || strings.Contains(s, "nothing to do") {
		t.Errorf("open tasks for Sarah = %q", s)
	}
	// ...and so do her own, which the page already lists inline.
	if s := taskTextsOf(sarah.OpenTasks); strings.Contains(s, "her own page task") {
		t.Errorf("a document's own tasks must not repeat in its rollup: %q", s)
	}

	// The company gets it too, through the same meeting.
	acme, err := svc.GetDocument("companies/acme.md")
	if err != nil {
		t.Fatal(err)
	}
	if s := taskTextsOf(acme.OpenTasks); !strings.Contains(s, "send the deck") {
		t.Errorf("open tasks for Acme = %q", s)
	}
	// Sarah's own page names Acme in frontmatter, so her task counts as Acme's.
	if s := taskTextsOf(acme.OpenTasks); !strings.Contains(s, "her own page task") {
		t.Errorf("open tasks for Acme = %q, want the task from Sarah's page", s)
	}

	// A plain note is not an entity and carries no rollup.
	note, err := svc.GetDocument("notes/unrelated.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(note.OpenTasks) != 0 {
		t.Errorf("a note should have no rollup, got %q", taskTextsOf(note.OpenTasks))
	}
}

func TestSearchByDateAndCompletion(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"notes/shipped.md": "# Shipped\n\n- [x] shipped the thing ✅ 2026-09-01\n- [x] older win ✅ 2026-06-01\n- [ ] still open\n",
	})

	// "What did I finish this week", the Friday-afternoon question.
	hits, err := svc.Search("is:done after:2026-08-28", 20)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, h := range hits {
		titles = append(titles, h.Title)
	}
	got := strings.Join(titles, ",")
	if !strings.Contains(got, "shipped the thing") {
		t.Errorf("is:done after: = %q, want this week's completion", got)
	}
	if strings.Contains(got, "older win") || strings.Contains(got, "still open") {
		t.Errorf("is:done after: = %q, want only recent completions", got)
	}

	// Open-task search is unchanged and still excludes completed work.
	open, err := svc.Search("is:task", 20)
	if err != nil {
		t.Fatal(err)
	}
	titles = nil
	for _, h := range open {
		titles = append(titles, h.Title)
	}
	if got := strings.Join(titles, ","); !strings.Contains(got, "still open") || strings.Contains(got, "shipped the thing") {
		t.Errorf("is:task = %q", got)
	}
}
