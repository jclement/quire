// Task operations: views, quick capture, and the write-back that keeps
// markdown checkboxes and task views synchronized. Write-back is surgical —
// exactly one line of the source document changes (fidelity rule).
package service

import (
	"fmt"
	"strings"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/markdown"
	"github.com/jclement/quire/internal/vault"
)

// TaskView re-exports the index views for transports.
func (s *Service) Tasks(view string) ([]Task, error) {
	rows, err := s.Index.Tasks(index.TaskView(view), s.today())
	if err != nil {
		return nil, err
	}
	return tasksFromRows(rows), nil
}

// CreateTask appends a task line to today's daily note (creating it if
// needed) — quick capture's contract: no required fields, lands in Inbox.
func (s *Service) CreateTask(text, due, deferDate string) (Task, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Task{}, fmt.Errorf("task text is required")
	}
	line := "- [ ] " + text
	if due != "" {
		line += " 📅 " + due
	}
	if deferDate != "" {
		line += " 🛫 " + deferDate
	}

	daily, err := s.EnsureDaily(s.today())
	if err != nil {
		return Task{}, err
	}

	content := daily.Markdown
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += line + "\n"

	doc, err := s.UpdateDocument(daily.Path, content, daily.SHA256)
	if err != nil {
		return Task{}, err
	}

	// Find the task we just appended (last one matching by normalized text).
	scanned := markdown.Scan(doc.Path, []byte(doc.Markdown))
	for i := len(scanned.Tasks) - 1; i >= 0; i-- {
		row, err := s.Index.TaskByID(scanned.Tasks[i].ID)
		if err == nil && row.Line == scanned.Tasks[i].Line {
			return taskFromRow(row), nil
		}
	}
	return Task{}, fmt.Errorf("created task not found after indexing")
}

// ToggleTask flips a task's checkbox in its source document, stamping or
// removing the ✅ completion date, and reindexes synchronously. Returns the
// task's new state (its ID changes when completion stamps alter the text —
// callers should use the returned task).
func (s *Service) ToggleTask(id string) (Task, error) {
	row, err := s.Index.TaskByID(id)
	if err != nil {
		return Task{}, fmt.Errorf("task %s: %w", id, vault.ErrNotFound)
	}

	f, err := s.Vault.Read(row.DocPath)
	if err != nil {
		return Task{}, err
	}

	lines := strings.Split(string(f.Raw), "\n")
	lineIdx := findTaskLine(lines, row)
	if lineIdx < 0 {
		return Task{}, fmt.Errorf("task %s: source line not found (file changed); reindex and retry", id)
	}

	newLine, nowDone := toggleTaskLine(lines[lineIdx], s.today())
	lines[lineIdx] = newLine

	if _, err := s.UpdateDocument(row.DocPath, strings.Join(lines, "\n"), f.SHA256); err != nil {
		return Task{}, err
	}

	// The edit may have changed the task's content hash (✅ stamp) — locate
	// the resulting task by its line.
	newScan := markdown.Scan(row.DocPath, []byte(strings.Join(lines, "\n")))
	for _, t := range newScan.Tasks {
		if t.Line == lineIdx+1 {
			updated, err := s.Index.TaskByID(t.ID)
			if err == nil {
				return taskFromRow(updated), nil
			}
			// Duplicate-text ordinal edge: fall back to scan shape.
			return Task{
				ID: t.ID, DocPath: row.DocPath, DocTitle: row.DocTitle, Line: t.Line,
				Text: t.Text, Done: nowDone, Due: optStr(t.Due), Defer: optStr(t.Defer),
				Priority: t.Priority, Waiting: t.Waiting, Tags: t.Tags,
				CompletedOn: optStr(t.CompletedOn),
			}, nil
		}
	}
	return Task{}, fmt.Errorf("task vanished after toggle")
}

// findTaskLine locates the task's line: trust the line hint when it still
// matches, otherwise scan the document for a checkbox line with the same
// normalized text (edits above the task move it without orphaning it).
func findTaskLine(lines []string, row index.TaskRow) int {
	matches := func(line string) bool {
		doc := markdown.Scan(row.DocPath, []byte(line))
		return len(doc.Tasks) == 1 && doc.Tasks[0].Text == row.Text && doc.Tasks[0].Done == row.Done
	}
	if row.Line-1 >= 0 && row.Line-1 < len(lines) && matches(lines[row.Line-1]) {
		return row.Line - 1
	}
	for i, line := range lines {
		if matches(line) {
			return i
		}
	}
	return -1
}

// toggleTaskLine flips one checkbox line, maintaining the ✅ stamp.
func toggleTaskLine(line, today string) (string, bool) {
	if idx := strings.Index(line, "- [ ]"); idx >= 0 {
		flipped := line[:idx] + "- [x]" + line[idx+len("- [ ]"):]
		return flipped + " ✅ " + today, true
	}
	for _, marker := range []string{"- [x]", "- [X]"} {
		if idx := strings.Index(line, marker); idx >= 0 {
			flipped := line[:idx] + "- [ ]" + line[idx+len(marker):]
			return stripCompletionStamp(flipped), false
		}
	}
	return line, false
}

// stripCompletionStamp removes a trailing "✅ YYYY-MM-DD" (and tidies the
// space it leaves) when a task is reopened.
func stripCompletionStamp(line string) string {
	idx := strings.Index(line, "✅")
	if idx < 0 {
		return line
	}
	rest := line[idx+len("✅"):]
	rest = strings.TrimLeft(rest, " ")
	// Drop a leading date if present; keep anything after it.
	if len(rest) >= 10 && isDate(rest[:10]) {
		rest = rest[10:]
	}
	return strings.TrimRight(strings.TrimRight(line[:idx], " ")+rest, " ")
}

func isDate(s string) bool {
	for i, r := range s {
		if i == 4 || i == 7 {
			if r != '-' {
				return false
			}
		} else if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) == 10
}
