package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/jclement/quire/internal/vault"
)

// The Frances case end to end: a first name dangles, the person page is
// suggested, adding the alias keeps the aliases already there, and the
// name leaves the unwritten list because every link to it now resolves.
func TestAddAliasResolvesFirstNameLinks(t *testing.T) {
	s := newTestService(t)
	person, err := s.CreateDocument(vault.TypePerson, "Frances Bagley", "")
	if err != nil {
		t.Fatal(err)
	}
	// An alias written by hand as a block list — the shape a line-based
	// writer most easily corrupts.
	raw := "---\ntype: person\naliases:\n  - Fran\n---\n# Frances Bagley\n"
	if _, err := s.UpdateDocument(person.Path, raw, person.SHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDocument(vault.TypeNote, "Budget", "# Budget\n\nAsk [[Frances]] about Q3.\n"); err != nil {
		t.Fatal(err)
	}

	unwritten, err := s.UnwrittenLinks(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(unwritten) != 1 || unwritten[0].Name != "Frances" {
		t.Fatalf("unwritten = %+v", unwritten)
	}
	matches := unwritten[0].LikelyMatches
	if len(matches) != 1 || matches[0].Path != person.Path || matches[0].Reason != "first_name" {
		t.Fatalf("likely matches = %+v", matches)
	}

	doc, err := s.AddAlias(person.Path, "Frances")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Markdown, "aliases: [Fran, Frances]\n") {
		t.Errorf("aliases not appended in place:\n%s", doc.Markdown)
	}
	if got := s.Index.ResolveLink("Frances"); got != person.Path {
		t.Errorf("[[Frances]] resolves to %q", got)
	}
	if got := s.Index.ResolveLink("Fran"); got != person.Path {
		t.Errorf("existing alias lost: [[Fran]] resolves to %q", got)
	}
	unwritten, err = s.UnwrittenLinks(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(unwritten) != 0 {
		t.Errorf("still unwritten: %+v", unwritten)
	}

	// Again, in another case: already an alias, nothing rewritten.
	again, err := s.AddAlias(person.Path, "frances")
	if err != nil {
		t.Fatal(err)
	}
	if again.SHA256 != doc.SHA256 {
		t.Errorf("re-adding an alias rewrote the file")
	}
}

func TestAddAliasRejectsUnlinkableText(t *testing.T) {
	s := newTestService(t)
	person, err := s.CreateDocument(vault.TypePerson, "James Burke", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"", "  ", "James|JB", "[[James]]", "line\nbreak"} {
		if _, err := s.AddAlias(person.Path, alias); !errors.Is(err, ErrValidation) {
			t.Errorf("AddAlias(%q) = %v, want a validation error", alias, err)
		}
	}
	if _, err := s.AddAlias("people/nobody.md", "Nobody"); !errors.Is(err, vault.ErrNotFound) {
		t.Errorf("missing document: %v", err)
	}
}
