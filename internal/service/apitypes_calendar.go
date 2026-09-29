// Wire shapes for the calendar feed and meeting prep. Kept apart from
// apitypes.go so the feature reads as one unit; tygo reads both files
// (tygo.yaml) into web/src/api/generated.ts.
package service

// CalendarEvent is one occurrence on the owner's calendar. Times are
// RFC 3339 in the configured time zone.
type CalendarEvent struct {
	// UID identifies the series; with Date it identifies one occurrence.
	UID string `json:"uid"`
	// RecurrenceID is the occurrence's original start when it belongs to a
	// recurring series, "" for a one-off.
	RecurrenceID string `json:"recurrence_id"`
	Title        string `json:"title"`
	Start        string `json:"start"`
	End          string `json:"end"`
	// Date is the start's day (YYYY-MM-DD) in the configured zone.
	Date   string `json:"date"`
	AllDay bool   `json:"all_day"`
	// Location is the free-text location; Link is where to join, if any.
	Location  string          `json:"location"`
	Link      string          `json:"link"`
	Attendees []EventAttendee `json:"attendees"`
	// NotePath is the meeting note made for this occurrence (matched by
	// event_uid and date), null when there is none yet.
	NotePath *string `json:"note_path"`
}

// EventAttendee is one invitee, matched to a person document when one has
// their email (or, failing that, answers to their name).
type EventAttendee struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Organizer bool   `json:"organizer"`
	// Person is the matched person document; null when unmatched.
	Person *CalendarDoc `json:"person"`
}

// CalendarFeed is one subscribed feed as Settings shows it. The URL is a
// secret and is never returned; URL here is the masked form.
type CalendarFeed struct {
	ID  string `json:"id"`
	URL string `json:"url"`
	// LastSuccess and LastAttempt are RFC 3339, null before the first try.
	LastAttempt *string `json:"last_attempt"`
	LastSuccess *string `json:"last_success"`
	// Error is the last failure (never containing the URL); "" when the
	// latest fetch worked.
	Error    string `json:"error"`
	Failures int    `json:"failures"`
	// Events is how many VEVENTs the feed held at its last success.
	Events int `json:"events"`
	// Fetching is true while a just-added feed's first fetch is running.
	Fetching bool `json:"fetching"`
}

// MeetingPrep is what to know walking into a meeting: for each attendee
// with a person page, when you last met, what is open between you, and
// what they are in the middle of.
type MeetingPrep struct {
	Title string `json:"title"`
	// Date is the meeting's start ("YYYY-MM-DDTHH:MM"), "" when unknown.
	Date string `json:"date"`
	// Path is the meeting note, null when prepping a calendar event that
	// has no note yet.
	Path     *string      `json:"path"`
	EventUID string       `json:"event_uid"`
	People   []PrepPerson `json:"people"`
	// Unmatched are attendees with no person document, by name or email.
	Unmatched []string `json:"unmatched"`
}

// PrepPerson is one attendee's context.
type PrepPerson struct {
	Person DocMeta `json:"person"`
	// Company is the person's company (frontmatter company:), null when
	// unset; Path is null inside it when the company has no page.
	Company *PrepCompany `json:"company"`
	// LastMeeting is the most recent other meeting linking them that
	// started before this one; null when this is the first.
	LastMeeting *PrepMeeting `json:"last_meeting"`
	// OpenTasks are open tasks elsewhere that mention them, minus the
	// waiting ones, which are in Waiting.
	OpenTasks []Task `json:"open_tasks"`
	Waiting   []Task `json:"waiting"`
	// RecentNotes are the latest non-meeting documents linking them.
	RecentNotes []DocMeta `json:"recent_notes"`
}

// PrepCompany names a person's company.
type PrepCompany struct {
	Name string  `json:"name"`
	Path *string `json:"path"`
}

// PrepMeeting is a previous meeting and when it was.
type PrepMeeting struct {
	// Embedded: DocMeta's fields marshal inline, so TS extends rather than nests.
	DocMeta `tstype:",extends,required"`
	Date    string `json:"date"`
}
