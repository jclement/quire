package api

import (
	"net/http"
	"testing"

	"github.com/jclement/quire/internal/service"
)

// The two triage endpoints over HTTP: the grouped Waiting view, and filing
// a task as a note.
func TestWaitingGroupsAndTaskToNote(t *testing.T) {
	ts := newTestServer(t)
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{"type": "person", "title": "Frances Bagley"}, http.StatusCreated, nil)

	var waiting service.Task
	doJSON(t, "POST", ts.URL+"/api/v1/tasks", map[string]any{"text": "SOC evidence", "waiting_on": "Frances Bagley"}, http.StatusCreated, &waiting)
	var groups []service.WaitingGroup
	doJSON(t, "GET", ts.URL+"/api/v1/waiting", nil, http.StatusOK, &groups)
	if len(groups) != 1 || groups[0].Name != "Frances Bagley" || len(groups[0].Tasks) != 1 {
		t.Fatalf("groups = %+v", groups)
	}

	var feedback service.Task
	doJSON(t, "POST", ts.URL+"/api/v1/tasks", map[string]any{"text": "The editor toolbar wraps badly on a phone"}, http.StatusCreated, &feedback)
	var note service.Document
	doJSON(t, "POST", ts.URL+"/api/v1/notes/from-task", map[string]any{"id": feedback.ID, "title": "Toolbar on mobile"}, http.StatusCreated, &note)
	if note.Path != "notes/toolbar-on-mobile.md" || len(note.Backlinks) != 1 {
		t.Errorf("note = %+v", note.DocMeta)
	}
	doJSON(t, "POST", ts.URL+"/api/v1/notes/from-task", map[string]any{"id": "nope"}, http.StatusNotFound, nil)
}
