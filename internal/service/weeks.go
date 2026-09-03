// ISO week arithmetic for the weekly note. Weeks run Monday to Sunday and
// are labelled "2026-W36"; the label's year is the one holding the week's
// Thursday, so the first days of January can belong to the previous year's
// last week. Everything here is pure so the review payload is testable
// without a clock.
package service

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var weekRe = regexp.MustCompile(`^(\d{4})-[Ww](\d{1,2})$`)

// Week is one ISO week resolved to its bounds.
type Week struct {
	// Label is "2026-W36".
	Label string
	// Start is the Monday, End the Sunday, both YYYY-MM-DD.
	Start, End string
	// Prev and Next are the neighbouring labels.
	Prev, Next string
	// StartTime is the Monday at midnight in loc, for mtime comparisons.
	StartTime time.Time
}

// ParseWeek resolves a "2026-W36" label. "" and "this" mean the week now
// falls in, relative to today.
func ParseWeek(label string, now time.Time) (Week, error) {
	if label == "" || label == "this" || label == "current" {
		return weekOfDay(now), nil
	}
	m := weekRe.FindStringSubmatch(label)
	if m == nil {
		return Week{}, fmt.Errorf("%w: invalid week %q (want YYYY-Www, e.g. 2026-W36)", ErrValidation, label)
	}
	year, _ := strconv.Atoi(m[1])
	week, _ := strconv.Atoi(m[2])
	if week < 1 || week > 53 {
		return Week{}, fmt.Errorf("%w: week %d is out of range", ErrValidation, week)
	}
	monday := isoWeekStart(year, week, now.Location())
	// A year has 52 or 53 weeks; asking for a 53rd that does not exist
	// rolls into the next year, which is a typo rather than an intent.
	if got := vaultWeekOf(monday); got != fmt.Sprintf("%04d-W%02d", year, week) {
		return Week{}, fmt.Errorf("%w: %s has no week %d", ErrValidation, m[1], week)
	}
	return weekOfDay(monday), nil
}

func weekOfDay(day time.Time) Week {
	year, week := day.ISOWeek()
	monday := isoWeekStart(year, week, day.Location())
	sunday := monday.AddDate(0, 0, 6)
	return Week{
		Label:     vaultWeekOf(monday),
		Start:     monday.Format("2006-01-02"),
		End:       sunday.Format("2006-01-02"),
		Prev:      vaultWeekOf(monday.AddDate(0, 0, -7)),
		Next:      vaultWeekOf(monday.AddDate(0, 0, 7)),
		StartTime: monday,
	}
}

func vaultWeekOf(day time.Time) string {
	year, week := day.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

// isoWeekStart is the Monday of an ISO week. 4 January is always in week 1,
// so the first Monday on or before it starts the year.
func isoWeekStart(year, week int, loc *time.Location) time.Time {
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, loc)
	offset := int(time.Monday - jan4.Weekday())
	if offset > 0 {
		offset -= 7
	}
	return jan4.AddDate(0, 0, offset+(week-1)*7)
}
