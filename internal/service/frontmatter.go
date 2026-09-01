// Frontmatter editing: the API behind entity linking (put a person in a
// company, attendees on a meeting) and any properties UI. Edits go through
// vault's key-preserving writer, so setting `company:` rewrites exactly that
// line and leaves key order, comments and quoting alone.
package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jclement/quire/internal/vault"
)

// SetFrontmatter applies a set of top-level frontmatter changes to a
// document. A JSON null removes the key. Strings that look like wikilinks
// are quoted so the YAML stays valid; lists are written inline
// (`people: ["[[Sarah Chen]]"]`) because that is what a human writing this
// file by hand would type.
func (s *Service) SetFrontmatter(path string, values map[string]any, baseSHA string) (Document, error) {
	if len(values) == 0 {
		return Document{}, fmt.Errorf("no frontmatter changes given")
	}
	f, err := s.Vault.Read(path)
	if err != nil {
		return Document{}, err
	}
	if baseSHA != "" && f.SHA256 != baseSHA {
		return Document{}, fmt.Errorf("%s: %w", path, vault.ErrConflict)
	}

	content := f.Raw
	for key, value := range values {
		if err := validFrontmatterKey(key); err != nil {
			return Document{}, err
		}
		if value == nil {
			content = vault.RemoveFrontmatterKey(content, key)
			continue
		}
		encoded, err := encodeYAMLValue(value)
		if err != nil {
			return Document{}, fmt.Errorf("frontmatter %q: %w", key, err)
		}
		content = vault.SetFrontmatterKey(content, key, encoded)
	}
	return s.UpdateDocument(path, string(content), f.SHA256)
}

// LinkEntity is the convenience behind "add this person to that company":
// it appends a wikilink to a list-valued key (people, projects…) without
// duplicating an entry that is already there.
func (s *Service) LinkEntity(path, key, target string) (Document, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return Document{}, fmt.Errorf("link target is required")
	}
	f, err := s.Vault.Read(path)
	if err != nil {
		return Document{}, err
	}
	existing := stringsFromFrontmatter(vault.ParseFrontmatter(f.Raw), key)
	for _, item := range existing {
		if strings.EqualFold(unwrapWikilink(item), unwrapWikilink(target)) {
			// Already linked — succeed without rewriting the file.
			return s.GetDocument(path)
		}
	}
	updated := append(existing, wrapWikilink(target))
	list := make([]any, 0, len(updated))
	for _, item := range updated {
		list = append(list, item)
	}
	return s.SetFrontmatter(path, map[string]any{key: list}, "")
}

// UnlinkEntity removes a wikilink from a list-valued frontmatter key.
func (s *Service) UnlinkEntity(path, key, target string) (Document, error) {
	f, err := s.Vault.Read(path)
	if err != nil {
		return Document{}, err
	}
	var kept []any
	for _, item := range stringsFromFrontmatter(vault.ParseFrontmatter(f.Raw), key) {
		if !strings.EqualFold(unwrapWikilink(item), unwrapWikilink(target)) {
			kept = append(kept, item)
		}
	}
	if len(kept) == 0 {
		return s.SetFrontmatter(path, map[string]any{key: nil}, "")
	}
	return s.SetFrontmatter(path, map[string]any{key: kept}, "")
}

// ---- encoding helpers ----

// validFrontmatterKey keeps edits to simple top-level keys; anything with
// YAML structure in it would break the line-oriented writer.
func validFrontmatterKey(key string) error {
	if key == "" || strings.ContainsAny(key, ": \t\n\"'#[]{}") {
		return fmt.Errorf("invalid frontmatter key %q", key)
	}
	return nil
}

// encodeYAMLValue renders a JSON value as the YAML a person would write.
func encodeYAMLValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return quoteYAMLString(v), nil
	case bool, float64, int:
		return fmt.Sprint(v), nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("list values must be strings")
			}
			parts = append(parts, quoteYAMLString(str))
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	default:
		// Anything else (nested objects) is out of scope for a line-oriented
		// editor; the caller should write the document directly.
		raw, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("unsupported value")
		}
		return "", fmt.Errorf("unsupported frontmatter value %s", raw)
	}
}

// quoteYAMLString quotes when YAML would otherwise misread the value —
// wikilinks start with `[`, which YAML reads as a flow sequence.
func quoteYAMLString(s string) string {
	if s == "" {
		return `""`
	}
	needsQuotes := strings.ContainsAny(s, `:[]{}#,&*!|>'"%@`+"`") ||
		strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ")
	if !needsQuotes {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func wrapWikilink(target string) string {
	if strings.HasPrefix(target, "[[") {
		return target
	}
	return "[[" + target + "]]"
}

func unwrapWikilink(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[[")
	s = strings.TrimSuffix(s, "]]")
	if target, _, found := strings.Cut(s, "|"); found {
		return strings.TrimSpace(target)
	}
	return s
}

// stringsFromFrontmatter reads a frontmatter value as a list of strings,
// tolerating a bare scalar (`company: "[[Acme]]"`).
func stringsFromFrontmatter(fm map[string]any, key string) []string {
	if fm == nil {
		return nil
	}
	switch v := fm[key].(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}
