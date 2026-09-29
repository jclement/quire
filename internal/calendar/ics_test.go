package calendar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// expandFixture parses testdata/<name> and expands it over [from, to).
func expandFixture(t *testing.T, name string, from, to time.Time, loc *time.Location) []Event {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	cals, err := Parse(raw)
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return Expand(cals, "feed1", from, to, loc)
}

func TestRecurringWeeklyWithExdateOverrideAndCancellation(t *testing.T) {
	// New York rather than the owner's Edmonton: Alberta's tzdata has no
	// DST change after 2026, and the test wants one inside the window.
	newYork := mustZone(t, "America/New_York")
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, newYork)
	to := time.Date(2026, 11, 30, 0, 0, 0, 0, newYork)
	events := expandFixture(t, "recurring.ics", from, to, newYork)

	var got []string
	for _, e := range events {
		got = append(got, e.Start.In(newYork).Format("01-02 15:04")+" "+e.Title)
	}
	want := []string{
		"09-01 10:00 Sarah / Jeff 1:1, weekly",
		"09-08 10:00 Sarah / Jeff 1:1, weekly",
		// 09-15 is an EXDATE.
		// 09-22 moved to Wednesday afternoon by a RECURRENCE-ID override.
		"09-23 14:00 Sarah / Jeff 1:1 (moved)",
		"09-29 10:00 Sarah / Jeff 1:1, weekly",
		// 10-06 is overridden as CANCELLED.
		"10-13 10:00 Sarah / Jeff 1:1, weekly",
		"10-20 10:00 Sarah / Jeff 1:1, weekly",
		"10-27 10:00 Sarah / Jeff 1:1, weekly",
		// DST ends Nov 1: still 10:00 local, an hour later in UTC.
		"11-03 10:00 Sarah / Jeff 1:1, weekly",
		// UNTIL 2026-11-10T00:00Z stops the series before 11-10.
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("occurrences:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if before, after := events[0].Start.UTC().Hour(), events[len(events)-1].Start.UTC().Hour(); before != 14 || after != 15 {
		t.Errorf("10:00 New York is 14:00 UTC before DST ends and 15:00 after, got %d and %d", before, after)
	}
	moved := events[2]
	if moved.RecurrenceID != time.Date(2026, 9, 22, 10, 0, 0, 0, newYork).Format(time.RFC3339) {
		t.Errorf("moved occurrence keeps its original RECURRENCE-ID, got %q", moved.RecurrenceID)
	}
	if moved.UID != "weekly-1on1@example.com" {
		t.Errorf("override shares the series UID, got %q", moved.UID)
	}

	first := events[0]
	if first.End.Sub(first.Start) != 30*time.Minute {
		t.Errorf("duration = %v", first.End.Sub(first.Start))
	}
	if first.Link != "https://meet.google.com/abc-defg-hij" {
		t.Errorf("link from LOCATION = %q", first.Link)
	}
	// Organizer first, deduplicated against the attendee list; rooms dropped;
	// mailto stripped and emails lowercased; quoted CN unquoted.
	var people []string
	for _, a := range first.Attendees {
		people = append(people, a.Name+" <"+a.Email+">")
	}
	wantPeople := "Jeff Clement <jeff@example.com>, Sarah Chen <sarah.chen@acme.example>, Bob Unknown <bob@elsewhere.example>"
	if strings.Join(people, ", ") != wantPeople {
		t.Errorf("attendees = %s", strings.Join(people, ", "))
	}
	if !first.Attendees[0].Organizer {
		t.Error("the organizer is marked")
	}
}

func TestOverrideShowsEvenWhenOriginalSlotIsOutsideWindow(t *testing.T) {
	newYork := mustZone(t, "America/New_York")
	// Only Wednesday the 23rd: the original Tuesday slot is outside.
	from := time.Date(2026, 9, 23, 0, 0, 0, 0, newYork)
	events := expandFixture(t, "recurring.ics", from, from.AddDate(0, 0, 1), newYork)
	if len(events) != 1 || events[0].Title != "Sarah / Jeff 1:1 (moved)" {
		t.Fatalf("events = %+v", events)
	}
}

func TestTimeZones(t *testing.T) {
	edmonton := mustZone(t, "America/Edmonton")
	day := time.Date(2026, 9, 29, 0, 0, 0, 0, edmonton)
	events := expandFixture(t, "timezones.ics", day, day.AddDate(0, 0, 1), edmonton)

	byTitle := map[string]Event{}
	for _, e := range events {
		byTitle[e.Title] = e
	}
	for title, want := range map[string]string{
		"Gym":                         "08:00", // floating: read in the owner's zone
		"Call with the London office": "08:00", // 15:00 BST = 08:00 MDT
		"Vendor review":               "10:00", // Windows TZID, 09:00 PDT
		"Standup":                     "14:00", // 20:00Z
	} {
		event, ok := byTitle[title]
		if !ok {
			t.Errorf("%s missing from %v", title, events)
			continue
		}
		if got := event.Start.In(edmonton).Format("15:04"); got != want {
			t.Errorf("%s starts %s Edmonton time, want %s", title, got, want)
		}
	}
	if d := byTitle["Vendor review"].End.Sub(byTitle["Vendor review"].Start); d != 45*time.Minute {
		t.Errorf("DURATION gives the end: %v", d)
	}
	// The zoom link is lifted from the description; the privacy URL is not.
	if link := byTitle["Call with the London office"].Link; link != "https://acme.zoom.us/j/123456" {
		t.Errorf("description link = %q", link)
	}
	if byTitle["Vendor review"].Link != "" {
		t.Errorf("a room name is not a link: %q", byTitle["Vendor review"].Link)
	}
}

