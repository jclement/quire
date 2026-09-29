// Meeting prep: person_context for everyone in the room at once. For each
// attendee with a person page — the last meeting you had together, what is
// open that mentions them, what you are waiting on them for, their company,
// and what else you have written about them lately.
//
// It is composed from the same index queries person_context and the person
// page use (Backlinks, TasksMentioning, link resolution), so the three can
// never disagree about who someone is or what is owed; nothing here is new
// SQL. Waiting ages ("since <date>") are deliberately not computed here —
// they arrive with the waiting-on grammar and belong in the Task shape.
package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jclement/quire/internal/calendar"
	"github.com/jclement/quire/internal/vault"
)

// maxRecentNotes bounds each attendee's "recently written about" list.
const maxRecentNotes = 5

// eventPrepWindow is how far around today MeetingPrepForEvent looks for an
// occurrence when no date is given: last week through next month.
const (
	eventPrepBackDays    = 7
	eventPrepForwardDays = 30
)

// MeetingPrep prepares for a meeting note: its people (frontmatter people
// and attendees) resolved to person pages, each with their context. Names
// that resolve to no person are listed as unmatched.
func (s *Service) MeetingPrep(path string) (MeetingPrep, error) {
	f, err := s.Vault.Read(path)
	if err != nil {
		return MeetingPrep{}, err
	}
	fm := vault.ParseFrontmatter(f.Raw)
	title := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".md")
	if row, err := s.Index.GetDocMeta(path); err == nil {
		title = row.Title
	}
	prep := MeetingPrep{Title: title, Path: &path, Date: frontmatterDate(fm)}
	if uid, ok := fm["event_uid"].(string); ok {
		prep.EventUID = uid
	}

	var names []string
	names = append(names, stringsFromFrontmatter(fm, "people")...)
	names = append(names, stringsFromFrontmatter(fm, "attendees")...)
	var people []string
	for _, name := range names {
		if person := s.personPath(unwrapWikilink(name)); person != "" {
			people = append(people, person)
		} else if clean := unwrapWikilink(name); clean != "" {
			prep.Unmatched = append(prep.Unmatched, clean)
		}
	}
	prep.People, err = s.prepPeople(people, path, prep.Date)
	if prep.Unmatched == nil {
		prep.Unmatched = []string{}
	}
	return prep, err
}

// MeetingPrepForEvent prepares for a calendar event by uid. day picks the
// occurrence; without one, the next occurrence from today (or, failing
// that, the most recent in the last week) is used. When the occurrence
// already has a meeting note, that note is left out of "last meeting".
func (s *Service) MeetingPrepForEvent(uid, day string) (MeetingPrep, error) {
	if strings.TrimSpace(uid) == "" {
		return MeetingPrep{}, fmt.Errorf("%w: event_uid is required", ErrValidation)
	}
	event, err := s.eventOccurrence(uid, day)
	if err != nil {
		return MeetingPrep{}, err
	}
	shaped, err := s.shapeEvents([]calendar.Event{event})
	if err != nil {
		return MeetingPrep{}, err
	}
	ev := shaped[0]
	prep := MeetingPrep{
		Title: ev.Title, EventUID: uid, Path: ev.NotePath, Unmatched: []string{},
		Date: event.Start.In(s.Location()).Format("2006-01-02T15:04"),
	}
	var people []string
	seen := map[string]bool{}
	for _, a := range ev.Attendees {
		switch {
		case a.Person != nil && !seen[a.Person.Path]:
			seen[a.Person.Path] = true
			people = append(people, a.Person.Path)
		case a.Person == nil:
			prep.Unmatched = append(prep.Unmatched, a.displayName())
		}
	}
	exclude := ""
	if ev.NotePath != nil {
		exclude = *ev.NotePath
	}
	prep.People, err = s.prepPeople(people, exclude, prep.Date)
	return prep, err
}

