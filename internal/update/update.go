// Package update answers one question for the Settings page: is there a
// newer release than the one running? It asks GitHub's releases API at most
// once a day, in the background, and fails silent — an update check must
// never slow down or break the app it is checking.
//
// It is opt-out (QUIRE_UPDATE_CHECK=false) because a self-hosted app
// reaching out to a third party should be something you can say no to.
// Dev builds never check: "dev" has no meaningful comparison.
package update

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultReleasesURL = "https://api.github.com/repos/jclement/quire/releases/latest"
	checkInterval      = 24 * time.Hour
	checkTimeout       = 10 * time.Second
)

// Checker holds the last known answer. The zero value reports false, which
// is the right answer when nothing has been checked yet.
type Checker struct {
	current   string
	available atomic.Bool
	// url is the endpoint to ask; overridden in tests so the check can be
	// exercised without reaching GitHub.
	url string
}

// Start begins periodic checking against the running version and returns a
// Checker. It returns nil for a dev build, so callers can leave the wiring
// unconditional and get an honest "no" for free.
func Start(ctx context.Context, currentVersion string) *Checker {
	if currentVersion == "" || currentVersion == "dev" {
		return nil
	}
	c := &Checker{current: currentVersion, url: defaultReleasesURL}
	go func() {
		// A short initial delay keeps startup off the network path.
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				c.checkOnce(ctx)
				timer.Reset(checkInterval)
			}
		}
	}()
	return c
}

// Available reports whether a newer release was seen. Safe on a nil Checker.
func (c *Checker) Available() bool {
	return c != nil && c.available.Load()
}

func (c *Checker) checkOnce(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Debug("update check failed", "err", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		slog.Debug("update check returned an error", "status", res.StatusCode)
		return
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return
	}
	newer := IsNewer(c.current, body.TagName)
	if newer {
		slog.Info("a newer quire release is available", "running", c.current, "latest", body.TagName)
	}
	c.available.Store(newer)
}

// IsNewer compares two semver-ish version strings, tolerating a leading "v"
// and any pre-release suffix. Anything it cannot parse compares as "not
// newer": a bad parse must not nag the user forever.
func IsNewer(current, latest string) bool {
	cur, okCur := parseVersion(current)
	lat, okLat := parseVersion(latest)
	if !okCur || !okLat {
		return false
	}
	for i := range 3 {
		if lat[i] != cur[i] {
			return lat[i] > cur[i]
		}
	}
	return false
}

// parseVersion splits "v1.2.3-rc1" into [1 2 3].
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
