// Inline decisions: the bullets under a heading named "Decisions" — how a
// meeting note records what was settled in the room — and any bullet or
// line tagged #decision, wherever it is written. Plural on purpose: a
// decision record (templates/decision.md) has a singular "## Decision"
// section that *is* the document's decision, and the record is already
// counted as a whole, so reading that section too would list it twice.
package markdown

import (
	"regexp"
	"strings"

	"github.com/jclement/quire/internal/vault"
)

// Decision is one bullet under a Decisions heading.
type Decision struct {
	Line  int    // 1-based line in the full document
	Text  string // the bullet's text, markdown intact (wikilinks and all)
	Links []Link // wikilinks inside the bullet
}

// decisionsHeading is the section title, matched case-insensitively at any
// heading level; a trailing colon ("## Decisions:") is tolerated.
const decisionsHeading = "decisions"

var (
	headingRe = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	// A top-level bullet or numbered item. Indented lines are the
	// elaboration of the decision above them, not decisions of their own.
	bulletRe = regexp.MustCompile(`^(?:[-*+]|\d+[.)])\s+(.*\S)\s*$`)
	// A checkbox, even an empty template one ("- [ ] ") or a cancelled one
	// ("- [-] "), is a task.
	checkboxRe = regexp.MustCompile(`^\[[ xX-]\](\s|$)`)
)

// ScanDecisions extracts the bullets under every "Decisions" heading. The
// section runs to the next heading of the same or a higher level, so
// "### Pricing" inside "## Decisions" stays part of it. Empty template
// bullets ("- ") and checkboxes (those are tasks) are skipped, as is
// anything in a fenced block.
func ScanDecisions(raw []byte) []Decision {
	_, body, hasFM := vault.SplitFrontmatter(raw)
	offset := 0
	if hasFM {
		offset = strings.Count(string(raw[:len(raw)-len(body)]), "\n")
	}

	var out []Decision
	inFence := false
	sectionLevel := 0 // 0 = not inside a Decisions section
	for i, line := range strings.Split(string(body), "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			switch {
			case isDecisionsHeading(m[2]):
				sectionLevel = level
			case sectionLevel > 0 && level <= sectionLevel:
				sectionLevel = 0
			}
			continue
		}
		text, ok := decisionText(line, sectionLevel > 0)
		if !ok {
			continue
		}
		lineNo := offset + i + 1
		d := Decision{Line: lineNo, Text: text}
		for _, lm := range wikilinkRe.FindAllStringSubmatch(text, -1) {
			d.Links = append(d.Links, Link{Raw: strings.TrimSpace(lm[1]), Display: strings.TrimSpace(lm[2]), Line: lineNo})
		}
		out = append(out, d)
	}
	return out
}

// decisionText reads one line as a decision: any top-level bullet inside a
// Decisions section, or — anywhere — a bullet or line carrying a #decision
// tag, which is the quickest way to log one from a daily note. The tag is
// dropped from the text. Checkboxes are tasks, whatever they carry.
func decisionText(line string, inSection bool) (string, bool) {
	tagged := hasDecisionTag(line)
	if !inSection && !tagged {
		return "", false
	}
	text := strings.TrimSpace(line)
	if m := bulletRe.FindStringSubmatch(line); m != nil {
		text = m[1]
	} else if inSection && !tagged {
		return "", false // prose between a section's bullets is not a decision
	}
	if checkboxRe.MatchString(text) || taskRe.MatchString(line) {
		return "", false
	}
	if tagged {
		text = strings.Join(strings.Fields(tagRe.ReplaceAllStringFunc(text, func(m string) string {
			if isDecisionTag(m) {
				return " "
			}
			return m
		})), " ")
	}
	return text, text != ""
}

func hasDecisionTag(line string) bool {
	for _, m := range tagRe.FindAllString(line, -1) {
		if isDecisionTag(m) {
			return true
		}
	}
	return false
}

// isDecisionTag reports whether a tagRe match is #decision exactly — not
// #decisions or #decision-log, which are tags of their own.
func isDecisionTag(match string) bool {
	return strings.EqualFold(strings.TrimLeft(strings.TrimSpace(match), "#"), decisionTagName)
}

// decisionTagName is the tag that marks a line as a decision (in the body)
// or a document as a decision record (in frontmatter tags:).
const decisionTagName = "decision"

func isDecisionsHeading(title string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(title), ":"), decisionsHeading)
}
