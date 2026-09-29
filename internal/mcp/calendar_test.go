package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jclement/quire/internal/calendar"
)

// TestCalendarToolsOverTheWire drives the three calendar tools through a
// real MCP session, so their schemas and outputs are exercised as a client
// sees them.
func TestCalendarToolsOverTheWire(t *testing.T) {
	session, svc := connectWithService(t)
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:test\r\n" +
			"BEGIN:VEVENT\r\nUID:planning@example.com\r\nDTSTAMP:20260801T000000Z\r\n" +
			"DTSTART:20260901T150000\r\nDTEND:20260901T160000\r\nSUMMARY:Planning\r\n" +
			"ATTENDEE;CN=Sarah Chen:mailto:sarah@acme.example\r\n" +
			"END:VEVENT\r\nEND:VCALENDAR\r\n"))
	}))
	t.Cleanup(feed.Close)
	svc.Feeds = calendar.NewFetcher(calendar.OpenStore(filepath.Join(t.TempDir(), "feeds.json")))
	if _, err := svc.AddCalendarFeed(context.Background(), feed.URL+"/x.ics"); err != nil {
		t.Fatal(err)
	}
	call(t, session, "create_document", map[string]any{"type": "person", "title": "Sarah Chen"})

	events := call(t, session, "calendar_events", map[string]any{"from": "today"})["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("events = %v", events)
	}
	event := events[0].(map[string]any)
	attendee := event["attendees"].([]any)[0].(map[string]any)
	if attendee["person"].(map[string]any)["path"] != "people/sarah-chen.md" {
		t.Errorf("attendee matched by name: %v", attendee)
	}

	args := map[string]any{"event_uid": "planning@example.com", "date": event["date"]}
	first := call(t, session, "create_meeting_from_event", args)
	second := call(t, session, "create_meeting_from_event", args)
	if first["created"] != true || second["created"] != false {
		t.Errorf("created: %v then %v", first["created"], second["created"])
	}
	path := first["document"].(map[string]any)["path"]

	prep := call(t, session, "meeting_prep", map[string]any{"event_uid": "planning@example.com"})
	if prep["path"] != path || len(prep["people"].([]any)) != 1 {
		t.Errorf("prep = %v", prep)
	}
}
