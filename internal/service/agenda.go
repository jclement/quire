// The agenda: calendar-feed events shaped for Today, the month view and
// agents, with each invitee matched to a person page, and the one write
// the feature makes — a meeting note for an event.
//
// Events are never stored in the vault. The link from an event to its note
// is the note's own frontmatter (event_uid plus the date it already has),
// so a note made here is an ordinary meeting that survives the feed being
// removed, and "does this event have a note?" is a lookup over the
// meetings indexed for that day.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jclement/quire/internal/calendar"
	"github.com/jclement/quire/internal/vault"
)

// maxEventRangeDays bounds calendar_events: a quarter is plenty for an
// agent, and an unbounded range over a daily series is a lot of rows.
const maxEventRangeDays = 92

// defaultEventRangeDays is the window when the caller gives no end: the
// coming week, which is what "what's on my calendar" usually means.
const defaultEventRangeDays = 7

// ---- feeds (Settings) ----

// errNoCalendar is returned by the feed routes when main wired none.
var errNoCalendar = fmt.Errorf("%w: the calendar is not available on this instance", ErrValidation)

// CalendarFeeds lists the subscribed feeds with their last fetch outcome.
// URLs come back masked; the secret never leaves the server once saved.
func (s *Service) CalendarFeeds() ([]CalendarFeed, error) {
	if s.Feeds == nil {
		return []CalendarFeed{}, nil
	}
	statuses, err := s.Feeds.Status()
	if err != nil {
		return nil, err
	}
	out := make([]CalendarFeed, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, CalendarFeed{
			ID: st.ID, URL: st.Masked, Error: st.Error, Failures: st.Failures, Events: st.Events,
			LastAttempt: optTime(st.LastAttempt), LastSuccess: optTime(st.LastSuccess),
		})
	}
	return out, nil
}

// AddCalendarFeed subscribes to an ICS URL and fetches it at once, so the
// response already says whether the URL works.
func (s *Service) AddCalendarFeed(ctx context.Context, rawURL string) ([]CalendarFeed, error) {
	if s.Feeds == nil {
		return nil, errNoCalendar
	}
	if _, err := s.Feeds.Store.Add(rawURL); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	s.Feeds.Refresh(ctx)
	return s.CalendarFeeds()
}

// RemoveCalendarFeed unsubscribes; false when no feed has that id.
func (s *Service) RemoveCalendarFeed(id string) (bool, error) {
	if s.Feeds == nil {
		return false, errNoCalendar
	}
	removed, err := s.Feeds.Store.Remove(id)
	if removed {
		s.Feeds.Forget(id)
	}
	return removed, err
}

// RefreshCalendar re-fetches every feed now, ignoring backoff.
func (s *Service) RefreshCalendar(ctx context.Context) ([]CalendarFeed, error) {
	if s.Feeds == nil {
		return nil, errNoCalendar
	}
	s.Feeds.Refresh(ctx)
	return s.CalendarFeeds()
}

func optTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	formatted := t.Format(time.RFC3339)
	return &formatted
}

// ---- reading events ----

// rawEvents expands the feeds over [from, to) in the configured zone.
func (s *Service) rawEvents(from, to time.Time) []calendar.Event {
	if s.Feeds == nil {
		return nil
	}
	return s.Feeds.Events(from, to, s.Location())
}

// dayBounds is midnight to midnight for a YYYY-MM-DD in the configured zone.
func (s *Service) dayBounds(day string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01-02", day, s.Location())
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: invalid date %q (want YYYY-MM-DD)", ErrValidation, day)
	}
	return start, start.AddDate(0, 0, 1), nil
}

// CalendarEvents lists events from `from` through `to` (inclusive days,
// YYYY-MM-DD or natural forms like "today" or "fri"). from defaults to
// today and to to a week after from.
func (s *Service) CalendarEvents(from, to string) ([]CalendarEvent, error) {
	now := s.Now()
	fromDay, err := ParseWhen(from, now)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if fromDay == "" {
		fromDay = now.Format("2006-01-02")
	}
	start, _, err := s.dayBounds(fromDay)
	if err != nil {
		return nil, err
	}
	toDay, err := ParseWhen(to, now)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	end := start.AddDate(0, 0, defaultEventRangeDays)
	if toDay != "" {
		if _, end, err = s.dayBounds(toDay); err != nil {
			return nil, err
		}
	}
	if !end.After(start) {
		return nil, fmt.Errorf("%w: to (%s) is before from (%s)", ErrValidation, toDay, fromDay)
	}
	if end.Sub(start) > maxEventRangeDays*24*time.Hour+time.Hour {
		return nil, fmt.Errorf("%w: at most %d days at a time", ErrValidation, maxEventRangeDays)
	}
	return s.shapeEvents(s.rawEvents(start, end))
}

// todayEvents is Today's agenda: every event overlapping the day.
func (s *Service) todayEvents(day string) ([]CalendarEvent, error) {
	start, end, err := s.dayBounds(day)
	if err != nil {
		return nil, err
	}
	return s.shapeEvents(s.rawEvents(start, end))
}

