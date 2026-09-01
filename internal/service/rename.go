// Rename: moving a document is a UI operation that keeps the link graph
// intact. Links that referenced the old filename/path are rewritten in the
// documents that hold them (each edited surgically, wikilink targets only);
// links by title or alias survive a move untouched and are left alone.
package service

import (
	"fmt"
	"path"
	"strings"

	"github.com/jclement/quire/internal/markdown"
	"github.com/jclement/quire/internal/vault"
)

// RenameResult reports what a rename touched.
type RenameResult struct {
	Document  Document `json:"document"`
	Rewritten []string `json:"rewritten"` // docs whose links were updated
}

// RenameDocument moves oldPath to newPath. With rewriteLinks, inbound
// wikilinks that referenced the old filename or path are re-pointed at the
// new filename. The set of affected documents is the old path's backlinks —
// callers can show those to the user before committing.
func (s *Service) RenameDocument(oldPath, newPath string, rewriteLinks bool) (RenameResult, error) {
	if err := vault.ValidatePath(newPath); err != nil {
		return RenameResult{}, err
	}
	if !strings.HasSuffix(newPath, ".md") {
		return RenameResult{}, fmt.Errorf("new path must end in .md")
	}
	if s.Vault.Exists(newPath) {
		return RenameResult{}, fmt.Errorf("%s already exists: %w", newPath, vault.ErrConflict)
	}
	old, err := s.Vault.Read(oldPath)
	if err != nil {
		return RenameResult{}, err
	}

	// Names that stop resolving once the file moves: the path- and
	// filename-shaped references, MINUS every name the document still
	// answers to afterwards (title, slug, aliases, the new filename). A file
	// created from its title has stem == slug(title), and links written as
	// [[that-name]] keep resolving via the title — rewriting them would be
	// churn.
	oldStem := strings.TrimSuffix(path.Base(oldPath), ".md")
	newStem := strings.TrimSuffix(path.Base(newPath), ".md")

	surviving := map[string]bool{
		strings.ToLower(newPath):                            true,
		strings.ToLower(strings.TrimSuffix(newPath, ".md")): true,
		strings.ToLower(newStem):                            true,
	}
	scanned := markdown.Scan(oldPath, old.Raw)
	if scanned.Title != "" {
		surviving[strings.ToLower(scanned.Title)] = true
		surviving[vault.Slugify(scanned.Title)] = true
	}
	if fm := vault.ParseFrontmatter(old.Raw); fm != nil {
		if aliases, ok := fm["aliases"].([]any); ok {
			for _, a := range aliases {
				if name, ok := a.(string); ok {
					surviving[strings.ToLower(strings.TrimSpace(name))] = true
				}
			}
		}
	}

	dying := map[string]bool{}
	for _, name := range []string{oldPath, strings.TrimSuffix(oldPath, ".md"), oldStem} {
		if lower := strings.ToLower(name); !surviving[lower] {
			dying[lower] = true
		}
	}

	var rewritten []string
	if rewriteLinks {
		backlinks, err := s.Index.Backlinks(oldPath)
		if err != nil {
			return RenameResult{}, err
		}
		for _, src := range backlinks {
			changed, err := s.rewriteLinksIn(src.Path, dying, newStem)
			if err != nil {
				return RenameResult{}, fmt.Errorf("rewriting links in %s: %w", src.Path, err)
			}
			if changed {
				rewritten = append(rewritten, src.Path)
			}
		}
	}

	// Move: write the new file, index it, then drop the old one. Ordered so
	// a crash mid-rename leaves both files rather than neither.
	if _, err := s.Vault.Write(newPath, old.Raw, ""); err != nil {
		return RenameResult{}, err
	}
	if _, err := s.Index.IndexFile(newPath); err != nil {
		return RenameResult{}, err
	}
	if err := s.Vault.Delete(oldPath); err != nil {
		return RenameResult{}, err
	}
	if err := s.Index.Remove(oldPath); err != nil {
		return RenameResult{}, err
	}

	doc, err := s.GetDocument(newPath)
	if err != nil {
		return RenameResult{}, err
	}
	if rewritten == nil {
		rewritten = []string{}
	}
	return RenameResult{Document: doc, Rewritten: rewritten}, nil
}

// rewriteLinksIn re-targets matching wikilinks in one document, preserving
// aliases and every other byte. Returns whether anything changed.
func (s *Service) rewriteLinksIn(docPath string, dying map[string]bool, newTarget string) (bool, error) {
	f, err := s.Vault.Read(docPath)
	if err != nil {
		return false, err
	}
	scanned := markdown.Scan(docPath, f.Raw)

	content := string(f.Raw)
	changed := false
	for _, link := range scanned.Links {
		if !dying[strings.ToLower(strings.TrimSpace(link.Raw))] {
			continue
		}
		oldRef := "[[" + link.Raw
		newRef := "[[" + newTarget
		if link.Display != "" {
			// Alias kept: [[old|Alias]] → [[new|Alias]].
			oldRef += "|" + link.Display
			newRef += "|" + link.Display
		} else if link.Raw != newTarget {
			// Preserve what the reader saw: the old name becomes the alias.
			newRef += "|" + link.Raw
		}
		oldRef += "]]"
		newRef += "]]"
		if strings.Contains(content, oldRef) {
			content = strings.ReplaceAll(content, oldRef, newRef)
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if _, err := s.UpdateDocument(docPath, content, f.SHA256); err != nil {
		return false, err
	}
	return true, nil
}
