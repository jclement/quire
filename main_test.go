package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOAuthConsentCanRedirectToTheClient: the consent form's approval is a
// POST answered by a redirect to the OAuth client's callback. Browsers apply
// CSP form-action to that redirect, so the app's form-action 'self' must
// not cover /oauth/ — this is exactly the bug where Approve did nothing.
func TestOAuthConsentCanRedirectToTheClient(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	get := func(path string) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Header().Get("Content-Security-Policy")
	}
	if csp := get("/oauth/authorize"); strings.Contains(csp, "form-action 'self'") || !strings.Contains(csp, "form-action *") {
		t.Errorf("consent page CSP must allow redirecting to the client: %q", csp)
	}
	if csp := get("/settings"); !strings.Contains(csp, "form-action 'self'") {
		t.Errorf("the SPA keeps form-action 'self': %q", csp)
	}
	if csp := get("/s/abc"); csp != "" {
		t.Errorf("share pages set their own policy, got %q", csp)
	}
}
