// The CLI verbs, driven against a stub server. These had no coverage at
// all, which matters more than it looks: they are the surface you reach for
// from a terminal or a script, and their failures are silent — a mistyped
// flag that swallows the next word, or a date the server never sees.
package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// stubServer stands in for a running quire, recording what the CLI sent.
type stubServer struct {
	*httptest.Server
	lastMethod string
	lastPath   string
	lastQuery  string
	lastBody   map[string]any
	lastAuth   string
	respond    func(w http.ResponseWriter, r *http.Request)
}

func newStub(t *testing.T) *stubServer {
	t.Helper()
	s := &stubServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.lastMethod, s.lastPath, s.lastQuery = r.Method, r.URL.Path, r.URL.RawQuery
		s.lastAuth = r.Header.Get("Authorization")
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &s.lastBody)
		}
		if s.respond != nil {
			s.respond(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(s.Close)
	t.Setenv("QUIRE_URL", s.URL)
	t.Setenv("QUIRE_TOKEN", "")
	return s
}

// captureStdout runs fn with stdout redirected, returning what it printed.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = original
	out, _ := io.ReadAll(r)
	return string(out), runErr
}

func TestTaskAddSendsResolvedDates(t *testing.T) {
	stub := newStub(t)
	stub.respond = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"text":"Send Sarah the diagram","due":"2026-09-04","doc_path":"daily/2026-09-01.md"}}`))
	}

	out, err := captureStdout(t, func() error {
		return Run([]string{"add", "Send", "Sarah", "the", "diagram", "--due", "fri"})
	})
	if err != nil {
		t.Fatal(err)
	}

	if stub.lastMethod != "POST" || stub.lastPath != "/api/v1/tasks" {
		t.Errorf("sent %s %s", stub.lastMethod, stub.lastPath)
	}
	// Multi-word text must survive flag parsing intact.
	if stub.lastBody["text"] != "Send Sarah the diagram" {
		t.Errorf("text = %v", stub.lastBody["text"])
	}
	// "fri" is resolved before it leaves the client, so the server never
	// sees a word it would have to guess about.
	due, _ := stub.lastBody["due"].(string)
	if _, parseErr := time.Parse("2006-01-02", due); parseErr != nil {
		t.Errorf("due = %q, want an ISO date", due)
	}
	if !strings.Contains(out, "added: Send Sarah the diagram") || !strings.Contains(out, "2026-09-04") {
		t.Errorf("output = %q", out)
	}
}

func TestTaskAddFlagHandling(t *testing.T) {
	newStub(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no text at all", []string{"add"}},
		{"only flags", []string{"add", "--due", "fri"}},
		{"--due with no value", []string{"add", "buy milk", "--due"}},
		{"--defer with no value", []string{"add", "buy milk", "--defer"}},
		{"unparseable due", []string{"add", "buy milk", "--due", "someday"}},
		{"unparseable defer", []string{"add", "buy milk", "--defer", "whenever"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := captureStdout(t, func() error { return Run(tc.args) }); err == nil {
				t.Errorf("Run(%v) should have failed", tc.args)
			}
		})
	}
}

func TestSearchAndToday(t *testing.T) {
	stub := newStub(t)
	stub.respond = func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/search") {
			_, _ = w.Write([]byte(`{"data":[{"path":"meetings/acme.md","type":"meeting","title":"Acme Sync","snippet":"reporting"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"date":"2026-09-01","meetings":[],"overdue":[],"due":[],"available":[],"waiting":[],"recent":[],"birthdays":[]}}`))
	}

	out, err := captureStdout(t, func() error { return Run([]string{"search", "type:meeting", "acme"}) })
	if err != nil {
		t.Fatal(err)
	}
	// The whole query reaches the server as one string, not just the first word.
	if !strings.Contains(stub.lastQuery, "type%3Ameeting+acme") && !strings.Contains(stub.lastQuery, "type:meeting+acme") {
		t.Errorf("search query = %q", stub.lastQuery)
	}
	// Output is type + path + snippet: the path is what you pipe into
	// another command, so it is the field that must be there.
	if !strings.Contains(out, "meetings/acme.md") || !strings.Contains(out, "meeting") {
		t.Errorf("search output = %q", out)
	}

	if _, err := captureStdout(t, func() error { return Run([]string{"today"}) }); err != nil {
		t.Fatalf("today: %v", err)
	}
	if stub.lastPath != "/api/v1/today" {
		t.Errorf("today hit %s", stub.lastPath)
	}
}

func TestUnknownCommandAndUsage(t *testing.T) {
	newStub(t)
	for _, args := range [][]string{{}, {"frobnicate"}} {
		if _, err := captureStdout(t, func() error { return Run(args) }); err == nil {
			t.Errorf("Run(%v) should have failed", args)
		}
	}
}

// TestTokenIsSent: the CLI is how a scripted agent talks to a remote quire,
// so QUIRE_TOKEN has to actually reach the wire.
func TestTokenIsSent(t *testing.T) {
	stub := newStub(t)
	t.Setenv("QUIRE_TOKEN", "sk_deadbeef")

	_, _ = captureStdout(t, func() error { return Run([]string{"today"}) })

	if stub.lastAuth != "Bearer sk_deadbeef" {
		t.Errorf("Authorization = %q", stub.lastAuth)
	}
}

// TestServerErrorsSurface: a failure has to reach the exit code, not be
// printed as if it worked.
func TestServerErrorsSurface(t *testing.T) {
	stub := newStub(t)
	stub.respond = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"valid bearer token required"}}`))
	}

	_, err := captureStdout(t, func() error { return Run([]string{"today"}) })
	if err == nil {
		t.Fatal("a 401 should be an error")
	}
	if !strings.Contains(err.Error(), "valid bearer token required") {
		t.Errorf("error should carry the server's message, got %v", err)
	}
}
