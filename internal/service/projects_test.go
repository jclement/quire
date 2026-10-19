package service

import (
	"errors"
	"strings"
	"testing"
)

// The board reads every project as one of five statuses, whatever an older
// vault or a person typed — without rewriting a single file to get there.
func TestNormalizeProjectStatus(t *testing.T) {
	for raw, want := range map[string]string{
		"planned": "planned", "Someday": "planned", "backlog": "planned",
		"active": "active", "In Progress": "active", "in-progress": "active",
		"hold": "hold", "On Hold": "hold", "waiting": "hold", "on_hold": "hold",
		"complete": "complete", "completed": "complete", "Done": "complete", "archived": "complete",
		"cancelled": "cancelled", "canceled": "cancelled", "dropped": "cancelled",
	} {
		if got, ok := NormalizeProjectStatus(raw); !ok || got != want {
			t.Errorf("%q → %q, %v; want %q", raw, got, ok, want)
		}
	}
	if _, ok := NormalizeProjectStatus("sideways"); ok {
		t.Error("an unknown word was read as a status")
	}
}

func projectStatusOf(t *testing.T, s *Service, path string) ProjectMeta {
	t.Helper()
	docs, err := s.ListDocuments("project", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		if d.Path == path {
			if d.Project == nil {
				t.Fatalf("%s listed without project fields", path)
			}
			return *d.Project
		}
	}
	t.Fatalf("%s not listed", path)
	return ProjectMeta{}
}

// Setting a status writes the canonical word and stamps the close date;
// reopening takes the stamp away; an unchanged status leaves it alone.
func TestSetProjectStatusStampsClosingDate(t *testing.T) {
	s := newTestService(t)
	path := "projects/apollo.md"
	original := "---\nstatus: waiting\ncompany: \"[[Acme|ACME]]\"\ndue: 2026-11-01\n---\n# Apollo\n\nBody.\n"
	if _, err := s.UpdateDocument(path, original, ""); err != nil {
		t.Fatal(err)
	}
	if got := projectStatusOf(t, s, path); got.Status != "hold" || got.Company != "ACME" || got.Due != "2026-11-01" || got.Rank != nil {
		t.Fatalf("legacy project read as %+v", got)
	}

	today := fixedNow.Format("2006-01-02")
	doc, err := s.SetFrontmatter(path, map[string]any{"status": "Done", "rank": 1500.0}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Markdown, "status: complete\n") || !strings.Contains(doc.Markdown, "completed: "+today+"\n") {
		t.Fatalf("complete:\n%s", doc.Markdown)
	}
	if !strings.HasSuffix(doc.Markdown, "---\n# Apollo\n\nBody.\n") || !strings.Contains(doc.Markdown, "company: \"[[Acme|ACME]]\"\n") {
		t.Fatalf("the rest of the file moved:\n%s", doc.Markdown)
	}
	if got := projectStatusOf(t, s, path); got.Status != "complete" || got.Closed != today || got.Rank == nil || *got.Rank != 1500 {
		t.Fatalf("after completing: %+v", got)
	}

	// The same status again (a drag within the column) keeps the date.
	if _, err := s.UpdateDocument(path, strings.Replace(doc.Markdown, "completed: "+today, "completed: 2026-01-02", 1), doc.SHA256); err != nil {
		t.Fatal(err)
	}
	doc, err = s.SetFrontmatter(path, map[string]any{"status": "complete", "rank": 900.0}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Markdown, "completed: 2026-01-02\n") {
		t.Fatalf("an unchanged status re-stamped the date:\n%s", doc.Markdown)
	}

	// Cancelling swaps the stamp; reopening removes it.
	doc, err = s.SetFrontmatter(path, map[string]any{"status": "cancelled"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc.Markdown, "completed:") || !strings.Contains(doc.Markdown, "cancelled: "+today+"\n") {
		t.Fatalf("cancel:\n%s", doc.Markdown)
	}
	doc, err = s.SetFrontmatter(path, map[string]any{"status": "active"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc.Markdown, "cancelled:") || strings.Contains(doc.Markdown, "completed:") {
		t.Fatalf("reopen:\n%s", doc.Markdown)
	}
	if doc.Project == nil || doc.Project.Status != "active" || doc.Project.Closed != "" {
		t.Fatalf("reopened document: %+v", doc.Project)
	}
}

func TestSetProjectStatusRefusesUnknownWords(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("projects/x.md", "---\nstatus: active\n---\n# X\n", ""); err != nil {
		t.Fatal(err)
	}
	_, err := s.SetFrontmatter("projects/x.md", map[string]any{"status": "sideways"}, "")
	if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), "planned, active, hold, complete, cancelled") {
		t.Fatalf("err = %v", err)
	}
	// Only projects have statuses; a note's status: is just a key.
	if _, err := s.UpdateDocument("notes/n.md", "# N\n", ""); err != nil {
		t.Fatal(err)
	}
	doc, err := s.SetFrontmatter("notes/n.md", map[string]any{"status": "sideways"}, "")
	if err != nil || !strings.Contains(doc.Markdown, "status: sideways") || doc.Project != nil {
		t.Fatalf("note: %v\n%s", err, doc.Markdown)
	}
}
