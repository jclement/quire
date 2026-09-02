package vault

import (
	"errors"
	"io/fs"
	"testing"
	"time"
)

func newTestVault(t *testing.T) *Vault {
	t.Helper()
	v, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestWriteReadRoundTrip(t *testing.T) {
	v := newTestVault(t)
	// Trailing whitespace, no trailing newline, CRLF-free odd spacing — all
	// must survive byte-for-byte.
	content := []byte("---\ntype: note\n---\n# Hi  \n\ttabbed\nno trailing newline")
	f, err := v.Write("notes/a.md", content, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Read("notes/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Raw) != string(content) {
		t.Errorf("round trip changed bytes:\ngot  %q\nwant %q", got.Raw, content)
	}
	if got.SHA256 != f.SHA256 {
		t.Errorf("hash mismatch after read")
	}
}

func TestWriteConflicts(t *testing.T) {
	v := newTestVault(t)
	f, err := v.Write("notes/a.md", []byte("one\n"), "")
	if err != nil {
		t.Fatal(err)
	}

	// Create over existing = conflict.
	if _, err := v.Write("notes/a.md", []byte("x\n"), ""); !errors.Is(err, ErrConflict) {
		t.Errorf("create-over-existing: got %v, want ErrConflict", err)
	}
	// Stale base = conflict.
	if _, err := v.Write("notes/a.md", []byte("x\n"), "deadbeef"); !errors.Is(err, ErrConflict) {
		t.Errorf("stale base: got %v, want ErrConflict", err)
	}
	// Correct base succeeds.
	if _, err := v.Write("notes/a.md", []byte("two\n"), f.SHA256); err != nil {
		t.Errorf("valid CAS write failed: %v", err)
	}
}

func TestValidatePath(t *testing.T) {
	bad := []string{"", "/etc/passwd", "../up.md", "notes/../../x.md", "notes/.hidden.md", ".quire/index.db", `notes\win.md`}
	for _, p := range bad {
		if err := ValidatePath(p); err == nil {
			t.Errorf("ValidatePath(%q) should fail", p)
		}
	}
	good := []string{"notes/a.md", "people/sarah-chen.md", "notes/sub/deep.md", "daily/2026-09-01.md"}
	for _, p := range good {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) = %v", p, err)
		}
	}
}

// TestInferTypeIsCaseInsensitive: an imported vault very often uses
// "People/" or "Projects/". Matching only the lowercase form typed all of it
// as plain notes, which silently disabled the entity model on someone's
// existing library. A directory that is simply named differently — "Meeting
// Notes/" — stays a note on purpose: guessing at intent would be worse than
// honouring a frontmatter `type:` the user wrote deliberately.
func TestInferTypeIsCaseInsensitive(t *testing.T) {
	for path, want := range map[string]DocType{
		"people/sarah.md":       TypePerson,
		"People/sarah.md":       TypePerson,
		"PEOPLE/sarah.md":       TypePerson,
		"Projects/apollo.md":    TypeProject,
		"Companies/acme.md":     TypeCompany,
		"Meetings/sync.md":      TypeMeeting,
		"Daily/2026-09-01.md":   TypeDaily,
		"Projects/Archive/x.md": TypeProject, // nesting keeps the top-level type
		"Meeting Notes/sync.md": TypeNote,    // a different name, not a case variant
		"notes/x.md":            TypeNote,
	} {
		if got := InferType(path); got != want {
			t.Errorf("InferType(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestInferTypeAndNewDocPath(t *testing.T) {
	if got := InferType("people/sarah.md"); got != TypePerson {
		t.Errorf("InferType people = %v", got)
	}
	if got := InferType("random/x.md"); got != TypeNote {
		t.Errorf("InferType random dir = %v", got)
	}
	if got := InferType("top.md"); got != TypeNote {
		t.Errorf("InferType top level = %v", got)
	}

	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)
	cases := map[string]string{
		string(TypePerson):  "people/sarah-chen.md",
		string(TypeMeeting): "meetings/2026-09-01-sarah-chen.md",
		string(TypeNote):    "notes/sarah-chen.md",
	}
	for typ, want := range cases {
		if got := NewDocPath(DocType(typ), "Sarah Chen", now); got != want {
			t.Errorf("NewDocPath(%s) = %q, want %q", typ, got, want)
		}
	}
	if got := Slugify("  Héllo,  World! 42 "); got != "héllo-world-42" {
		t.Errorf("Slugify = %q", got)
	}
}

func TestWalkMarkdownSkipsHidden(t *testing.T) {
	v := newTestVault(t)
	mustWrite := func(p, s string) {
		t.Helper()
		if _, err := v.Write(p, []byte(s), ""); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("notes/a.md", "a")
	mustWrite("people/b.md", "b")

	var seen []string
	if err := v.WalkMarkdown(func(rel string, _ fs.FileInfo) error {
		seen = append(seen, rel)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Errorf("walked %v, want 2 files", seen)
	}
}
