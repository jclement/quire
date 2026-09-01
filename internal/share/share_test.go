package share

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

func newTestManager(t *testing.T) (*Manager, *service.Service) {
	t.Helper()
	v, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := service.New(v, &index.Index{DB: db, Vault: v})
	svc.Now = func() time.Time { return time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local) }

	store, err := auth.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	return NewManager(store, svc, "https://quire.example.ts.net"), svc
}

func serveShares(m *Manager) *httptest.Server {
	mux := http.NewServeMux()
	m.Routes(mux)
	return httptest.NewServer(mux)
}

func TestShareLifecycle(t *testing.T) {
	m, svc := newTestManager(t)
	if _, err := svc.CreateDocument(vault.TypeNote, "Sitter Info", "# Sitter Info\n\nBedtime 8pm. Call [[Grandma]] if stuck.\n\n- [x] stocked the fridge\n"); err != nil {
		t.Fatal(err)
	}

	// Sharing a nonexistent doc fails.
	if _, err := m.Create("notes/nope.md", 0); err == nil {
		t.Fatal("shared a missing document")
	}

	info, err := m.Create("notes/sitter-info.md", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.URL, "https://quire.example.ts.net/s/") {
		t.Errorf("url = %q", info.URL)
	}

	ts := serveShares(m)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/s/" + info.Token)
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if res.StatusCode != 200 {
		t.Fatalf("share page = %d", res.StatusCode)
	}
	for _, want := range []string{"<title>Sitter Info</title>", "Bedtime 8pm", "<em>Grandma</em>", `type="checkbox"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	// Wikilink syntax must not leak.
	if strings.Contains(body, "[[") {
		t.Errorf("raw wikilink leaked into page")
	}

	// View count bumped.
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ViewCount != 1 {
		t.Errorf("list = %+v", list)
	}

	// Revoked shares 404 indistinguishably.
	if err := m.Revoke(info.Token); err != nil {
		t.Fatal(err)
	}
	res2, _ := http.Get(ts.URL + "/s/" + info.Token)
	res2.Body.Close()
	if res2.StatusCode != 404 {
		t.Errorf("revoked share = %d", res2.StatusCode)
	}
}

func TestShareAttachmentGating(t *testing.T) {
	m, svc := newTestManager(t)
	att, err := svc.SaveAttachment("dosage.png", strings.NewReader("png-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := svc.SaveAttachment("secret.png", strings.NewReader("secret-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDocument(vault.TypeNote, "Med Sheet", "# Med Sheet\n\n"+att.Markdown+"\n"); err != nil {
		t.Fatal(err)
	}
	info, err := m.Create("notes/med-sheet.md", 0)
	if err != nil {
		t.Fatal(err)
	}

	ts := serveShares(m)
	defer ts.Close()

	// Referenced attachment is served (via the <base>-resolved path).
	res, _ := http.Get(ts.URL + "/s/" + info.Token + "/" + att.Path)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" {
		t.Errorf("referenced attachment = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}

	// Unreferenced attachment is not — a share is a window onto one doc.
	res, _ = http.Get(ts.URL + "/s/" + info.Token + "/" + secret.Path)
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("unreferenced attachment = %d", res.StatusCode)
	}

	// Markdown never serves through a share's file route.
	res, _ = http.Get(ts.URL + "/s/" + info.Token + "/notes/med-sheet.md")
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("markdown through file route = %d", res.StatusCode)
	}
}

func TestExpiredShare(t *testing.T) {
	m, svc := newTestManager(t)
	if _, err := svc.CreateDocument(vault.TypeNote, "Ephemeral", ""); err != nil {
		t.Fatal(err)
	}
	sh, err := m.Auth.CreateShare("notes/ephemeral.md", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := m.Auth.ResolveShare(sh.Token); err == nil {
		t.Errorf("expired share resolved")
	}
}

// preprocess only flattens wikilinks now; callouts are handled in the AST so
// the marker must survive it untouched, and fenced code must be verbatim.
func TestPreprocess(t *testing.T) {
	in := "> [!warning] Hot stove\n> Careful.\n\n```\n[[NotALink]]\n> [!note]\n```\n\nSee [[Sarah Chen|Sarah]].\n"
	out := preprocess(in)
	for _, want := range []string{"[!warning] Hot stove", "*Sarah*", "[[NotALink]]"} {
		if !strings.Contains(out, want) {
			t.Errorf("preprocess missing %q in:\n%s", want, out)
		}
	}
}

// Callouts render as titled panels carrying their type, and the body text
// still goes through goldmark's escaping (these pages are public).
func TestCalloutRendering(t *testing.T) {
	page, err := renderPage("Doc", "tok", `# Doc

> [!warning] Hot stove
> Careful, it is <hot>.

> [!nonsense] Unknown type

> [!note]
> Titleless.

> Just an ordinary quote.
`)
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)

	if !strings.Contains(html, `<div class="callout" data-callout="warning">`) {
		t.Errorf("warning callout panel missing:\n%s", html)
	}
	if !strings.Contains(html, `<p class="callout-title">Hot stove</p>`) {
		t.Errorf("callout title missing")
	}
	// The marker line must not survive into the body.
	if strings.Contains(html, "[!warning]") {
		t.Errorf("callout marker leaked into output")
	}
	// Unknown types fall back to note, like Obsidian.
	if !strings.Contains(html, `data-callout="note"`) {
		t.Errorf("unknown callout type did not fall back to note")
	}
	// A title-only callout still gets a default title from its type.
	if !strings.Contains(html, ">Note</p>") {
		t.Errorf("default title missing")
	}
	// Ordinary blockquotes are untouched.
	if !strings.Contains(html, "<blockquote>") {
		t.Errorf("plain blockquote lost")
	}
	// The callout body renders, and raw HTML in it never reaches the page
	// (goldmark drops it with unsafe rendering off).
	if !strings.Contains(html, "Careful, it is") {
		t.Errorf("callout body missing:\n%s", html)
	}
	if strings.Contains(html, "<hot>") {
		t.Errorf("raw HTML leaked out of a callout body")
	}
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := res.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}
