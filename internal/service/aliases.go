// Aliases: the other names a document answers to. Wikilink resolution
// already reads `aliases:` (index.insertDocNames); this is the write side,
// and the discovery that feeds it — for each name the vault links to but
// never wrote up, which existing person or company it probably means.
package service

import (
	"fmt"
	"strings"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/vault"
)

// aliasesKey is the frontmatter key Obsidian and quire both resolve.
const aliasesKey = "aliases"

// likelyMatches turns the index's ranking into the wire shape; never nil,
// so the client can map over it without a guard.
func likelyMatches(name string, candidates []index.MatchCandidate) []LikelyMatch {
	ranked := index.RankMatches(name, candidates)
	out := make([]LikelyMatch, 0, len(ranked))
	for _, m := range ranked {
		out = append(out, LikelyMatch{Path: m.Path, Title: m.Title, Type: m.Type, Reason: string(m.Reason)})
	}
	return out
}

// AddAlias appends alias to the document's `aliases:` list, keeping every
// alias already there, so links written as that name resolve to it. Adding
// one the document already answers to (case-insensitively) succeeds without
// touching the file.
func (s *Service) AddAlias(path, alias string) (Document, error) {
	alias = strings.TrimSpace(alias)
	if err := validAlias(alias); err != nil {
		return Document{}, err
	}
	f, err := s.Vault.Read(path)
	if err != nil {
		return Document{}, err
	}
	existing := stringsFromFrontmatter(vault.ParseFrontmatter(f.Raw), aliasesKey)
	list := make([]any, 0, len(existing)+1)
	for _, a := range existing {
		if strings.EqualFold(strings.TrimSpace(a), alias) {
			return s.GetDocument(path)
		}
		list = append(list, a)
	}
	list = append(list, alias)
	// CAS against the file just read: a concurrent edit between the read
	// and the write fails rather than dropping an alias someone else added.
	return s.SetFrontmatter(path, map[string]any{aliasesKey: list}, f.SHA256)
}

// validAlias rejects what could never match a link target: a wikilink's
// target stops at "|", "#" and "^", and a newline would break the YAML.
func validAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("%w: an alias needs some text", ErrValidation)
	}
	if strings.ContainsAny(alias, "\n\r|#^[]") {
		return fmt.Errorf("%w: alias %q cannot contain [ ] | # ^ or a line break", ErrValidation, alias)
	}
	return nil
}
