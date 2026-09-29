package api

import (
	"net/http"
	"testing"

	"github.com/jclement/quire/internal/service"
)

func TestAddAliasEndpoint(t *testing.T) {
	ts := newTestServer(t)
	var person service.Document
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{"type": "person", "title": "David Piepgrass"}, http.StatusCreated, &person)
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{"type": "note", "title": "Standup", "markdown": "# Standup\n\n[[David]] is out.\n"}, http.StatusCreated, nil)

	var unwritten []service.Unwritten
	doJSON(t, "GET", ts.URL+"/api/v1/unwritten", nil, http.StatusOK, &unwritten)
	if len(unwritten) != 1 || len(unwritten[0].LikelyMatches) != 1 || unwritten[0].LikelyMatches[0].Path != person.Path {
		t.Fatalf("unwritten = %+v", unwritten)
	}

	var doc service.Document
	doJSON(t, "POST", ts.URL+"/api/v1/aliases", map[string]any{"path": person.Path, "alias": "David"}, http.StatusOK, &doc)
	if aliases, _ := doc.Frontmatter["aliases"].([]any); len(aliases) != 1 || aliases[0] != "David" {
		t.Errorf("aliases = %v", doc.Frontmatter["aliases"])
	}
	doJSON(t, "GET", ts.URL+"/api/v1/unwritten", nil, http.StatusOK, &unwritten)
	if len(unwritten) != 0 {
		t.Errorf("still unwritten after alias: %+v", unwritten)
	}

	doJSON(t, "POST", ts.URL+"/api/v1/aliases", map[string]any{"path": person.Path, "alias": "Dave|D"}, http.StatusBadRequest, nil)
	doJSON(t, "POST", ts.URL+"/api/v1/aliases", map[string]any{"path": "people/nobody.md", "alias": "Nobody"}, http.StatusNotFound, nil)
}
