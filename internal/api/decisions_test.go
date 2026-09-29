package api

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/jclement/quire/internal/service"
)

func TestListDecisionsEndpoint(t *testing.T) {
	ts := newTestServer(t)
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{"type": "project", "title": "Apollo"}, http.StatusCreated, nil)
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{"type": "note", "title": "Standup",
		"markdown": "# Standup\n\n## Decisions\n\n- Freeze [[Apollo]] scope\n- Lunch at noon\n"}, http.StatusCreated, nil)

	var got []service.Decision
	doJSON(t, "GET", ts.URL+"/api/v1/decisions?entity="+url.QueryEscape("projects/apollo.md"), nil, http.StatusOK, &got)
	if len(got) != 1 || got[0].Text != "Freeze [[Apollo]] scope" || len(got[0].Entities) != 1 {
		t.Errorf("Apollo's decisions = %+v", got)
	}
	doJSON(t, "GET", ts.URL+"/api/v1/decisions?q=lunch", nil, http.StatusOK, &got)
	if len(got) != 1 || got[0].Text != "Lunch at noon" {
		t.Errorf("text filter = %+v", got)
	}
	doJSON(t, "GET", ts.URL+"/api/v1/decisions?entity=Nobody", nil, http.StatusBadRequest, nil)
}
