package semantic

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/semantic/semantictest"
	"github.com/jclement/quire/internal/vault"
)

func fakeOpenAI(t *testing.T, failFirst int) (*httptest.Server, *atomic.Int32) {
	return semantictest.Server(t, failFirst)
}

func fakeVector(text string) []float32 { return semantictest.Vector(text, Dimensions) }

func TestChunksFollowHeadingsAndFoldTinySections(t *testing.T) {
	raw := []byte("---\ntitle: Ops\n---\n# Ops\n\nintro paragraph that is long enough to stand on its own as a section body here.\n\n## Deploy\n\n" +
		strings.Repeat("Deploy notes sentence. ", 120) + "\n\n## Tiny\n\nx\n")
	chunks := Chunks("Ops", raw)
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks: %+v", len(chunks), chunks)
	}
	if !strings.HasPrefix(chunks[0].Text, "Ops\n\n") {
		t.Errorf("first chunk should carry the title: %q", chunks[0].Text[:20])
	}
	deploy := 0
	for _, c := range chunks {
		if c.Heading == "Deploy" {
			deploy++
			if len(c.Text) > targetChunkChars+200 {
				t.Errorf("chunk too long: %d", len(c.Text))
			}
			if !strings.HasPrefix(c.Text, "Ops › Deploy\n\n") {
				t.Errorf("chunk prefix = %q", c.Text[:30])
			}
		}
		if c.Heading == "Tiny" {
			t.Error("tiny section should fold into its neighbour")
		}
	}
	if deploy < 2 {
		t.Errorf("long section should split, got %d pieces", deploy)
	}
	// Fenced code with a # line is not a heading.
	code := Chunks("C", []byte("```\n# not a heading\n```\nbody text that is long enough to be a section of its own right here.\n"))
	if len(code) != 1 || code[0].Heading != "" {
		t.Errorf("fence handling: %+v", code)
	}
	if Fingerprint("m", "a") == Fingerprint("m", "b") || Fingerprint("m", "a") == Fingerprint("n", "a") {
		t.Error("fingerprints must vary by text and model")
	}
}

func TestClientHonoursIndexAndNormalises(t *testing.T) {
	srv, _ := fakeOpenAI(t, 0)
	c := NewClient(srv.URL+"/v1", "test-key", "")
	vecs, err := c.Embed(context.Background(), []string{"alpha beta", "gamma"})
	if err != nil {
		t.Fatal(err)
	}
	want := normalize(fakeVector("alpha beta"))
	if dot(vecs[0], want) < 0.999 {
		t.Error("vector 0 does not match its input (index ignored?)")
	}
	if n := dot(vecs[1], vecs[1]); n < 0.999 || n > 1.001 {
		t.Errorf("vectors should be unit length, got %f", n)
	}
	bad := NewClient(srv.URL+"/v1", "wrong", "")
	if _, err := bad.Embed(context.Background(), []string{"x"}); err == nil {
		t.Error("a 401 should be an error")
	} else if _, retryable := err.(RetryableError); retryable {
		t.Error("a 401 is not retryable")
	}
}

// newVault writes documents and indexes them, the state Start expects.
func newVault(t *testing.T, docs map[string]string) (*index.Index, *vault.Vault) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.New(root)
	if err != nil {
		t.Fatal(err)
	}
	for rel, body := range docs {
		full := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	db, _, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ix := &index.Index{DB: db, Vault: v}
	if err := ix.FullScan(); err != nil {
		t.Fatal(err)
	}
	return ix, v
}

func waitIdle(t *testing.T, e *Embedder) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if e.Status().Pending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("embedder never went idle: %+v", e.Status())
}

