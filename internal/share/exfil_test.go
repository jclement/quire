// Security tests for the share surface: a share is a window onto exactly one
// document, and these prove an anonymous holder of a share link cannot widen
// it into a view of the rest of the vault.
package share

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jclement/quire/internal/vault"
)

// TestShareAttachmentScope is the authorization test for /s/{token}/{path}.
// The rule is "only attachments this document references"; the cases below
// are the ways an anonymous caller might try to read something else.
func TestShareAttachmentScope(t *testing.T) {
	m, svc := newTestManager(t)
	root := svc.Vault.Dir

	// Two attachments: one the shared note embeds, one it must never serve.
	for rel, content := range map[string]string{
		"attachments/diagram.png":       "PNG-EMBEDDED",
		"attachments/private/pay.pdf":   "PDF-SECRET",
		"attachments/2026/passport.jpg": "JPG-SECRET",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The shared note embeds one image, and *mentions* two other paths as
	// ordinary prose — the way anyone writing notes actually would.
	markdown := "# Sitter Info\n\n" +
		"![diagram](attachments/diagram.png)\n\n" +
		"Scans live in attachments/private/pay.pdf and I uploaded\n" +
		"https://example.com/attachments/2026/passport.jpg last week.\n"
	if _, err := svc.CreateDocument(vault.TypeNote, "Sitter Info", markdown); err != nil {
		t.Fatal(err)
	}
	// A second document that the share must not reach at all.
	if _, err := svc.CreateDocument(vault.TypeNote, "Salaries", "# Salaries\n\nCEO: 400k\n"); err != nil {
		t.Fatal(err)
	}

	info, err := m.Create("notes/sitter-info.md", 0)
	if err != nil {
		t.Fatal(err)
	}
	ts := serveShares(m)
	defer ts.Close()

	get := func(path string) (int, string) {
		res, err := http.Get(ts.URL + "/s/" + info.Token + "/" + path)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, readBody(t, res)
	}

	// The embedded attachment is the whole point of the route.
	if code, body := get("attachments/diagram.png"); code != 200 || body != "PNG-EMBEDDED" {
		t.Errorf("embedded attachment = %d %q, want 200", code, body)
	}

	// Everything else must 404, whether or not its path happens to appear in
	// the document's text. Mentioning a filename is not publishing it.
	for _, rel := range []string{
		"attachments/private/pay.pdf",   // named in prose, never embedded
		"attachments/2026/passport.jpg", // appears only inside an unrelated URL
		"notes/salaries.md",             // another document
		"../auth.db",                    // traversal
		".quire/auth.db",                // app state
	} {
		if code, body := get(rel); code != 404 {
			t.Errorf("SHARE LEAK: %s = %d %q, want 404", rel, code, body)
		}
	}
}
