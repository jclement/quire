// The fetcher: a background loop that downloads every subscribed feed
// every ten minutes, keeps the last good copy of each in memory, and backs
// off a feed that is failing. Nothing is persisted — the feeds are derived
// data a restart re-fetches in seconds — and nothing is expanded here:
// queries call Events, which expands the cached calendars for exactly the
// window and zone asked for.
//
// A feed's URL is its password, and Go's HTTP errors quote the URL they
// failed on ("Get \"https://…/secret.ics\": dial tcp …"). Every error is
// therefore stripped of the URL before it is logged or stored for the
// Settings page; the tests check the secret never appears in either.
package calendar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
)

// Defaults for the loop. Ten minutes matches how often calendar providers
// regenerate a published feed; polling harder only costs their bandwidth.
const (
	DefaultInterval = 10 * time.Minute
	minBackoff      = time.Minute
	maxBackoff      = time.Hour
	fetchTimeout    = 30 * time.Second
	// DefaultAddWait is how long adding a feed waits for its first fetch
	// before answering "still fetching". Short, because the request may be
	// behind a tunnel (Cloudflare gives up at ~100s) and the feed is saved
	// whether or not the first fetch has finished.
	DefaultAddWait = 5 * time.Second
	// maxFeedBytes bounds one download; years of history in a busy
	// calendar is a few megabytes.
	maxFeedBytes = 20 << 20
)

// FeedStatus is what Settings shows for one feed.
type FeedStatus struct {
	ID string
	// Masked is the URL with its secret elided.
	Masked      string
	LastAttempt time.Time
	LastSuccess time.Time
	// Error is the last failure, already free of the URL; "" when the most
	// recent fetch worked.
	Error    string
	Failures int
	Events   int
	// Fetching is true while a newly added feed's first fetch is running.
	Fetching bool
}

// feedState is the in-memory cache and bookkeeping for one feed.
type feedState struct {
	calendars   []*ical.Calendar
	lastAttempt time.Time
	lastSuccess time.Time
	nextAttempt time.Time
	err         string
	failures    int
	components  int
	fetching    bool
}

// Fetcher keeps the subscribed feeds fresh. The zero value is not usable;
// build one with NewFetcher.
type Fetcher struct {
	Store  *Store
	Client *http.Client
	// Interval between successful fetches of a feed.
	Interval time.Duration
	// AddWait bounds how long Add waits for a new feed's first fetch.
	AddWait time.Duration
	// Now is the clock, pinned by tests.
	Now func() time.Time

	mu     sync.Mutex
	states map[string]*feedState
	// passMu serialises fetch passes so an on-demand refresh and the
	// timer never download the same feed twice at once.
	passMu sync.Mutex
	wake   chan struct{}
}

// NewFetcher returns a fetcher over the store's feeds.
func NewFetcher(store *Store) *Fetcher {
	return &Fetcher{
		Store:    store,
		Client:   &http.Client{Timeout: fetchTimeout},
		Interval: DefaultInterval,
		AddWait:  DefaultAddWait,
		Now:      time.Now,
		states:   map[string]*feedState{},
		wake:     make(chan struct{}, 1),
	}
}

// Run fetches feeds as they fall due until ctx is cancelled. A feed is due
// Interval after its last success, or after its backoff when failing.
func (f *Fetcher) Run(ctx context.Context) {
	for {
		f.fetchDue(ctx)
		timer := time.NewTimer(f.untilNextDue())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-f.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// Kick asks the background loop to look again now — after a feed is added,
// say — without waiting for the fetch.
func (f *Fetcher) Kick() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

// Refresh fetches every feed now, ignoring backoff, and returns when done.
// It is the Settings page's "Refresh" and what adding a feed runs, so the
// owner sees at once whether the URL works.
func (f *Fetcher) Refresh(ctx context.Context) {
	feeds, err := f.Store.Feeds()
	if err != nil {
		slog.Warn("calendar: reading feeds", "err", err)
		return
	}
	f.fetchAll(ctx, feeds)
}

// Add subscribes to a feed and starts its first fetch — that feed only, not
// a pass over every feed, which could take 30s per slow feed. It waits up
// to AddWait for the fetch so the common case answers with a result, then
// returns regardless; Status reports Fetching until the fetch lands.
func (f *Fetcher) Add(ctx context.Context, rawURL string) (Feed, error) {
	feed, err := f.Store.Add(rawURL)
	if err != nil {
		return Feed{}, err
	}
	f.mu.Lock()
	// Marked due only after the fetch timeout, so the background loop does
	// not start a second download of it meanwhile.
	f.states[feed.ID] = &feedState{fetching: true, nextAttempt: f.Now().Add(fetchTimeout)}
	f.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Detached from the request, which ends before a slow fetch does;
		// download still bounds it with fetchTimeout.
		f.fetchOne(context.WithoutCancel(ctx), feed)
	}()
	timer := time.NewTimer(f.AddWait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	case <-ctx.Done():
	}
	return feed, nil
}

func (f *Fetcher) fetchDue(ctx context.Context) {
	feeds, err := f.Store.Feeds()
	if err != nil {
		slog.Warn("calendar: reading feeds", "err", err)
		return
	}
	now := f.Now()
	var due []Feed
	f.mu.Lock()
	for _, feed := range feeds {
		state := f.states[feed.ID]
		if state == nil || !now.Before(state.nextAttempt) {
			due = append(due, feed)
		}
	}
	f.mu.Unlock()
	f.fetchAll(ctx, due)
}

func (f *Fetcher) fetchAll(ctx context.Context, feeds []Feed) {
	f.passMu.Lock()
	defer f.passMu.Unlock()
	for _, feed := range feeds {
		if ctx.Err() != nil {
			return
		}
		f.fetchOne(ctx, feed)
	}
}

// fetchOne downloads and parses one feed, recording the outcome. A failure
// keeps the last good calendars: an hour-old agenda beats an empty one.
func (f *Fetcher) fetchOne(ctx context.Context, feed Feed) {
	calendars, err := f.download(ctx, feed)
	now := f.Now()

	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.states[feed.ID]
	if state == nil {
		state = &feedState{}
		f.states[feed.ID] = state
	}
	state.lastAttempt = now
	state.fetching = false
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down; not the feed's fault
		}
		state.failures++
		state.err = redact(err, feed)
		state.nextAttempt = now.Add(backoff(state.failures))
		slog.Warn("calendar: fetch failed", "feed", feed.Label(), "failures", state.failures,
			"retry_in", backoff(state.failures), "err", state.err)
		return
	}
	state.calendars = calendars
	state.lastSuccess = now
	state.failures = 0
	state.err = ""
	state.nextAttempt = now.Add(f.Interval)
	state.components = 0
	for _, cal := range calendars {
		state.components += len(cal.Events())
	}
}

