package settings

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultsThenRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Open(path)

	cfg, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Areas) != 0 {
		t.Fatalf("areas are opt-in; defaults = %+v", cfg.Areas)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("Load must not create the file")
	}

	cfg.Areas = append(cfg.Areas, AreaDef{Name: " Side-Project ", Color: "violet"})
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	again, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Areas) != 1 || again.Areas[0].Name != "side-project" {
		t.Errorf("saved areas = %+v (names normalize on save)", again.Areas)
	}
}

func TestValidateAreas(t *testing.T) {
	for _, bad := range [][]AreaDef{
		{{Name: ""}},
		{{Name: "none"}},
		{{Name: "work"}, {Name: "Work"}},      // duplicate after normalizing
		{{Name: "work", Color: "chartreuse"}}, // not in the palette
		{{Name: "a:b"}},
	} {
		if err := ValidateAreas(bad); err == nil {
			t.Errorf("ValidateAreas(%+v) should fail", bad)
		}
	}
	ok := []AreaDef{{Name: "Work"}, {Name: "personal", Color: "green"}}
	if err := ValidateAreas(ok); err != nil {
		t.Fatal(err)
	}
	if ok[0].Name != "work" || ok[0].Color != "slate" {
		t.Errorf("normalization: %+v", ok[0])
	}
}

func TestTimezoneRoundTripAndValidation(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "settings.json"))
	if store.Location() != time.Local {
		t.Error("unset zone should be the server's")
	}
	if err := store.Save(Settings{Timezone: "Mars/Olympus"}); err == nil {
		t.Error("an unknown zone must be refused")
	}
	if err := store.Save(Settings{Timezone: "America/Edmonton"}); err != nil {
		t.Fatal(err)
	}
	if store.Location().String() != "America/Edmonton" {
		t.Errorf("location = %s", store.Location())
	}
	// A fresh store reads the file, not a cache.
	again := Open(store.path)
	cfg, _ := again.Load()
	if cfg.Timezone != "America/Edmonton" {
		t.Errorf("reloaded timezone = %q", cfg.Timezone)
	}
}
