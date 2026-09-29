package auth

import (
	"net/http/httptest"
	"testing"
)

// Turning a task into a note creates a document, so it must need the write
// scope — which is why it is routed under /notes rather than /tasks, where
// the narrower tasks scope would let a todo-only token create documents.
func TestRequiredScope(t *testing.T) {
	cases := []struct{ method, path, want string }{
		{"GET", "/api/v1/waiting", ScopeRead},
		{"POST", "/api/v1/tasks/abc/toggle", ScopeTasks},
		{"PATCH", "/api/v1/tasks/abc", ScopeTasks},
		{"POST", "/api/v1/notes/from-task", ScopeWrite},
	}
	for _, c := range cases {
		if got := requiredScope(httptest.NewRequest(c.method, c.path, nil)); got != c.want {
			t.Errorf("%s %s needs %q, want %q", c.method, c.path, got, c.want)
		}
	}
}
