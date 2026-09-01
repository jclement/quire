// The month calendar: one screen showing what happened on each day — whether
// a daily note exists, which documents were touched, meetings held, tasks
// completed. Everything comes from the index, so it costs one pass over a
// month's rows and needs no new storage.
package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jclement/quire/internal/index"
)

// CalendarDoc is a document touched on a day.
type CalendarDoc struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// CalendarDay is one cell of the month grid.
type CalendarDay struct {
	Date string `json:"date"` // YYYY-MM-DD
	// HasDaily reports whether a daily note exists for this date (whether or
	// not it was touched today — it is the day's anchor).
	HasDaily  bool          `json:"has_daily"`
	Touched   []CalendarDoc `json:"touched"`  // documents modified that day
	Meetings  []CalendarDoc `json:"meetings"` // meetings scheduled that day
	Completed int           `json:"completed_tasks"`
}

// CalendarMonth is the month payload.
type CalendarMonth struct {
	Month string        `json:"month"` // YYYY-MM
	Prev  string        `json:"prev"`
	Next  string        `json:"next"`
	Days  []CalendarDay `json:"days"` // every day of the month, in order
}

// maxTouchedPerDay keeps a busy day's cell (and the payload) bounded; the UI
// shows "+N more" and the day view has the rest.
const maxTouchedPerDay = 12

// Calendar builds the month view for "YYYY-MM" (empty means the current
// month). Days are local-time, matching how daily notes are named.
func (s *Service) Calendar(month string) (CalendarMonth, error) {
	now := s.Now()
	if month == "" {
		month = now.Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01", month, now.Location())
	if err != nil {
		return CalendarMonth{}, fmt.Errorf("invalid month %q (want YYYY-MM)", month)
	}
	end := start.AddDate(0, 1, 0)
	firstDay, lastDay := start.Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02")

	byDay := map[string]*CalendarDay{}
	payload := CalendarMonth{
		Month: month,
		Prev:  start.AddDate(0, -1, 0).Format("2006-01"),
		Next:  end.Format("2006-01"),
	}
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		cell := &CalendarDay{
			Date:     day.Format("2006-01-02"),
			Touched:  []CalendarDoc{},
			Meetings: []CalendarDoc{},
			HasDaily: s.Vault.Exists("daily/" + day.Format("2006-01-02") + ".md"),
		}
		byDay[cell.Date] = cell
		payload.Days = append(payload.Days, CalendarDay{})
	}

	touched, err := s.Index.DocsModifiedBetween(start, end)
	if err != nil {
		return CalendarMonth{}, err
	}
	for _, doc := range touched {
		if cell, ok := byDay[doc.Mtime.Format("2006-01-02")]; ok {
			cell.Touched = append(cell.Touched, calendarDoc(doc))
		}
	}

	meetings, err := s.Index.MeetingsBetween(firstDay, lastDay)
	if err != nil {
		return CalendarMonth{}, err
	}
	for _, meeting := range meetings {
		var fm struct {
			Date string `json:"date"`
		}
		day := ""
		if err := json.Unmarshal(meeting.Frontmatter, &fm); err == nil && len(fm.Date) >= 10 {
			day = fm.Date[:10]
		}
		if cell, ok := byDay[day]; ok {
			cell.Meetings = append(cell.Meetings, calendarDoc(meeting))
		}
	}

	completed, err := s.Index.TasksCompletedBetween(firstDay, lastDay)
	if err != nil {
		return CalendarMonth{}, err
	}
	for day, count := range completed {
		if cell, ok := byDay[day]; ok {
			cell.Completed = count
		}
	}

	for i := range payload.Days {
		cell := byDay[start.AddDate(0, 0, i).Format("2006-01-02")]
		if len(cell.Touched) > maxTouchedPerDay {
			cell.Touched = cell.Touched[:maxTouchedPerDay]
		}
		payload.Days[i] = *cell
	}
	return payload, nil
}

func calendarDoc(d index.DocRow) CalendarDoc {
	return CalendarDoc{Path: d.Path, Title: d.Title, Type: d.Type}
}
