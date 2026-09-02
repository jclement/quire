// Resolving the dates a human (or an agent) types into the ISO dates that
// go into markdown.
//
// This lives in the service layer because every transport needs it and they
// must agree. It used to live only in the CLI, which meant `quire task add
// --due fri` resolved correctly while POST /api/v1/tasks {"due":"today"}
// wrote the literal word "today" into the file — producing a task with an
// unparseable due date that no view could ever surface, and no error to say
// so. Silently corrupting the user's markdown is the worst possible failure
// for a file-is-truth app, so unparseable input is now an error.
package service

import (
	"fmt"
	"strings"
	"time"
)

// weekdayNames is matched by prefix, so "mon", "monda" and "monday" all
// work while "monday-ish" does not — a three-character prefix test alone
// would happily accept the latter and silently pick a date.
var weekdayNames = []struct {
	name string
	day  time.Weekday
}{
	{"sunday", time.Sunday}, {"monday", time.Monday}, {"tuesday", time.Tuesday},
	{"wednesday", time.Wednesday}, {"thursday", time.Thursday},
	{"friday", time.Friday}, {"saturday", time.Saturday},
}

// weekdayFor resolves a name or unambiguous prefix (at least 3 characters).
func weekdayFor(s string) (time.Weekday, bool) {
	if len(s) < 3 {
		return 0, false
	}
	for _, wd := range weekdayNames {
		if strings.HasPrefix(wd.name, s) {
			return wd.day, true
		}
	}
	return 0, false
}

// ParseWhen resolves a date expression to YYYY-MM-DD, relative to now.
// Accepts an ISO date, "today", "tomorrow"/"tom", a weekday name (the next
// occurrence, never today), and "+Nd". An empty string resolves to empty:
// no date is a valid answer, an unparseable one is not.
func ParseWhen(raw string, now time.Time) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return s, nil
	}
	switch s {
	case "today":
		return now.Format("2006-01-02"), nil
	case "tomorrow", "tom":
		return now.AddDate(0, 0, 1).Format("2006-01-02"), nil
	case "yesterday":
		return now.AddDate(0, 0, -1).Format("2006-01-02"), nil
	}
	if wd, ok := weekdayFor(s); ok {
		days := (int(wd) - int(now.Weekday()) + 7) % 7
		if days == 0 {
			days = 7 // "fri" on a Friday means the next one, not today
		}
		return now.AddDate(0, 0, days).Format("2006-01-02"), nil
	}
	if strings.HasPrefix(s, "+") && strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "+%dd", &n); err == nil {
			return now.AddDate(0, 0, n).Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("can't parse date %q (try YYYY-MM-DD, today, tomorrow, fri, +3d)", raw)
}
