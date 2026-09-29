// Aliases: the write side of "[[Frances]] probably means Frances Bagley".
// The Unwritten list suggests the match; this adds the name to that
// document's aliases so every link written that way resolves.
package api

import "net/http"

// handleAddAlias appends one alias to a document's `aliases:` frontmatter.
// Body {path, alias}; idempotent, and responds with the rewritten document.
func (s *Server) handleAddAlias(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string `json:"path"`
		Alias string `json:"alias"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	doc, err := s.Service.AddAlias(body.Path, body.Alias)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, doc)
}
