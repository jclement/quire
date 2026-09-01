// Filesystem watcher: fsnotify over the vault tree, debounced, drained by a
// single goroutine. External edits (vim, git pull) land in the index within
// ~a second; quire's own writes are naturally skipped because IndexFile
// compares content hashes before doing any work.
package index

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/jclement/quire/internal/vault"
)

// debounceWindow batches rapid event bursts (editor save dances, git pull
// touching hundreds of files) into one indexing pass per file.
const debounceWindow = 300 * time.Millisecond

// Watch blocks, watching the vault until ctx is cancelled. Call in a
// goroutine after the initial FullScan.
func (ix *Index) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	// fsnotify is non-recursive: watch every directory, and add new ones as
	// they appear (create events for dirs).
	if err := watchTree(w, ix.Vault.Dir); err != nil {
		return err
	}

	pending := map[string]struct{}{}
	timer := time.NewTimer(time.Hour)
	timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
				if ev.Has(fsnotify.Create) {
					if err := watchTree(w, ev.Name); err != nil {
						slog.Warn("watching new directory", "dir", ev.Name, "err", err)
					}
				}
				continue
			}
			rel, ok := ix.relMarkdownPath(ev.Name)
			if !ok {
				continue
			}
			pending[rel] = struct{}{}
			timer.Reset(debounceWindow)

		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			slog.Warn("watcher error", "err", err)

		case <-timer.C:
			for rel := range pending {
				ix.applyChange(rel)
			}
			pending = map[string]struct{}{}
		}
	}
}

// applyChange indexes or removes one path depending on whether it still
// exists — event *kinds* are unreliable across editors (vim renames, atomic
// saves), so the filesystem state is what we trust.
func (ix *Index) applyChange(rel string) {
	_, err := ix.Vault.Read(rel)
	switch {
	case err == nil:
		if _, err := ix.IndexFile(rel); err != nil {
			slog.Warn("indexing changed file", "path", rel, "err", err)
		}
	case errors.Is(err, vault.ErrNotFound):
		if err := ix.Remove(rel); err != nil {
			slog.Warn("removing deleted file from index", "path", rel, "err", err)
		}
	default:
		slog.Warn("reading changed file", "path", rel, "err", err)
	}
}

// relMarkdownPath converts an absolute fsnotify path to a vault-relative
// markdown path, filtering out non-markdown and hidden files (editor swap
// files, .quire, temp writes).
func (ix *Index) relMarkdownPath(abs string) (string, bool) {
	rel, err := filepath.Rel(ix.Vault.Dir, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasSuffix(rel, ".md") || vault.ValidatePath(rel) != nil {
		return "", false
	}
	return rel, true
}

func watchTree(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != root {
			return filepath.SkipDir
		}
		return w.Add(p)
	})
}
