// The weekly tier: ISO week arithmetic, and a review composed from the
// index rather than remembered.
package service

import (
	"strings"
	"testing"
	"time"
)

func TestParseWeek(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	// 2026-09-03 is a Thursday in ISO week 36.
	this, err := ParseWeek("", now)
	if err != nil {
		t.Fatal(err)
	}
	if this.Label != "2026-W36" || this.Start != "2026-08-31" || this.End != "2026-09-06" {
		t.Errorf("this week = %+v", this)
	}
	if this.Prev != "2026-W35" || this.Next != "2026-W37" {
		t.Errorf("neighbours = %s / %s", this.Prev, this.Next)
	}
	named, err := ParseWeek("2026-W01", now)
	if err != nil {
		t.Fatal(err)
	}
	// ISO week 1 of 2026 starts on Monday 29 December 2025.
	if named.Start != "2025-12-29" || named.Label != "2026-W01" {
		t.Errorf("2026-W01 = %+v", named)
	}
	for _, bad := range []string{"2026-W99", "nonsense", "2026-13", "2026-W00"} {
		if _, err := ParseWeek(bad, now); err == nil {
			t.Errorf("ParseWeek(%q) should fail", bad)
		}
	}
}

func TestWeekReviewComposesTheWeek(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"notes/done.md": "# Done\n\n- [x] landed this week ✅ 2026-09-01\n- [x] landed long ago ✅ 2026-01-05\n" +
			"- [ ] slipped 📅 2026-08-20\n- [ ] waiting on someone ⏳\n",
		"projects/quiet.md":              "---\nstatus: active\n---\n# Quiet Project\n",
		"projects/busy.md":               "---\nstatus: active\n---\n# Busy Project\n\n- [ ] something to do\n",
		"projects/parked.md":             "---\nstatus: someday\n---\n# Parked Project\n",
		"meetings/2026-09-01-standup.md": "---\ndate: 2026-09-01T09:00\n---\n# Standup\n",
	})

	week, err := svc.WeekReview("2026-W36", "")
	if err != nil {
		t.Fatal(err)
	}
	if week.Week != "2026-W36" || week.Start != "2026-08-31" {
		t.Errorf("week = %+v", week)
	}
	if s := taskTextsOf(week.Completed); !strings.Contains(s, "landed this week") || strings.Contains(s, "landed long ago") {
		t.Errorf("completed = %q, want only this week's", s)
	}
	if s := taskTextsOf(week.Slipped); !strings.Contains(s, "slipped") {
		t.Errorf("slipped = %q", s)
	}
	if s := taskTextsOf(week.Waiting); !strings.Contains(s, "waiting on someone") {
		t.Errorf("waiting = %q", s)
	}
	titles := func(docs []DocMeta) string {
		var out []string
		for _, d := range docs {
			out = append(out, d.Title)
		}
		return strings.Join(out, ",")
	}
	// The one active project nothing open points at — not the busy one, and
	// not the one deliberately parked as someday.
	stalled := titles(week.Stalled)
	if !strings.Contains(stalled, "Quiet Project") {
		t.Errorf("stalled = %q, want the project with no next action", stalled)
	}
	if strings.Contains(stalled, "Busy Project") || strings.Contains(stalled, "Parked Project") {
		t.Errorf("stalled = %q", stalled)
	}
	if s := titles(week.Meetings); !strings.Contains(s, "Standup") {
		t.Errorf("meetings = %q", s)
	}
	if week.Note != nil {
		t.Error("the week's note should be nil until it is written")
	}

	// Writing it makes it appear, from the template.
	if _, err := svc.InstallStarterTemplates(); err != nil {
		t.Fatal(err)
	}
	doc, err := svc.EnsureWeekly("2026-W36")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != "weekly/2026-W36.md" || doc.Type != "weekly" {
		t.Errorf("weekly note = %s (%s)", doc.Path, doc.Type)
	}
	if !strings.Contains(doc.Markdown, "This week is for") {
		t.Errorf("the weekly template should shape it:\n%s", doc.Markdown)
	}
	again, err := svc.WeekReview("2026-W36", "")
	if err != nil {
		t.Fatal(err)
	}
	if again.Note == nil || again.Note.Path != doc.Path {
		t.Error("the review should carry the week's note once written")
	}
}
