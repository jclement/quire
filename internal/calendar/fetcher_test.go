package calendar

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// secretPath is the part of a feed URL that is the credential.
const secretPath = "/dav/calendars/user/jeff/s3cr3t-t0ken-4f9a/basic.ics"

// newTestFetcher returns a fetcher on a fresh store with one feed at url,
// and a clock the test can move.
func newTestFetcher(t *testing.T, url string) (*Fetcher, *time.Time) {
	t.Helper()
	store := OpenStore(filepath.Join(t.TempDir(), "calendar-feeds.json"))
	if url != "" {
		if _, err := store.Add(url); err != nil {
			t.Fatal(err)
		}
	}
	fetcher := NewFetcher(store)
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	fetcher.Now = func() time.Time { return now }
	return fetcher, &now
}

// captureLogs routes slog to a buffer for the test's duration.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func TestFetchSuccessServesEvents(t *testing.T) {
	fixture, err := os.ReadFile("testdata/timezones.ics")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != secretPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(server.Close)

	fetcher, _ := newTestFetcher(t, server.URL+secretPath)
	fetcher.Refresh(context.Background())

	status, err := fetcher.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 1 || status[0].Error != "" || status[0].LastSuccess.IsZero() || status[0].Events != 4 {
		t.Fatalf("status = %+v", status)
	}
	day := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	events := fetcher.Events(day, day.AddDate(0, 0, 1), time.UTC)
	if len(events) != 4 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Feed != status[0].ID {
		t.Errorf("events carry their feed id: %q vs %q", events[0].Feed, status[0].ID)
	}
}

func TestFailureBacksOffKeepsLastGoodCopyAndNeverLeaksTheURL(t *testing.T) {
	logs := captureLogs(t)
	fixture, _ := os.ReadFile("testdata/allday.ics")
	var healthy atomic.Bool
	healthy.Store(true)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if !healthy.Load() {
			http.Error(w, "gone", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(server.Close)

	fetcher, now := newTestFetcher(t, server.URL+secretPath)
	ctx := context.Background()
	fetcher.fetchDue(ctx)
	if hits.Load() != 1 {
		t.Fatalf("first pass fetches, hits = %d", hits.Load())
	}

	// Not due again until the interval passes.
	fetcher.fetchDue(ctx)
	if hits.Load() != 1 {
		t.Fatalf("a fresh feed is not refetched early, hits = %d", hits.Load())
	}

	healthy.Store(false)
	*now = now.Add(DefaultInterval)
	fetcher.fetchDue(ctx)
	status, _ := fetcher.Status()
	if status[0].Failures != 1 || !strings.Contains(status[0].Error, "503") {
		t.Fatalf("status after failure = %+v", status[0])
	}
	// The cached calendar survives a failed refresh.
	day := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	if got := fetcher.Events(day, day.AddDate(0, 0, 1), time.UTC); len(got) != 2 {
		t.Errorf("last good copy should still serve, got %+v", got)
	}

	// Backoff: one minute, then two.
	*now = now.Add(30 * time.Second)
	fetcher.fetchDue(ctx)
	if hits.Load() != 2 {
		t.Fatalf("inside the backoff nothing is fetched, hits = %d", hits.Load())
	}
	*now = now.Add(31 * time.Second)
	fetcher.fetchDue(ctx)
	if hits.Load() != 3 {
		t.Fatalf("after a minute it retries, hits = %d", hits.Load())
	}
	*now = now.Add(time.Minute + time.Second)
	fetcher.fetchDue(ctx)
	if hits.Load() != 3 {
		t.Fatalf("the second backoff is two minutes, hits = %d", hits.Load())
	}

	// A connection failure is the case where Go quotes the URL.
	server.Close()
	fetcher.Refresh(ctx)
	status, _ = fetcher.Status()
	if status[0].Error == "" {
		t.Fatal("a refused connection is an error")
	}
	for _, text := range []string{logs.String(), status[0].Error, status[0].Masked} {
		if strings.Contains(text, "s3cr3t") {
			t.Errorf("the feed secret leaked: %s", text)
		}
	}
	if !strings.Contains(logs.String(), "calendar: fetch failed") {
		t.Errorf("failures are logged (by label): %s", logs.String())
	}

	healthy.Store(true)
}

func TestBackoffIsCapped(t *testing.T) {
	for failures, want := range map[int]time.Duration{1: time.Minute, 2: 2 * time.Minute, 3: 4 * time.Minute, 7: time.Hour, 40: time.Hour} {
		if got := backoff(failures); got != want {
			t.Errorf("backoff(%d) = %v, want %v", failures, got, want)
		}
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	fetcher, _ := newTestFetcher(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		fetcher.Run(ctx)
		close(done)
	}()
	fetcher.Kick()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestStoreIsPrivateAndMasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calendar-feeds.json")
	store := OpenStore(path)
	feed, err := store.Add("webcal://caldav.fastmail.com" + secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(feed.URL, "https://") {
		t.Errorf("webcal is https: %s", feed.URL)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("feed file mode = %v, want 0600", info.Mode().Perm())
	}
	if masked := feed.Masked(); strings.Contains(masked, "s3cr3t") || !strings.HasPrefix(masked, "https://caldav.fastmail.com/") {
		t.Errorf("masked = %q", masked)
	}
	if _, err := store.Add("https://caldav.fastmail.com" + secretPath); err == nil {
		t.Error("the same feed twice is refused")
	}
	if _, err := store.Add("ftp://example.com/x.ics"); err == nil {
		t.Error("only http(s) feeds")
	}
	removed, err := store.Remove(feed.ID)
	if err != nil || !removed {
		t.Fatalf("remove = %v, %v", removed, err)
	}
	if feeds, _ := store.Feeds(); len(feeds) != 0 {
		t.Errorf("feeds after remove = %+v", feeds)
	}
}

// panickingTransport stands in for anything below Parse that might panic.
type panickingTransport struct{}

func (panickingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("transport exploded")
}

// A feed that makes the decoder panic, or a fetch that panics outright,
// must leave the server running, the feed marked failing, and the last good
// copy serving — the URL is refetched at every start, so a panic here was a
// crash loop.
func TestFetchSurvivesPanics(t *testing.T) {
	good, _ := os.ReadFile("testdata/allday.ics")
	var broken atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if broken.Load() {
			_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nATTENDEE;CN=Bo"))
			return
		}
		_, _ = w.Write(good)
	}))
	t.Cleanup(server.Close)
	fetcher, _ := newTestFetcher(t, server.URL+secretPath)
	ctx := context.Background()
	fetcher.Refresh(ctx)

	broken.Store(true)
	fetcher.Refresh(ctx)
	status, _ := fetcher.Status()
	if status[0].Error == "" || status[0].Failures != 1 {
		t.Fatalf("a malformed feed is a failure: %+v", status[0])
	}
	day := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	if got := fetcher.Events(day, day.AddDate(0, 0, 1), time.UTC); len(got) != 2 {
		t.Errorf("last good copy keeps serving, got %+v", got)
	}

	fetcher.Client = &http.Client{Transport: panickingTransport{}}
	fetcher.Refresh(ctx)
	status, _ = fetcher.Status()
	if status[0].Failures != 2 || !strings.Contains(status[0].Error, "exploded") {
		t.Errorf("a panicking fetch is recorded as a failure: %+v", status[0])
	}
}
