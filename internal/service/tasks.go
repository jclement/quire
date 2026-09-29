// Task operations: views, quick capture, and the write-back that keeps
// markdown checkboxes and task views synchronized. Write-back is surgical —
// exactly one line of the source document changes (fidelity rule).
package service

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/markdown"
	"github.com/jclement/quire/internal/vault"
)

// TaskEdit lives here rather than in apitypes.go because it is a REQUEST
// body, not a response: "field omitted = leave unchanged" is a shape tygo
// cannot express, so the client declares it by hand.
// TaskEdit is a partial task update; nil fields are untouched, empty strings
// clear (a snooze is due:"2026-09-05"; un-scheduling is due:"").
type TaskEdit struct {
	Due      *string `json:"due"`
	Defer    *string `json:"defer"`
	Priority *int    `json:"priority"` // 0 none, 1 high, 2 medium, 3 low
	// Waiting toggles the ⏳ delegation marker.
	Waiting *bool `json:"waiting"`
	// Recur sets the 🔁 spec ("every month", "every 3 weeks when done");
	// an empty string stops it repeating.
	Recur *string `json:"recur"`
	// Text replaces the task's words, keeping every marker on the line.
	// The content hash changes with it, so the returned task has a new id.
	Text *string `json:"text"`
}

// TaskSpec is everything a new task can carry. Path targets a document
// other than today's daily note — a project page, a meeting — so an agent
// can file work where it belongs rather than only in the day.
type TaskSpec struct {
	Path     string
	Text     string
	Due      string
	Defer    string
	Priority int
	Waiting  bool
	Recur    string
	Section  string
}

// TaskView re-exports the index views for transports.
func (s *Service) Tasks(view string) ([]Task, error) { return s.TasksIn(view, "") }

// TasksIn is a task view narrowed to an area ("" = all, "none" = unclassified).
func (s *Service) TasksIn(view, area string) ([]Task, error) {
	rows, err := s.Index.Tasks(index.TaskView(view), s.today(), area)
	if err != nil {
		return nil, err
	}
	return tasksFromRows(rows), nil
}

// CreateTask appends a task line to today's daily note (creating it if
// needed) — quick capture's contract: no required fields, lands in Inbox.
func (s *Service) CreateTask(text, due, deferDate string) (Task, error) {
	return s.CreateTaskWithAttachment(text, due, deferDate, Attachment{})
}

// CreateTaskWith is the full form: any of the task grammar's markers, and
// any document as the target. An empty Path means today's daily note.
func (s *Service) CreateTaskWith(spec TaskSpec) (Task, error) {
	return s.createTask(spec, "")
}

// CreateTaskWithAttachment is the photo→task gesture: one call captures a
// snapped permission slip as a dated task with the image attached inline.
// Text may be empty when an attachment is present (the filename stands in).
func (s *Service) CreateTaskWithAttachment(text, due, deferDate string, att Attachment) (Task, error) {
	text = strings.TrimSpace(text)
	if text == "" && att.Path != "" {
		text = strings.TrimSuffix(path.Base(att.Path), path.Ext(att.Path))
	}
	return s.createTask(TaskSpec{Text: text, Due: due, Defer: deferDate}, att.Markdown)
}

// createTask is both creators' body. attachment is markdown (an image
// embed) placed straight after the task's words, ahead of its markers.
func (s *Service) createTask(spec TaskSpec, attachment string) (Task, error) {
	line, err := s.taskLine(spec, attachment)
	if err != nil {
		return Task{}, err
	}
	target := spec.Path
	if target == "" {
		daily, err := s.EnsureDaily(s.today())
		if err != nil {
			return Task{}, err
		}
		target = daily.Path
	}
	section := spec.Section
	if section == "" && target == "daily/"+s.today()+".md" {
		section = captureHeading
	}
	return reapplying(s, target, func() (Task, error) {
		doc, err := s.GetDocument(target)
		if err != nil {
			return Task{}, err
		}
		content := appendUnderHeading(doc.Markdown, strings.ToLower(section), line)
		written, err := s.UpdateDocument(doc.Path, content, doc.SHA256)
		if err != nil {
			return Task{}, err
		}
		return s.taskOnLastLine(written, line)
	})
}

