// Waiting-for: what am I waiting on, from whom, and for how long.
//
// The grammar is one marker and a date — `⏳ 2026-09-20` — and nothing
// more. Who is owed is not new syntax: it is the [[wikilink]] people already
// write on the line, resolved by the index (see index.resolveWaitingOn). Age
// is reckoned here because only the service knows what "today" is in the
// owner's zone.
package service

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/markdown"
)

// StaleWaitingDays is how long a wait may run before it is flagged for
// chasing. A week: long enough that a reply was not merely slow, short
// enough that the chase still lands in the same conversation.
const StaleWaitingDays = 7

// waitingFor is the delegation half of an index row, without an age (the
// row carries no clock); ageTasks adds that.
func waitingFor(t index.TaskRow) *WaitingFor {
	if !t.Waiting {
		return nil
	}
	return &WaitingFor{
		Since:  optStr(t.WaitingSince),
		On:     optStr(t.WaitingOn.Name),
		OnPath: optStr(t.WaitingOn.Path),
	}
}

// tasksOut converts rows for a response and ages their waits.
func (s *Service) tasksOut(rows []index.TaskRow) []Task {
	return ageTasks(tasksFromRows(rows), s.today())
}

// ageTasks fills WaitingFor.Days and Stale relative to today (YYYY-MM-DD).
func ageTasks(tasks []Task, today string) []Task {
	now, err := time.Parse("2006-01-02", today)
	if err != nil {
		return tasks
	}
	for i := range tasks {
		w := tasks[i].WaitingFor
		if w == nil || w.Since == nil {
			continue
		}
		since, err := time.Parse("2006-01-02", *w.Since)
		if err != nil {
			continue
		}
		// Both dates are UTC midnights, so the division is exact — no DST
		// hour to round away.
		days := int(now.Sub(since).Hours() / 24)
		w.Days = &days
		w.Stale = days > StaleWaitingDays
	}
	return tasks
}

// staleOnly keeps the waits past the threshold, in their existing order.
func staleOnly(tasks []Task) []Task {
	out := []Task{}
	for _, t := range tasks {
		if t.WaitingFor != nil && t.WaitingFor.Stale {
			out = append(out, t)
		}
	}
	return out
}

// WaitingGroups is the Waiting view: open waits grouped by who they are
// owed by. People and companies come first, then other names (a project,
// a page not yet written), then the unassigned; within each tier, the group
// with the oldest wait leads, and each group lists oldest first.
func (s *Service) WaitingGroups(area string) ([]WaitingGroup, error) {
	rows, err := s.Index.Tasks(index.ViewWaiting, s.today(), area)
	if err != nil {
		return nil, err
	}
	return groupWaiting(rows, s.tasksOut(rows)), nil
}

// groupWaiting groups tasks (converted from rows, same order) by who. Rows
// arrive oldest wait first, so first-seen order is already oldest-first.
func groupWaiting(rows []index.TaskRow, tasks []Task) []WaitingGroup {
	groups := []WaitingGroup{}
	at := map[string]int{}
	for i, row := range rows {
		who := row.WaitingOn
		key := who.Path
		if key == "" && who.Name != "" {
			key = "name:" + strings.ToLower(who.Name)
		}
		n, ok := at[key]
		if !ok {
			n = len(groups)
			at[key] = n
			groups = append(groups, WaitingGroup{Name: who.Name, Path: optStr(who.Path), Type: who.Type, Tasks: []Task{}})
		}
		groups[n].Tasks = append(groups[n].Tasks, tasks[i])
		if tasks[i].WaitingFor != nil && tasks[i].WaitingFor.Stale {
			groups[n].Stale++
		}
	}
	tier := func(g WaitingGroup) int {
		switch {
		case g.Type == "person" || g.Type == "company":
			return 0
		case g.Name != "":
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(groups, func(i, j int) bool { return tier(groups[i]) < tier(groups[j]) })
	return groups
}

// waitingMarkRe is the ⏳ marker with its date, and the space before it.
var waitingMarkRe = regexp.MustCompile(`\s*⏳(?:\s*\d{4}-\d{2}-\d{2})?`)

// setWaiting adds or removes the ⏳ marker. Adding stamps today, so the
// wait has an age from the moment it starts; a task already waiting keeps
// its date, because re-marking it did not restart the wait. Removing takes
// the date with it.
func setWaiting(line string, waiting bool, today string) string {
	has := strings.Contains(line, "⏳")
	switch {
	case waiting && !has:
		return line + " ⏳ " + today
	case !waiting && has:
		return strings.TrimRight(waitingMarkRe.ReplaceAllString(line, ""), " ")
	}
	return line
}

// withWhoLink makes sure the task text names who it is waiting on, adding
// [[Name]] when no link on the line already resolves to them. A name with
// a page is written as that page's title (so an alias typed by an agent
// becomes the canonical link); a name without one is written as given,
// and joins the Unwritten list like any other dangling link.
func (s *Service) withWhoLink(text, name string) string {
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(name), "[["), "]]"))
	if name == "" {
		return text
	}
	path := s.Index.ResolveLink(name)
	title := name
	if path != "" {
		if row, err := s.Index.GetDocMeta(path); err == nil && row.Title != "" {
			title = row.Title
		}
	}
	scanned := markdown.Scan("", []byte("- [ ] "+text+"\n"))
	if len(scanned.Tasks) == 1 {
		for _, link := range scanned.Tasks[0].Links {
			if strings.EqualFold(link.Raw, name) || strings.EqualFold(link.Raw, title) ||
				(path != "" && s.Index.ResolveLink(link.Raw) == path) {
				return text
			}
		}
	}
	return text + " [[" + title + "]]"
}

// waitOn is EditTask's waiting_on: name the who on the line and mark it
// waiting. currentText is the task's words as they stand after any rename
// in the same edit.
func (s *Service) waitOn(line, currentText, name string) string {
	line = setTaskText(line, currentText, s.withWhoLink(currentText, name))
	return setWaiting(line, true, s.today())
}
