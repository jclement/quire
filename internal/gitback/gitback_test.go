package gitback

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
)

func TestInitAndCommit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := Start(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Start must have initialized a repo and captured the initial snapshot.
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("no repo after Start: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("no initial commit: %v", err)
	}

	// A change plus an explicit commit produces a new head; a clean tree
	// commits nothing.
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("hello again\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.commit("quire: autosave"); err != nil {
		t.Fatal(err)
	}
	head2, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if head.Hash() == head2.Hash() {
		t.Errorf("commit after change did not advance HEAD")
	}
	if err := c.commit("quire: autosave"); err != nil {
		t.Fatal(err)
	}
	head3, _ := repo.Head()
	if head2.Hash() != head3.Hash() {
		t.Errorf("clean tree still produced a commit")
	}

	// Restarting over an existing repo must not error or re-init.
	cancel()
	c.Wait(5 * time.Second) // let the first committer's flush finish first
	ctx2, cancel2 := context.WithCancel(context.Background())
	second, err := Start(ctx2, dir)
	if err != nil {
		t.Fatalf("Start over existing repo: %v", err)
	}
	// Shut the second one down inside the test too: leaving a committer
	// running past the end races t.TempDir's cleanup, and its flush then
	// fails against a half-deleted worktree — a warning that looks like a
	// product bug and would hide a real one.
	cancel2()
	second.Wait(5 * time.Second)
}

// TestShutdownFlushesPendingEdits: "a shutdown never strands edits" is a
// data-safety claim — autocommit debounces for a full minute, so without the
// flush a quit shortly after writing would leave that writing uncommitted.
func TestShutdownFlushesPendingEdits(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c, err := Start(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}

	// Write and quit well inside the debounce window.
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("written just before quitting\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Poke()
	cancel()
	c.Wait(5 * time.Second)

	after, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if after.Hash() == before.Hash() {
		t.Fatal("shutdown stranded the edit: no commit was made")
	}
	// And the committed content is the edit, not the earlier version.
	commit, err := repo.CommitObject(after.Hash())
	if err != nil {
		t.Fatal(err)
	}
	file, err := commit.File("note.md")
	if err != nil {
		t.Fatal(err)
	}
	contents, err := file.Contents()
	if err != nil {
		t.Fatal(err)
	}
	if contents != "written just before quitting\n" {
		t.Errorf("committed %q", contents)
	}
}

// TestPokeNeverBlocks: Poke is called from the indexer's notify path, so a
// blocked Poke would stall indexing — the same hazard as the SSE broadcaster.
func TestPokeNeverBlocks(t *testing.T) {
	c := &Committer{dir: t.TempDir(), pokes: make(chan struct{}, 1), done: make(chan struct{})}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Nothing is draining pokes; the buffer holds one and the rest must
		// be dropped rather than block.
		for range 1000 {
			c.Poke()
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Poke blocked with nothing draining it — this would stall indexing")
	}
}

// TestWaitReturnsOnTimeout: Wait must not hang process exit if the flush
// never completes.
func TestWaitReturnsOnTimeout(t *testing.T) {
	c := &Committer{dir: t.TempDir(), pokes: make(chan struct{}, 1), done: make(chan struct{})}

	start := time.Now()
	c.Wait(100 * time.Millisecond) // done is never closed
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Wait took %s despite its timeout", elapsed)
	}
}