// taskLine validates a new task and renders its markdown line. Dates are
// resolved before anything is written: an unparseable one must fail loudly
// rather than land in the markdown as a word no view can match.
func (s *Service) taskLine(spec TaskSpec, attachment string) (string, error) {
	text := strings.TrimSpace(spec.Text)
	if text == "" {
		return "", fmt.Errorf("%w: task text is required", ErrValidation)
	}
	due, err := ParseWhen(spec.Due, s.Now())
	if err != nil {
		return "", fmt.Errorf("%w: due date: %s", ErrValidation, err)
	}
	deferDate, err := ParseWhen(spec.Defer, s.Now())
	if err != nil {
		return "", fmt.Errorf("%w: defer date: %s", ErrValidation, err)
	}
	if spec.Recur != "" {
		if _, _, _, err := parseRecur(spec.Recur); err != nil {
			return "", fmt.Errorf("%w: %s (try \"every month\" or \"every 3 weeks when done\")", ErrValidation, err)
		}
	}
	if spec.Priority < 0 || spec.Priority > 3 {
		return "", fmt.Errorf("%w: priority must be 0 (none), 1 (high), 2 (medium) or 3 (low)", ErrValidation)
	}
	// Text made only of markers ("⏫", "📅") scans as a task with no words:
	// it would be written, then be invisible in every view and impossible
	// to find again.
	if scanned := markdown.Scan("", []byte("- [ ] "+text)); len(scanned.Tasks) != 1 || strings.TrimSpace(scanned.Tasks[0].Text) == "" {
		return "", fmt.Errorf("%w: task text needs words, not only markers", ErrValidation)
	}

	line := "- [ ] " + text
	if attachment != "" {
		line += " " + attachment
	}
	if sym, ok := prioritySymbols[spec.Priority]; ok {
		line += " " + sym
	}
	if due != "" {
		line += " 📅 " + due
	}
	if deferDate != "" {
		line += " 🛫 " + deferDate
	}
	if spec.Waiting {
		line += " ⏳"
	}
	if spec.Recur != "" {
		line += " 🔁 " + spec.Recur
	}
	return line, nil
}

// taskOnLastLine finds the task just written: the last line that is exactly
// the one appended, resolved through the index for its real id.
func (s *Service) taskOnLastLine(doc Document, line string) (Task, error) {
	lines := strings.Split(doc.Markdown, "\n")
	scanned := markdown.Scan(doc.Path, []byte(doc.Markdown))
	for i := len(scanned.Tasks) - 1; i >= 0; i-- {
		at := scanned.Tasks[i].Line
		if at < 1 || at > len(lines) || lines[at-1] != line {
			continue
		}
		row, err := s.Index.TaskAt(doc.Path, at)
		if err != nil {
			return Task{}, fmt.Errorf("created task not found after indexing: %w", err)
		}
		return taskFromRow(row), nil
	}
	return Task{}, fmt.Errorf("created task not found after indexing")
}

// ToggleTask flips a task's checkbox in its source document, stamping or
// removing the ✅ completion date, and reindexes synchronously. Returns the
// task's new state (its ID changes when completion stamps alter the text —
// callers should use the returned task).
//
// The flip is decided once, from the task as indexed, and then applied as a
// target state. Re-applying a flip after a lost race would undo the write
// that won it: a tick saved from the editor, then this click re-read and
// reopened.
func (s *Service) ToggleTask(id string) (Task, error) {
	row, err := s.Index.TaskByID(id)
	if err != nil {
		return Task{}, fmt.Errorf("task %s: %w", id, vault.ErrNotFound)
	}
	return s.setTaskDone(row, !row.Done)
}

