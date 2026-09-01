package gitback

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	if _, err := Start(ctx2, dir); err != nil {
		t.Fatalf("Start over existing repo: %v", err)
	}
}
