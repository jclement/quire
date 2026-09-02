package service

import (
	"strings"
	"testing"

	"github.com/jclement/quire/internal/vault"
)

// TestAreasPartitionEverythingButDaily is the model in one test: an area is
// a frontmatter key, discovered from the vault and merged with the seeds;
// every view can be narrowed to one; "none" means unclassified; and daily
// notes belong to every area rather than to none.
func TestAreasPartitionEverythingButDaily(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"notes/roadmap.md":               "---\narea: Work\n---\n# Roadmap\n\n- [ ] ship it 📅 2026-09-01\n",
		"notes/holiday.md":               "---\narea: personal\n---\n# Holiday\n\n- [ ] book flights 📅 2026-09-01\n",
		"notes/idea.md":                  "# Idea\n\n- [ ] unfiled thought\n",
		"daily/2026-09-01.md":            "# 2026-09-01\n\n- [ ] daily task 📅 2026-09-01\n",
		"meetings/2026-09-01-standup.md": "---\narea: work\ndate: 2026-09-01T09:00\n---\n# Standup\n",
	})

	// Discovery merges with the seeds, and "Work" normalizes to "work".
	areas, err := svc.Areas()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, a := range areas {
		got[a.Area] = a.Count
	}
	if got["work"] != 2 || got["personal"] != 1 {
		t.Errorf("area counts = %v", got)
	}
	if _, seeded := got["personal"]; !seeded {
		t.Error("seed areas must always be offered")
	}

	titles := func(docs []DocMeta) string {
		var out []string
		for _, d := range docs {
			out = append(out, d.Title)
		}
		return strings.Join(out, ",")
	}
	work, _ := svc.ListDocuments("", "", "work", 50)
	if s := titles(work); !strings.Contains(s, "Roadmap") || !strings.Contains(s, "Standup") || strings.Contains(s, "Holiday") {
		t.Errorf("work docs = %s", s)
	}
	// Unclassified: no area, and not a daily note.
	none, _ := svc.ListDocuments("", "", "none", 50)
	if s := titles(none); s != "Idea" {
		t.Errorf("unclassified docs = %s, want just Idea", s)
	}
	// All: everything, dailies included.
	all, _ := svc.ListDocuments("", "", "", 50)
	if len(all) != 5 {
		t.Errorf("all = %d docs", len(all))
	}

	// Tasks follow their document's area; the daily task is in every area.
	taskTexts := func(tasks []Task) string {
		var out []string
		for _, task := range tasks {
			out = append(out, task.Text)
		}
		return strings.Join(out, ",")
	}
	workTasks, _ := svc.TasksIn("today", "work")
	if s := taskTexts(workTasks); !strings.Contains(s, "ship it") || strings.Contains(s, "book flights") {
		t.Errorf("work tasks = %s", s)
	}
	if s := taskTexts(workTasks); strings.Contains(s, "daily task") {
		t.Errorf("a daily-note task must not appear under a single area: %s", s)
	}
	noneTasks, _ := svc.TasksIn("inbox", "none")
	if s := taskTexts(noneTasks); s != "unfiled thought" {
		t.Errorf("unclassified inbox = %s", s)
	}

	// Today narrows meetings and tasks too.
	today, err := svc.TodayIn("personal")
	if err != nil {
		t.Fatal(err)
	}
	if len(today.Meetings) != 0 || taskTexts(today.DueToday) != "book flights" {
		t.Errorf("personal today = meetings %d, due %s", len(today.Meetings), taskTexts(today.DueToday))
	}

	// The search grammar knows area: too.
	hits, err := svc.Search("area:work", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Errorf("area:work search = %d hits", len(hits))
	}
}

// TestCreateDocumentFilesUnderArea: a document created in an area carries
// it as frontmatter, so the filesystem stays the source of truth.
func TestCreateDocumentFilesUnderArea(t *testing.T) {
	svc := newTestService(t)
	doc, err := svc.CreateDocumentIn(vault.TypeNote, "Filed Note", "", "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Area != "personal" {
		t.Errorf("area = %q, want personal (normalized)", doc.Area)
	}
	if !strings.Contains(doc.Markdown, "area: personal") {
		t.Errorf("area not written to frontmatter:\n%s", doc.Markdown)
	}
	// "none" and "" both mean unclassified and write nothing.
	for _, none := range []string{"", "none"} {
		d, err := svc.CreateDocumentIn(vault.TypeNote, "Unfiled "+none, "", none)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(d.Markdown, "area:") {
			t.Errorf("unclassified doc got an area key:\n%s", d.Markdown)
		}
	}
}