// SetTaskDone moves a task to done or open. A task already in that state
// is returned without a write, so a retried or duplicated request can never
// reopen what the first one completed.
func (s *Service) SetTaskDone(id string, done bool) (Task, error) {
	row, err := s.Index.TaskByID(id)
	if err != nil {
		return Task{}, fmt.Errorf("task %s: %w", id, vault.ErrNotFound)
	}
	return s.setTaskDone(row, done)
}

// CompleteTask marks a task done, idempotently: agents retry, and
// completing twice must neither reopen the task nor, for a repeating one,
// spawn a second next occurrence.
func (s *Service) CompleteTask(id string) (Task, error) { return s.SetTaskDone(id, true) }

func (s *Service) setTaskDone(row index.TaskRow, done bool) (Task, error) {
	return reapplying(s, row.DocPath, func() (Task, error) { return s.applyTaskDone(row, done) })
}

// applyTaskDone is one attempt at setTaskDone against the file as it is
// now. row is the task as indexed before the attempt; its line is a hint.
func (s *Service) applyTaskDone(row index.TaskRow, done bool) (Task, error) {
	f, err := s.Vault.Read(row.DocPath)
	if err != nil {
		return Task{}, err
	}
	lines := strings.Split(string(f.Raw), "\n")
	reached, pending := row, row
	reached.Done, pending.Done = done, !done

	// Checked in this order so a completed repeating task is never confused
	// with its next occurrence, which shares its text: the indexed line
	// already in the wanted state is a retry, not a new request.
	if at := row.Line - 1; at >= 0 && at < len(lines) && taskLineMatches(lines[at], reached) {
		return s.unchangedTask(row, lines, at)
	}
	lineIdx, err := findTaskLine(lines, pending)
	if err != nil {
		return Task{}, err
	}
	if lineIdx < 0 {
		if at, err := findTaskLine(lines, reached); err == nil && at >= 0 {
			return s.unchangedTask(row, lines, at)
		}
		return Task{}, fmt.Errorf("task %s: source line not found (file changed); reindex and retry", row.ID)
	}

	original := lines[lineIdx]
	lines[lineIdx], _ = toggleTaskLine(original, s.today())

	// Completing a recurring task spawns its next occurrence on the line
	// below. If the spec is malformed we complete anyway — losing a
	// checkbox-tick to a typo'd recurrence would be worse.
	if done && row.Recur != "" {
		if next, err := nextOccurrenceLine(original, row, s.today()); err == nil {
			lines = append(lines[:lineIdx+1], append([]string{next}, lines[lineIdx+1:]...)...)
		}
	}

	if _, err := s.UpdateDocument(row.DocPath, strings.Join(lines, "\n"), f.SHA256); err != nil {
		return Task{}, err
	}
	return s.taskOnLine(row, lines, lineIdx)
}

// unchangedTask answers a request whose target state was already reached.
// Whatever reached it may have been an outside write the watcher has not
// indexed yet, and the answer must come from the file, not a stale row.
func (s *Service) unchangedTask(row index.TaskRow, lines []string, at int) (Task, error) {
	if _, err := s.Index.IndexFile(row.DocPath); err != nil {
		return Task{}, fmt.Errorf("indexing %s: %w", row.DocPath, err)
	}
	return s.taskOnLine(row, lines, at)
}

// taskOnLine returns the task on lines[at]. By line, not id: a ✅ stamp
// changes the task's content hash.
func (s *Service) taskOnLine(row index.TaskRow, lines []string, at int) (Task, error) {
	scanned := markdown.Scan(row.DocPath, []byte(strings.Join(lines, "\n")))
	for _, t := range scanned.Tasks {
		if t.Line != at+1 {
			continue
		}
		if updated, err := s.Index.TaskByID(t.ID); err == nil {
			return taskFromRow(updated), nil
		}
		// Duplicate-text ordinal edge, or not reindexed yet: scan shape.
		return Task{
			ID: t.ID, DocPath: row.DocPath, DocTitle: row.DocTitle, Line: t.Line,
			Text: t.Text, Done: t.Done, Due: optStr(t.Due), Defer: optStr(t.Defer),
			Priority: t.Priority, Waiting: t.Waiting, Tags: t.Tags,
			CompletedOn: optStr(t.CompletedOn),
		}, nil
	}
	return Task{}, fmt.Errorf("task vanished after toggle")
}

