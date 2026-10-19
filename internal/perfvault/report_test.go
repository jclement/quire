package perfvault_test

// The performance report: generates (once, cached) a synthetic vault, wires
// the real API over it, and prints per-endpoint latencies — first request,
// sequential median/p95, a page's requests fired at once, and the same
// again while a writer keeps re-indexing a file. Off by default; run with
//
//	QUIRE_PERF=1 go test ./internal/perfvault -run TestPerfReport -v
//
// QUIRE_PERF_SCALE=2 for DESIGN's 10k-document budget, and add
// -cpuprofile cpu.out to see where the time goes. QUIRE_PERF_ONLY=<url>
// repeats just that request ({today}, {person} and {project} expand).

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jclement/quire/internal/api"
	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/perfvault"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

var perfToday = time.Date(2026, 10, 18, 10, 0, 0, 0, time.Local)

type perfEnv struct {
	svc     *service.Service
	handler http.Handler
	close   func()
}

func openPerfEnv(t *testing.T, dir string) perfEnv {
	t.Helper()
	v, err := vault.New(filepath.Join(dir, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	ix, _, err := index.OpenIndex(filepath.Join(dir, "index.db"), v)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := ix.FullScan(); err != nil {
		t.Fatal(err)
	}
	t.Logf("full scan: %v", time.Since(start).Round(time.Millisecond))
	svc := service.New(v, ix)
	svc.Now = func() time.Time { return perfToday }
	s := &api.Server{Service: svc, Events: api.NewBroadcaster(), Version: "perf"}
	mux := http.NewServeMux()
	s.Routes(mux)
	return perfEnv{svc: svc, handler: mux, close: func() { ix.Close() }}
}

func (e perfEnv) get(t *testing.T, url string) time.Duration {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	start := time.Now()
	e.handler.ServeHTTP(rec, req)
	d := time.Since(start)
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("GET %s: %d %s", url, rec.Code, body)
	}
	return d
}

func quantile(ds []time.Duration, q float64) time.Duration {
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[min(int(float64(len(s))*q), len(s)-1)]
}

func ms(d time.Duration) string { return fmt.Sprintf("%7.2f", float64(d.Microseconds())/1000) }

func TestPerfReport(t *testing.T) {
	if os.Getenv("QUIRE_PERF") == "" {
		t.Skip("set QUIRE_PERF=1 to run the performance report")
	}
	scale := 1.0
	if s := os.Getenv("QUIRE_PERF_SCALE"); s != "" {
		var err error
		if scale, err = strconv.ParseFloat(s, 64); err != nil {
			t.Fatal(err)
		}
	}
	dir := os.Getenv("QUIRE_PERF_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("quire-perf-x%g", scale))
	}
	cfg := perfvault.Default(perfToday).Scaled(scale)
	if _, err := os.Stat(filepath.Join(dir, "vault")); err != nil {
		start := time.Now()
		st, err := perfvault.Generate(filepath.Join(dir, "vault"), cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("generated %d docs, %d tasks in %v", st.Docs, st.Tasks, time.Since(start).Round(time.Millisecond))
	}
	env := openPerfEnv(t, dir)
	defer env.close()

	today := perfToday.Format("2006-01-02")
	hotPerson := "people/" + perfvault.HotPersonSlug(cfg) + ".md"
	hotProject := "projects/" + perfvault.HotProjectSlug(cfg) + ".md"
	endpoints := []string{
		"/api/v1/today",
		"/api/v1/daily/" + today,
		"/api/v1/daily/" + perfToday.AddDate(0, 0, -40).Format("2006-01-02"),
		"/api/v1/daily?before=" + today + "&limit=7",
		"/api/v1/documents?limit=50",
		"/api/v1/documents?type=person&limit=500",
		"/api/v1/documents/" + hotPerson,
		"/api/v1/documents/" + hotProject,
		"/api/v1/tasks?view=today",
		"/api/v1/tasks?view=upcoming",
		"/api/v1/tasks?view=waiting",
		"/api/v1/tasks?view=inbox",
		"/api/v1/tasks?view=logbook",
		"/api/v1/waiting",
		"/api/v1/search?q=pipeline",
		"/api/v1/search?q=" + "sarah",
		"/api/v1/calendar?month=" + perfToday.Format("2006-01"),
		"/api/v1/tags",
		"/api/v1/areas",
		"/api/v1/unwritten",
		"/api/v1/decisions",
		"/api/v1/weekly/" + isoWeek(perfToday),
	}

	n := 15
	if s := os.Getenv("QUIRE_PERF_N"); s != "" {
		n, _ = strconv.Atoi(s)
	}
	// QUIRE_PERF_ONLY=<url> just repeats that one request n times: what to
	// pair with -cpuprofile when one endpoint is the question.
	if only := os.Getenv("QUIRE_PERF_ONLY"); only != "" {
		only = strings.NewReplacer("{today}", today, "{person}", hotPerson, "{project}", hotProject).Replace(only)
		var ds []time.Duration
		for range n {
			ds = append(ds, env.get(t, only))
		}
		fmt.Printf("%-60s p50 %s  p95 %s\n", only, ms(quantile(ds, 0.5)), ms(quantile(ds, 0.95)))
		return
	}
	fmt.Printf("\nscale x%g  (%s)\n%-60s %8s %8s %8s\n", scale, dir, "endpoint", "first", "p50", "p95")
	for _, url := range endpoints {
		first := env.get(t, url)
		var ds []time.Duration
		for range n {
			ds = append(ds, env.get(t, url))
		}
		fmt.Printf("%-60s %s %s %s\n", url, ms(first), ms(quantile(ds, 0.5)), ms(quantile(ds, 0.95)))
	}

	// What the Today and Daily pages fire together on load (shell + page).
	pages := map[string][]string{
		"today page":  {"/api/v1/today", "/api/v1/areas", "/api/v1/tasks?view=inbox", "/api/v1/documents?limit=50", "/api/v1/tags"},
		"daily page":  {"/api/v1/daily/" + today, "/api/v1/daily?before=" + today + "&limit=7", "/api/v1/areas", "/api/v1/tasks?view=inbox", "/api/v1/documents?limit=50"},
		"person page": {"/api/v1/documents/" + hotPerson, "/api/v1/areas", "/api/v1/tasks?view=inbox", "/api/v1/documents?limit=50"},
	}
	burst := func(urls []string) time.Duration {
		var wg sync.WaitGroup
		start := time.Now()
		for _, u := range urls {
			wg.Go(func() { env.get(t, u) })
		}
		wg.Wait()
		return time.Since(start)
	}
	run := func(label string) {
		fmt.Printf("\n%s\n%-60s %8s %8s\n", label, "page burst (all requests at once)", "p50", "p95")
		for _, name := range []string{"today page", "daily page", "person page"} {
			var ds []time.Duration
			for range n {
				ds = append(ds, burst(pages[name]))
			}
			fmt.Printf("%-60s %s %s\n", name, ms(quantile(ds, 0.5)), ms(quantile(ds, 0.95)))
		}
	}
	run("idle")

	// A write on its own: a body edit (a task appended) to a busy note.
	{
		path := "notes/perf-writer.md"
		var ds []time.Duration
		for i := range n {
			content := fmt.Sprintf("# Perf writer\n\n- [ ] Solo %d [[%s]] 📅 %s\n", i, perfvault.HotPersonTitle(cfg), today)
			start := time.Now()
			if _, err := env.svc.UpdateDocument(path, content, currentSHA(env.svc, path)); err != nil {
				t.Fatal(err)
			}
			ds = append(ds, time.Since(start))
		}
		fmt.Printf("\n%-60s %8s %8s\n%-60s %s %s\n", "write", "p50", "p95", "UpdateDocument (body edit)", ms(quantile(ds, 0.5)), ms(quantile(ds, 0.95)))
	}

	// A writer re-indexing a meeting note every 25ms: an agent or the
	// watcher busy while someone reads.
	stop := make(chan struct{})
	var writes atomic.Int64
	var wg sync.WaitGroup
	wg.Go(func() {
		path := "notes/perf-writer.md"
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			content := fmt.Sprintf("# Perf writer\n\n- [ ] Write %d [[%s]] 📅 %s\n", i, perfvault.HotPersonTitle(cfg), today)
			if _, err := env.svc.UpdateDocument(path, content, currentSHA(env.svc, path)); err != nil {
				t.Error(err)
				return
			}
			writes.Add(1)
			time.Sleep(25 * time.Millisecond)
		}
	})
	run("under write load")
	close(stop)
	wg.Wait()
	fmt.Printf("(%d writes during the run)\n", writes.Load())
}

func currentSHA(s *service.Service, path string) string {
	f, err := s.Vault.Read(path)
	if err != nil {
		return ""
	}
	return f.SHA256
}

func isoWeek(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}
