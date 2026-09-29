package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jclement/quire/internal/settings"
)

func TestWorkItemInstructions(t *testing.T) {
	svc := newScopeTestService(t)
	if got := workItemInstructions(svc); got != "" {
		t.Errorf("no settings store: %q, want nothing", got)
	}
	svc.Settings = settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if got := workItemInstructions(svc); got != "" {
		t.Errorf("feature off: %q, want nothing", got)
	}
	const ado = "https://dev.azure.com/barreleye/Barreleye/_workitems/edit/{id}"
	if err := svc.SetWorkItemURL(ado); err != nil {
		t.Fatal(err)
	}
	if got := workItemInstructions(svc); !strings.Contains(got, ado) || !strings.Contains(got, "AB#1234") {
		t.Errorf("instructions = %q", got)
	}
}