// eventCounts is the month view's dot count per day, keyed YYYY-MM-DD. A
// multi-day event counts on its first day only.
func (s *Service) eventCounts(from, to time.Time) map[string]int {
	counts := map[string]int{}
	for _, e := range s.rawEvents(from, to) {
		counts[e.Start.In(s.Location()).Format("2006-01-02")]++
	}
	return counts
}

// shapeEvents converts to the wire shape, matching attendees to people
// and each occurrence to its meeting note.
func (s *Service) shapeEvents(events []calendar.Event) ([]CalendarEvent, error) {
	out := make([]CalendarEvent, 0, len(events))
	if len(events) == 0 {
		return out, nil
	}
	matcher, err := s.newAttendeeMatcher()
	if err != nil {
		return nil, err
	}
	notes := map[string]string{}
	daysLooked := map[string]bool{}
	loc := s.Location()
	for _, e := range events {
		day := e.Start.In(loc).Format("2006-01-02")
		if !daysLooked[day] {
			if err := s.collectEventNotes(day, notes); err != nil {
				return nil, err
			}
			daysLooked[day] = true
		}
		shaped := CalendarEvent{
			UID: e.UID, RecurrenceID: e.RecurrenceID, Title: e.Title,
			Start: e.Start.In(loc).Format(time.RFC3339), End: e.End.In(loc).Format(time.RFC3339),
			Date: day, AllDay: e.AllDay, Location: e.Location, Link: e.Link,
			Attendees: matcher.match(e.Attendees),
		}
		if path := notes[eventKey(e.UID, day)]; path != "" {
			shaped.NotePath = &path
		}
		out = append(out, shaped)
	}
	return out, nil
}

func eventKey(uid, day string) string { return uid + "\x00" + day }

// collectEventNotes records, for one day, which meeting notes carry an
// event_uid — the link from an occurrence to its note. The date is the
// note's own date: that is the occurrence it was made for.
func (s *Service) collectEventNotes(day string, into map[string]string) error {
	meetings, err := s.Index.MeetingsBetween(day, day)
	if err != nil {
		return err
	}
	for _, m := range meetings {
		var fm struct {
			EventUID any `json:"event_uid"`
		}
		if json.Unmarshal(m.Frontmatter, &fm) != nil {
			continue
		}
		uid, _ := fm.EventUID.(string)
		if uid == "" {
			continue
		}
		key := eventKey(uid, day)
		if _, taken := into[key]; !taken {
			into[key] = m.Path // MeetingsBetween is ordered, so the earliest wins
		}
	}
	return nil
}

// findEvent returns the occurrence of uid starting on day.
func (s *Service) findEvent(uid, day string) (calendar.Event, error) {
	start, end, err := s.dayBounds(day)
	if err != nil {
		return calendar.Event{}, err
	}
	for _, e := range s.rawEvents(start, end) {
		if e.UID == uid && e.Start.In(s.Location()).Format("2006-01-02") == day {
			return e, nil
		}
	}
	return calendar.Event{}, fmt.Errorf("%w: no event %q on %s in the calendar feeds", vault.ErrNotFound, uid, day)
}

// ---- attendee matching ----

// attendeeMatcher resolves invitees to person documents: by email first
// (a person's email: may be one address or a list), then by display name
// through ordinary link resolution, so "Sarah Chen" finds a page titled or
// aliased that way even with no email recorded.
type attendeeMatcher struct {
	svc     *Service
	byEmail map[string]CalendarDoc
}

func (s *Service) newAttendeeMatcher() (*attendeeMatcher, error) {
	people, err := s.Index.PeopleWithEmail()
	if err != nil {
		return nil, err
	}
	m := &attendeeMatcher{svc: s, byEmail: map[string]CalendarDoc{}}
	for _, p := range people {
		var fm map[string]any
		if json.Unmarshal(p.Frontmatter, &fm) != nil {
			continue
		}
		for _, email := range stringsFromFrontmatter(fm, "email") {
			email = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(email, "mailto:")))
			if _, taken := m.byEmail[email]; email != "" && !taken {
				m.byEmail[email] = calendarDoc(p)
			}
		}
	}
	return m, nil
}

func (m *attendeeMatcher) match(attendees []calendar.Attendee) []EventAttendee {
	out := make([]EventAttendee, 0, len(attendees))
	for _, a := range attendees {
		shaped := EventAttendee{Name: a.Name, Email: a.Email, Organizer: a.Organizer}
		if person, ok := m.byEmail[a.Email]; ok && a.Email != "" {
			shaped.Person = &person
		} else if person, ok := m.byName(a.Name); ok {
			shaped.Person = &person
		}
		out = append(out, shaped)
	}
	return out
}

func (m *attendeeMatcher) byName(name string) (CalendarDoc, bool) {
	if strings.TrimSpace(name) == "" {
		return CalendarDoc{}, false
	}
	path := m.svc.Index.ResolveLink(name)
	if path == "" {
		return CalendarDoc{}, false
	}
	row, err := m.svc.Index.GetDocMeta(path)
	if err != nil || row.Type != string(vault.TypePerson) {
		return CalendarDoc{}, false
	}
	return calendarDoc(row), true
}

