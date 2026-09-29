// Package calendar reads the owner's calendar from secret ICS feed URLs
// (Fastmail, Google and iCloud all publish one) and answers "what is on
// between these two instants" — read-only; quire never writes to a calendar.
//
// This file is the pure half: parsing an iCalendar document and expanding
// it into concrete occurrences over a window, in a given time zone. The
// fetcher (fetcher.go) keeps the parsed feeds in memory and calls Expand on
// every query, so a change to the owner's time zone applies at once and any
// window can be asked for, without a re-fetch.
//
// Parsing is emersion/go-ical (RFC 5545 line folding, escaping, parameters)
// and recurrence is teambition/rrule-go (a port of python-dateutil's rrule,
// the reference implementation most calendar software is checked against).
// go-ical's own RecurrenceSet helper is deliberately not used: it fails the
// whole event on a TZID the Go runtime does not know (Outlook's "Pacific
// Standard Time") and on comma-separated EXDATE lists, both of which are
// common in real feeds.
package calendar

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// Event is one occurrence of a calendar event: a single meeting, or one
// instance of a recurring series with its overrides applied.
type Event struct {
	// UID is the series identifier, shared by every occurrence.
	UID string
	// RecurrenceID is the occurrence's original start (RFC 3339) when it
	// belongs to a recurring series, "" for a one-off. Together with UID it
	// identifies one occurrence even after it has been moved.
	RecurrenceID string
	Title        string
	Location     string
	// Link is where to join: a URL in LOCATION, the URL property, or a
	// meeting-service link found in the description — first found wins.
	Link      string
	Start     time.Time
	End       time.Time
	AllDay    bool
	Attendees []Attendee
	// Feed is the ID of the feed the event came from.
	Feed string
}

// Attendee is one person invited, as the feed names them.
type Attendee struct {
	Name      string
	Email     string
	Organizer bool
}

// maxOccurrences bounds one series' expansion inside a window, so a
// malformed or absurd rule (FREQ=SECONDLY) cannot stall a request.
const maxOccurrences = 2000

// Parse decodes an ICS document. A feed may hold several VCALENDARs
// back to back; all of them are returned.
//
// go-ical's decoder panics on some malformed input (a quoted parameter
// followed by more text, a line truncated mid-parameter) instead of
// returning an error. A feed URL is persisted and refetched at startup, so
// an unrecovered panic here would be a crash loop; it becomes an error the
// feed's status shows, and the last good copy keeps serving.
func Parse(raw []byte) (cals []*ical.Calendar, err error) {
	defer func() {
		if r := recover(); r != nil {
			cals, err = nil, fmt.Errorf("not a valid iCalendar document: %v", r)
		}
	}()
	dec := ical.NewDecoder(bytes.NewReader(raw))
	var out []*ical.Calendar
	for {
		cal, err := dec.Decode()
		switch {
		case err == nil:
			out = append(out, cal)
		case errors.Is(err, io.EOF) && len(out) > 0:
			return out, nil
		default:
			return nil, fmt.Errorf("not an iCalendar document: %w", err)
		}
	}
}

