package index

import (
	"path/filepath"
	"testing"
)

// An index.db made before an index existed gains it on the next open,
// without a reindex: lateIndexes is how an index ships to existing installs.
func TestOpenAddsLateIndexes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"task_links_task", "tasks_done_completed"} {
		if _, err := db.Exec("DROP INDEX " + name); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	db, needsReindex, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if needsReindex {
		t.Error("adding an index should not ask for a reindex")
	}
	for _, name := range []string{"task_links_task", "tasks_done_completed"} {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("index %s missing after reopening", name)
		}
	}
}
