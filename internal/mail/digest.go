// The morning digest: one email that runs the day — meetings, birthdays,
// overdue, due today, waiting. Email is the inbox people check involuntarily
// (the life-admin review's words), so this is the reminder channel. The HTML
// is table-free and inline-styled so it survives Gmail and Outlook: a single
// card, a dated masthead, sections with counts, each task a row with its
// due date set off to the right, everything linking back into quire.
package mail

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/jclement/quire/internal/service"
)

// Palette, deliberately close to the app's own tokens.
const (
	inkHeading = "#1c1b19"
	inkBody    = "#3d3b37"
	inkMuted   = "#8a877f"
	inkAccent  = "#5b5bd6"
	inkDanger  = "#b42318"
	paperCard  = "#ffffff"
	paperPage  = "#f4f3ef"
	rule       = "#e5e3dd"
)

// BuildDigest renders the Today payload as an email; empty=true means there
// is nothing worth sending (no meetings, no dated tasks, no birthdays — a
// quiet day sends no mail, an empty inbox beats a hollow one).
func BuildDigest(today service.TodayPayload, baseURL string) (msg Message, empty bool) {
	if len(today.Meetings) == 0 && len(today.Overdue) == 0 && len(today.DueToday) == 0 &&
		len(today.Waiting) == 0 && len(today.Birthdays) == 0 {
		return Message{}, true
	}
	return render(today, baseURL, ""), false
}

// BuildSample is the test email: today's real digest when there is one,
// otherwise a small made-up day so the layout can be judged. The note at
// the top says which.
func BuildSample(today service.TodayPayload, baseURL string) Message {
	if msg, empty := BuildDigest(today, baseURL); !empty {
		return withNote(msg, "This is a test send of today's digest.")
	}
	age := 40
	sample := service.TodayPayload{
		Date: today.Date,
		Meetings: []service.DocMeta{
			{Path: "meetings/sample-standup.md", Title: "Platform standup", Type: "meeting"},
		},
		Overdue: []service.Task{
			{Text: "Send the revised quote", Due: samplePtr(shiftDay(today.Date, -2)), DocPath: "daily/sample.md", DocTitle: "Daily"},
		},
		DueToday: []service.Task{
			{Text: "Book the dentist", Due: samplePtr(today.Date), DocPath: "notes/life-admin.md", DocTitle: "Life admin"},
			{Text: "Review the roadmap draft", Due: samplePtr(today.Date), DocPath: "projects/roadmap.md", DocTitle: "Roadmap"},
		},
		Waiting: []service.Task{
			{Text: "Contract back from legal", DocPath: "projects/acme.md", DocTitle: "Acme"},
		},
		Birthdays: []service.Birthday{{Path: "people/sam.md", Title: "Sam Rivera", DaysUntil: 3, Age: &age}},
	}
	return withNote(render(sample, baseURL, ""), "This is a test send. Today has nothing to report, so this is a sample day.")
}

func samplePtr(s string) *string { return &s }

func shiftDay(day string, days int) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}

func withNote(msg Message, note string) Message {
	msg.Subject = "[test] " + msg.Subject
	msg.Text = note + "\n\n" + msg.Text
	msg.HTML = strings.Replace(msg.HTML, "<!--note-->",
		fmt.Sprintf(`<p style="margin:0 0 16px;padding:8px 12px;border-radius:6px;background:#fff7d6;color:#7a5a00;font-size:13px">%s</p>`, html.EscapeString(note)), 1)
	return msg
}

// section is one block of the digest.
type section struct {
	title string
	tone  string // text colour for the count badge
	rows  []row
}

type row struct {
	text string
	meta string // right-aligned: a date, "today!", "in 3 days"
	href string
	warn bool
}

