// The decision log over REST and MCP: shaping index rows, and letting an
// entity be named the way people name them ("Project Apollo") as well as by
// path.
package service

import (
	"fmt"
	"strings"

	"github.com/jclement/quire/internal/index"
)

// DecisionQuery narrows the log; zero values mean no filter.
type DecisionQuery struct {
	// Text: every word must appear in the decision or its source's title.
	Text string
	// Entity is a vault path or a name ("Sarah Chen"): decisions about it,
	// or made in its own page.
	Entity string
	Area   string
	Limit  int
}

// ListDecisions returns decisions newest first.
func (s *Service) ListDecisions(q DecisionQuery) ([]Decision, error) {
	entity := strings.TrimSpace(q.Entity)
	if entity != "" && !strings.HasSuffix(entity, ".md") {
		resolved := s.Index.ResolveLink(entity)
		if resolved == "" {
			return nil, fmt.Errorf("%w: no document is called %q — search for it first", ErrValidation, entity)
		}
		entity = resolved
	}
	rows, err := s.Index.Decisions(index.DecisionFilter{Text: q.Text, Entity: entity, Area: q.Area, Limit: q.Limit})
	if err != nil {
		return nil, err
	}
	out := make([]Decision, 0, len(rows))
	for _, r := range rows {
		entities := make([]EntityRef, 0, len(r.Entities))
		for _, e := range r.Entities {
			entities = append(entities, EntityRef{Path: e.Path, Title: e.Title, Type: e.Type})
		}
		out = append(out, Decision{
			Kind: r.Kind, Text: r.Text, Date: r.Date,
			Path: r.DocPath, Title: r.DocTitle, Type: r.DocType, Line: r.Line, Area: r.Area,
			Entities: entities,
		})
	}
	return out, nil
}
