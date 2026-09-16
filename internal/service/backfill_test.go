package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jclement/quire/internal/vision"
)

func TestFindUndescribedImages(t *testing.T) {
	images := map[string]bool{
		"attachments/2026/09/shot.png":      true,
		"attachments/2026/09/two words.png": true,
		"notes/local.jpg":                   true,
		"attachments/2026/09/drawing.svg":   true,
	}
	exists := func(rel string) bool { return images[rel] }

	tests := []struct {
		name     string
		doc      string
		raw      string
		wantAlts []string // raw[AltStart:AltEnd] for each ref, in order
		wantPath []string
	}{
		{"filename alt", "notes/a.md",
			"Look: ![shot.png](attachments/2026/09/shot.png)\n",
			[]string{"shot.png"}, []string{"attachments/2026/09/shot.png"}},
		{"empty alt", "notes/a.md",
			"![](attachments/2026/09/shot.png)\n",
			[]string{""}, []string{"attachments/2026/09/shot.png"}},
		{"alt someone wrote is left alone", "notes/a.md",
			"![The whiteboard after planning](attachments/2026/09/shot.png)\n",
			nil, nil},
		{"fenced block is an example, not an image", "notes/a.md",
			"```md\n![shot.png](attachments/2026/09/shot.png)\n```\n",
			nil, nil},
		{"inline code is an example too", "notes/a.md",
			"Write `![shot.png](attachments/2026/09/shot.png)` to embed.\n",
			nil, nil},
		{"angle brackets and a title", "notes/a.md",
			"![two words.png](<attachments/2026/09/two words.png> \"Title\")\n",
			[]string{"two words.png"}, []string{"attachments/2026/09/two words.png"}},
		{"percent-encoded path", "notes/a.md",
			"![x.png](attachments/2026/09/two%20words.png)\n",
			[]string{"x.png"}, []string{"attachments/2026/09/two words.png"}},
		{"relative to the note, for imported vaults", "notes/a.md",
			"![local.jpg](local.jpg)\n",
			[]string{"local.jpg"}, []string{"notes/local.jpg"}},
		{"remote images are someone else's", "notes/a.md",
			"![shot.png](https://example.com/shot.png)\n",
			nil, nil},
		{"missing file", "notes/a.md",
			"![gone.png](attachments/2026/09/gone.png)\n",
			nil, nil},
		{"formats no vision API reads", "notes/a.md",
			"![drawing.svg](attachments/2026/09/drawing.svg)\n",
			nil, nil},
		{"two on one line, offsets counted past frontmatter", "notes/a.md",
			"---\ntitle: A\n---\n![](attachments/2026/09/shot.png) and ![shot.png](attachments/2026/09/shot.png)\n",
			[]string{"", "shot.png"},
			[]string{"attachments/2026/09/shot.png", "attachments/2026/09/shot.png"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := findUndescribedImages(tt.doc, []byte(tt.raw), exists)
			if len(refs) != len(tt.wantAlts) {
				t.Fatalf("got %d refs, want %d: %+v", len(refs), len(tt.wantAlts), refs)
			}
			for i, ref := range refs {
				if got := tt.raw[ref.AltStart:ref.AltEnd]; got != tt.wantAlts[i] {
					t.Errorf("ref %d alt = %q, want %q", i, got, tt.wantAlts[i])
				}
				if ref.Target != tt.wantPath[i] {
					t.Errorf("ref %d target = %q, want %q", i, ref.Target, tt.wantPath[i])
				}
			}
		})
	}
}

// backfillVault seeds a service with two images and notes referring to them,
// every note backdated so a changed modified time is detectable.
func backfillVault(t *testing.T) (*Service, time.Time) {
	t.Helper()
	s := newTestService(t)
	for _, img := range []string{"attachments/2026/09/shot.png", "attachments/2026/09/board.png"} {
		if _, err := s.Vault.Write(img, testPNG, ""); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Date(2025, 3, 14, 9, 0, 0, 0, time.UTC)
	for rel, body := range map[string]string{
		"notes/one.md": "---\ntitle: One\n---\n# One\n\nBefore ![shot.png](attachments/2026/09/shot.png) after.\n\n" +
			"```md\n![shot.png](attachments/2026/09/shot.png)\n```\n\n" +
			"![Our whiteboard](attachments/2026/09/board.png)\n",
		"notes/two.md": "# Two\n\n![](attachments/2026/09/shot.png)\n",
	} {
		if _, err := s.Vault.Write(rel, []byte(body), ""); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(s.Vault.Dir, rel), old, old); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Index.IndexFile(rel); err != nil {
			t.Fatal(err)
		}
	}
	return s, old
}

