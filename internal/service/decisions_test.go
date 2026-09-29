package service

import (
	"errors"
	"testing"

	"github.com/jclement/quire/internal/vault"
)

// A meeting from the starter template carries a Decisions section, and
// what is written there reaches the log attributed to the room's people.
func TestMeetingTemplateDecisionsReachTheLog(t *testing.T) {
	s := newTestService(t)
	if _, err := s.InstallStarterTemplates(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDocument(vault.TypePerson, "Sarah Chen", ""); err != nil {
		t.Fatal(err)
	}
	meeting, err := s.CreateDocument(vault.TypeMeeting, "Pricing sync", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkEntity(meeting.Path, "people", "Sarah Chen"); err != nil {
		t.Fatal(err)
	}
	// Nothing decided yet: the template's empty bullet is not a decision.
	if got, err := s.ListDecisions(DecisionQuery{}); err != nil || len(got) != 0 {
		t.Fatalf("empty template produced decisions: %+v, %v", got, err)
	}
	if _, err := s.AppendToDocument(meeting.Path, "- Keep the rate card at v3", "Decisions"); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDecisions(DecisionQuery{Entity: "Sarah Chen"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "Keep the rate card at v3" || got[0].Path != meeting.Path ||
		got[0].Date != "2026-09-01" || len(got[0].Entities) != 1 || got[0].Entities[0].Path != "people/sarah-chen.md" {
		t.Errorf("decisions about Sarah = %+v", got)
	}

	if _, err := s.ListDecisions(DecisionQuery{Entity: "Nobody At All"}); !errors.Is(err, ErrValidation) {
		t.Errorf("unknown entity: %v", err)
	}
}

// A record from the decision template is dated by its frontmatter — the
// day it was written — not by whenever the file was last touched.
func TestDecisionRecordFromTemplate(t *testing.T) {
	s := newTestService(t)
	if _, err := s.InstallStarterTemplates(); err != nil {
		t.Fatal(err)
	}
	templates, err := s.Templates()
	if err != nil {
		t.Fatal(err)
	}
	for _, tpl := range templates {
		if tpl.Path == "templates/decision.md" && tpl.For != "note" {
			t.Errorf("decision template's frontmatter no longer parses: %+v", tpl)
		}
	}
	doc, err := s.CreateDocumentWith(vault.TypeNote, "Adopt Postgres", "", CreateOptions{Template: "decision"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ListDecisions(DecisionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != "record" || got[0].Path != doc.Path || got[0].Text != "Adopt Postgres" || got[0].Date != "2026-09-01" {
		t.Errorf("decision records = %+v (document:\n%s)", got, doc.Markdown)
	}
}
