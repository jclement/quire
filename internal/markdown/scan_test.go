package markdown

import (
	"slices"
	"testing"
)

const sampleDoc = `---
type: meeting
people: ["[[Sarah Chen]]"]
---
# Acme Reporting Sync

Discussed [[Project Apollo|Apollo]] with [[Sarah Chen]]. #acme #q3

- [ ] Send Sarah the architecture diagram 📅 2026-09-02 ⏫ #apollo
- [x] Book follow-up ✅ 2026-08-31
- [ ] Chase legal ⏳ [[Dan Roe]]
- [ ] Prep slides 🛫 2026-09-05

` + "```go\n// [[NotALink]] #nottag\n- [ ] not a task\n```\n"

func TestScan(t *testing.T) {
	doc := Scan("meetings/2026-08-31-acme.md", []byte(sampleDoc))

	if doc.Title != "Acme Reporting Sync" {
		t.Errorf("title = %q", doc.Title)
	}

	var linkTargets []string
	for _, l := range doc.Links {
		linkTargets = append(linkTargets, l.Raw)
	}
	// Fenced code must not contribute links.
	if slices.Contains(linkTargets, "NotALink") {
		t.Errorf("link inside fence leaked: %v", linkTargets)
	}
	if !slices.Contains(linkTargets, "Project Apollo") || !slices.Contains(linkTargets, "Sarah Chen") {
		t.Errorf("missing links: %v", linkTargets)
	}

	if slices.Contains(doc.Tags, "nottag") {
		t.Errorf("tag inside fence leaked: %v", doc.Tags)
	}
	for _, want := range []string{"acme", "q3", "apollo"} {
		if !slices.Contains(doc.Tags, want) {
			t.Errorf("missing tag %q in %v", want, doc.Tags)
		}
	}

	if len(doc.Tasks) != 4 {
		t.Fatalf("got %d tasks, want 4: %+v", len(doc.Tasks), doc.Tasks)
	}

	send := doc.Tasks[0]
	if send.Text != "Send Sarah the architecture diagram #apollo" {
		t.Errorf("task text = %q", send.Text)
	}
	if send.Due != "2026-09-02" || send.Priority != 1 || send.Done {
		t.Errorf("task meta wrong: %+v", send)
	}
	if !slices.Contains(send.Tags, "apollo") {
		t.Errorf("task tags = %v", send.Tags)
	}
	// Line numbers count frontmatter: frontmatter is lines 1-4, H1 line 5.
	if send.Line != 9 {
		t.Errorf("task line = %d, want 9", send.Line)
	}

	done := doc.Tasks[1]
	if !done.Done || done.CompletedOn != "2026-08-31" {
		t.Errorf("completed task: %+v", done)
	}

	waiting := doc.Tasks[2]
	if !waiting.Waiting {
		t.Errorf("waiting task not flagged: %+v", waiting)
	}
	if len(waiting.Links) != 1 || waiting.Links[0].Raw != "Dan Roe" {
		t.Errorf("waiting task links: %+v", waiting.Links)
	}

	deferred := doc.Tasks[3]
	if deferred.Defer != "2026-09-05" {
		t.Errorf("defer date: %+v", deferred)
	}
}

func TestTaskIDStableAcrossMoves(t *testing.T) {
	a := Scan("notes/x.md", []byte("intro\n\n- [ ] Call the dentist\n"))
	b := Scan("notes/x.md", []byte("intro\nnew line above\nmore\n\n- [ ] Call the dentist\n"))
	if a.Tasks[0].ID != b.Tasks[0].ID {
		t.Errorf("task ID changed when lines moved: %s vs %s", a.Tasks[0].ID, b.Tasks[0].ID)
	}
	c := Scan("notes/y.md", []byte("- [ ] Call the dentist\n"))
	if a.Tasks[0].ID == c.Tasks[0].ID {
		t.Errorf("task ID should be scoped by document")
	}
}

func TestScanNoFrontmatterTitleFallthrough(t *testing.T) {
	doc := Scan("notes/x.md", []byte("plain text, no heading\n"))
	if doc.Title != "" {
		t.Errorf("title = %q, want empty (caller falls back to filename)", doc.Title)
	}
}
