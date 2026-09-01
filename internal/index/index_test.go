package index

import (
	"path/filepath"
	"testing"

	"github.com/jclement/quire/internal/vault"
)

// newTestIndex builds a vault + index over a temp dir with a few documents
// exercising links, tags, tasks, and entity types.
func newTestIndex(t *testing.T) *Index {
	t.Helper()
	v, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ix := &Index{DB: db, Vault: v}
	write := func(path, content string) {
		t.Helper()
		if _, err := v.Write(path, []byte(content), ""); err != nil {
			t.Fatal(err)
		}
	}

	write("people/sarah-chen.md", "---\ntype: person\naliases: [Sarah]\ncompany: \"[[Acme]]\"\n---\n# Sarah Chen\n\nRuns infra at [[Acme]].\n")
	write("companies/acme.md", "# Acme\n\nBig co. #customer\n")
	write("projects/apollo.md", "---\nstatus: active\n---\n# Project Apollo\n\n- [ ] Draft migration plan 📅 2026-09-03\n")
	write("meetings/2026-09-01-acme-sync.md", "---\ndate: 2026-09-01T14:00\npeople: [\"[[Sarah Chen]]\"]\nproject: \"[[Project Apollo]]\"\n---\n# Acme Sync\n\n- [ ] Send [[Sarah Chen]] the diagram 📅 2026-09-02\n- [ ] Chase legal ⏳\n- [x] Book room ✅ 2026-08-30\n")
	write("daily/2026-09-01.md", "- [ ] Unfiled thought\n\nCalled [[Sarah Chen]] about [[Project Apollo]].\n")

	if err := ix.FullScan(); err != nil {
		t.Fatal(err)
	}
	return ix
}

func TestFullScanAndBacklinks(t *testing.T) {
	ix := newTestIndex(t)

	docs, err := ix.ListDocuments("", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 5 {
		t.Fatalf("indexed %d docs, want 5", len(docs))
	}

	// Sarah is linked by title and resolves via docnames.
	back, err := ix.Backlinks("people/sarah-chen.md")
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, d := range back {
		paths[d.Path] = true
	}
	if !paths["meetings/2026-09-01-acme-sync.md"] || !paths["daily/2026-09-01.md"] {
		t.Errorf("backlinks = %v", paths)
	}

	// Alias resolution.
	if got := ix.ResolveLink("Sarah"); got != "people/sarah-chen.md" {
		t.Errorf("ResolveLink(Sarah) = %q", got)
	}
	// Company linked before its title case matches ([[Acme]] vs "# Acme").
	if got := ix.ResolveLink("acme"); got != "companies/acme.md" {
		t.Errorf("ResolveLink(acme) = %q", got)
	}
}

func TestTaskViews(t *testing.T) {
	ix := newTestIndex(t)
	const today = "2026-09-02"

	views := map[TaskView][]string{
		ViewInbox:   {"Unfiled thought"},
		ViewToday:   {"Send [[Sarah Chen]] the diagram"},
		ViewWaiting: {"Chase legal"},
		ViewLogbook: {"Book room"},
	}
	for view, want := range views {
		tasks, err := ix.Tasks(view, today)
		if err != nil {
			t.Fatalf("%s: %v", view, err)
		}
		var texts []string
		for _, task := range tasks {
			texts = append(texts, task.Text)
		}
		if len(texts) != len(want) {
			t.Errorf("%s = %v, want %v", view, texts, want)
			continue
		}
		for i := range want {
			if texts[i] != want[i] {
				t.Errorf("%s[%d] = %q, want %q", view, i, texts[i], want[i])
			}
		}
	}

	// Upcoming (from Sep 1): the Apollo task due Sep 3.
	up, err := ix.Tasks(ViewUpcoming, "2026-09-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(up) != 1 || up[0].Due != "2026-09-03" {
		t.Errorf("upcoming = %+v", up)
	}
	// Project rollup: the Apollo task belongs to the project via its source doc.
	if up[0].ProjectPath != "projects/apollo.md" {
		t.Errorf("project path = %q", up[0].ProjectPath)
	}
}

func TestMeetingTaskProjectRollup(t *testing.T) {
	ix := newTestIndex(t)
	// The meeting task links [[Sarah Chen]] (a person) and its doc has
	// project: [[Project Apollo]] — the project join must pick the project.
	tasks, err := ix.Tasks(ViewToday, "2026-09-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("today = %+v", tasks)
	}
	if tasks[0].ProjectPath != "projects/apollo.md" {
		t.Errorf("meeting task project = %q, want apollo", tasks[0].ProjectPath)
	}
}

func TestSearch(t *testing.T) {
	ix := newTestIndex(t)

	hits, err := ix.Search("infra", 10, "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "people/sarah-chen.md" {
		t.Errorf("search infra = %+v", hits)
	}

	hits, err = ix.Search("type:meeting sync", 10, "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Type != "meeting" {
		t.Errorf("filtered search = %+v", hits)
	}

	hits, err = ix.Search("tag:customer", 10, "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "companies/acme.md" {
		t.Errorf("tag search = %+v", hits)
	}

	// FTS syntax injection must not error.
	if _, err := ix.Search(`"unclosed AND (`, 10, "2026-09-01"); err != nil {
		t.Errorf("hostile query errored: %v", err)
	}

	// is:task searches the task index, due: filters by date.
	hits, err = ix.Search("is:task diagram", 10, "2026-09-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Type != "task" || hits[0].Path != "meetings/2026-09-01-acme-sync.md" {
		t.Errorf("task search = %+v", hits)
	}
	hits, err = ix.Search("due:today", 10, "2026-09-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Title != "Send [[Sarah Chen]] the diagram" {
		t.Errorf("due:today = %+v", hits)
	}
	hits, err = ix.Search("due:week", 10, "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Errorf("due:week = %+v", hits)
	}
}

func TestReindexIsIdempotentAndDetectsChanges(t *testing.T) {
	ix := newTestIndex(t)

	// Unchanged file: IndexFile reports no work (self-write suppression).
	changed, err := ix.IndexFile("companies/acme.md")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Errorf("unchanged file reported as changed")
	}

	// External-style change gets picked up.
	f, err := ix.Vault.Read("companies/acme.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Vault.Write("companies/acme.md", []byte("# Acme Corp\n\nRenamed.\n"), f.SHA256); err != nil {
		t.Fatal(err)
	}
	changed, err = ix.IndexFile("companies/acme.md")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Errorf("changed file not reindexed")
	}
	meta, err := ix.GetDocMeta("companies/acme.md")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Acme Corp" {
		t.Errorf("title after reindex = %q", meta.Title)
	}

	// Deleting on disk + FullScan removes rows.
	if err := ix.Vault.Delete("daily/2026-09-01.md"); err != nil {
		t.Fatal(err)
	}
	if err := ix.FullScan(); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.GetDocMeta("daily/2026-09-01.md"); err == nil {
		t.Errorf("deleted doc still indexed")
	}
}
