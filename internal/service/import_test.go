// Importing an existing vault is an MVP requirement ("point at or import an
// existing directory containing Markdown and attachments … preserving users'
// files is") and it has one honest test: drop a realistic Obsidian-shaped
// library on disk, index it, and check quire understands it.
//
// Two things this caught the first time it ran:
//   - directory type inference was case-sensitive, so a vault using
//     "People/" and "Projects/" had every document typed as a plain note and
//     the entity model silently did nothing;
//   - "[[Page#Heading]]" resolved to no document at all, so heading links
//     were reported dangling and grew no backlink.
package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeVault lays out files under the service's vault root.
func writeVault(t *testing.T, svc *Service, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		full := filepath.Join(svc.Vault.Dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Index.FullScan(); err != nil {
		t.Fatal(err)
	}
}

func TestImportExistingVault(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		// Title-cased directories, spaces in names, deep nesting — all
		// ordinary in a vault that predates quire.
		"People/Sarah Chen.md":             "---\ncompany: \"[[Acme Corp]]\"\nbirthday: 1985-03-12\n---\n# Sarah Chen\n",
		"Projects/Project Apollo.md":       "---\nstatus: active\n---\n# Project Apollo\n\n## Timeline\nQ3.\n",
		"Projects/Archive/Old Thing.md":    "# Old Thing\n",
		"Meetings/2026-08-15 Acme Sync.md": "# Acme Sync\n\nWith [[Sarah Chen|Sarah]] on [[Project Apollo#Timeline]].\n",
		"Daily/2026-08-15.md":              "---\ntags: [journal]\n---\n# 2026-08-15\n\n- [ ] Follow up 📅 2026-08-20\n",
		"assets/arch.png":                  "PNGDATA",
	})

	// Paths are compared case-folded: a case-insensitive filesystem (macOS)
	// folds "People/" into the "people/" directory quire pre-creates, while
	// a case-sensitive one (Linux, and CI) keeps them apart. The typing
	// behaviour under test is the same either way; the path spelling is not.
	types := map[string]string{}
	paths := map[string]string{}
	docs, err := svc.ListDocuments("", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		key := strings.ToLower(d.Path)
		types[key] = string(d.Type)
		paths[key] = d.Path
	}

	// Case-insensitive directory inference: the entity model has to engage
	// on a library that was not created by quire.
	for path, want := range map[string]string{
		"people/sarah chen.md":             "person",
		"projects/project apollo.md":       "project",
		"projects/archive/old thing.md":    "project",
		"meetings/2026-08-15 acme sync.md": "meeting",
		"daily/2026-08-15.md":              "daily",
	} {
		if got := types[path]; got != want {
			t.Errorf("%s typed as %q, want %q (indexed paths: %v)", path, got, want, paths)
		}
	}

	// Frontmatter survives untouched.
	person, err := svc.GetDocument(paths["people/sarah chen.md"])
	if err != nil {
		t.Fatal(err)
	}
	if person.Frontmatter["birthday"] != "1985-03-12" {
		t.Errorf("frontmatter lost: %+v", person.Frontmatter)
	}

	// A heading link is a link to the page. Both the alias form and the
	// anchor form must reach their targets.
	apollo, err := svc.GetDocument(paths["projects/project apollo.md"])
	if err != nil {
		t.Fatal(err)
	}
	if len(apollo.Backlinks) == 0 {
		t.Error("[[Project Apollo#Timeline]] produced no backlink")
	}
	if len(person.Backlinks) == 0 {
		t.Error("[[Sarah Chen|Sarah]] produced no backlink")
	}

	// Tasks in an imported daily note are real tasks.
	tasks, err := svc.Tasks("today")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, task := range tasks {
		if strings.Contains(task.Text, "Follow up") {
			found = true
		}
	}
	if !found {
		t.Error("an imported task was not indexed")
	}

	// Attachments outside quire's own directory are still readable.
	if _, err := svc.ReadAttachment("assets/arch.png"); err != nil {
		t.Errorf("attachment in an imported directory: %v", err)
	}
}
