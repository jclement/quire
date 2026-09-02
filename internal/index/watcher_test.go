// The watcher is what makes "edit your markdown outside the app" true, and
// it had no test. Everything here drives the real fsnotify loop against a
// real temp vault, because the interesting behaviour is precisely what the
// filesystem does: editors that save by atomic rename, files that vanish,
// directories that appear later.
package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jclement/quire/internal/vault"
)

// startWatcher runs Watch in the background over a fresh vault and returns
// the index plus the vault root.
func startWatcher(t *testing.T) (*Index, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.New(root)
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ix := &Index{DB: db, Vault: v}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := ix.Watch(ctx); err != nil {
			t.Errorf("Watch returned %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Watch did not return after its context was cancelled")
		}
	})
	// fsnotify needs a moment to register its watches before the first write.
	time.Sleep(150 * time.Millisecond)
	return ix, root
}

// indexedPaths is what the index currently believes exists.
func indexedPaths(t *testing.T, ix *Index) map[string]bool {
	t.Helper()
	rows, err := ix.DB.Query("SELECT path FROM documents")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		out[p] = true
	}
	return out
}

// waitFor polls until cond holds or the deadline passes. The watcher
// debounces by 300ms, so every assertion here is necessarily eventual.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestWatcherIndexesNewAndChangedFiles(t *testing.T) {
	ix, root := startWatcher(t)

	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("notes/watched.md", "# Watched\n\noriginal\n")
	waitFor(t, "the new file to be indexed", func() bool {
		return indexedPaths(t, ix)["notes/watched.md"]
	})

	// A change to an existing file must be picked up, not just its creation.
	write("notes/watched.md", "# Watched\n\nrewritten\n")
	waitFor(t, "the change to be reindexed", func() bool {
		var title string
		var sha string
		_ = ix.DB.QueryRow("SELECT title, sha256 FROM documents WHERE path = ?", "notes/watched.md").
			Scan(&title, &sha)
		f, err := ix.Vault.Read("notes/watched.md")
		return err == nil && sha == f.SHA256
	})
}

func TestWatcherRemovesDeletedFiles(t *testing.T) {
	ix, root := startWatcher(t)
	full := filepath.Join(root, "notes", "doomed.md")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# Doomed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the file to be indexed", func() bool {
		return indexedPaths(t, ix)["notes/doomed.md"]
	})

	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the deletion to reach the index", func() bool {
		return !indexedPaths(t, ix)["notes/doomed.md"]
	})
}

// TestWatcherHandlesAtomicRenameSaves: vim, and quire's own writer, save by
// writing a temp file and renaming it over the target. The event kinds that
// produces are not the ones a naive watcher expects, which is why
// applyChange trusts the filesystem rather than the event.
func TestWatcherHandlesAtomicRenameSaves(t *testing.T) {
	ix, root := startWatcher(t)
	target := filepath.Join(root, "notes", "atomic.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("# Atomic\n\nfirst\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the file to be indexed", func() bool {
		return indexedPaths(t, ix)["notes/atomic.md"]
	})

	tmp := filepath.Join(root, "notes", ".atomic.md.swp-like")
	if err := os.WriteFile(tmp, []byte("# Atomic\n\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, target); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "the renamed-over content to be indexed", func() bool {
		f, err := ix.Vault.Read("notes/atomic.md")
		if err != nil {
			return false
		}
		var sha string
		_ = ix.DB.QueryRow("SELECT sha256 FROM documents WHERE path = ?", "notes/atomic.md").Scan(&sha)
		return sha == f.SHA256
	})
}

// TestWatcherPicksUpDirectoriesCreatedLater: fsnotify is not recursive, so a
// directory that appears after startup has to be added to the watch set or
// everything filed under it is invisible until the next restart.
func TestWatcherPicksUpDirectoriesCreatedLater(t *testing.T) {
	ix, root := startWatcher(t)

	deep := filepath.Join(root, "projects", "apollo", "sub")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	// Give the create event time to register the new directories.
	time.Sleep(400 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(deep, "deep.md"), []byte("# Deep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "a file in a newly created directory to be indexed", func() bool {
		return indexedPaths(t, ix)["projects/apollo/sub/deep.md"]
	})
}

// TestWatcherIgnoresNonMarkdownAndHiddenFiles: editor swap files and quire's
// own state directory must not churn the index.
func TestWatcherIgnoresNonMarkdownAndHiddenFiles(t *testing.T) {
	ix, root := startWatcher(t)

	for rel, body := range map[string]string{
		"notes/image.png":    "PNGDATA",
		"notes/.hidden.md":   "# Hidden\n",
		"notes/.swap.md.swp": "swap",
		"notes/real.md":      "# Real\n",
	} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	waitFor(t, "the real document to be indexed", func() bool {
		return indexedPaths(t, ix)["notes/real.md"]
	})
	// By now the ignored writes have had at least as long to be noticed.
	paths := indexedPaths(t, ix)
	for _, rel := range []string{"notes/image.png", "notes/.hidden.md", "notes/.swap.md.swp"} {
		if paths[rel] {
			t.Errorf("%s should not be indexed", rel)
		}
	}
}
