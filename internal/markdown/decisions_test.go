package markdown

import (
	"reflect"
	"testing"
)

func TestScanDecisions(t *testing.T) {
	doc := "---\npeople: [\"[[Sarah Chen]]\"]\n---\n" + // lines 1-3
		"# Pricing sync\n" + // 4
		"\n" + // 5
		"## Notes\n" + // 6
		"- Not a decision, just a note\n" + // 7
		"## Decisions\n" + // 8
		"- Ship [[Project Apollo]] on the 12th\n" + // 9
		"  - elaboration, not its own decision\n" + // 10
		"- \n" + // 11: the template's empty bullet
		"- [ ] a task sitting in the wrong place\n" + // 12
		"### Pricing\n" + // 13: deeper heading, still inside
		"1. Rate card stays at [[Rate Card|v3]]\n" + // 14
		"```\n- fenced, ignored\n```\n" + // 15-17
		"## Action items\n" + // 18: closes the section
		"- [ ] Tell [[Sarah Chen]]\n" + // 19
		"- Not a decision either\n" + // 20
		"#### decisions:\n" + // 21: any level, any case, trailing colon
		"* Hire a second SRE\n" // 22

	got := ScanDecisions([]byte(doc))
	type row struct {
		Line  int
		Text  string
		Links []string
	}
	var rows []row
	for _, d := range got {
		var links []string
		for _, l := range d.Links {
			links = append(links, l.Raw)
		}
		rows = append(rows, row{d.Line, d.Text, links})
	}
	want := []row{
		{9, "Ship [[Project Apollo]] on the 12th", []string{"Project Apollo"}},
		{14, "Rate card stays at [[Rate Card|v3]]", []string{"Rate Card"}},
		{22, "Hire a second SRE", nil},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("decisions =\n%+v\nwant\n%+v", rows, want)
	}
}

// A decision record's own singular "## Decision" section is the record
// itself, counted once as a document — never again as an inline decision.
func TestScanDecisionsIgnoresSingularHeading(t *testing.T) {
	record := "---\ntags: [decision]\n---\n# Adopt Postgres\n\n## Context\n\n- we outgrew SQLite\n\n## Decision\n\n- Move to Postgres in Q4\n\n## Consequences\n\n- ops load\n"
	if got := ScanDecisions([]byte(record)); len(got) != 0 {
		t.Errorf("singular Decision section leaked: %+v", got)
	}
}