func render(today service.TodayPayload, baseURL, _ string) Message {
	docHref := func(path string) string {
		return strings.TrimRight(baseURL, "/") + "/doc/" + path
	}
	taskRows := func(tasks []service.Task, warn bool) []row {
		var out []row
		for _, t := range tasks {
			r := row{text: t.Text, href: docHref(t.DocPath), warn: warn}
			if t.Due != nil {
				r.meta = prettyDate(*t.Due, today.Date)
			} else if t.DocTitle != "" {
				r.meta = t.DocTitle
			}
			out = append(out, r)
		}
		return out
	}
	var meetings []row
	for _, m := range today.Meetings {
		meetings = append(meetings, row{text: m.Title, href: docHref(m.Path)})
	}
	var birthdays []row
	for _, b := range today.Birthdays {
		when := fmt.Sprintf("in %d days", b.DaysUntil)
		if b.DaysUntil == 0 {
			when = "today!"
		} else if b.DaysUntil == 1 {
			when = "tomorrow"
		}
		text := "🎂 " + b.Title
		if b.Age != nil {
			text += fmt.Sprintf(" turns %d", *b.Age)
		}
		birthdays = append(birthdays, row{text: text, meta: when, href: docHref(b.Path)})
	}
	sections := []section{
		{"Meetings", inkAccent, meetings},
		{"Birthdays", inkAccent, birthdays},
		{"Overdue", inkDanger, taskRows(today.Overdue, true)},
		{"Due today", inkHeading, taskRows(today.DueToday, false)},
		{"Waiting for", inkMuted, taskRows(today.Waiting, false)},
	}

	var text, body strings.Builder
	for _, s := range sections {
		if len(s.rows) == 0 {
			continue
		}
		fmt.Fprintf(&text, "%s\n", strings.ToUpper(s.title))
		fmt.Fprintf(&body, `<div style="margin:22px 0 0"><div style="display:flex;align-items:baseline;gap:8px;margin:0 0 6px;padding:0 0 6px;border-bottom:1px solid %s"><span style="font-size:11px;font-weight:600;letter-spacing:.08em;text-transform:uppercase;color:%s">%s</span><span style="font-size:11px;color:%s">%d</span></div>`,
			rule, inkMuted, html.EscapeString(s.title), s.tone, len(s.rows))
		for _, r := range s.rows {
			meta := ""
			if r.meta != "" {
				meta = " — " + r.meta
			}
			fmt.Fprintf(&text, "  • %s%s\n", r.text, meta)
			colour := inkBody
			if r.warn {
				colour = inkDanger
			}
			fmt.Fprintf(&body, `<a href="%s" style="display:flex;justify-content:space-between;gap:12px;padding:7px 0;border-bottom:1px solid %s;text-decoration:none"><span style="color:%s;font-size:15px;line-height:1.4">%s</span><span style="color:%s;font-size:12px;white-space:nowrap;padding-top:3px">%s</span></a>`,
				html.EscapeString(r.href), rule, colour, html.EscapeString(r.text), inkMuted, html.EscapeString(r.meta))
		}
		text.WriteString("\n")
		body.WriteString("</div>")
	}
	fmt.Fprintf(&text, "— quire · %s\n", baseURL)

	dateLine := prettyLong(today.Date)
	page := fmt.Sprintf(`<!doctype html><html><body style="margin:0;padding:24px 12px;background:%s">
<div style="max-width:560px;margin:0 auto;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Inter,Helvetica,Arial,sans-serif;color:%s">
<div style="background:%s;border:1px solid %s;border-radius:10px;padding:24px 28px">
<!--note-->
<div style="font-size:11px;font-weight:600;letter-spacing:.12em;text-transform:uppercase;color:%s">quire</div>
<h1 style="margin:4px 0 0;font-size:22px;font-weight:600;color:%s;letter-spacing:-.01em">%s</h1>
%s
<p style="margin:28px 0 0;font-size:12px;color:%s">Open <a href="%s" style="color:%s;text-decoration:none">quire</a> to work the day.</p>
</div></div></body></html>`,
		paperPage, inkBody, paperCard, rule, inkMuted, inkHeading, html.EscapeString(dateLine), body.String(),
		inkMuted, html.EscapeString(strings.TrimRight(baseURL, "/")), inkAccent)

	return Message{
		Subject: "quire — " + dateLine,
		Text:    text.String(),
		HTML:    page,
	}
}

// prettyDate: "today", "yesterday", "3 days ago", "Fri 12 Sep".
func prettyDate(day, today string) string {
	t, err1 := time.Parse("2006-01-02", day)
	n, err2 := time.Parse("2006-01-02", today)
	if err1 != nil || err2 != nil {
		return day
	}
	diff := int(t.Sub(n).Hours() / 24)
	switch {
	case diff == 0:
		return "today"
	case diff == -1:
		return "yesterday"
	case diff < 0:
		return fmt.Sprintf("%d days ago", -diff)
	case diff == 1:
		return "tomorrow"
	}
	return t.Format("Mon 2 Jan")
}

// prettyLong: "Tuesday, 2 September 2026".
func prettyLong(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format("Monday, 2 January 2006")
}
