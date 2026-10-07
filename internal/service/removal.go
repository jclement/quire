// Taking things out: a task that should not exist, a task that will not be
// done, and lines of a note that are wrong or stale. Each is a surgical
// line edit — nothing but the named lines changes — and none of them can
// remove a document.
package service

import (
	"fmt"
	"strings"

	"github.com/jclement/quire/internal/vault"
)

// markCancelled stamps the day a task was cancelled, as ✅ does completion.
const markCancelled = "❌"

// DeleteTask removes the task's line from its document and returns the
// task as it was. Only that line goes: anything nested beneath it stays.
func (s *Service) DeleteTask(id string) (Task, error) {
	row, err := s.Index.TaskByID(id)
	if err != nil {
		return Task{}, fmt.Errorf("task %s: %w", id, vault.ErrNotFound)
	}
	return reapplying(s, row.DocPath, func() (Task, error) {
		f, err := s.Vault.Read(row.DocPath)
		if err != nil {
			return Task{}, err
		}
		lines := strings.Split(string(f.Raw), "\n")
		at, err := locateTaskLine(lines, row)
		if err != nil {
			return Task{}, err
		}
		lines = append(lines[:at], lines[at+1:]...)
		if _, err := s.UpdateDocument(row.DocPath, strings.Join(lines, "\n"), f.SHA256); err != nil {
			return Task{}, err
		}
		return taskFromRow(row), nil
	})
}

// CancelTask marks an open task as one that will not be done: its checkbox
// becomes [-] and the line gains a ❌ date. A [-] line is not a task, so it
// leaves every view while the note keeps the record of what was dropped. A
// repeating task mints no next occurrence — cancelling ends it. Returns the
// task as it was and the line as it now reads.
func (s *Service) CancelTask(id string) (Task, string, error) {
	row, err := s.Index.TaskByID(id)
	if err != nil {
		return Task{}, "", fmt.Errorf("task %s: %w", id, vault.ErrNotFound)
	}
	if row.Done {
		return Task{}, "", fmt.Errorf("%w: task %q is already complete; only an open task can be cancelled", ErrValidation, row.Text)
	}
	var cancelled string
	task, err := reapplying(s, row.DocPath, func() (Task, error) {
		f, err := s.Vault.Read(row.DocPath)
		if err != nil {
			return Task{}, err
		}
		lines := strings.Split(string(f.Raw), "\n")
		at, err := locateTaskLine(lines, row)
		if err != nil {
			return Task{}, err
		}
		m := checkboxRe.FindStringSubmatchIndex(lines[at])
		if m == nil {
			return Task{}, fmt.Errorf("task %q: no checkbox on its line in %s", row.Text, row.DocPath)
		}
		cancelled = lines[at][:m[3]] + "[-]" + strings.TrimRight(lines[at][m[1]:], " \r") + " " + markCancelled + " " + s.today()
		lines[at] = cancelled
		if _, err := s.UpdateDocument(row.DocPath, strings.Join(lines, "\n"), f.SHA256); err != nil {
			return Task{}, err
		}
		return taskFromRow(row), nil
	})
	if err != nil {
		return Task{}, "", err
	}
	return task, cancelled, nil
}

// RemoveLines deletes whole lines from a document's body. Each entry of
// blocks is the exact text of one line, or of several consecutive lines
// separated by newlines; trailing whitespace is ignored, indentation is
// not. Every block must be found exactly once — a line that occurs twice
// is ambiguous, and removing the wrong one is worse than asking for a
// neighbouring line to pin it down — and the edit is all or nothing.
// Frontmatter is out of reach: SetFrontmatter edits that.
func (s *Service) RemoveLines(path string, blocks []string) (Document, error) {
	if len(blocks) == 0 {
		return Document{}, fmt.Errorf("%w: lines is required", ErrValidation)
	}
	return reapplying(s, path, func() (Document, error) {
		f, err := s.Vault.Read(path)
		if err != nil {
			return Document{}, err
		}
		lines := strings.Split(string(f.Raw), "\n")
		bodyStart := 0
		if _, body, ok := vault.SplitFrontmatter(f.Raw); ok {
			bodyStart = len(lines) - len(strings.Split(string(body), "\n"))
		}
		drop := make([]bool, len(lines))
		for _, block := range blocks {
			want := strings.Split(strings.Trim(block, "\r\n"), "\n")
			for i := range want {
				want[i] = strings.TrimRight(want[i], " \t\r")
			}
			if strings.TrimSpace(strings.Join(want, "")) == "" {
				return Document{}, fmt.Errorf("%w: a blank entry names no line to remove", ErrValidation)
			}
			found, count := -1, 0
			for at := bodyStart; at+len(want) <= len(lines); at++ {
				if linesMatch(lines[at:at+len(want)], want) {
					found = at
					count++
				}
			}
			switch {
			case count == 0:
				return Document{}, fmt.Errorf("%w: no line in %s reads %q — lines must match exactly, indentation included (nothing was removed)",
					ErrValidation, path, block)
			case count > 1:
				return Document{}, fmt.Errorf("%w: %q occurs %d times in %s — pass it together with a neighbouring line as one multi-line entry so it is found once (nothing was removed)",
					ErrValidation, block, count, path)
			}
			for i := range want {
				drop[found+i] = true
			}
		}
		kept := lines[:0:0]
		for i, line := range lines {
			if !drop[i] {
				kept = append(kept, line)
			}
		}
		return s.UpdateDocument(path, strings.Join(kept, "\n"), f.SHA256)
	})
}

// linesMatch compares a run of document lines with the wanted ones,
// ignoring trailing whitespace.
func linesMatch(have, want []string) bool {
	for i := range want {
		if strings.TrimRight(have[i], " \t\r") != want[i] {
			return false
		}
	}
	return true
}