func TestAllDayEvents(t *testing.T) {
	edmonton := mustZone(t, "America/Edmonton")
	day := time.Date(2026, 9, 30, 0, 0, 0, 0, edmonton)
	events := expandFixture(t, "allday.ics", day, day.AddDate(0, 0, 1), edmonton)
	if len(events) != 2 {
		t.Fatalf("want the offsite (day 2 of 2) and the yearly birthday, got %+v", events)
	}
	for _, e := range events {
		if !e.AllDay {
			t.Errorf("%s should be all-day", e.Title)
		}
		if e.Start.Location() != edmonton || e.Start.Hour() != 0 {
			t.Errorf("%s: all-day dates are midnight in the owner's zone, got %v", e.Title, e.Start)
		}
	}
	if events[0].Title != "Leadership offsite" || !events[0].End.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, edmonton)) {
		t.Errorf("offsite = %+v", events[0])
	}
	if events[1].Title != "Mum's birthday" || events[1].Start.Year() != 2026 {
		t.Errorf("birthday = %+v", events[1])
	}

	// The day after the offsite ends shows nothing from it.
	after := expandFixture(t, "allday.ics", day.AddDate(0, 0, 1), day.AddDate(0, 0, 2), edmonton)
	if len(after) != 0 {
		t.Errorf("DTEND is exclusive; got %+v", after)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte("<html>login</html>")); err == nil {
		t.Error("an HTML login page is not a calendar")
	}
}

// go-ical's decoder panics on some malformed lines rather than erroring;
// a feed like that must become an error, not a crashed server.
func TestParseSurvivesDecoderPanics(t *testing.T) {
	for name, line := range map[string]string{
		"quoted param with trailing text": `X;P="a"b:v`,
		"truncated attendee":              `ATTENDEE;CN=Bo`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART:20260929T100000Z\r\n" + line + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
			if _, err := Parse([]byte(raw)); err == nil {
				t.Error("a malformed feed is an error")
			}
		})
	}
}

// UNTIL without a Z is wall-clock in DTSTART's zone (and a date-only UNTIL
// includes that whole day); rrule-go reads it as UTC, which west of UTC
// drops the last occurrence. Google writes the date-only form.
func TestUntilWithoutZoneIsLocalAndInclusive(t *testing.T) {
	edmonton := mustZone(t, "America/Edmonton")
	newYork := mustZone(t, "America/New_York")
	for _, tc := range []struct {
		name, start, until string
		loc                *time.Location
		want               []string
	}{
		{"date-only all-day weekly", "DTSTART;VALUE=DATE:20260929", "20261013", edmonton,
			[]string{"09-29", "10-06", "10-13"}},
		{"floating timed daily", "DTSTART:20260929T180000", "20261001T180000", edmonton,
			[]string{"09-29", "09-30", "10-01"}},
		{"date-only timed weekly", "DTSTART;TZID=America/Edmonton:20260929T180000", "20261013", edmonton,
			[]string{"09-29", "10-06", "10-13"}},
		// All-day across DST (New York falls back Nov 1): still one per week,
		// still midnight, last one kept.
		{"all-day across DST", "DTSTART;VALUE=DATE:20261027", "20261110", newYork,
			[]string{"10-27", "11-03", "11-10"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			freq := "WEEKLY"
			if strings.Contains(tc.name, "daily") {
				freq = "DAILY"
			}
			raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:t\r\nBEGIN:VEVENT\r\nUID:u\r\nDTSTAMP:20260101T000000Z\r\n" +
				tc.start + "\r\nRRULE:FREQ=" + freq + ";UNTIL=" + tc.until + "\r\nSUMMARY:S\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
			cals, err := Parse([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			from := time.Date(2026, 9, 1, 0, 0, 0, 0, tc.loc)
			var got []string
			for _, e := range Expand(cals, "f", from, from.AddDate(0, 4, 0), tc.loc) {
				got = append(got, e.Start.In(tc.loc).Format("01-02"))
				if e.AllDay && e.Start.In(tc.loc).Hour() != 0 {
					t.Errorf("all-day occurrence not at midnight: %v", e.Start)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("occurrences %v, want %v", got, tc.want)
			}
		})
	}
}

// An old high-frequency series still shows its current occurrences: the
// runaway guard must count inside the window, not from DTSTART.
func TestOldHighFrequencySeriesStillExpands(t *testing.T) {
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:t\r\nBEGIN:VEVENT\r\nUID:h\r\nDTSTAMP:20200101T000000Z\r\n" +
		"DTSTART:20200101T000000Z\r\nDTEND:20200101T001500Z\r\nRRULE:FREQ=HOURLY\r\nSUMMARY:Ping\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cals, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	if got := Expand(cals, "f", day, day.AddDate(0, 0, 1), time.UTC); len(got) != 24 {
		t.Errorf("hourly since 2020 should give 24 today, got %d", len(got))
	}
	// And a runaway rule inside the window is still bounded.
	raw = strings.Replace(raw, "FREQ=HOURLY", "FREQ=SECONDLY", 1)
	cals, _ = Parse([]byte(raw))
	if got := Expand(cals, "f", day, day.AddDate(0, 0, 1), time.UTC); len(got) > maxOccurrences {
		t.Errorf("runaway rule gave %d occurrences, cap is %d", len(got), maxOccurrences)
	}
}
