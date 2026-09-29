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

// ⏳ may carry the date the wait began. The date is metadata like any other
// marker's: out of the display text, and out of the id, so stamping it
// never orphans the task.
func TestScanWaitingSince(t *testing.T) {
	cases := []struct {
		name, line  string
		waiting     bool
		since, text string
	}{
		{"dated", "- [ ] Get SOC evidence from [[Frances Bagley]] ⏳ 2026-09-20", true, "2026-09-20", "Get SOC evidence from [[Frances Bagley]]"},
		{"bare", "- [ ] Chase legal ⏳", true, "", "Chase legal"},
		{"bare mid-line", "- [ ] Chase ⏳ legal 📅 2026-10-01", true, "", "Chase legal"},
		{"dated before other markers", "- [ ] Quote back ⏳ 2026-09-01 📅 2026-10-01 ⏫", true, "2026-09-01", "Quote back"},
		{"not waiting", "- [ ] Plain task 📅 2026-10-01", false, "", "Plain task"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := Scan("daily/x.md", []byte(c.line+"\n"))
			if len(doc.Tasks) != 1 {
				t.Fatalf("got %d tasks", len(doc.Tasks))
			}
			task := doc.Tasks[0]
			if task.Waiting != c.waiting || task.WaitingSince != c.since || task.Text != c.text {
				t.Errorf("got waiting=%v since=%q text=%q, want %v %q %q",
					task.Waiting, task.WaitingSince, task.Text, c.waiting, c.since, c.text)
			}
		})
	}
	bare := Scan("daily/x.md", []byte("- [ ] Chase legal ⏳\n")).Tasks[0]
	dated := Scan("daily/x.md", []byte("- [ ] Chase legal ⏳ 2026-09-20\n")).Tasks[0]
	if bare.ID != dated.ID {
		t.Errorf("stamping the waiting date changed the id: %s vs %s", bare.ID, dated.ID)
	}
}

// A wikilink immediately after ⏳ (and its optional date) is the explicit
// who. Like the date it is marker metadata: out of the display text and the
// id, so re-assigning who never orphans the task — but still a link.
func TestScanExplicitWaitingOn(t *testing.T) {
	cases := []struct {
		name, line, since, who, text string
	}{
		{"dated", "- [ ] Ask [[Frances Bagley]] for the CFO intro ⏳ 2026-09-20 [[Dan Roe]]", "2026-09-20", "Dan Roe", "Ask [[Frances Bagley]] for the CFO intro"},
		{"bare, aliased", "- [ ] Chase legal ⏳ [[Dan Roe|Dan]] 📅 2026-10-01", "", "Dan Roe", "Chase legal"},
		{"words between are not the slot", "- [ ] Quote ⏳ 2026-09-20 from [[Acme]]", "2026-09-20", "", "Quote from [[Acme]]"},
		{"no marker, no slot", "- [ ] Talk to [[Dan Roe]]", "", "", "Talk to [[Dan Roe]]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			task := Scan("daily/x.md", []byte(c.line+"\n")).Tasks[0]
			if task.WaitingSince != c.since || task.WaitingOn != c.who || task.Text != c.text {
				t.Errorf("since=%q who=%q text=%q, want %q %q %q", task.WaitingSince, task.WaitingOn, task.Text, c.since, c.who, c.text)
			}
			if c.who != "" && !slices.ContainsFunc(task.Links, func(l Link) bool { return l.Raw == c.who }) {
				t.Errorf("the explicit who should still be a link: %+v", task.Links)
			}
		})
	}
	dan := Scan("d.md", []byte("- [ ] Contract ⏳ 2026-09-20 [[Dan Roe]]\n")).Tasks[0]
	acme := Scan("d.md", []byte("- [ ] Contract ⏳ 2026-09-21 [[Acme]]\n")).Tasks[0]
	if dan.ID != acme.ID {
		t.Errorf("re-assigning who changed the id")
	}
}

// Work-item references are links, not tags: a tag needs a letter, as in
// Obsidian, so "#2433" and "AB#2433" never reach the tag index (and the
// tag page never fills up with ticket numbers).
func TestWorkItemNumbersAreNotTags(t *testing.T) {
	doc := Scan("daily/2026-09-01.md", []byte("- [ ] #2433 Auto-update of environments #ops\n\nSee AB#2440 and (#2441).\n"))
	if !slices.Equal(doc.Tags, []string{"ops"}) {
		t.Errorf("tags = %v, want only [ops]", doc.Tags)
	}
	if len(doc.Tasks) != 1 || !slices.Equal(doc.Tasks[0].Tags, []string{"ops"}) {
		t.Errorf("task tags = %+v", doc.Tasks)
	}
}
