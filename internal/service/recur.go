// Recurrence: the deliberately tiny spec "🔁 every [N] day|week|month|year[s]
// [when done]". Completing a recurring task inserts the next occurrence on
// the line below, preserving the due→defer gap — that gap IS the lead time
// ("surface the registration renewal three weeks before it's due"), so it
// carries forward automatically. "when done" repeats from the completion day
// instead of the due date (furnace filters, not anniversaries).
package service

import (
	"fmt"
	"github.com/jclement/quire/internal/vault"
	"regexp"
	"strings"
	"time"

	"github.com/jclement/quire/internal/index"
)

var recurSpecRe = regexp.MustCompile(`^every(?:\s+(\d+))?\s+(day|week|month|year)s?(\s+when\s+done)?$`)

// parseRecur validates a recurrence spec into (count, unit, whenDone).
func parseRecur(spec string) (int, string, bool, error) {
	m := recurSpecRe.FindStringSubmatch(strings.TrimSpace(spec))
	if m == nil {
		return 0, "", false, fmt.Errorf("invalid recurrence %q", spec)
	}
	n := 1
	if m[1] != "" {
		fmt.Sscanf(m[1], "%d", &n)
	}
	return n, m[2], m[3] != "", nil
}

func addInterval(day string, n int, unit string) (string, error) {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return "", fmt.Errorf("invalid date %q: %w", day, err)
	}
	switch unit {
	case "day":
		t = t.AddDate(0, 0, n)
	case "week":
		t = t.AddDate(0, 0, 7*n)
	case "month":
		t = t.AddDate(0, n, 0)
	case "year":
		t = t.AddDate(n, 0, 0)
	}
	return t.Format("2006-01-02"), nil
}

// nextOccurrenceLine builds the next instance of a recurring task from its
// original (pre-completion) line, swapping the dates in place so every other
// byte of the user's text survives verbatim.
func nextOccurrenceLine(line string, row index.TaskRow, today string) (string, error) {
	n, unit, whenDone, err := parseRecur(row.Recur)
	if err != nil {
		return "", err
	}

	base := row.Due
	if whenDone || base == "" {
		base = today
	}
	newDue, err := addInterval(base, n, unit)
	if err != nil {
		return "", err
	}

	out := line
	if row.Due != "" {
		out = replaceMarkerDate(out, "📅", row.Due, newDue)
	} else {
		out += " 📅 " + newDue
	}

	if row.Defer != "" {
		var newDefer string
		if row.Due != "" {
			// Preserve the lead time: the defer stays the same distance
			// before the due date.
			dueT, err1 := time.Parse("2006-01-02", row.Due)
			deferT, err2 := time.Parse("2006-01-02", row.Defer)
			newDueT, err3 := time.Parse("2006-01-02", newDue)
			if err1 != nil || err2 != nil || err3 != nil {
				return "", fmt.Errorf("unparseable dates on recurring task")
			}
			newDefer = newDueT.Add(-dueT.Sub(deferT)).Format("2006-01-02")
		} else {
			newDefer, err = addInterval(row.Defer, n, unit)
			if err != nil {
				return "", err
			}
		}
		out = replaceMarkerDate(out, "🛫", row.Defer, newDefer)
	}
	return out, nil
}

// replaceMarkerDate swaps the date following an emoji marker, tolerating
// whatever whitespace the user put between them.
func replaceMarkerDate(line, marker, oldDate, newDate string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(marker) + `\s*` + regexp.QuoteMeta(oldDate))
	return re.ReplaceAllString(line, marker+" "+newDate)
}

// RecurrenceProblems reports repeating tasks that have stopped repeating.
func (s *Service) RecurrenceProblems() ([]RecurrenceProblem, error) {
	rows, err := s.Index.RecurrenceProblems()
	if err != nil {
		return nil, err
	}
	out := make([]RecurrenceProblem, 0, len(rows))
	for _, r := range rows {
		out = append(out, RecurrenceProblem{Task: taskFromRow(r.Task), Reason: r.Reason})
	}
	return out, nil
}

// RestoreRecurrence writes the missing next occurrence of a completed
// repeating task, below the line that completed it — the repair for a
// renewal ticked off outside quire. Refuses anything that is not a stopped
// recurrence, so it cannot quietly duplicate live work.
func (s *Service) RestoreRecurrence(id string) (Task, error) {
	row, err := s.Index.TaskByID(id)
	if err != nil {
		return Task{}, fmt.Errorf("task %s: %w", id, vault.ErrNotFound)
	}
	if !row.Done || row.Recur == "" {
		return Task{}, fmt.Errorf("%w: task %s is not a completed repeating task", ErrValidation, id)
	}
	f, err := s.Vault.Read(row.DocPath)
	if err != nil {
		return Task{}, err
	}
	lines := strings.Split(string(f.Raw), "\n")
	at := findTaskLine(lines, row)
	if at < 0 {
		return Task{}, fmt.Errorf("%w: task %s: source line not found", ErrValidation, id)
	}
	// Build the successor from the completed line with its stamp removed, so
	// the next occurrence looks exactly as the toggle would have written it.
	original := stripCompletionStamp(strings.Replace(lines[at], "[x]", "[ ]", 1))
	original = strings.Replace(original, "[X]", "[ ]", 1)
	next, err := nextOccurrenceLine(original, row, s.today())
	if err != nil {
		return Task{}, fmt.Errorf("%w: recurrence %q: %s", ErrValidation, row.Recur, err)
	}
	lines = append(lines[:at+1], append([]string{next}, lines[at+1:]...)...)

	doc, err := s.UpdateDocument(row.DocPath, strings.Join(lines, "\n"), f.SHA256)
	if err != nil {
		return Task{}, err
	}
	// By line, not by id: the successor shares its text with the occurrence
	// above it, so they share a content hash.
	fresh, err := s.Index.TaskAt(doc.Path, at+2)
	if err != nil {
		return Task{}, fmt.Errorf("restored task not found after indexing: %w", err)
	}
	return taskFromRow(fresh), nil
}
