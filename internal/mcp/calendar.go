// Calendar and meeting-prep tools. Registered from newServer through
// registerCalendarTools, against the same scopes as everything else: reading the
// calendar and composing prep are reads; making a meeting note is a write.
// Managing feeds is deliberately not exposed — a feed URL is a credential,
// and handing an agent a tool that takes one invites pasting it into chat.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/service"
)

func registerCalendarTools(s *sdk.Server, t *tools, allows func(string) bool) {
	if allows(auth.ScopeRead) {
		sdk.AddTool(s, &sdk.Tool{Name: "calendar_events", Annotations: readOnly,
			Description: "The owner's actual calendar, from the ICS feeds subscribed in Settings: each event occurrence with start and end (in the owner's time zone), location, join link, attendees matched to person documents, and note_path when a meeting note already exists for it. from and to are inclusive days (YYYY-MM-DD or today, tomorrow, fri, +3d); from defaults to today and to to a week later, at most 92 days. Use this for 'what's on tomorrow' or 'when am I next meeting Sarah'; the calendar tool is the vault's own history of days, not the schedule."},
			t.calendarEvents)
		sdk.AddTool(s, &sdk.Tool{Name: "meeting_prep", Annotations: readOnly,
			Description: "Everything to know walking into a meeting, for every attendee with a person page at once: the last meeting you had together, open tasks that mention them, what you are waiting on them for (⏳), their company, and recent notes about them — plus the attendees who have no page. Pass path for a meeting note, or event_uid (from today or calendar_events) for a calendar event, with date to pick one occurrence of a recurring series. Prefer it to calling person_context once per attendee."},
			t.meetingPrep)
	}
	if allows(auth.ScopeWrite) {
		sdk.AddTool(s, &sdk.Tool{Name: "create_meeting_from_event", Annotations: idempotent,
			Description: "Make the meeting note for one calendar event occurrence (event_uid and its date, from today or calendar_events), from the meeting template: dated at the event's start, attendees with person pages linked in people:, the rest named on the Attendees line, and event_uid recorded. Idempotent — when that occurrence already has a note, the existing note is returned rather than a second one made."},
			t.createMeetingFromEvent)
	}
}

type calendarEventsIn struct {
	From string `json:"from,omitempty" jsonschema:"first day, YYYY-MM-DD or natural (today, fri, +3d); default today"`
	To   string `json:"to,omitempty" jsonschema:"last day, inclusive; default a week after from"`
}

type calendarEventsOut struct {
	Events []service.CalendarEvent `json:"events"`
}

type meetingPrepIn struct {
	Path     string `json:"path,omitempty" jsonschema:"vault path of a meeting note, e.g. meetings/2026-09-01-acme-sync.md"`
	EventUID string `json:"event_uid,omitempty" jsonschema:"a calendar event's uid, when there is no note (or you have the event, not the note)"`
	Date     string `json:"date,omitempty" jsonschema:"with event_uid: which occurrence (YYYY-MM-DD); default the next one"`
}

type eventNoteIn struct {
	EventUID string `json:"event_uid" jsonschema:"the event's uid, from today or calendar_events"`
	Date     string `json:"date" jsonschema:"the occurrence's date, YYYY-MM-DD (the event's date field)"`
}

type eventNoteOut struct {
	Document service.Document `json:"document"`
	// Created is false when the occurrence already had a note.
	Created bool `json:"created"`
}

func (t *tools) calendarEvents(_ context.Context, _ *sdk.CallToolRequest, in calendarEventsIn) (*sdk.CallToolResult, calendarEventsOut, error) {
	events, err := t.svc.CalendarEvents(in.From, in.To)
	return nil, calendarEventsOut{Events: events}, err
}

func (t *tools) meetingPrep(_ context.Context, _ *sdk.CallToolRequest, in meetingPrepIn) (*sdk.CallToolResult, service.MeetingPrep, error) {
	if in.Path != "" {
		prep, err := t.svc.MeetingPrep(in.Path)
		return nil, prep, err
	}
	prep, err := t.svc.MeetingPrepForEvent(in.EventUID, in.Date)
	return nil, prep, err
}

func (t *tools) createMeetingFromEvent(_ context.Context, _ *sdk.CallToolRequest, in eventNoteIn) (*sdk.CallToolResult, eventNoteOut, error) {
	doc, created, err := t.svc.CreateMeetingFromEvent(in.EventUID, in.Date)
	if created || err != nil {
		t.record("create_meeting_from_event", doc.Path, in.EventUID+" "+in.Date, err)
	}
	return nil, eventNoteOut{Document: doc, Created: created}, err
}
