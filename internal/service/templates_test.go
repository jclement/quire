package service

import (
	"strings"
	"testing"

	"github.com/jclement/quire/internal/vault"
)

func TestTemplatesShapeNewDocuments(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		// The meeting default: applies automatically.
		"templates/meeting.md": "---\ndescription: Standard meeting\n---\n# {{title}}\n\nHeld {{date}} at {{time}}.\n\n## Action items\n\n- [ ] \n",
		// A named template for notes, carrying frontmatter the new doc inherits.
		"templates/decision.md": "---\nfor: note\ndescription: ADR\ntags: [decision]\n---\n# {{title}}\n\n## Context\n",
		// The daily default.
		"templates/daily.md": "# {{date}}\n\n## Focus\n\n- \n",
	})

	templates, err := svc.Templates()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]TemplateInfo{}
	for _, tpl := range templates {
		byName[tpl.Name] = tpl
	}
	if !byName["meeting"].Default || byName["meeting"].For != "meeting" {
		t.Errorf("meeting template = %+v, want the meeting default", byName["meeting"])
	}
	if byName["decision"].Default || byName["decision"].For != "note" || byName["decision"].Description != "ADR" {
		t.Errorf("decision template = %+v", byName["decision"])
	}

	// A meeting created with no body takes the default and expands it.
	meeting, err := svc.CreateDocumentWith(vault.TypeMeeting, "Acme Sync", "", CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(meeting.Markdown, "# Acme Sync") || !strings.Contains(meeting.Markdown, "Held "+svc.today()) {
		t.Errorf("meeting body not templated:\n%s", meeting.Markdown)
	}
	if strings.Contains(meeting.Markdown, "{{") {
		t.Errorf("unexpanded placeholder:\n%s", meeting.Markdown)
	}
	// The template's own metadata is not copied, the type's seed is kept.
	if strings.Contains(meeting.Markdown, "description:") || !strings.Contains(meeting.Markdown, "date:") {
		t.Errorf("frontmatter wrong:\n%s", meeting.Markdown)
	}

	// A named template: its frontmatter is inherited.
	adr, err := svc.CreateDocumentWith(vault.TypeNote, "Use SQLite", "", CreateOptions{Template: "decision"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(adr.Markdown, "tags: [decision]") || !strings.Contains(adr.Markdown, "## Context") {
		t.Errorf("decision doc:\n%s", adr.Markdown)
	}
	if adr.Type != "note" {
		t.Errorf("type = %s", adr.Type)
	}

	// An explicit body always wins over any template.
	plain, err := svc.CreateDocumentWith(vault.TypeMeeting, "Quick Chat", "# Quick Chat\n\njust this\n", CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.Markdown, "Action items") {
		t.Error("a supplied body must not be replaced by the template")
	}

	// An unknown template is an error, not a silent plain note.
	if _, err := svc.CreateDocumentWith(vault.TypeNote, "X", "", CreateOptions{Template: "nope"}); err == nil {
		t.Error("unknown template should fail")
	}

	// The daily default shapes new days.
	day, err := svc.EnsureDaily("2030-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(day.Markdown, "# 2030-01-01") || !strings.Contains(day.Markdown, "## Focus") {
		t.Errorf("daily not templated:\n%s", day.Markdown)
	}

	// Templates stay out of everyday listings and search, and area "all".
	all, _ := svc.ListDocuments("", "", "", 100)
	for _, d := range all {
		if d.Type == "template" {
			t.Errorf("template %s leaked into the default listing", d.Path)
		}
	}
	hits, _ := svc.Search("decision", 10)
	for _, h := range hits {
		if strings.HasPrefix(h.Path, "templates/") {
			t.Errorf("template %s leaked into search", h.Path)
		}
	}
	only, _ := svc.ListDocuments("template", "", "", 100)
	if len(only) != 3 {
		t.Errorf("asking for templates by type returned %d", len(only))
	}
}

func TestInstallStarterTemplatesIsIdempotentAndNonDestructive(t *testing.T) {
	svc := newTestService(t)
	// A template the user already has must survive.
	writeVault(t, svc, map[string]string{"templates/decision.md": "# mine\n"})

	written, err := svc.InstallStarterTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != len(starterTemplates)-1 {
		t.Errorf("wrote %d, want %d (all but the existing one)", len(written), len(starterTemplates)-1)
	}
	mine, _ := svc.GetDocument("templates/decision.md")
	if !strings.Contains(mine.Markdown, "# mine") {
		t.Error("an existing template was overwritten")
	}
	again, _ := svc.InstallStarterTemplates()
	if len(again) != 0 {
		t.Errorf("second install wrote %d files", len(again))
	}
	// Every starter template parses and expands without leftovers.
	templates, _ := svc.Templates()
	if len(templates) != len(starterTemplates) {
		t.Errorf("listed %d templates, want %d", len(templates), len(starterTemplates))
	}
	for _, tpl := range templates {
		if tpl.For == "" {
			t.Errorf("%s declares no type", tpl.Path)
		}
	}
}
