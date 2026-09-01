// The morning digest: one email that runs the day — meetings, birthdays,
// overdue, due today, waiting. Email is the inbox people check involuntarily
// (the life-admin review's words), so this is the reminder channel.
package mail

import (
	"fmt"
	"html"
	"strings"

	"github.com/jclement/quire/internal/service"
)

// BuildDigest renders the Today payload as an email; empty=true means there
// is nothing worth sending (no meetings, no dated tasks, no birthdays — a
// quiet day sends no mail, an empty inbox beats a hollow one).
func BuildDigest(today service.TodayPayload, baseURL string) (msg Message, empty bool) {
	if len(today.Meetings) == 0 && len(today.Overdue) == 0 && len(today.DueToday) == 0 &&
		len(today.Waiting) == 0 && len(today.Birthdays) == 0 {
		return Message{}, true
	}

	var text, htmlBody strings.Builder
	section := func(title string, lines []string) {
		if len(lines) == 0 {
			return
		}
		fmt.Fprintf(&text, "%s\n", title)
		fmt.Fprintf(&htmlBody, `<h3 style="margin:1.2em 0 .3em;font-size:14px;color:#666;text-transform:uppercase;letter-spacing:.05em">%s</h3><ul style="margin:0;padding-left:1.2em">`, html.EscapeString(title))
		for _, line := range lines {
			fmt.Fprintf(&text, "  • %s\n", line)
			fmt.Fprintf(&htmlBody, `<li style="margin:.2em 0">%s</li>`, html.EscapeString(line))
		}
		text.WriteString("\n")
		htmlBody.WriteString("</ul>")
	}

	taskLines := func(tasks []service.Task) []string {
		var out []string
		for _, t := range tasks {
			line := t.Text
			if t.Due != nil {
				line += " (due " + *t.Due + ")"
			}
			out = append(out, line)
		}
		return out
	}

	var meetings []string
	for _, m := range today.Meetings {
		meetings = append(meetings, m.Title)
	}
	var birthdays []string
	for _, b := range today.Birthdays {
		when := fmt.Sprintf("in %d days", b.DaysUntil)
		if b.DaysUntil == 0 {
			when = "today!"
		}
		line := fmt.Sprintf("🎂 %s — %s", b.Title, when)
		if b.Age != nil {
			line += fmt.Sprintf(" (turns %d)", *b.Age)
		}
		birthdays = append(birthdays, line)
	}

	section("Meetings", meetings)
	section("Birthdays", birthdays)
	section("Overdue", taskLines(today.Overdue))
	section("Due today", taskLines(today.DueToday))
	section("Waiting for", taskLines(today.Waiting))

	fmt.Fprintf(&text, "— quire · %s\n", baseURL)
	page := fmt.Sprintf(`<div style="font:15px/1.6 -apple-system,sans-serif;max-width:36rem;margin:0 auto;color:#222">
<h2 style="font-size:18px">%s</h2>%s
<p style="margin-top:2em;font-size:12px;color:#999">quire · <a href="%s" style="color:#4662d7">%s</a></p></div>`,
		html.EscapeString(today.Date), htmlBody.String(), html.EscapeString(baseURL), html.EscapeString(baseURL))

	return Message{
		Subject: "quire — " + today.Date,
		Text:    text.String(),
		HTML:    page,
	}, false
}
