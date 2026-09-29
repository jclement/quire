// Waiting-for: what am I waiting on, from whom, and for how long.
//
// The grammar is one marker, a date and an optional who —
// `⏳ 2026-09-20 [[Dan Roe]]`. Without that explicit who, it is the
// [[wikilink]] people already write on the line, resolved by the index (see
// index.resolveWaitingOn). Age is reckoned here because only the service
// knows what "today" is in the owner's zone.
package service

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jclement/quire/internal/index"
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
		// A date ahead of today is a wait that has not started, not a
		// negative one.
		days := max(0, int(now.Sub(since).Hours()/24))
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

// waitingMarkRe is the whole ⏳ slot — marker, optional date (group 1),
// optional explicit who link — with the space before it.
var waitingMarkRe = regexp.MustCompile(`\s*⏳(?:\s*(\d{4}-\d{2}-\d{2}))?(?:\s*\[\[[^\[\]]+\]\])?`)

// setWaiting adds or removes the ⏳ marker. Adding stamps today, so the
// wait has an age from the moment it starts; a task already waiting keeps
// its date, because re-marking it did not restart the wait. Removing takes
// the date and any explicit who with it.
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

// whoLink is the wikilink for a who named by an agent or the UI: the
// document's title when the name (or an alias) reaches one, the name as
// given when it does not — a dangling link joins the Unwritten list like
// any other.
func (s *Service) whoLink(name string) string {
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(name), "[["), "]]"))
	if path := s.Index.ResolveLink(name); path != "" {
		if row, err := s.Index.GetDocMeta(path); err == nil && row.Title != "" {
			name = row.Title
		}
	}
	return "[[" + name + "]]"
}

// waitOn is EditTask's waiting_on: write who into the explicit slot right
// after ⏳ and its date, which outranks any link elsewhere on the line.
// An existing slot is replaced in place and its date kept — re-assigning
// who does not restart the wait; a task not yet waiting is stamped today.
// The task's words are untouched, so its id is too.
func (s *Service) waitOn(line, name string) string {
	link := s.whoLink(name)
	m := waitingMarkRe.FindStringSubmatchIndex(line)
	if m == nil {
		return line + " ⏳ " + s.today() + " " + link
	}
	since := s.today()
	if m[2] >= 0 {
		since = line[m[2]:m[3]]
	}
	return line[:m[0]] + " ⏳ " + since + " " + link + line[m[1]:]
}
