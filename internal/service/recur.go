// Recurrence: the deliberately tiny spec "🔁 every [N] day|week|month|year[s]
// [when done]". Completing a recurring task inserts the next occurrence on
// the line below, preserving the due→defer gap — that gap IS the lead time
// ("surface the registration renewal three weeks before it's due"), so it
// carries forward automatically. "when done" repeats from the completion day
// instead of the due date (furnace filters, not anniversaries).
package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jclement/quire/internal/index"
)

var recurSpecRe = regexp.MustCompile(`^every(?:\s+(\d+))?\s+(day|week|month|year)s?(\s+when\s+done)?$`)

// parseRecur validates a recurrence spec into (count, unit, whenDone).
func parseRecur(spec string) (int, string, bool, error) {
	m := recurSpecRe.FindStringSubmatch(strings.TrimSpace(spec))
	if m == nil {
		return 0, "", false, fmt.Errorf("invalid recurrence %q", spec)
	}
	n := 1
	if m[1] != "" {
		fmt.Sscanf(m[1], "%d", &n)
	}
	return n, m[2], m[3] != "", nil
}

func addInterval(day string, n int, unit string) (string, error) {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return "", fmt.Errorf("invalid date %q: %w", day, err)
	}
	switch unit {
	case "day":
		t = t.AddDate(0, 0, n)
	case "week":
		t = t.AddDate(0, 0, 7*n)
	case "month":
		t = t.AddDate(0, n, 0)
	case "year":
		t = t.AddDate(n, 0, 0)
	}
	return t.Format("2006-01-02"), nil
}

// nextOccurrenceLine builds the next instance of a recurring task from its
// original (pre-completion) line, swapping the dates in place so every other
// byte of the user's text survives verbatim.
func nextOccurrenceLine(line string, row index.TaskRow, today string) (string, error) {
	n, unit, whenDone, err := parseRecur(row.Recur)
	if err != nil {
		return "", err
	}

	base := row.Due
	if whenDone || base == "" {
		base = today
	}
	newDue, err := addInterval(base, n, unit)
	if err != nil {
		return "", err
	}

	out := line
	if row.Due != "" {
		out = replaceMarkerDate(out, "📅", row.Due, newDue)
	} else {
		out += " 📅 " + newDue
	}

	if row.Defer != "" {
		var newDefer string
		if row.Due != "" {
			// Preserve the lead time: the defer stays the same distance
			// before the due date.
			dueT, err1 := time.Parse("2006-01-02", row.Due)
			deferT, err2 := time.Parse("2006-01-02", row.Defer)
			newDueT, err3 := time.Parse("2006-01-02", newDue)
			if err1 != nil || err2 != nil || err3 != nil {
				return "", fmt.Errorf("unparseable dates on recurring task")
			}
			newDefer = newDueT.Add(-dueT.Sub(deferT)).Format("2006-01-02")
		} else {
			newDefer, err = addInterval(row.Defer, n, unit)
			if err != nil {
				return "", err
			}
		}
		out = replaceMarkerDate(out, "🛫", row.Defer, newDefer)
	}
	return out, nil
}

// replaceMarkerDate swaps the date following an emoji marker, tolerating
// whatever whitespace the user put between them.
func replaceMarkerDate(line, marker, oldDate, newDate string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(marker) + `\s*` + regexp.QuoteMeta(oldDate))
	return re.ReplaceAllString(line, marker+" "+newDate)
}
