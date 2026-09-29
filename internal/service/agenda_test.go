package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jclement/quire/internal/calendar"
	"github.com/jclement/quire/internal/settings"
	"github.com/jclement/quire/internal/vault"
)

// agendaICS is a one-off sync and a daily standup, both in Edmonton time.
// The sync's invitees are matched three ways: Sarah by the second address
// in an email list (in different case), Priya by name alone, Bob not at all.
const agendaICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:test\r\n" +
	"BEGIN:VEVENT\r\nUID:acme-sync@example.com\r\nDTSTAMP:20260801T000000Z\r\n" +
	"DTSTART;TZID=America/Edmonton:20260901T140000\r\nDTEND;TZID=America/Edmonton:20260901T150000\r\n" +
	"SUMMARY:Acme sync\r\nLOCATION:https://meet.example.com/acme\r\n" +
	"ORGANIZER;CN=Jeff:mailto:jeff@example.com\r\n" +
	"ATTENDEE;CN=Sarah C.:mailto:S.Chen@Home.example\r\n" +
	"ATTENDEE;CN=Priya Patel:mailto:priya@other.example\r\n" +
	"ATTENDEE;CN=Bob Unknown:mailto:bob@elsewhere.example\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:standup@example.com\r\nDTSTAMP:20260801T000000Z\r\n" +
	"DTSTART;TZID=America/Edmonton:20260825T090000\r\nDTEND;TZID=America/Edmonton:20260825T091500\r\n" +
	"RRULE:FREQ=DAILY\r\nSUMMARY:Standup\r\n" +
	"ATTENDEE;CN=Sarah Chen:mailto:sarah@acme.example\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:holiday@example.com\r\nDTSTAMP:20260801T000000Z\r\n" +
	"DTSTART;VALUE=DATE:20260901\r\nSUMMARY:Office closed\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// newCalendarService is a test service in Edmonton at 08:00 on 2026-09-01,
// subscribed to a feed serving ics.
func newCalendarService(t *testing.T, ics string) *Service {
	t.Helper()
	s := newTestService(t)
	s.Settings = settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err := s.SetTimezone("America/Edmonton"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, s.Location())
	s.Now = func() time.Time { return now }

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(ics))
	}))
	t.Cleanup(server.Close)
	s.Feeds = calendar.NewFetcher(calendar.OpenStore(filepath.Join(t.TempDir(), "feeds.json")))
	if _, err := s.AddCalendarFeed(context.Background(), server.URL+"/secret.ics"); err != nil {
		t.Fatal(err)
	}
	return s
}