// backoff doubles from a minute to an hour: a feed that is down for a
// moment recovers quickly, one that is gone stops being hammered.
func backoff(failures int) time.Duration {
	wait := minBackoff
	for i := 1; i < failures && wait < maxBackoff; i++ {
		wait *= 2
	}
	return min(wait, maxBackoff)
}

// download fetches and parses one feed. Parse already turns go-ical's
// panics into errors; the recover here is the backstop for anything else
// on this path, since this runs on the server's own goroutine and a feed
// is refetched at every start — a panic would be a crash loop.
func (f *Fetcher) download(ctx context.Context, feed Feed) (cals []*ical.Calendar, err error) {
	defer func() {
		if r := recover(); r != nil {
			cals, err = nil, fmt.Errorf("fetching the feed failed unexpectedly: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "quire (calendar feed reader)")
	req.Header.Set("Accept", "text/calendar")
	res, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the calendar server answered %s — check the feed URL is still the current secret link", res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxFeedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading the feed: %w", err)
	}
	if len(raw) > maxFeedBytes {
		return nil, fmt.Errorf("the feed is over %dMB", maxFeedBytes>>20)
	}
	return Parse(raw)
}

// redact turns an error into text safe to log and show: the *url.Error
// wrapper that quotes the URL is peeled off, and any remaining mention of
// the URL (or its path, which is where the secret lives) is masked.
func redact(err error, feed Feed) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	msg := strings.ReplaceAll(err.Error(), feed.URL, feed.Masked())
	if parsed, perr := url.Parse(feed.URL); perr == nil && len(parsed.Path) > 1 {
		msg = strings.ReplaceAll(msg, parsed.Path, "/…")
		if parsed.RawQuery != "" {
			msg = strings.ReplaceAll(msg, parsed.RawQuery, "…")
		}
	}
	return msg
}

func (f *Fetcher) untilNextDue() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	wait := f.Interval
	now := f.Now()
	for _, state := range f.states {
		if until := state.nextAttempt.Sub(now); until < wait {
			wait = until
		}
	}
	// A feed with no state yet was just added and will be fetched on the
	// next pass; never spin.
	return max(wait, time.Second)
}

// Status reports every configured feed, in the order they were added.
func (f *Fetcher) Status() ([]FeedStatus, error) {
	feeds, err := f.Store.Feeds()
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FeedStatus, 0, len(feeds))
	for _, feed := range feeds {
		status := FeedStatus{ID: feed.ID, Masked: feed.Masked()}
		if state := f.states[feed.ID]; state != nil {
			status.LastAttempt = state.lastAttempt
			status.LastSuccess = state.lastSuccess
			status.Error = state.err
			status.Failures = state.failures
			status.Events = state.components
			status.Fetching = state.fetching
		}
		out = append(out, status)
	}
	return out, nil
}

// Events expands every configured feed's cached calendars over [from, to)
// in loc, sorted by start. A feed removed from the store stops contributing
// at once, even though its cache lingers until the next pass.
func (f *Fetcher) Events(from, to time.Time, loc *time.Location) []Event {
	feeds, err := f.Store.Feeds()
	if err != nil {
		return nil
	}
	f.mu.Lock()
	cached := map[string][]*ical.Calendar{}
	for _, feed := range feeds {
		if state := f.states[feed.ID]; state != nil {
			cached[feed.ID] = state.calendars
		}
	}
	f.mu.Unlock()

	var out []Event
	for _, feed := range feeds {
		out = append(out, Expand(cached[feed.ID], feed.ID, from, to, loc)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// Forget drops a removed feed's cache.
func (f *Fetcher) Forget(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.states, id)
}