// eventOccurrence finds the occurrence to prepare for.
func (s *Service) eventOccurrence(uid, day string) (calendar.Event, error) {
	if day != "" {
		resolved, err := ParseWhen(day, s.Now())
		if err != nil {
			return calendar.Event{}, fmt.Errorf("%w: %v", ErrValidation, err)
		}
		return s.findEvent(uid, resolved)
	}
	now := s.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var latestPast *calendar.Event
	for _, e := range s.rawEvents(today.AddDate(0, 0, -eventPrepBackDays), today.AddDate(0, 0, eventPrepForwardDays)) {
		if e.UID != uid {
			continue
		}
		if !e.Start.Before(today) {
			return e, nil
		}
		latest := e
		latestPast = &latest
	}
	if latestPast != nil {
		return *latestPast, nil
	}
	return calendar.Event{}, fmt.Errorf("%w: no event %q in the calendar feeds around today", vault.ErrNotFound, uid)
}

// personPath resolves a name to a person document, "" when it names
// something else or nothing.
func (s *Service) personPath(name string) string {
	path := s.Index.ResolveLink(name)
	if path == "" {
		return ""
	}
	if row, err := s.Index.GetDocMeta(path); err != nil || row.Type != string(vault.TypePerson) {
		return ""
	}
	return path
}

func (s *Service) prepPeople(paths []string, exclude, before string) ([]PrepPerson, error) {
	out := make([]PrepPerson, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		person, err := s.prepPerson(path, exclude, before)
		if err != nil {
			return nil, fmt.Errorf("prep for %s: %w", path, err)
		}
		out = append(out, person)
	}
	return out, nil
}

// prepPerson is one attendee's context. exclude is the meeting being
// prepared for (it links them too, and is not "the last meeting"); before
// is its start ("YYYY-MM-DDTHH:MM"), so a later meeting already on file is
// not mistaken for the previous one. An empty before means now.
func (s *Service) prepPerson(path, exclude, before string) (PrepPerson, error) {
	row, err := s.Index.GetDocMeta(path)
	if err != nil {
		return PrepPerson{}, err
	}
	person := PrepPerson{Person: metaFromRow(row), OpenTasks: []Task{}, Waiting: []Task{}, RecentNotes: []DocMeta{}}
	var fm map[string]any
	_ = json.Unmarshal(row.Frontmatter, &fm)
	if companies := stringsFromFrontmatter(fm, "company"); len(companies) > 0 {
		name := unwrapWikilink(companies[0])
		company := &PrepCompany{Name: name}
		if resolved := s.Index.ResolveLink(name); resolved != "" {
			company.Path = &resolved
			if companyRow, err := s.Index.GetDocMeta(resolved); err == nil {
				company.Name = companyRow.Title
			}
		}
		person.Company = company
	}

	if before == "" {
		before = s.Now().Format("2006-01-02T15:04")
	}
	backlinks, err := s.Index.Backlinks(path)
	if err != nil {
		return PrepPerson{}, err
	}
	for _, doc := range backlinks {
		if doc.Path == exclude {
			continue
		}
		if doc.Type != string(vault.TypeMeeting) {
			if len(person.RecentNotes) < maxRecentNotes {
				person.RecentNotes = append(person.RecentNotes, metaFromRow(doc))
			}
			continue
		}
		var meetingFM map[string]any
		_ = json.Unmarshal(doc.Frontmatter, &meetingFM)
		date := frontmatterDate(meetingFM)
		if date == "" || date >= before {
			continue
		}
		if person.LastMeeting == nil || date > person.LastMeeting.Date {
			person.LastMeeting = &PrepMeeting{DocMeta: metaFromRow(doc), Date: date}
		}
	}

	tasks, err := s.Index.TasksMentioning(path)
	if err != nil {
		return PrepPerson{}, err
	}
	for _, t := range tasks {
		if t.Waiting {
			person.Waiting = append(person.Waiting, taskFromRow(t))
		} else {
			person.OpenTasks = append(person.OpenTasks, taskFromRow(t))
		}
	}
	return person, nil
}

// frontmatterDate reads a meeting's date: as "YYYY-MM-DDTHH:MM" (a bare
// date sorts before any time that day, which is the right way round for
// "before this meeting"). YAML may hand back a string or a timestamp.
func frontmatterDate(fm map[string]any) string {
	switch v := fm["date"].(type) {
	case string:
		v = strings.TrimSpace(strings.Replace(v, " ", "T", 1))
		if len(v) > len("2006-01-02T15:04") {
			v = v[:len("2006-01-02T15:04")]
		}
		return v
	case time.Time:
		return v.Format("2006-01-02T15:04")
	}
	return ""
}
