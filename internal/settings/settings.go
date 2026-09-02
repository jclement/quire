// Package settings is the owner's app-level configuration that is not
// server config (that is env/config.yaml) and not vault content (that is
// markdown): today, the list of areas and their colours. A JSON file under
// the state dir — precious like auth.db, backed up with it, and readable by
// hand when something looks wrong.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Colors is the palette an area may use. Names, not hex: the CSS defines a
// light and a dark value for each, so a colour chosen once reads on both
// grounds. Order is the order swatches are offered.
var Colors = []string{"slate", "red", "orange", "amber", "green", "teal", "blue", "violet", "pink"}

// AreaDef is one defined area.
type AreaDef struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Settings is everything the file holds.
type Settings struct {
	Areas []AreaDef `json:"areas"`
}

// DefaultAreas is empty on purpose: areas are opt-in. Nothing area-shaped
// appears in the app until two or more are defined in Settings.
var DefaultAreas []AreaDef

// Store reads and writes the settings file.
type Store struct {
	path string
	mu   sync.Mutex
}

// Open returns a store for path. The file need not exist yet.
func Open(path string) *Store { return &Store{path: path} }

// Load reads the file, returning defaults when it does not exist.
func (s *Store) Load() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{Areas: append([]AreaDef(nil), DefaultAreas...)}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("reading settings: %w", err)
	}
	var cfg Settings
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Settings{}, fmt.Errorf("settings file %s is not valid JSON: %w", s.path, err)
	}
	return cfg, nil
}

// Save validates and writes atomically.
func (s *Store) Save(cfg Settings) error {
	if err := ValidateAreas(cfg.Areas); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	return os.Rename(tmp, s.path)
}

// ValidateAreas normalizes names in place and rejects duplicates, empty
// names, the reserved "none", and colours outside the palette. Errors are
// written for the person who typed the value.
func ValidateAreas(areas []AreaDef) error {
	seen := map[string]bool{}
	for i := range areas {
		name := strings.ToLower(strings.TrimSpace(areas[i].Name))
		switch {
		case name == "":
			return fmt.Errorf("an area needs a name")
		case name == "none" || name == "all":
			return fmt.Errorf("%q is reserved — it means the unclassified set", name)
		case strings.ContainsAny(name, "\n:#"):
			return fmt.Errorf("area %q: names cannot contain ':' or '#'", name)
		case seen[name]:
			return fmt.Errorf("area %q is listed twice", name)
		}
		seen[name] = true
		areas[i].Name = name
		if areas[i].Color == "" {
			areas[i].Color = "slate"
		}
		if sort.SearchStrings(sortedColors(), areas[i].Color) >= len(Colors) || !isColor(areas[i].Color) {
			return fmt.Errorf("area %q: %q is not a colour; choose one of %s", name, areas[i].Color, strings.Join(Colors, ", "))
		}
	}
	return nil
}

func isColor(c string) bool {
	for _, known := range Colors {
		if known == c {
			return true
		}
	}
	return false
}

func sortedColors() []string {
	out := append([]string(nil), Colors...)
	sort.Strings(out)
	return out
}
