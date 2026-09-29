package vault

import (
	"bytes"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantYAML string
		wantBody string
		wantOK   bool
	}{
		{"no frontmatter", "# Hello\n", "", "# Hello\n", false},
		{"basic", "---\ntype: person\n---\n# Sarah\n", "type: person", "# Sarah\n", true},
		{"empty block", "---\n---\nbody\n", "", "body\n", true},
		{"unclosed", "---\ntype: person\n# Sarah\n", "", "---\ntype: person\n# Sarah\n", false},
		{"hr not frontmatter", "text\n---\nmore\n", "", "text\n---\nmore\n", false},
		{"closing at EOF no newline", "---\na: 1\n---", "a: 1", "", true},
		{"multikey preserves body exactly", "---\na: 1\nb: two\n---\n\nbody  \n", "a: 1\nb: two", "\nbody  \n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			y, body, ok := SplitFrontmatter([]byte(tc.raw))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if string(y) != tc.wantYAML {
				t.Errorf("yaml = %q, want %q", y, tc.wantYAML)
			}
			if string(body) != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

func TestParseFrontmatterBrokenYAMLIsNil(t *testing.T) {
	raw := []byte("---\n: : : not yaml [\n---\nbody\n")
	if m := ParseFrontmatter(raw); m != nil {
		t.Fatalf("broken YAML should parse to nil, got %v", m)
	}
}

// The round-trip guarantee: splitting and reassembling changes nothing, and
// SetFrontmatterKey touches only the key's line.
func TestSetFrontmatterKeySurgical(t *testing.T) {
	raw := []byte("---\n# a comment\ntype: person\nemail: 'sarah@x.com'\n---\nBody stays.\n")
	got := SetFrontmatterKey(raw, "type", "company")
	want := "---\n# a comment\ntype: company\nemail: 'sarah@x.com'\n---\nBody stays.\n"
	if string(got) != want {
		t.Errorf("replace:\ngot  %q\nwant %q", got, want)
	}

	got = SetFrontmatterKey(raw, "due", "2026-10-01")
	want = "---\n# a comment\ntype: person\nemail: 'sarah@x.com'\ndue: 2026-10-01\n---\nBody stays.\n"
	if string(got) != want {
		t.Errorf("append:\ngot  %q\nwant %q", got, want)
	}

	got = SetFrontmatterKey([]byte("plain body\n"), "type", "note")
	want = "---\ntype: note\n---\nplain body\n"
	if string(got) != want {
		t.Errorf("create block:\ngot  %q\nwant %q", got, want)
	}
}

func TestBuildDoc(t *testing.T) {
	got := BuildDoc([][2]string{{"type", "person"}, {"company", `"[[Acme]]"`}}, "# Sarah\n")
	want := "---\ntype: person\ncompany: \"[[Acme]]\"\n---\n# Sarah\n"
	if !bytes.Equal(got, []byte(want)) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A key written as a YAML block list owns the indented lines under it.
// Replacing only the key line left "  - Fran" orphaned under whatever key
// came next — broken YAML, and the document silently lost its frontmatter.
func TestFrontmatterKeyReplacesBlockList(t *testing.T) {
	raw := []byte("---\ntype: person\naliases:\n  - Fran\n  - F.B.\nrole: CFO\n---\n# Frances\n")

	set := SetFrontmatterKey(raw, "aliases", "[Fran, F.B., Frances]")
	want := "---\ntype: person\naliases: [Fran, F.B., Frances]\nrole: CFO\n---\n# Frances\n"
	if string(set) != want {
		t.Errorf("set:\n%s\nwant:\n%s", set, want)
	}

	removed := RemoveFrontmatterKey(raw, "aliases")
	want = "---\ntype: person\nrole: CFO\n---\n# Frances\n"
	if string(removed) != want {
		t.Errorf("remove:\n%s\nwant:\n%s", removed, want)
	}
}

// A key's span runs through blank lines while what follows is still its
// indented content — a block list with a gap, or a `|` block scalar with a
// paragraph break — and stops at the next top-level key or comment, leaving
// the blank line that separated them in place.
func TestFrontmatterKeySpanCrossesBlankLines(t *testing.T) {
	cases := []struct {
		name, raw, key, set, wantSet, wantRemove string
	}{
		{
			name:       "block list with a gap",
			raw:        "---\naliases:\n  - a\n\n  - b\nrole: CFO\n---\nbody\n",
			key:        "aliases",
			set:        "[a, b, c]",
			wantSet:    "---\naliases: [a, b, c]\nrole: CFO\n---\nbody\n",
			wantRemove: "---\nrole: CFO\n---\nbody\n",
		},
		{
			name:       "block scalar with a paragraph break",
			raw:        "---\ndescription: |\n  first\n\n  second\ntype: note\n---\nbody\n",
			key:        "description",
			set:        "short",
			wantSet:    "---\ndescription: short\ntype: note\n---\nbody\n",
			wantRemove: "---\ntype: note\n---\nbody\n",
		},
		{
			name:       "blank line before the next key stays",
			raw:        "---\naliases:\n  - a\n\n# people\nrole: CFO\n---\nbody\n",
			key:        "aliases",
			set:        "[a]",
			wantSet:    "---\naliases: [a]\n\n# people\nrole: CFO\n---\nbody\n",
			wantRemove: "---\n\n# people\nrole: CFO\n---\nbody\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(SetFrontmatterKey([]byte(tc.raw), tc.key, tc.set)); got != tc.wantSet {
				t.Errorf("set:\n%q\nwant\n%q", got, tc.wantSet)
			}
			if got := string(RemoveFrontmatterKey([]byte(tc.raw), tc.key)); got != tc.wantRemove {
				t.Errorf("remove:\n%q\nwant\n%q", got, tc.wantRemove)
			}
			if ParseFrontmatter([]byte(SetFrontmatterKey([]byte(tc.raw), tc.key, tc.set))) == nil {
				t.Error("the rewritten frontmatter no longer parses")
			}
		})
	}
}
