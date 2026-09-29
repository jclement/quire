// list_decisions: the decision log for agents — "what did we decide about
// pricing", "what has been settled on Apollo", the input to a status update.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/service"
)

// registerDecisionTools adds the decision-log tools a caller's scopes allow.
func registerDecisionTools(s *sdk.Server, t *tools, allows func(string) bool) {
	if !allows(auth.ScopeRead) {
		return
	}
	sdk.AddTool(s, &sdk.Tool{Name: "list_decisions", Annotations: readOnly,
		Description: "The decision log, newest first: every bullet under a \"Decisions\" heading in any note (meeting notes have one) plus every decision record (a document tagged decision), each with its date, source document and the people/companies/projects it is about. Use it for 'what did we decide about X', before re-opening a settled question, or to write a status update. entity narrows to one person, company or project (name or path); query filters by words; area narrows to an area. To record a new decision, append a bullet under the meeting's Decisions section with append_to_document (section: Decisions)."},
		t.listDecisions)
}

type listDecisionsIn struct {
	Query  string `json:"query,omitempty" jsonschema:"words that must all appear in the decision or its source document's title"`
	Entity string `json:"entity,omitempty" jsonschema:"a person, company or project by name (Sarah Chen) or path — decisions about it or made in its page"`
	Area   string `json:"area,omitempty" jsonschema:"area to narrow to (e.g. work, personal), or none for unclassified; omit for all"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max results (default 50)"`
}

type decisionsOut struct {
	Decisions []service.Decision `json:"decisions"`
}

func (t *tools) listDecisions(_ context.Context, _ *sdk.CallToolRequest, in listDecisionsIn) (*sdk.CallToolResult, decisionsOut, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	decisions, err := t.svc.ListDecisions(service.DecisionQuery{Text: in.Query, Entity: in.Entity, Area: in.Area, Limit: limit})
	if err != nil {
		return nil, decisionsOut{}, err
	}
	return nil, decisionsOut{Decisions: decisions}, nil
}
