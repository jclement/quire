// The feed list. An ICS feed URL is a bearer secret — anyone holding it
// reads the whole calendar — so it is stored apart from settings.json
// (which is meant to be readable by hand and is written 0644) in its own
// 0600 file under the state dir, next to auth.db. It is never logged and
// never returned by the API after it is saved: callers see Masked().
package calendar

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Feed is one subscribed calendar.
type Feed struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Masked is what may be shown of the URL: scheme, host and the last four
// characters, enough to tell two feeds apart and nowhere near enough to
// read either.
func (f Feed) Masked() string {
	parsed, err := url.Parse(f.URL)
	if err != nil || parsed.Host == "" {
		return "••••"
	}
	tail := f.URL
	if len(tail) > 4 {
		tail = tail[len(tail)-4:]
	}
	return parsed.Scheme + "://" + parsed.Host + "/…" + tail
}

// Label names the feed in logs and errors without its secret.
func (f Feed) Label() string {
	if parsed, err := url.Parse(f.URL); err == nil && parsed.Host != "" {
		return "feed " + f.ID + " (" + parsed.Host + ")"
	}
	return "feed " + f.ID
}

// Store persists the feed list.
type Store struct {
	path string
	mu   sync.Mutex
}

// OpenStore returns a store for path; the file need not exist yet.
func OpenStore(path string) *Store { return &Store{path: path} }

// Feeds returns the configured feeds, none when the file does not exist.
func (s *Store) Feeds() ([]Feed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *Store) read() ([]Feed, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading calendar feeds: %w", err)
	}
	var feeds []Feed
	if err := json.Unmarshal(raw, &feeds); err != nil {
		return nil, fmt.Errorf("calendar feed file %s is not valid JSON: %w", s.path, err)
	}
	return feeds, nil
}

// ValidateURL accepts an http(s) URL. webcal:// — what "subscribe" links
// usually are — is the same thing over https, so it is rewritten.
func ValidateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(raw, "webcal://"); ok {
		raw = "https://" + rest
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", fmt.Errorf("a calendar feed must be an https:// (or webcal://) URL — copy the secret ICS link from your calendar's sharing settings")
	}
	return raw, nil
}

// Add appends a feed and returns it with its new ID.
func (s *Store) Add(rawURL string) (Feed, error) {
	clean, err := ValidateURL(rawURL)
	if err != nil {
		return Feed{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	feeds, err := s.read()
	if err != nil {
		return Feed{}, err
	}
	for _, f := range feeds {
		if f.URL == clean {
			return Feed{}, fmt.Errorf("that feed is already subscribed (%s)", f.Masked())
		}
	}
	feed := Feed{ID: newID(), URL: clean}
	if err := s.write(append(feeds, feed)); err != nil {
		return Feed{}, err
	}
	return feed, nil
}

// Remove deletes a feed by ID; false when there was none.
func (s *Store) Remove(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	feeds, err := s.read()
	if err != nil {
		return false, err
	}
	kept := feeds[:0]
	for _, f := range feeds {
		if f.ID != id {
			kept = append(kept, f)
		}
	}
	if len(kept) == len(feeds) {
		return false, nil
	}
	return true, s.write(kept)
}

// write replaces the file atomically, owner-readable only.
func (s *Store) write(feeds []Feed) error {
	if feeds == nil {
		feeds = []Feed{}
	}
	raw, err := json.MarshalIndent(feeds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing calendar feeds: %w", err)
	}
	return os.Rename(tmp, s.path)
}

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
