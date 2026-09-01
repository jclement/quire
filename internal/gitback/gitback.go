// Package gitback keeps the vault in a local git repository: auto-init on
// first run, debounced auto-commit after changes settle. Commit-only by
// design — quire never merges, never pushes, never touches remotes (add one
// yourself and push whenever you like; quire won't interfere). This is the
// vault's time machine, not a sync system.
package gitback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// debounceWindow: a burst of edits (a writing session, a git-pull-like sweep
// of watcher events) becomes one commit once the vault has been quiet this
// long.
const debounceWindow = 60 * time.Second

// Committer owns the repo and the debounce loop.
type Committer struct {
	dir   string
	pokes chan struct{}
	done  chan struct{} // closed after the shutdown flush completes
}

// Start ensures dir is a git repository (initializing and making a first
// commit if not) and returns a running Committer. Call Poke after every
// vault change; Watch runs until ctx is done, then flushes a final commit.
func Start(ctx context.Context, dir string) (*Committer, error) {
	_, err := git.PlainOpen(dir)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		if _, err := git.PlainInit(dir, false); err != nil {
			return nil, fmt.Errorf("git init %s: %w", dir, err)
		}
		slog.Info("vault git repository initialized", "dir", dir)
	} else if err != nil {
		return nil, fmt.Errorf("opening vault repo: %w", err)
	}

	c := &Committer{dir: dir, pokes: make(chan struct{}, 1), done: make(chan struct{})}
	if err := c.commit("quire: initial snapshot"); err != nil {
		slog.Warn("initial vault commit", "err", err)
	}
	go c.loop(ctx)
	return c, nil
}

// Wait blocks until the shutdown flush has run (or the timeout passes) —
// callers use it to keep process exit from racing the final commit.
func (c *Committer) Wait(timeout time.Duration) {
	select {
	case <-c.done:
	case <-time.After(timeout):
	}
}

// Poke signals that the vault changed; coalesces freely.
func (c *Committer) Poke() {
	select {
	case c.pokes <- struct{}{}:
	default:
	}
}

func (c *Committer) loop(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case <-ctx.Done():
			// Flush whatever is pending so a shutdown never strands edits.
			if err := c.commit("quire: autosave"); err != nil {
				slog.Warn("final vault commit", "err", err)
			}
			close(c.done)
			return
		case <-c.pokes:
			timer.Reset(debounceWindow)
		case <-timer.C:
			if err := c.commit("quire: autosave"); err != nil {
				slog.Warn("vault autocommit", "err", err)
			}
		}
	}
}

// commit stages everything and commits if anything actually changed.
func (c *Committer) commit(message string) error {
	repo, err := git.PlainOpen(c.dir)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("staging: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return err
	}
	if status.IsClean() {
		return nil
	}
	_, err = wt.Commit(fmt.Sprintf("%s (%d file(s))", message, len(status)), &git.CommitOptions{
		Author: &object.Signature{Name: "quire", Email: "quire@localhost", When: time.Now()},
	})
	if err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	slog.Debug("vault committed", "files", len(status))
	return nil
}