func TestEmbedderIndexesSearchesAndRelates(t *testing.T) {
	ix, v := newVault(t, map[string]string{
		"notes/k8s.md":     "# Cluster rollout\n\nWe moved the kubernetes cluster rollout to Tuesday after the ingress controller upgrade.\n",
		"notes/lunch.md":   "# Lunch\n\nTacos with the platform team, then a walk.\n",
		"notes/ingress.md": "# Ingress upgrade\n\nThe ingress controller upgrade needs the kubernetes cluster drained first.\n",
	})
	srv, calls := fakeOpenAI(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e, err := Start(ctx, ix.DB, v, NewClient(srv.URL+"/v1", "test-key", ""))
	if err != nil {
		t.Fatal(err)
	}
	waitIdle(t, e)
	if st := e.Status(); st.Documents != 3 || st.LastError != "" {
		t.Fatalf("status = %+v", st)
	}

	hits, err := e.Search(ctx, "kubernetes rollout", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Path != "notes/k8s.md" {
		t.Errorf("search ranked %+v", hits)
	}
	if hits[len(hits)-1].Path != "notes/lunch.md" {
		t.Errorf("lunch should rank last: %+v", hits)
	}

	related := e.Related("notes/k8s.md", 5)
	if len(related) == 0 || related[0].Path != "notes/ingress.md" {
		t.Errorf("related = %+v", related)
	}
	for _, h := range related {
		if h.Path == "notes/k8s.md" {
			t.Error("a document must not relate to itself")
		}
	}

	// A restart re-embeds nothing: fingerprints are current.
	before := calls.Load()
	cancel()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	e2, err := Start(ctx2, ix.DB, v, NewClient(srv.URL+"/v1", "test-key", ""))
	if err != nil {
		t.Fatal(err)
	}
	waitIdle(t, e2)
	if calls.Load() != before {
		t.Errorf("warm start made %d API calls", calls.Load()-before)
	}

	// Editing one document re-embeds just it; deleting one drops it.
	if _, err := v.Write("notes/lunch.md", []byte("# Lunch\n\nSushi and a kubernetes cluster chat.\n"), ""); err == nil {
		t.Fatal("expected a conflict on blind write")
	}
	f, _ := v.Read("notes/lunch.md")
	if _, err := v.Write("notes/lunch.md", []byte("# Lunch\n\nSushi and a kubernetes cluster chat.\n"), f.SHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.IndexFile("notes/lunch.md"); err != nil {
		t.Fatal(err)
	}
	e2.Notify(index.Event{Path: "notes/lunch.md", Action: "upsert"})
	waitIdle(t, e2)
	hits, _ = e2.Search(ctx2, "kubernetes cluster", 10)
	if len(hits) != 3 || hits[len(hits)-1].Path == "notes/lunch.md" {
		t.Errorf("after edit, lunch should no longer rank last: %+v", hits)
	}
	e2.Notify(index.Event{Path: "notes/lunch.md", Action: "delete"})
	hits, _ = e2.Search(ctx2, "sushi", 10)
	for _, h := range hits {
		if h.Path == "notes/lunch.md" {
			t.Error("deleted document still searchable")
		}
	}
	if e2.Status().Documents != 2 {
		t.Errorf("documents = %d after delete", e2.Status().Documents)
	}
}

func TestEmbedderRetriesRateLimits(t *testing.T) {
	ix, v := newVault(t, map[string]string{"notes/a.md": "# A\n\nsome words here\n"})
	srv, calls := fakeOpenAI(t, 1) // first call 429s
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e, err := Start(ctx, ix.DB, v, NewClient(srv.URL+"/v1", "test-key", ""))
	if err != nil {
		t.Fatal(err)
	}
	// The retry waits retryDelay; check the failure was recorded and the
	// document is still pending rather than dropped.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && calls.Load() < 1 {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	st := e.Status()
	if st.LastError == "" || !strings.Contains(st.LastError, "429") {
		t.Errorf("status after 429 = %+v", st)
	}
	if st.Documents != 0 {
		t.Errorf("nothing should be embedded yet: %+v", st)
	}
}
