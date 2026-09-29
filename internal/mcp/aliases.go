// add_alias: settling a dangling first name against the person it means.
// Kept beside, not inside, the main tool table so the name-resolution
// tools read as one unit.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jclement/quire/internal/auth"
	"github.com/jclement/quire/internal/service"
)

// registerAliasTools adds the alias tools a caller's scopes allow.
func registerAliasTools(s *sdk.Server, t *tools, allows func(string) bool) {
	if !allows(auth.ScopeWrite) {
		return
	}
	// A dedicated tool rather than set_frontmatter: that one replaces the
	// whole value, so an agent would have to read the aliases, merge, and
	// write back — and would drop one added in between.
	sdk.AddTool(s, &sdk.Tool{Name: "add_alias", Annotations: idempotent,
		Description: "Add another name a document answers to (its frontmatter aliases), keeping the ones already there, so every [[link]] written that way resolves to it. The fix for a dangling first name: when list_unwritten shows \"Frances\" with a likely match of people/frances-bagley.md, add_alias(path, \"Frances\") instead of creating a duplicate person. Only do it when the match is clear — two likely matches means ask the owner which one. Idempotent."},
		t.addAlias)
}

type addAliasIn struct {
	Path  string `json:"path" jsonschema:"vault-relative path of the document that should answer to the name, e.g. people/frances-bagley.md"`
	Alias string `json:"alias" jsonschema:"the other name, exactly as links write it, e.g. Frances"`
}

func (t *tools) addAlias(_ context.Context, _ *sdk.CallToolRequest, in addAliasIn) (*sdk.CallToolResult, service.Document, error) {
	doc, err := t.svc.AddAlias(in.Path, in.Alias)
	t.record("add_alias", in.Path, in.Alias, err)
	return nil, doc, err
}
