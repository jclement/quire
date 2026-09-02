package service

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/vault"
)

var fixedNow = time.Date(2026, 9, 1, 10, 30, 0, 0, time.Local)

func newTestService(t *testing.T) *Service {
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
	s := New(v, &index.Index{DB: db, Vault: v})
	s.Now = func() time.Time { return fixedNow }
	return s
}

func TestCreateAndGetDocument(t *testing.T) {
	s := newTestService(t)

	doc, err := s.CreateDocument(vault.TypePerson, "Sarah Chen", "")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != "people/sarah-chen.md" || doc.Type != "person" {
		t.Errorf("created doc = %+v", doc.DocMeta)
	}

	// Duplicate title gets a suffixed path, not a conflict.
	dup, err := s.CreateDocument(vault.TypePerson, "Sarah Chen", "")
	if err != nil {
		t.Fatal(err)
	}
	if dup.Path != "people/sarah-chen-2.md" {
		t.Errorf("dup path = %q", dup.Path)
	}

	// A meeting linking Sarah shows up in her backlinks.
	if _, err := s.CreateDocument(vault.TypeMeeting, "Acme Sync", "# Acme Sync\n\nWith [[Sarah Chen]].\n"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDocument("people/sarah-chen.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Backlinks) != 1 || got.Backlinks[0].Path != "meetings/2026-09-01-acme-sync.md" {
		t.Errorf("backlinks = %+v", got.Backlinks)
	}
}

func TestUpdateDocumentConflict(t *testing.T) {
	s := newTestService(t)
	doc, err := s.CreateDocument(vault.TypeNote, "Test", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDocument(doc.Path, "new content\n", "stale-hash"); !errors.Is(err, vault.ErrConflict) {
		t.Errorf("stale write: got %v, want ErrConflict", err)
	}
	if _, err := s.UpdateDocument(doc.Path, "new content\n", doc.SHA256); err != nil {
		t.Errorf("valid write: %v", err)
	}
}

func TestCreateTaskLandsInDailyAndInbox(t *testing.T) {
	s := newTestService(t)

	task, err := s.CreateTask("Call the dentist", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if task.DocPath != "daily/2026-09-01.md" || task.Text != "Call the dentist" {
		t.Errorf("task = %+v", task)
	}

	inbox, err := s.Tasks("inbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0].ID != task.ID {
		t.Errorf("inbox = %+v", inbox)
	}

	// With a due date it goes to today's list, not inbox.
	dated, err := s.CreateTask("Buy cake", "2026-09-01", "")
	if err != nil {
		t.Fatal(err)
	}
	if dated.Due == nil || *dated.Due != "2026-09-01" {
		t.Errorf("dated task = %+v", dated)
	}
	inbox, _ = s.Tasks("inbox")
	if len(inbox) != 1 {
		t.Errorf("inbox after dated capture = %+v", inbox)
	}
}

// The sync-trust test: toggling from a view surgically edits exactly one line
// of the markdown, and toggling back restores it byte-identically.
func TestToggleTaskSurgical(t *testing.T) {
	s := newTestService(t)
	original := "# Notes\n\nsome prose  \n\n- [ ] First task 📅 2026-09-02\n- [ ] Second task\n\ntrailing prose\n"
	if _, err := s.UpdateDocument("notes/t.md", original, ""); err != nil {
		t.Fatal(err)
	}

	doc, err := s.GetDocument("notes/t.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Tasks) != 2 {
		t.Fatalf("tasks = %+v", doc.Tasks)
	}

	toggled, err := s.ToggleTask(doc.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !toggled.Done || toggled.CompletedOn == nil || *toggled.CompletedOn != "2026-09-01" {
		t.Errorf("toggled = %+v", toggled)
	}

	after, err := s.GetDocument("notes/t.md")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, "- [ ] First task 📅 2026-09-02", "- [x] First task 📅 2026-09-02 ✅ 2026-09-01", 1)
	if after.Markdown != want {
		t.Errorf("after complete:\ngot  %q\nwant %q", after.Markdown, want)
	}

	// Toggle back: byte-identical to the original.
	reopened, err := s.ToggleTask(toggled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Done {
		t.Errorf("reopened still done: %+v", reopened)
	}
	final, _ := s.GetDocument("notes/t.md")
	if final.Markdown != original {
		t.Errorf("reopen not byte-identical:\ngot  %q\nwant %q", final.Markdown, original)
	}
}

func TestToggleSurvivesLineShift(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("notes/t.md", "- [ ] Movable task\n", ""); err != nil {
		t.Fatal(err)
	}
	doc, _ := s.GetDocument("notes/t.md")
	id := doc.Tasks[0].ID

	// Simulate an external edit inserting lines above, without reindexing
	// (index still has the old line hint).
	f, _ := s.Vault.Read("notes/t.md")
	if _, err := s.Vault.Write("notes/t.md", []byte("# New heading\n\nprose\n\n- [ ] Movable task\n"), f.SHA256); err != nil {
		t.Fatal(err)
	}

	toggled, err := s.ToggleTask(id)
	if err != nil {
		t.Fatal(err)
	}
	if !toggled.Done {
		t.Errorf("toggle after shift failed: %+v", toggled)
	}
}

func TestTodayPayload(t *testing.T) {
	s := newTestService(t)

	if _, err := s.CreateDocument(vault.TypeMeeting, "Standup", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTask("Overdue thing", "2026-08-30", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTask("Due today", "2026-09-01", ""); err != nil {
		t.Fatal(err)
	}

	today, err := s.Today()
	if err != nil {
		t.Fatal(err)
	}
	if today.Date != "2026-09-01" {
		t.Errorf("date = %q", today.Date)
	}
	if len(today.Meetings) != 1 {
		t.Errorf("meetings = %+v", today.Meetings)
	}
	if len(today.Overdue) != 1 || today.Overdue[0].Text != "Overdue thing" {
		t.Errorf("overdue = %+v", today.Overdue)
	}
	if len(today.DueToday) != 1 || today.DueToday[0].Text != "Due today" {
		t.Errorf("due today = %+v", today.DueToday)
	}
	// CreateTask created the daily note, so it should be present.
	if today.Daily == nil {
		t.Errorf("daily missing")
	}
}

// A caller-supplied body may already carry frontmatter — agents do this
// routinely because create_document invites passing markdown. Seeding must
// fill in only what's missing, never prepend a second `---` block (which
// rendered as body text and shadowed the real values).
func TestCreateDocumentRespectsSuppliedFrontmatter(t *testing.T) {
	s := newTestService(t)
	body := "---\ntype: meeting\ndate: 2026-09-01T14:00\n---\n# Acme Sync\n\nNotes.\n"
	doc, err := s.CreateDocument(vault.TypeMeeting, "Acme Sync", body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(doc.Markdown, "---\n") != 2 {
		t.Errorf("expected exactly one frontmatter block:\n%s", doc.Markdown)
	}
	// The caller's date wins; the missing seed key is added.
	if got := doc.Frontmatter["date"]; got != "2026-09-01T14:00" {
		t.Errorf("date = %v, want the caller's 14:00", got)
	}
	if _, ok := doc.Frontmatter["people"]; !ok {
		t.Errorf("missing seed key not added: %v", doc.Frontmatter)
	}
	// And the body is the body — no YAML leaked into it.
	if strings.Contains(doc.Markdown, "---\n---") || !strings.Contains(doc.Markdown, "# Acme Sync") {
		t.Errorf("body mangled:\n%s", doc.Markdown)
	}

	// A body with no frontmatter still gets the full seed block.
	plain, err := s.CreateDocument(vault.TypeMeeting, "Standup", "# Standup\n")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Frontmatter["date"] == nil || plain.Frontmatter["people"] == nil {
		t.Errorf("seed missing: %v", plain.Frontmatter)
	}
}

// TestToggleTaskAnyMarker: the indexer accepts -, * and + bullets, so the
// toggle must too. A `* [ ]` task was clickable and did nothing — the
// bug behind "checking the box in read mode does nothing".
func TestToggleTaskAnyMarker(t *testing.T) {
	s := newTestService(t)
	doc, err := s.CreateDocument(vault.TypeNote, "Markers", "* [ ] star task\n+ [ ] plus task\n- [ ] dash task\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Tasks) != 3 {
		t.Fatalf("indexed %d tasks, want 3", len(doc.Tasks))
	}
	for _, task := range doc.Tasks {
		done, err := s.ToggleTask(task.ID)
		if err != nil {
			t.Fatalf("toggle %q: %v", task.Text, err)
		}
		if !done.Done || done.CompletedOn == nil {
			t.Errorf("%q should be done after one toggle: %+v", task.Text, done)
		}
	}
	f, _ := s.Vault.Read(doc.Path)
	for _, want := range []string{"* [x] star task ✅ ", "+ [x] plus task ✅ ", "- [x] dash task ✅ "} {
		if !strings.Contains(string(f.Raw), want) {
			t.Errorf("file should contain %q:\n%s", want, f.Raw)
		}
	}
	// And back again, keeping each line's own marker.
	after, _ := s.GetDocument(doc.Path)
	for _, task := range after.Tasks {
		if _, err := s.ToggleTask(task.ID); err != nil {
			t.Fatal(err)
		}
	}
	f, _ = s.Vault.Read(doc.Path)
	if string(f.Raw) != "* [ ] star task\n+ [ ] plus task\n- [ ] dash task\n" {
		t.Errorf("reopening should restore the original lines, got:\n%s", f.Raw)
	}
}
