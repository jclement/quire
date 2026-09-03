package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/settings"
)

func TestTimezoneSetting(t *testing.T) {
	ts := newTestServerWith(t, func(svc *service.Service) {
		// The harness pins the clock; this test wants the real one, read in
		// the configured zone, so borrow New's clock bound to these settings.
		svc.Settings = settings.Open(filepath.Join(t.TempDir(), "settings.json"))
		clock := service.New(svc.Vault, svc.Index)
		clock.Settings = svc.Settings
		svc.Now = clock.Now
	})
	var info service.TimezoneInfo
	doJSON(t, "GET", ts.URL+"/api/v1/timezone", nil, http.StatusOK, &info)
	if info.Timezone != "" || info.Effective == "" {
		t.Errorf("default = %+v", info)
	}
	doJSON(t, "PUT", ts.URL+"/api/v1/timezone", map[string]any{"timezone": "Pacific/Auckland"}, http.StatusOK, &info)
	if info.Timezone != "Pacific/Auckland" || info.Effective != "Pacific/Auckland" {
		t.Errorf("after set = %+v", info)
	}
	if len(info.Now) < 6 || (info.Now[len(info.Now)-6:] != "+12:00" && info.Now[len(info.Now)-6:] != "+13:00") {
		t.Errorf("now should be reckoned in Auckland: %s", info.Now)
	}
	doJSON(t, "PUT", ts.URL+"/api/v1/timezone", map[string]any{"timezone": "Mars/Olympus"}, http.StatusBadRequest, nil)
}