// EditTask rewrites a task's metadata markers on its source line — the
// snooze/reschedule path. Only that one line changes.
func (s *Service) EditTask(id string, edit TaskEdit) (Task, error) {
	row, err := s.Index.TaskByID(id)
	if err != nil {
		return Task{}, fmt.Errorf("task %s: %w", id, vault.ErrNotFound)
	}
	return reapplying(s, row.DocPath, func() (Task, error) { return s.editTask(id, edit) })
}

func (s *Service) editTask(id string, edit TaskEdit) (Task, error) {
	row, err := s.Index.TaskByID(id)
	if err != nil {
		return Task{}, fmt.Errorf("task %s: %w", id, vault.ErrNotFound)
	}
	for _, d := range []*string{edit.Due, edit.Defer} {
		if d != nil && *d != "" {
			if _, err := time.Parse("2006-01-02", *d); err != nil {
				return Task{}, fmt.Errorf("invalid date %q (want YYYY-MM-DD)", *d)
			}
		}
	}

	f, err := s.Vault.Read(row.DocPath)
	if err != nil {
		return Task{}, err
	}
	lines := strings.Split(string(f.Raw), "\n")
	lineIdx, err := findTaskLine(lines, row)
	if err != nil {
		return Task{}, err
	}
	if lineIdx < 0 {
		return Task{}, fmt.Errorf("task %s: source line not found (file changed); reindex and retry", id)
	}

	line := lines[lineIdx]
	if edit.Due != nil {
		line = setMarkerDate(line, "📅", row.Due, *edit.Due)
	}
	if edit.Defer != nil {
		line = setMarkerDate(line, "🛫", row.Defer, *edit.Defer)
	}
	if edit.Priority != nil {
		line = setPriority(line, *edit.Priority)
	}
	if edit.Waiting != nil {
		line = setWaiting(line, *edit.Waiting)
	}
	if edit.Recur != nil {
		if *edit.Recur != "" {
			if _, _, _, err := parseRecur(*edit.Recur); err != nil {
				return Task{}, fmt.Errorf("%w: %s", ErrValidation, err)
			}
		}
		line = setRecur(line, row.Recur, *edit.Recur)
	}
	if edit.Text != nil {
		if strings.TrimSpace(*edit.Text) == "" {
			return Task{}, fmt.Errorf("%w: task text cannot be empty", ErrValidation)
		}
		line = setTaskText(line, row.Text, strings.TrimSpace(*edit.Text))
	}
	lines[lineIdx] = line

	if _, err := s.UpdateDocument(row.DocPath, strings.Join(lines, "\n"), f.SHA256); err != nil {
		return Task{}, err
	}
	newScan := markdown.Scan(row.DocPath, []byte(strings.Join(lines, "\n")))
	for _, t := range newScan.Tasks {
		if t.Line == lineIdx+1 {
			if updated, err := s.Index.TaskByID(t.ID); err == nil {
				return taskFromRow(updated), nil
			}
		}
	}
	return Task{}, fmt.Errorf("task vanished after edit")
}

// setMarkerDate sets, replaces, or removes an emoji-dated marker on a line.
func setMarkerDate(line, marker, oldDate, newDate string) string {
	switch {
	case oldDate != "" && newDate != "":
		return replaceMarkerDate(line, marker, oldDate, newDate)
	case oldDate != "" && newDate == "":
		re := regexp.MustCompile(`\s*` + regexp.QuoteMeta(marker) + `\s*` + regexp.QuoteMeta(oldDate))
		return re.ReplaceAllString(line, "")
	case newDate != "":
		return line + " " + marker + " " + newDate
	default:
		return line
	}
}

