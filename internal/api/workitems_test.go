package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/settings"
)

func TestWorkItemSetting(t *testing.T) {
	ts := newTestServerWith(t, func(svc *service.Service) {
		svc.Settings = settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	})
	var got service.WorkItemSettings
	doJSON(t, "GET", ts.URL+"/api/v1/work-items", nil, http.StatusOK, &got)
	if got.URLTemplate != "" {
		t.Errorf("default = %+v, want off", got)
	}
	const ado = "https://dev.azure.com/barreleye/Barreleye/_workitems/edit/{id}"
	doJSON(t, "PUT", ts.URL+"/api/v1/work-items", map[string]any{"url_template": "  " + ado + " "}, http.StatusOK, &got)
	if got.URLTemplate != ado {
		t.Errorf("after set = %+v", got)
	}
	doJSON(t, "PUT", ts.URL+"/api/v1/work-items", map[string]any{"url_template": "https://dev.azure.com/no-placeholder"}, http.StatusBadRequest, nil)
	doJSON(t, "GET", ts.URL+"/api/v1/work-items", nil, http.StatusOK, &got)
	if got.URLTemplate != ado {
		t.Errorf("a rejected value replaced the setting: %+v", got)
	}
	doJSON(t, "PUT", ts.URL+"/api/v1/work-items", map[string]any{"url_template": ""}, http.StatusOK, &got)
	if got.URLTemplate != "" {
		t.Errorf("clearing = %+v", got)
	}
}
