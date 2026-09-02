package api

import (
	"net/http"
	"testing"

	"github.com/jclement/quire/internal/service"
)

func TestAreaInheritanceThroughTheAPI(t *testing.T) {
	ts := newTestServer(t)
	var company, person, note service.Document
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{"type": "company", "title": "Globex", "markdown": "# Globex\n", "area": "work"}, http.StatusCreated, &company)
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{"type": "person", "title": "Hank Scorpio", "markdown": "# Hank Scorpio\n"}, http.StatusCreated, &person)
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{"type": "note", "title": "Volcano Plans", "markdown": "# Volcano Plans\n"}, http.StatusCreated, &note)
	doJSON(t, "POST", ts.URL+"/api/v1/link", map[string]any{"path": person.Path, "key": "company", "target": "Globex"}, http.StatusOK, nil)
	doJSON(t, "POST", ts.URL+"/api/v1/link", map[string]any{"path": note.Path, "key": "people", "target": "Hank Scorpio"}, http.StatusOK, nil)
	var got service.Document
	doJSON(t, "GET", ts.URL+"/api/v1/documents/"+person.Path, nil, http.StatusOK, &got)
	if got.Area != "work" || got.AreaFrom != company.Path {
		t.Errorf("person: area=%q from=%q", got.Area, got.AreaFrom)
	}
	doJSON(t, "GET", ts.URL+"/api/v1/documents/"+note.Path, nil, http.StatusOK, &got)
	if got.Area != "work" || got.AreaFrom != person.Path {
		t.Errorf("note: area=%q from=%q", got.Area, got.AreaFrom)
	}
}
