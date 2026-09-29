// The decision log: GET /api/v1/decisions, the /decisions page and the
// Decisions section of an entity page's rail.
package api

import (
	"net/http"
	"strconv"

	"github.com/jclement/quire/internal/service"
)

// handleListDecisions: ?q= text, ?entity= path or name, ?area=, ?limit=.
func (s *Server) handleListDecisions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	decisions, err := s.Service.ListDecisions(service.DecisionQuery{
		Text: q.Get("q"), Entity: q.Get("entity"), Area: q.Get("area"), Limit: limit,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, decisions)
}
