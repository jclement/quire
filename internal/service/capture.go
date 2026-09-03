// Where a capture lands inside the daily note. The starter template gives
// the day a `## Captured` section; appending to the end of the file instead
// means that once anything is typed under a later heading, every capture
// lands outside the section built to hold it.
package service

import (
	"fmt"
	"strings"
)

// CaptureNote files a fleeting thought in today's note: a line of prose
// under the same heading tasks go to. Not everything worth capturing is an
// action, and forcing an idea into a checkbox leaves it in the task inbox
// forever pretending to be one.
func (s *Service) CaptureNote(text string) (Document, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Document{}, fmt.Errorf("%w: nothing to capture", ErrValidation)
	}
	// One line: the daily note is a list, and a wall of text belongs in a
	// document of its own.
	text = strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " ")), " ")

	daily, err := s.EnsureDaily(s.today())
	if err != nil {
		return Document{}, err
	}
	content := appendUnderHeading(daily.Markdown, captureHeading, "- "+text)
	return s.UpdateDocument(daily.Path, content, daily.SHA256)
}

// captureHeading is the section quick capture writes into when the day's
// note has one. Matched case-insensitively, at any heading level.
const captureHeading = "captured"

// appendUnderHeading inserts line at the end of the named section, or at the
// end of the document when there is no such heading. The section ends at the
// next heading of the same or a higher level, and trailing blank lines stay
// below the insert so the shape of the note survives.
func appendUnderHeading(content, heading, line string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	start, level := -1, 0
	inFence := false
	for i, text := range lines {
		if strings.HasPrefix(strings.TrimSpace(text), "```") || strings.HasPrefix(strings.TrimSpace(text), "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(text, "#") {
			continue
		}
		hashes := len(text) - len(strings.TrimLeft(text, "#"))
		title := strings.ToLower(strings.TrimSpace(text[hashes:]))
		if start < 0 && title == heading {
			start, level = i, hashes
			continue
		}
		// The next heading at the same or a higher level closes the section.
		if start >= 0 && hashes <= level {
			return join(insertAt(lines, backUpOverBlanks(lines, i), line))
		}
	}
	if start < 0 {
		// No such section: append at the end, leaving the note's own
		// trailing shape alone.
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + line + "\n"
	}
	return join(insertAt(lines, backUpOverBlanks(lines, len(lines)), line))
}

// backUpOverBlanks moves an insert point above the blank lines that separate
// a section from what follows it.
func backUpOverBlanks(lines []string, at int) int {
	for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	return at
}

func insertAt(lines []string, at int, line string) []string {
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:at]...)
	out = append(out, line)
	return append(out, lines[at:]...)
}

func join(lines []string) string { return strings.Join(lines, "\n") + "\n" }
