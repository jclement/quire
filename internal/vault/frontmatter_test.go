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