var prioritySymbols = map[int]string{1: "⏫", 2: "🔼", 3: "🔽"}

// setWaiting adds or removes the ⏳ delegation marker.
func setWaiting(line string, waiting bool) string {
	has := strings.Contains(line, "⏳")
	switch {
	case waiting && !has:
		return line + " ⏳"
	case !waiting && has:
		return strings.TrimRight(strings.ReplaceAll(strings.ReplaceAll(line, " ⏳", ""), "⏳", ""), " ")
	}
	return line
}

// setRecur replaces, adds or removes the 🔁 spec, leaving the rest alone.
func setRecur(line, oldSpec, newSpec string) string {
	if oldSpec != "" {
		re := regexp.MustCompile(`\s*🔁\s*` + regexp.QuoteMeta(oldSpec))
		line = strings.TrimRight(re.ReplaceAllString(line, ""), " ")
	}
	if newSpec == "" {
		return line
	}
	return line + " 🔁 " + newSpec
}

// setTaskText swaps the task's words, keeping every marker in place. The
// text is matched as the scanner sees it, so markers are never disturbed.
func setTaskText(line, oldText, newText string) string {
	at := strings.Index(line, oldText)
	if at < 0 {
		return line
	}
	return line[:at] + newText + line[at+len(oldText):]
}

func setPriority(line string, priority int) string {
	for _, sym := range prioritySymbols {
		line = strings.ReplaceAll(line, " "+sym, "")
		line = strings.ReplaceAll(line, sym, "")
	}
	if sym, ok := prioritySymbols[priority]; ok {
		line += " " + sym
	}
	return line
}

// findTaskLine locates the task's line: trust the line hint when it still
// matches, otherwise scan the document for a checkbox line with the same
// normalized text (edits above the task move it without orphaning it).
// Returns -1 when no line matches, and a conflict when several do — two
// identical "- [ ] call mum" lines are indistinguishable once the hint is
// stale, and ticking the wrong one is worse than asking for a reload.
func findTaskLine(lines []string, row index.TaskRow) (int, error) {
	matches := func(line string) bool { return taskLineMatches(line, row) }
	if row.Line-1 >= 0 && row.Line-1 < len(lines) && matches(lines[row.Line-1]) {
		return row.Line - 1, nil
	}
	found, candidates := -1, 0
	for i, line := range lines {
		if matches(line) {
			found = i
			candidates++
		}
	}
	if candidates > 1 {
		return -1, fmt.Errorf("task %q: %d identical lines in %s and it has moved since it was indexed, so which one is meant is unclear: %w",
			row.Text, candidates, row.DocPath, vault.ErrConflict)
	}
	return found, nil
}

// taskLineMatches reports whether line is row's task: same normalized text,
// same done state.
func taskLineMatches(line string, row index.TaskRow) bool {
	doc := markdown.Scan(row.DocPath, []byte(line))
	return len(doc.Tasks) == 1 && doc.Tasks[0].Text == row.Text && doc.Tasks[0].Done == row.Done
}

// checkboxRe matches the checkbox of a task line with any list marker the
// indexer accepts (see markdown.taskRe) — a vault written with `* [ ]`
// used to index fine and then refuse to toggle, because this looked for
// "- [ ]" alone and quietly wrote the line back unchanged.
var checkboxRe = regexp.MustCompile(`^(\s*[-*+] )\[([ xX])\]`)

// toggleTaskLine flips one checkbox line, maintaining the ✅ stamp.
func toggleTaskLine(line, today string) (string, bool) {
	m := checkboxRe.FindStringSubmatchIndex(line)
	if m == nil {
		return line, false
	}
	prefix := line[m[2]:m[3]]
	rest := line[m[1]:]
	if line[m[4]:m[5]] == " " {
		return prefix + "[x]" + rest + " ✅ " + today, true
	}
	return stripCompletionStamp(prefix + "[ ]" + rest), false
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