// Expand returns every occurrence overlapping [from, to), sorted by start.
// Floating times and all-day dates are read in loc. Events that cannot be
// understood are skipped rather than failing the feed: one malformed
// invitation must not blank the owner's whole agenda.
func Expand(cals []*ical.Calendar, feed string, from, to time.Time, loc *time.Location) []Event {
	var masters []ical.Event
	overrides := map[string][]ical.Event{}
	for _, cal := range cals {
		for _, ev := range cal.Events() {
			uid := propText(ev.Props, ical.PropUID)
			if ev.Props.Get(ical.PropRecurrenceID) != nil {
				overrides[uid] = append(overrides[uid], ev)
				continue
			}
			masters = append(masters, ev)
		}
	}

	var out []Event
	for _, master := range masters {
		out = append(out, expandMaster(master, overrides, feed, from, to, loc)...)
	}
	for _, list := range overrides {
		for _, ov := range list {
			event, ok := toEvent(ov, feed, loc)
			if !ok || cancelled(ov) || !overlaps(event, from, to) {
				continue
			}
			rid, err := propTime(ov.Props.Get(ical.PropRecurrenceID), loc)
			if err == nil {
				event.RecurrenceID = rid.Format(time.RFC3339)
			}
			out = append(out, event)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// expandMaster yields a series' occurrences in the window, leaving out any
// that an override (RECURRENCE-ID) replaces — Expand emits those itself,
// at their new time, which may even fall outside the original's window.
func expandMaster(master ical.Event, overrides map[string][]ical.Event, feed string, from, to time.Time, loc *time.Location) []Event {
	base, ok := toEvent(master, feed, loc)
	if !ok || cancelled(master) {
		return nil
	}
	ruleProp := master.Props.Get(ical.PropRecurrenceRule)
	if ruleProp == nil {
		if overlaps(base, from, to) {
			return []Event{base}
		}
		return nil
	}

	set, err := recurrenceSet(master, base.Start, from.Add(-base.End.Sub(base.Start)), loc)
	if err != nil {
		// An unparseable rule still has its first occurrence.
		if overlaps(base, from, to) {
			return []Event{base}
		}
		return nil
	}
	replaced := map[int64]bool{}
	for _, ov := range overrides[base.UID] {
		if rid, err := propTime(ov.Props.Get(ical.PropRecurrenceID), loc); err == nil {
			replaced[rid.Unix()] = true
		}
	}

	duration := base.End.Sub(base.Start)
	var out []Event
	next := set.Iterator()
	for count := 0; count < maxOccurrences*10; count++ {
		start, more := next()
		if !more || !start.Before(to) {
			break
		}
		occurrence := base
		occurrence.Start = start
		occurrence.End = endAfter(start, duration, base.AllDay)
		occurrence.RecurrenceID = start.Format(time.RFC3339)
		if replaced[start.Unix()] || !overlaps(occurrence, from, to) {
			continue
		}
		out = append(out, occurrence)
		if len(out) >= maxOccurrences {
			break
		}
	}
	return out
}

// localUntil reads an RRULE's UNTIL when it has no "Z". rrule-go parses
// every UNTIL as UTC, but RFC 5545 says a floating or date-only UNTIL is in
// DTSTART's zone — so west of UTC the last occurrence was dropped (a weekly
// all-day series "until 2026-10-13" lost the 13th). A date-only UNTIL
// includes that whole day. ok is false when UNTIL is absent or already UTC.
func localUntil(rule string, zone *time.Location) (time.Time, bool) {
	for _, part := range strings.Split(rule, ";") {
		key, value, _ := strings.Cut(part, "=")
		if !strings.EqualFold(key, "UNTIL") || strings.HasSuffix(value, "Z") {
			continue
		}
		if len(value) == len("20060102") {
			day, err := time.ParseInLocation("20060102", value, zone)
			return day.AddDate(0, 0, 1).Add(-time.Second), err == nil
		}
		until, err := time.ParseInLocation("20060102T150405", value, zone)
		return until, err == nil
	}
	return time.Time{}, false
}

// skipAhead moves a series' start forward by whole recurrence periods to
// just before notBefore, so expanding a years-old high-frequency series
// (hourly since 2020) iterates over the window, not over its history — the
// runaway guard used to be spent before the window was reached, and the
// series showed nothing. Only fixed-length periods (weekly and finer) are
// skipped, and never for a COUNT rule, whose count runs from the real
// start; monthly and yearly series are too sparse to need it. Days move by
// calendar date, so wall-clock time survives DST.
func skipAhead(option rrule.ROption, start, notBefore time.Time) time.Time {
	if option.Count > 0 || !start.Before(notBefore) {
		return start
	}
	interval := max(option.Interval, 1)
	var days int
	var step time.Duration
	switch option.Freq {
	case rrule.WEEKLY:
		days = 7 * interval
	case rrule.DAILY:
		days = interval
	case rrule.HOURLY:
		step = time.Duration(interval) * time.Hour
	case rrule.MINUTELY:
		step = time.Duration(interval) * time.Minute
	case rrule.SECONDLY:
		step = time.Duration(interval) * time.Second
	default:
		return start
	}
	// One period of slack, so a BYDAY earlier in the landing week is kept.
	if days > 0 {
		periods := int(notBefore.Sub(start).Hours()/24)/days - 1
		if periods <= 0 {
			return start
		}
		return start.AddDate(0, 0, periods*days)
	}
	periods := int64(notBefore.Sub(start)/step) - 1
	if periods <= 0 {
		return start
	}
	return start.Add(time.Duration(periods) * step)
}

// endAfter keeps an all-day event's length in calendar days rather than
// hours, so a one-day event across a DST change still ends at midnight.
func endAfter(start time.Time, duration time.Duration, allDay bool) time.Time {
	if allDay {
		days := int((duration + 12*time.Hour) / (24 * time.Hour))
		return start.AddDate(0, 0, days)
	}
	return start.Add(duration)
}

// recurrenceSet builds the RRULE + RDATE − EXDATE set for a master event.
// The rule is anchored to the event's start in its own zone, so a weekly
// 10:00 meeting stays at 10:00 across daylight-saving changes. notBefore is
// the earliest start the caller could want; see skipAhead.
func recurrenceSet(master ical.Event, start, notBefore time.Time, loc *time.Location) (*rrule.Set, error) {
	option, err := master.Props.RecurrenceRule()
	if err != nil || option == nil {
		return nil, fmt.Errorf("recurrence rule: %v", err)
	}
	if until, ok := localUntil(master.Props.Get(ical.PropRecurrenceRule).Value, start.Location()); ok {
		option.Until = until
	}
	option.Dtstart = skipAhead(*option, start, notBefore)
	rule, err := rrule.NewRRule(*option)
	if err != nil {
		return nil, err
	}
	set := &rrule.Set{}
	set.RRule(rule)
	for _, prop := range master.Props.Values(ical.PropExceptionDates) {
		for _, t := range propTimes(prop, loc) {
			set.ExDate(t)
		}
	}
	for _, prop := range master.Props.Values(ical.PropRecurrenceDates) {
		for _, t := range propTimes(prop, loc) {
			set.RDate(t)
		}
	}
	return set, nil
}

// toEvent reads the fields quire shows. ok is false for an event with no
// usable start — there is nothing to put on an agenda.
func toEvent(ev ical.Event, feed string, loc *time.Location) (Event, bool) {
	startProp := ev.Props.Get(ical.PropDateTimeStart)
	if startProp == nil {
		return Event{}, false
	}
	start, err := propTime(startProp, loc)
	if err != nil {
		return Event{}, false
	}
	event := Event{
		UID:      propText(ev.Props, ical.PropUID),
		Title:    strings.TrimSpace(propText(ev.Props, ical.PropSummary)),
		Location: strings.TrimSpace(propText(ev.Props, ical.PropLocation)),
		Start:    start,
		AllDay:   isDate(startProp),
		Feed:     feed,
	}
	if event.Title == "" {
		event.Title = "(no title)"
	}
	event.End = eventEnd(ev, event, loc)
	event.Link = meetingLink(ev.Props, event.Location)
	event.Attendees = attendees(ev.Props)
	return event, true
}

// eventEnd is DTEND, else DTSTART + DURATION, else the RFC 5545 default:
// one day for an all-day event, zero length for a timed one.
func eventEnd(ev ical.Event, event Event, loc *time.Location) time.Time {
	if prop := ev.Props.Get(ical.PropDateTimeEnd); prop != nil {
		if end, err := propTime(prop, loc); err == nil && !end.Before(event.Start) {
			return end
		}
	}
	if prop := ev.Props.Get(ical.PropDuration); prop != nil {
		if duration, err := prop.Duration(); err == nil && duration >= 0 {
			return endAfter(event.Start, duration, event.AllDay)
		}
	}
	if event.AllDay {
		return event.Start.AddDate(0, 0, 1)
	}
	return event.Start
}

// overlaps is the half-open interval test; a zero-length event counts when
// its instant falls in the window.
func overlaps(event Event, from, to time.Time) bool {
	if event.End.Equal(event.Start) {
		return !event.Start.Before(from) && event.Start.Before(to)
	}
	return event.Start.Before(to) && event.End.After(from)
}

func cancelled(ev ical.Event) bool {
	return strings.EqualFold(propText(ev.Props, ical.PropStatus), "CANCELLED")
}

// ---- property helpers ----

// propText unescapes a TEXT value. go-ical's Prop.Text splits on unescaped
// commas (it treats every text as a list), which truncates "Lunch, then a
// walk" to "Lunch" — so the unescaping is done here instead.
func propText(props ical.Props, name string) string {
	prop := props.Get(name)
	if prop == nil {
		return ""
	}
	return textEscapes.Replace(prop.Value)
}

var textEscapes = strings.NewReplacer(`\\`, `\`, `\;`, ";", `\,`, ",", `\n`, "\n", `\N`, "\n")

// isDate reports an all-day value: VALUE=DATE, or a bare YYYYMMDD from a
// producer that left the parameter off.
func isDate(prop *ical.Prop) bool {
	return strings.EqualFold(prop.Params.Get(ical.ParamValue), "DATE") || len(prop.Value) == len("20060102")
}

// propTime parses one DATE or DATE-TIME value: UTC ("Z"), TZID-qualified,
// or floating (read in loc, as are all-day dates).
func propTime(prop *ical.Prop, loc *time.Location) (time.Time, error) {
	if prop == nil {
		return time.Time{}, fmt.Errorf("missing")
	}
	return parseValue(prop.Value, isDate(prop), zoneFor(prop.Params.Get(ical.PropTimezoneID), loc))
}

// propTimes parses a possibly comma-separated EXDATE/RDATE list.
func propTimes(prop ical.Prop, loc *time.Location) []time.Time {
	zone := zoneFor(prop.Params.Get(ical.PropTimezoneID), loc)
	var out []time.Time
	for _, value := range strings.Split(prop.Value, ",") {
		value = strings.TrimSpace(value)
		if t, err := parseValue(value, len(value) == len("20060102"), zone); err == nil {
			out = append(out, t)
		}
	}
	return out
}

func parseValue(value string, date bool, loc *time.Location) (time.Time, error) {
	switch {
	case date:
		return time.ParseInLocation("20060102", value, loc)
	case strings.HasSuffix(value, "Z"):
		return time.ParseInLocation("20060102T150405Z", value, time.UTC)
	default:
		return time.ParseInLocation("20060102T150405", value, loc)
	}
}

// windowsZones maps the Windows time zone names Outlook and Exchange write
// into TZID onto IANA names. Only the common ones: an unknown name falls
// back to the owner's own zone, which is right for most invitations anyway.
var windowsZones = map[string]string{
	"Pacific Standard Time":        "America/Los_Angeles",
	"Mountain Standard Time":       "America/Denver",
	"US Mountain Standard Time":    "America/Phoenix",
	"Central Standard Time":        "America/Chicago",
	"Eastern Standard Time":        "America/New_York",
	"Atlantic Standard Time":       "America/Halifax",
	"Newfoundland Standard Time":   "America/St_Johns",
	"Alaskan Standard Time":        "America/Anchorage",
	"Hawaiian Standard Time":       "Pacific/Honolulu",
	"GMT Standard Time":            "Europe/London",
	"Greenwich Standard Time":      "Atlantic/Reykjavik",
	"W. Europe Standard Time":      "Europe/Berlin",
	"Romance Standard Time":        "Europe/Paris",
	"Central Europe Standard Time": "Europe/Budapest",
	"E. Europe Standard Time":      "Europe/Chisinau",
	"India Standard Time":          "Asia/Kolkata",
	"China Standard Time":          "Asia/Shanghai",
	"Tokyo Standard Time":          "Asia/Tokyo",
	"AUS Eastern Standard Time":    "Australia/Sydney",
	"New Zealand Standard Time":    "Pacific/Auckland",
	"UTC":                          "UTC",
}

// zoneFor resolves a TZID: an IANA name, a known Windows name, else loc.
func zoneFor(tzid string, loc *time.Location) *time.Location {
	if tzid == "" {
		return loc
	}
	// Some producers quote or prefix the name ("/Europe/London").
	tzid = strings.Trim(tzid, `"/`)
	if zone, err := time.LoadLocation(tzid); err == nil {
		return zone
	}
	if name, ok := windowsZones[tzid]; ok {
		if zone, err := time.LoadLocation(name); err == nil {
			return zone
		}
	}
	return loc
}

// ---- attendees and links ----

// attendees lists the organizer first, then every ATTENDEE not already
// seen (organizers usually list themselves as an attendee too). Resources
// and rooms are left out: they are not people to prepare for.
func attendees(props ical.Props) []Attendee {
	var out []Attendee
	seen := map[string]bool{}
	add := func(prop ical.Prop, organizer bool) {
		if strings.EqualFold(prop.Params.Get(ical.ParamCalendarUserType), "RESOURCE") ||
			strings.EqualFold(prop.Params.Get(ical.ParamCalendarUserType), "ROOM") {
			return
		}
		email := strings.TrimSpace(prop.Value)
		if len(email) >= 7 && strings.EqualFold(email[:7], "mailto:") {
			email = email[7:]
		}
		email = strings.ToLower(email)
		name := strings.Trim(strings.TrimSpace(prop.Params.Get(ical.ParamCommonName)), `"`)
		key := email
		if key == "" {
			key = strings.ToLower(name)
		}
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Attendee{Name: name, Email: email, Organizer: organizer})
	}
	if organizer := props.Get(ical.PropOrganizer); organizer != nil {
		add(*organizer, true)
	}
	for _, prop := range props.Values(ical.PropAttendee) {
		add(prop, false)
	}
	return out
}

var urlPattern = regexp.MustCompile(`https?://[^\s<>"')\]]+`)

// meetingHosts are the video services whose links are worth lifting out of
// an invitation's description; an arbitrary first URL there is as likely
// to be a privacy policy as the call.
var meetingHosts = []string{"zoom.us", "meet.google.com", "teams.microsoft.com", "teams.live.com", "webex.com", "whereby.com", "meet.jit.si", "around.co", "chime.aws"}

func meetingLink(props ical.Props, location string) string {
	if link := urlPattern.FindString(location); link != "" {
		return link
	}
	if prop := props.Get(ical.PropURL); prop != nil {
		if parsed, err := url.Parse(prop.Value); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
			return prop.Value
		}
	}
	for _, link := range urlPattern.FindAllString(propText(props, ical.PropDescription), -1) {
		parsed, err := url.Parse(link)
		if err != nil {
			continue
		}
		for _, host := range meetingHosts {
			if parsed.Hostname() == host || strings.HasSuffix(parsed.Hostname(), "."+host) {
				return link
			}
		}
	}
	return ""
}