func TestBackfillRewritesOnlyTheAltAndKeepsModifiedTimes(t *testing.T) {
	s, old := backfillVault(t)
	calls := stubVision(t, s, "A deploy dashboard, all green.", http.StatusOK)

	report, err := s.BackfillImageDescriptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := BackfillReport{Found: 2, Described: 1, Documents: 2}
	if report != want {
		t.Errorf("report = %+v, want %+v", report, want)
	}
	// One image referenced from two notes is one call, not two.
	if n := calls.Load(); n != 1 {
		t.Errorf("vision called %d times, want 1", n)
	}

	one, _ := s.Vault.Read("notes/one.md")
	wantOne := "---\ntitle: One\n---\n# One\n\nBefore ![A deploy dashboard, all green.](attachments/2026/09/shot.png) after.\n\n" +
		"```md\n![shot.png](attachments/2026/09/shot.png)\n```\n\n" +
		"![Our whiteboard](attachments/2026/09/board.png)\n"
	if string(one.Raw) != wantOne {
		t.Errorf("one.md =\n%s\nwant\n%s", one.Raw, wantOne)
	}
	two, _ := s.Vault.Read("notes/two.md")
	if want := "# Two\n\n![A deploy dashboard, all green.](attachments/2026/09/shot.png)\n"; string(two.Raw) != want {
		t.Errorf("two.md = %q, want %q", two.Raw, want)
	}

	// Maintenance, not an edit: neither the file nor the index should now
	// claim these notes were worked on today.
	for _, rel := range []string{"notes/one.md", "notes/two.md"} {
		info, err := os.Stat(filepath.Join(s.Vault.Dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(old) {
			t.Errorf("%s modified time = %v, want %v", rel, info.ModTime(), old)
		}
		var indexed int64
		if err := s.Index.DB.QueryRow("SELECT mtime FROM documents WHERE path = ?", rel).Scan(&indexed); err != nil {
			t.Fatal(err)
		}
		if indexed != old.Unix() {
			t.Errorf("%s indexed mtime = %d, want %d", rel, indexed, old.Unix())
		}
	}

	// And the second start finds nothing and spends nothing.
	again, err := s.BackfillImageDescriptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again != (BackfillReport{}) {
		t.Errorf("second run report = %+v, want zero", again)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("second run called vision; total calls %d", n)
	}
}

func TestBackfillKeepsTheFilenameWhenDescribingFails(t *testing.T) {
	s, _ := backfillVault(t)
	before, _ := s.Vault.Read("notes/two.md")
	stubVision(t, s, "", http.StatusBadRequest)

	report, err := s.BackfillImageDescriptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed != 1 || report.Described != 0 || report.Documents != 0 {
		t.Errorf("report = %+v, want one failure and no rewrites", report)
	}
	after, _ := s.Vault.Read("notes/two.md")
	if string(after.Raw) != string(before.Raw) {
		t.Errorf("note changed despite the failure: %q", after.Raw)
	}
}

func TestBackfillRetriesRateLimits(t *testing.T) {
	s, _ := backfillVault(t)
	saved := backfillRetryDelays
	backfillRetryDelays = []time.Duration{time.Millisecond}
	t.Cleanup(func() { backfillRetryDelays = saved })

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A graph."}}]}`))
	}))
	t.Cleanup(srv.Close)
	s.Vision = vision.NewClient(srv.URL, "k", "m")

	report, err := s.BackfillImageDescriptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Described != 1 || report.Failed != 0 {
		t.Errorf("report = %+v, want the retry to succeed", report)
	}
}

func TestBackfillStopsWhenCancelled(t *testing.T) {
	s, _ := backfillVault(t)
	calls := stubVision(t, s, "never", http.StatusOK)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := s.BackfillImageDescriptions(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("vision called %d times after cancellation", n)
	}
}

func TestBackfillNeedsVision(t *testing.T) {
	s := newTestService(t)
	if _, err := s.BackfillImageDescriptions(context.Background()); !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}
