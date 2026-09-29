package api

import (
	"net/http"
	"testing"

	"github.com/jclement/quire/internal/service"
)

// TestToggleCarriesTheClickedState: the checkbox's state when it was
// clicked is the intent. A second click that arrives after the first has
// landed, from a view that had not refreshed yet, must not undo it.
func TestToggleCarriesTheClickedState(t *testing.T) {
	ts := newTestServer(t)

	var task service.Task
	doJSON(t, "POST", ts.URL+"/api/v1/tasks", map[string]string{"text": "Buy cake"}, http.StatusCreated, &task)

	for range 2 {
		var toggled service.Task
		doJSON(t, "POST", ts.URL+"/api/v1/tasks/"+task.ID+"/toggle",
			map[string]bool{"done": true}, http.StatusOK, &toggled)
		if !toggled.Done {
			t.Fatalf("toggle to done came back open: %+v", toggled)
		}
	}

	var logbook []service.Task
	doJSON(t, "GET", ts.URL+"/api/v1/tasks?view=logbook", nil, http.StatusOK, &logbook)
	if len(logbook) != 1 {
		t.Errorf("logbook = %+v", logbook)
	}
}