func mustCreate(t *testing.T, s *Service, docType vault.DocType, title, body string) Document {
	t.Helper()
	doc, err := s.CreateDocument(docType, title, body)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// seedPeople writes the attendee pages the agenda fixtures match against.
func seedPeople(t *testing.T, s *Service) {
	t.Helper()
	mustCreate(t, s, vault.TypeCompany, "Acme", "")
	mustCreate(t, s, vault.TypePerson, "Sarah Chen",
		"---\nemail: [sarah@acme.example, s.chen@home.example]\ncompany: \"[[Acme]]\"\n---\n# Sarah Chen\n")
	mustCreate(t, s, vault.TypePerson, "Priya Patel", "# Priya Patel\n")
	// A company named like an attendee must not be taken for a person.
	mustCreate(t, s, vault.TypeCompany, "Bob Unknown", "")
}

func TestTodayCarriesEventsWithMatchedAttendees(t *testing.T) {
	s := newCalendarService(t, agendaICS)
	seedPeople(t, s)

	today, err := s.Today()
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, e := range today.Events {
		titles = append(titles, e.Title)
	}
	if strings.Join(titles, ", ") != "Office closed, Standup, Acme sync" {
		t.Fatalf("today's events = %v", titles)
	}
	sync := today.Events[2]
	if sync.Start != "2026-09-01T14:00:00-06:00" || sync.Date != "2026-09-01" || sync.Link != "https://meet.example.com/acme" {
		t.Errorf("sync = %+v", sync)
	}
	if !today.Events[0].AllDay {
		t.Error("the holiday is all-day")
	}

	matched := map[string]string{}
	for _, a := range sync.Attendees {
		if a.Person != nil {
			matched[a.Name] = a.Person.Path
		} else {
			matched[a.Name] = ""
		}
	}
	want := map[string]string{
		"Jeff":        "",
		"Sarah C.":    "people/sarah-chen.md",  // by email, second in her list, case-folded
		"Priya Patel": "people/priya-patel.md", // by name
		"Bob Unknown": "",                      // resolves to a company, not a person
	}
	for name, path := range want {
		if matched[name] != path {
			t.Errorf("%s matched %q, want %q", name, matched[name], path)
		}
	}
	if sync.NotePath != nil {
		t.Errorf("no note yet, got %v", *sync.NotePath)
	}
}

func TestCreateMeetingFromEventIsIdempotentPerOccurrence(t *testing.T) {
	s := newCalendarService(t, agendaICS)
	seedPeople(t, s)
	if _, err := s.InstallStarterTemplates(); err != nil {
		t.Fatal(err)
	}

	doc, created, err := s.CreateMeetingFromEvent("acme-sync@example.com", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if !created || doc.Path != "meetings/2026-09-01-acme-sync.md" || doc.Type != "meeting" {
		t.Fatalf("created=%v doc=%+v", created, doc.DocMeta)
	}
	for _, want := range []string{
		"date: 2026-09-01T14:00\n",
		`people: ["[[Sarah Chen]]", "[[Priya Patel]]"]` + "\n",
		`event_uid: "acme-sync@example.com"` + "\n",
		"# Acme sync\n",
		// The template's attendee line lists those without a page.
		"**Attendees:** Jeff, Bob Unknown\n",
		"## Action items\n",
	} {
		if !strings.Contains(doc.Markdown, want) {
			t.Errorf("note lacks %q:\n%s", want, doc.Markdown)
		}
	}
	if strings.Contains(doc.Markdown, "description:") {
		t.Error("the template's own metadata is not copied")
	}

	again, created, err := s.CreateMeetingFromEvent("acme-sync@example.com", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if created || again.Path != doc.Path {
		t.Errorf("second create should return the same note, got created=%v %s", created, again.Path)
	}

	today, _ := s.Today()
	for _, e := range today.Events {
		switch e.UID {
		case "acme-sync@example.com":
			if e.NotePath == nil || *e.NotePath != doc.Path {
				t.Errorf("the agenda links the note: %v", e.NotePath)
			}
		default:
			if e.NotePath != nil {
				t.Errorf("%s has no note, got %s", e.Title, *e.NotePath)
			}
		}
	}

	// A recurring event gets one note per occurrence: the next day's
	// standup is a new note, and today's still has its own.
	first, _, err := s.CreateMeetingFromEvent("standup@example.com", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	second, created, err := s.CreateMeetingFromEvent("standup@example.com", "2026-09-02")
	if err != nil || !created || second.Path == first.Path {
		t.Fatalf("next occurrence: created=%v path=%s err=%v", created, second.Path, err)
	}
	if !strings.Contains(second.Markdown, "date: 2026-09-02T09:00") {
		t.Errorf("dated by the occurrence:\n%s", second.Markdown)
	}

	if _, _, err := s.CreateMeetingFromEvent("standup@example.com", "2026-07-01"); err == nil {
		t.Error("a date the series does not occur on is not found")
	}
}

func TestCreateMeetingFromEventWithoutTemplate(t *testing.T) {
	s := newCalendarService(t, agendaICS)
	doc, _, err := s.CreateMeetingFromEvent("acme-sync@example.com", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	// Nobody has a page, so people is empty and everyone is named in the body.
	if !strings.Contains(doc.Markdown, "people: []\n") ||
		!strings.Contains(doc.Markdown, "# Acme sync\n\n**Attendees:** Jeff, Sarah C., Priya Patel, Bob Unknown\n") {
		t.Errorf("bare note:\n%s", doc.Markdown)
	}
}

func TestMeetingPrepComposesPerAttendeeContext(t *testing.T) {
	s := newCalendarService(t, agendaICS)
	seedPeople(t, s)
	mustCreate(t, s, vault.TypeMeeting, "Acme kickoff",
		"---\ndate: 2026-08-20T10:00\npeople: [\"[[Sarah Chen]]\"]\n---\n# Acme kickoff\n\n- [ ] Send [[Sarah Chen]] the deck\n- [ ] Signed contract ⏳ [[Sarah Chen]]\n")
	mustCreate(t, s, vault.TypeMeeting, "Acme older",
		"---\ndate: 2026-08-01T10:00\npeople: [\"[[Sarah Chen]]\"]\n---\n# Acme older\n")
	mustCreate(t, s, vault.TypeMeeting, "Acme follow-up",
		"---\ndate: 2026-09-10T10:00\npeople: [\"[[Sarah Chen]]\"]\n---\n# Acme follow-up\n")
	mustCreate(t, s, vault.TypeNote, "Pricing thoughts", "# Pricing thoughts\n\nAsk [[Sarah Chen]] about tiers.\n")

	note, _, err := s.CreateMeetingFromEvent("acme-sync@example.com", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	prep, err := s.MeetingPrep(note.Path)
	if err != nil {
		t.Fatal(err)
	}
	if prep.Title != "Acme sync" || prep.Date != "2026-09-01T14:00" || prep.EventUID != "acme-sync@example.com" {
		t.Errorf("prep header = %+v", prep)
	}
	if len(prep.People) != 2 || prep.People[0].Person.Path != "people/sarah-chen.md" {
		t.Fatalf("people = %+v", prep.People)
	}
	sarah := prep.People[0]
	if sarah.Company == nil || sarah.Company.Name != "Acme" || sarah.Company.Path == nil || *sarah.Company.Path != "companies/acme.md" {
		t.Errorf("company = %+v", sarah.Company)
	}
	// The most recent meeting before this one — not the later follow-up,
	// not the older one, not this note itself.
	if sarah.LastMeeting == nil || sarah.LastMeeting.Title != "Acme kickoff" || sarah.LastMeeting.Date != "2026-08-20T10:00" {
		t.Errorf("last meeting = %+v", sarah.LastMeeting)
	}
	if len(sarah.OpenTasks) != 1 || !strings.Contains(sarah.OpenTasks[0].Text, "deck") {
		t.Errorf("open tasks = %+v", sarah.OpenTasks)
	}
	if len(sarah.Waiting) != 1 || !strings.Contains(sarah.Waiting[0].Text, "contract") {
		t.Errorf("waiting = %+v", sarah.Waiting)
	}
	if len(sarah.RecentNotes) != 1 || sarah.RecentNotes[0].Title != "Pricing thoughts" {
		t.Errorf("recent notes = %+v", sarah.RecentNotes)
	}
	priya := prep.People[1]
	if priya.LastMeeting != nil || len(priya.OpenTasks) != 0 || priya.Company != nil {
		t.Errorf("priya has no history: %+v", priya)
	}

	// The same prep from the calendar side names who has no page.
	fromEvent, err := s.MeetingPrepForEvent("acme-sync@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if fromEvent.Path == nil || *fromEvent.Path != note.Path {
		t.Errorf("event prep finds the note: %v", fromEvent.Path)
	}
	if strings.Join(fromEvent.Unmatched, ", ") != "Jeff, Bob Unknown" {
		t.Errorf("unmatched = %v", fromEvent.Unmatched)
	}
	if len(fromEvent.People) != 2 || fromEvent.People[0].LastMeeting == nil || fromEvent.People[0].LastMeeting.Title != "Acme kickoff" {
		t.Errorf("event prep people = %+v", fromEvent.People)
	}
}

func TestCalendarEventsRangeAndMonthDots(t *testing.T) {
	s := newCalendarService(t, agendaICS)
	events, err := s.CalendarEvents("today", "tomorrow")
	if err != nil {
		t.Fatal(err)
	}
	// Two days: the holiday, two standups, the sync.
	if len(events) != 4 {
		t.Fatalf("events = %+v", events)
	}
	if _, err := s.CalendarEvents("2026-09-01", "2027-09-01"); err == nil {
		t.Error("a year is over the cap")
	}
	if _, err := s.CalendarEvents("2026-09-05", "2026-09-01"); err == nil {
		t.Error("to before from is refused")
	}

	month, err := s.Calendar("2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if month.Days[0].Events != 3 || month.Days[1].Events != 1 {
		t.Errorf("dots: 1st=%d 2nd=%d", month.Days[0].Events, month.Days[1].Events)
	}
}

func TestNoCalendarMeansNoEvents(t *testing.T) {
	s := newTestService(t)
	today, err := s.Today()
	if err != nil {
		t.Fatal(err)
	}
	if today.Events == nil || len(today.Events) != 0 {
		t.Errorf("events should be an empty list, got %#v", today.Events)
	}
	if _, err := s.AddCalendarFeed(context.Background(), "https://example.com/x.ics"); err == nil {
		t.Error("adding a feed without a calendar is refused")
	}
}
