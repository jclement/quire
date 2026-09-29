// Likely matches for unwritten names: "[[Frances]]" dangles while
// people/frances-bagley.md exists, because nobody ever told the vault that
// Frances Bagley answers to her first name. Resolution stays exact (an alias
// is the fix, and it is the owner's call); this only *suggests* which
// document a dangling name probably means, so the Unwritten list can offer
// "add alias" instead of "create a duplicate person".
package index

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// MatchReason says why a candidate was suggested, strongest first.
type MatchReason string

const (
	// MatchFirstName: a one-word name equal to the title's first word
	// ("Frances" → "Frances Bagley"). The case this whole feature is for.
	MatchFirstName MatchReason = "first_name"
	// MatchPrefix: the title starts with the name ("Fran" → "Frances
	// Bagley", "James B" → "James Burke").
	MatchPrefix MatchReason = "prefix"
	// MatchInitials: the name is written as capitals and equals the title's
	// initials ("FB", "F.B." → "Frances Bagley").
	MatchInitials MatchReason = "initials"
)

// matchStrength orders reasons; only the strongest tier found is kept.
var matchStrength = map[MatchReason]int{MatchFirstName: 3, MatchPrefix: 2, MatchInitials: 1}

// minPrefixLen keeps "Jo" from suggesting every Joanne, Jonas and Jordan.
const minPrefixLen = 3

// maxLikelyMatches bounds a row of suggestions: past a handful of equally
// good candidates the name is too common for a suggestion to help.
const maxLikelyMatches = 5

// MatchCandidate is a document a dangling name might mean.
type MatchCandidate struct {
	Path  string
	Type  string
	Title string
}

// LikelyMatch is one suggestion for a dangling name.
type LikelyMatch struct {
	MatchCandidate
	Reason MatchReason
}

// MatchCandidates lists the documents a first name or initials plausibly
// point at: people and companies. Notes and meetings are not called by
// nicknames.
func (ix *Index) MatchCandidates() ([]MatchCandidate, error) {
	rows, err := ix.DB.Query(`SELECT path, type, title FROM documents
		WHERE type IN ('person', 'company') ORDER BY title COLLATE NOCASE, path`)
	if err != nil {
		return nil, fmt.Errorf("listing match candidates: %w", err)
	}
	defer rows.Close()
	var out []MatchCandidate
	for rows.Next() {
		var c MatchCandidate
		if err := rows.Scan(&c.Path, &c.Type, &c.Title); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RankMatches returns the candidates a dangling name most plausibly means:
// only the strongest tier that matched (a first-name hit makes a mere
// prefix hit noise), people before companies, then by title. Several
// results in one tier is an ambiguity for the owner to settle — two Davids
// are both shown, never one picked silently. Empty when nothing is
// plausible.
func RankMatches(name string, candidates []MatchCandidate) []LikelyMatch {
	var out []LikelyMatch
	best := 0
	for _, c := range candidates {
		reason, ok := matchReason(name, c.Title)
		if !ok {
			continue
		}
		strength := matchStrength[reason]
		if strength < best {
			continue
		}
		if strength > best {
			best, out = strength, nil
		}
		out = append(out, LikelyMatch{MatchCandidate: c, Reason: reason})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Type == "person") != (out[j].Type == "person") {
			return out[i].Type == "person"
		}
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	if len(out) > maxLikelyMatches {
		out = out[:maxLikelyMatches]
	}
	return out
}

// matchReason decides whether name plausibly refers to a document titled
// title, and why.
func matchReason(name, title string) (MatchReason, bool) {
	nameWords := strings.Fields(strings.ToLower(name))
	titleWords := strings.Fields(strings.ToLower(title))
	// A one-word title cannot be abbreviated by a first name: "Acme" and
	// "[[Acme]]" would already resolve, and "Acm" is a typo, not a name.
	if len(nameWords) == 0 || len(titleWords) < 2 {
		return "", false
	}
	joinedName := strings.Join(nameWords, " ")
	joinedTitle := strings.Join(titleWords, " ")
	if joinedName == joinedTitle {
		return "", false // the same name would resolve already
	}
	if len(nameWords) == 1 && nameWords[0] == titleWords[0] {
		return MatchFirstName, true
	}
	if len([]rune(joinedName)) >= minPrefixLen && strings.HasPrefix(joinedTitle, joinedName) {
		return MatchPrefix, true
	}
	if initials, ok := writtenInitials(name); ok && initials == titleInitials(titleWords) {
		return MatchInitials, true
	}
	return "", false
}

// writtenInitials reads "FB", "F.B." or "F. B." as initials. Only capitals
// count: "Jo" or "ed" in a link is a word, and treating it as initials would
// suggest every J— O— in the vault.
func writtenInitials(name string) (string, bool) {
	var letters []rune
	for _, r := range name {
		switch {
		case r == '.' || unicode.IsSpace(r):
			continue
		case unicode.IsUpper(r):
			letters = append(letters, unicode.ToLower(r))
		default:
			return "", false
		}
	}
	if len(letters) < 2 || len(letters) > 4 {
		return "", false
	}
	return string(letters), true
}

func titleInitials(words []string) string {
	var b strings.Builder
	for _, w := range words {
		for _, r := range w {
			b.WriteRune(r)
			break
		}
	}
	return b.String()
}
