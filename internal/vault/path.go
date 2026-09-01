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
)

// typeDirs maps top-level vault directories to types; anything else is a note.
var typeDirs = map[string]DocType{
	"people":    TypePerson,
	"companies": TypeCompany,
	"projects":  TypeProject,
	"meetings":  TypeMeeting,
	"daily":     TypeDaily,
}

// dirForType is the inverse: where new documents of each type are created.
var dirForType = map[DocType]string{
	TypeNote:    "notes",
	TypePerson:  "people",
	TypeCompany: "companies",
	TypeProject: "projects",
	TypeMeeting: "meetings",
	TypeDaily:   "daily",
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
func InferType(rel string) DocType {
	top, _, found := strings.Cut(rel, "/")
	if !found {
		return TypeNote
	}
	if t, ok := typeDirs[top]; ok {
		return t
	}
	return TypeNote
}

// DailyPath returns the canonical path for a day's daily note.
func DailyPath(day time.Time) string {
	return "daily/" + day.Format("2006-01-02") + ".md"
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
