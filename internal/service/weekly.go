// The weekly tier: a document per ISO week, and the review payload composed
// around it. Daily notes are for capture; the week is for planning and
// looking back, and everything it shows is derived from the index so the
// review is a reading rather than a remembering.
package service

import (
	"time"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/vault"
)

// GetWeekly returns the week's note, or not-found when it is unwritten.
func (s *Service) GetWeekly(label string) (Document, error) {
	week, err := ParseWeek(label, s.Now())
	if err != nil {
		return Document{}, err
	}
	return s.GetDocument("weekly/" + week.Label + ".md")
}

// EnsureWeekly returns the week's note, creating it from templates/weekly.md
// when it does not exist yet.
func (s *Service) EnsureWeekly(label string) (Document, error) {
	week, err := ParseWeek(label, s.Now())
	if err != nil {
		return Document{}, err
	}
	path := "weekly/" + week.Label + ".md"
	if s.Vault.Exists(path) {
		return s.GetDocument(path)
	}
	content := []byte("# " + week.Label + "\n\n")
	if tpl, ok := s.resolveTemplate(vault.TypeWeekly, ""); ok {
		// {{date}} is the week's Monday; {{title}} its label.
		monday, _ := time.ParseInLocation("2006-01-02", week.Start, s.Now().Location())
		if seed, body, err := s.renderTemplate(tpl, week.Label, monday); err == nil {
			content = []byte(body)
			for _, kv := range seed {
				content = vault.SetFrontmatterKey(content, kv[0], kv[1])
			}
		}
	}
	f, err := s.Vault.Write(path, content, "")
	if err != nil {
		return Document{}, err
	}
	if _, err := s.Index.IndexFile(path); err != nil {
		return Document{}, err
	}
	return s.buildDocument(f)
}

// WeekReview composes the review for a week. An area narrows everything
// that has one; the week's own note does not, because a week is a week.
func (s *Service) WeekReview(label, area string) (WeekPayload, error) {
	week, err := ParseWeek(label, s.Now())
	if err != nil {
		return WeekPayload{}, err
	}
	payload := WeekPayload{
		Week: week.Label, Start: week.Start, End: week.End,
		Prev: week.Prev, Next: week.Next,
		Completed: []Task{}, Slipped: []Task{}, Waiting: []Task{},
		Stalled: []DocMeta{}, Meetings: []DocMeta{}, Touched: []DocMeta{},
	}

	if doc, err := s.GetDocument("weekly/" + week.Label + ".md"); err == nil {
		payload.Note = &doc
	}

	completed, err := s.Index.TasksCompletedIn(week.Start, week.End, area)
	if err != nil {
		return payload, err
	}
	payload.Completed = tasksFromRows(completed)

	// Slipped: still open, and its due date fell on or before the week's end.
	overdue, err := s.Index.OpenTasksDue(week.End, area)
	if err != nil {
		return payload, err
	}
	payload.Slipped = tasksFromRows(overdue)

	waiting, err := s.Index.Tasks(index.ViewWaiting, s.today(), area)
	if err != nil {
		return payload, err
	}
	payload.Waiting = tasksFromRows(waiting)

	stalled, err := s.Index.ProjectsWithoutNextAction(area)
	if err != nil {
		return payload, err
	}
	payload.Stalled = metasFromRows(stalled)

	meetings, err := s.Index.MeetingsBetween(week.Start, week.End)
	if err != nil {
		return payload, err
	}
	payload.Meetings = metasFromRows(meetings)

	// Documents touched in the week, so "what was I even working on" has an
	// answer. The window is [Monday, next Monday).
	touched, err := s.Index.DocsModifiedBetween(week.StartTime, week.StartTime.AddDate(0, 0, 7))
	if err != nil {
		return payload, err
	}
	payload.Touched = metasFromRows(touched)

	return payload, nil
}
