package settings

import (
	"os"
	"path/filepath"
	"testing"
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
