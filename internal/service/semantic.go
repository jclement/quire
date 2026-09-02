// Semantic search and related documents, over the embedding pipeline in
// internal/semantic. Everything here answers "off" gracefully: the API and
// MCP surfaces exist only when the pipeline does.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jclement/quire/internal/semantic"
	"github.com/jclement/quire/internal/vault"
)

// ErrSemanticOff is returned when no API key is configured.
var ErrSemanticOff = errors.New("semantic search is not enabled (set QUIRE_OPENAI_API_KEY)")

// SemanticEnabled reports whether the pipeline is running.
func (s *Service) SemanticEnabled() bool { return s.Semantic != nil }

// SemanticStatus is the Settings payload; a disabled pipeline reports so.
func (s *Service) SemanticStatus() SemanticStatus {
	if s.Semantic == nil {
		return SemanticStatus{}
	}
	st := s.Semantic.Status()
	return SemanticStatus{Enabled: true, Model: st.Model, Documents: st.Documents, Pending: st.Pending, LastError: st.LastError}
}

// SemanticSearch ranks documents by meaning. The area filter is applied
// after ranking, from the index, so an out-of-area hit never surfaces.
func (s *Service) SemanticSearch(ctx context.Context, query string, limit int, area string) ([]SearchResult, error) {
	if s.Semantic == nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, ErrSemanticOff)
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	// Over-fetch so area filtering still fills the page.
	hits, err := s.Semantic.Search(ctx, query, limit*3)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}
	return s.hitsToResults(hits, area, limit), nil
}

// RelatedDocuments are the nearest other documents to this one, from its
// own stored vectors — no API call. Empty until the document is embedded.
func (s *Service) RelatedDocuments(path string, limit int) ([]SearchResult, error) {
	if s.Semantic == nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, ErrSemanticOff)
	}
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	f, err := s.Vault.Read(path)
	if err != nil {
		return nil, err
	}
	if bodyChars(f.Raw) < minRelatedBodyChars {
		return []SearchResult{}, nil
	}
	return s.hitsToResults(s.Semantic.Related(path, limit*2), "", limit), nil
}

// minRelatedBodyChars: below this much prose (frontmatter and the H1 aside)
// a note is a title, and every title is a little like every other.
const minRelatedBodyChars = 80

// bodyChars counts the characters of a document's body after frontmatter
// and its first heading are removed.
func bodyChars(raw []byte) int {
	body := raw
	if _, rest, ok := vault.SplitFrontmatter(raw); ok {
		body = rest
	}
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "# ") {
		if nl := strings.IndexByte(text, '\n'); nl >= 0 {
			text = text[nl+1:]
		} else {
			text = ""
		}
	}
	return len(strings.TrimSpace(text))
}

func (s *Service) hitsToResults(hits []semantic.Hit, area string, limit int) []SearchResult {
	out := make([]SearchResult, 0, len(hits))
	for _, h := range hits {
		row, err := s.Index.GetDocMeta(h.Path)
		if err != nil {
			continue // embedded but no longer indexed; the sweep will drop it
		}
		if area != "" && row.Type != "daily" {
			if area == "none" && row.Area != "" || area != "none" && row.Area != area {
				continue
			}
		}
		out = append(out, SearchResult{
			Path: row.Path, Type: string(row.Type), Title: row.Title,
			Snippet: h.Heading, Score: float64(h.Score),
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}