// displayName is how an attendee is written when they have no page.
func (a EventAttendee) displayName() string {
	if a.Name != "" {
		return a.Name
	}
	return a.Email
}

// ---- creating the note ----

// CreateMeetingFromEvent makes the meeting note for one occurrence (uid on
// day) from the meeting template: dated at the event's start, attendees
// with a person page in people: as wikilinks, the rest written as plain
// names on the Attendees line, and event_uid recorded so the agenda can
// find the note again. Idempotent: when the occurrence already has a note,
// that note is returned and created is false.
func (s *Service) CreateMeetingFromEvent(uid, day string) (doc Document, created bool, err error) {
	if strings.TrimSpace(uid) == "" {
		return Document{}, false, fmt.Errorf("%w: uid is required", ErrValidation)
	}
	existing := map[string]string{}
	if err := s.collectEventNotes(day, existing); err != nil {
		return Document{}, false, err
	}
	if path := existing[eventKey(uid, day)]; path != "" {
		doc, err := s.GetDocument(path)
		return doc, false, err
	}
	event, err := s.findEvent(uid, day)
	if err != nil {
		return Document{}, false, err
	}
	shaped, err := s.shapeEvents([]calendar.Event{event})
	if err != nil {
		return Document{}, false, err
	}
	content := s.meetingNoteContent(event, shaped[0].Attendees)
	path, err := s.freeMeetingPath(event.Title, event.Start.In(s.Location()))
	if err != nil {
		return Document{}, false, err
	}
	f, err := s.Vault.Write(path, content, "")
	if err != nil {
		return Document{}, false, err
	}
	if _, err := s.Index.IndexFile(path); err != nil {
		return Document{}, false, fmt.Errorf("indexing new meeting: %w", err)
	}
	doc, err = s.buildDocument(f)
	return doc, true, err
}

// meetingNoteContent renders the meeting template (or a bare heading) at
// the event's start and sets the event's frontmatter over whatever the
// template seeded.
func (s *Service) meetingNoteContent(event calendar.Event, attendees []EventAttendee) []byte {
	at := event.Start.In(s.Location())
	body := "# " + event.Title + "\n\n"
	var seed [][2]string
	if tpl, ok := s.resolveTemplate(vault.TypeMeeting, ""); ok {
		if templateSeed, rendered, err := s.renderTemplate(tpl, event.Title, at); err == nil {
			seed, body = templateSeed, rendered
		}
	}

	var people []any
	var unmatched []string
	seen := map[string]bool{}
	for _, a := range attendees {
		switch {
		case a.Person != nil && !seen[a.Person.Path]:
			seen[a.Person.Path] = true
			people = append(people, wrapWikilink(a.Person.Title))
		case a.Person == nil:
			unmatched = append(unmatched, a.displayName())
		}
	}
	body = fillAttendees(body, unmatched)

	content := vault.BuildDoc(nil, body)
	for _, kv := range seed {
		content = vault.SetFrontmatterKey(content, kv[0], kv[1])
	}
	peopleYAML, _ := encodeYAMLValue(append([]any{}, people...))
	if len(people) == 0 {
		peopleYAML = "[]"
	}
	content = vault.SetFrontmatterKey(content, "date", at.Format("2006-01-02T15:04"))
	content = vault.SetFrontmatterKey(content, "people", peopleYAML)
	content = vault.SetFrontmatterKey(content, "event_uid", quoteYAMLString(event.UID))
	return content
}

// attendeesPrefix is the starter meeting template's attendee line.
const attendeesPrefix = "**Attendees:**"

// fillAttendees writes the attendees without a page onto the template's
// "**Attendees:**" line, or adds that line under the title when the
// template has none. People with a page are in frontmatter already and
// render as links in the properties strip.
func fillAttendees(body string, names []string) string {
	if len(names) == 0 {
		return body
	}
	list := strings.Join(names, ", ")
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), attendeesPrefix) {
			existing := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), attendeesPrefix))
			if existing != "" {
				list = existing + ", " + list
			}
			lines[i] = attendeesPrefix + " " + list
			return strings.Join(lines, "\n")
		}
	}
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			rest := append([]string{"", attendeesPrefix + " " + list}, lines[i+1:]...)
			return strings.Join(append(lines[:i+1], rest...), "\n")
		}
	}
	return attendeesPrefix + " " + list + "\n\n" + body
}

// freeMeetingPath is the usual meetings/<date>-<slug>.md, dated by the
// event rather than by today, with a numeric suffix on collision.
func (s *Service) freeMeetingPath(title string, at time.Time) (string, error) {
	path := vault.NewDocPath(vault.TypeMeeting, title, at)
	base := strings.TrimSuffix(path, ".md")
	for n := 2; s.Vault.Exists(path); n++ {
		if n > 100 {
			return "", fmt.Errorf("could not find a free path for %q", title)
		}
		path = fmt.Sprintf("%s-%d.md", base, n)
	}
	return path, nil
}
