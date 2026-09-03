// Path rules: validation (vault paths come straight off URLs, so traversal
// must die here), document-type inference from the directory, and slug/path
// generation for new documents.
package vault

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// DocType is a document's first-class type. Inferred from its top-level
// directory; frontmatter `type:` may override at parse time.
type DocType string

const (
	TypeNote    DocType = "note"
	TypePerson  DocType = "person"
	TypeCompany DocType = "company"
	TypeProject DocType = "project"
	TypeMeeting DocType = "meeting"
	TypeDaily   DocType = "daily"
	// TypeWeekly is a document under weekly/, named by ISO week
	// (weekly/2026-W36.md): the planning and retro tier above the daily
	// capture note.
	TypeWeekly DocType = "weekly"
	// TypeTemplate is a document under templates/: the shape new documents
	// start from. Kept out of everyday listings and search.
	TypeTemplate DocType = "template"
)

// typeDirs maps top-level vault directories to types; anything else is a note.
var typeDirs = map[string]DocType{
	"people":    TypePerson,
	"companies": TypeCompany,
	"projects":  TypeProject,
	"meetings":  TypeMeeting,
	"daily":     TypeDaily,
	"weekly":    TypeWeekly,
	"templates": TypeTemplate,
}

// dirForType is the inverse: where new documents of each type are created.
var dirForType = map[DocType]string{
	TypeNote:     "notes",
	TypePerson:   "people",
	TypeCompany:  "companies",
	TypeProject:  "projects",
	TypeMeeting:  "meetings",
	TypeDaily:    "daily",
	TypeWeekly:   "weekly",
	TypeTemplate: "templates",
}

// ValidatePath rejects anything that could escape the vault or touch app
// state: empty, absolute, dot-segments, backslashes, or hidden components.
func ValidatePath(rel string) error {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") {
		return fmt.Errorf("invalid vault path %q", rel)
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".") {
			return fmt.Errorf("invalid vault path %q", rel)
		}
	}
	return nil
}

// InferType returns the document type implied by rel's top-level directory.
// Matching is case-insensitive: an imported vault very often uses "People/"
// or "Projects/", and typing all of it as generic notes would silently
// disable the entity model on someone's existing library.
func InferType(rel string) DocType {
	top, _, found := strings.Cut(rel, "/")
	if !found {
		return TypeNote
	}
	if t, ok := typeDirs[strings.ToLower(top)]; ok {
		return t
	}
	return TypeNote
}

// DailyPath returns the canonical path for a day's daily note.
func DailyPath(day time.Time) string {
	return "daily/" + day.Format("2006-01-02") + ".md"
}

// WeeklyPath is the vault path for a day's ISO week: weekly/2026-W36.md.
func WeeklyPath(day time.Time) string {
	return "weekly/" + WeekOf(day) + ".md"
}

// WeekOf is the ISO week label ("2026-W36") a day falls in. ISO weeks start
// on Monday and belong to the year holding their Thursday, which is why the
// label's year is not always the day's.
func WeekOf(day time.Time) string {
	year, week := day.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

// NewDocPath picks a path for a new document of the given type and title:
// slugified title in the type's directory, meetings prefixed with the date.
// The caller is responsible for uniqueness (Write with baseSHA "" conflicts
// if the path exists).
func NewDocPath(t DocType, title string, now time.Time) string {
	slug := Slugify(title)
	if slug == "" {
		slug = "untitled"
	}
	if t == TypeMeeting {
		slug = now.Format("2006-01-02") + "-" + slug
	}
	if t == TypeDaily {
		return DailyPath(now)
	}
	return dirForType[t] + "/" + slug + ".md"
}

// Slugify lowercases, converts runs of non-alphanumerics to single hyphens,
// and trims — "Sarah Chen" → "sarah-chen".
func Slugify(s string) string {
	var b strings.Builder
	lastHyphen := true // suppress leading hyphen
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastHyphen = false
		case !lastHyphen:
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}
