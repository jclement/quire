package index

import (
	"reflect"
	"testing"
)

func TestRankMatches(t *testing.T) {
	people := []MatchCandidate{
		{Path: "people/frances-bagley.md", Type: "person", Title: "Frances Bagley"},
		{Path: "people/david-piepgrass.md", Type: "person", Title: "David Piepgrass"},
		{Path: "people/david-ng.md", Type: "person", Title: "David Ng"},
		{Path: "people/james-burke.md", Type: "person", Title: "James Burke"},
		{Path: "companies/davidson-labs.md", Type: "company", Title: "Davidson Labs"},
		{Path: "companies/acme.md", Type: "company", Title: "Acme"},
		{Path: "companies/frances-foods.md", Type: "company", Title: "Frances Foods"},
	}
	paths := func(ms []LikelyMatch) []string {
		var out []string
		for _, m := range ms {
			out = append(out, m.Path+":"+string(m.Reason))
		}
		return out
	}
	cases := []struct {
		name string
		want []string
	}{
		// The headline case, case-insensitively; the person outranks a
		// company that happens to share the word.
		{"Frances", []string{"people/frances-bagley.md:first_name", "companies/frances-foods.md:first_name"}},
		{"james", []string{"people/james-burke.md:first_name"}},
		// Two Davids: both offered, alphabetical, and the Davidson prefix
		// hit is dropped because a stronger tier matched.
		{"David", []string{"people/david-ng.md:first_name", "people/david-piepgrass.md:first_name"}},
		{"Fran", []string{"people/frances-bagley.md:prefix", "companies/frances-foods.md:prefix"}},
		{"James B", []string{"people/james-burke.md:prefix"}},
		{"FB", []string{"people/frances-bagley.md:initials"}},
		{"J.B.", []string{"people/james-burke.md:initials"}},
		// Lowercase two letters is a word, not initials; two letters is too
		// short a prefix; nothing plausible means nothing suggested.
		{"jb", nil},
		{"Ja", nil},
		{"Zelda", nil},
		// A one-word title is never "abbreviated" by its own first word.
		{"Acm", nil},
		// The full title would resolve already and is never a suggestion.
		{"James Burke", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := paths(RankMatches(tc.name, people))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("RankMatches(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestRankMatchesCapsAmbiguity(t *testing.T) {
	var many []MatchCandidate
	for _, last := range []string{"A", "B", "C", "D", "E", "F", "G"} {
		many = append(many, MatchCandidate{Path: "people/" + last + ".md", Type: "person", Title: "Sam " + last})
	}
	if got := RankMatches("Sam", many); len(got) != maxLikelyMatches {
		t.Errorf("got %d matches, want the cap of %d", len(got), maxLikelyMatches)
	}
}
