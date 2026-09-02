package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jclement/quire/internal/semantic"
	"github.com/jclement/quire/internal/semantic/semantictest"
	"github.com/jclement/quire/internal/service"
)

func TestSemanticSearchOffByDefault(t *testing.T) {
	ts := newTestServer(t)
	var health service.Health
	doJSON(t, "GET", ts.URL+"/api/v1/health", nil, http.StatusOK, &health)
	if health.SemanticSearch {
		t.Error("health should report semantic search off")
	}
	res, err := http.Get(ts.URL + "/api/v1/search?q=anything&mode=semantic")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("semantic search while off = %d, want 400", res.StatusCode)
	}
	res, _ = http.Get(ts.URL + "/api/v1/related?path=notes/x.md")
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("related while off = %d, want 400", res.StatusCode)
	}
	var status service.SemanticStatus
	doJSON(t, "GET", ts.URL+"/api/v1/semantic/status", nil, http.StatusOK, &status)
	if status.Enabled {
		t.Error("status should say disabled")
	}
}

func TestSemanticSearchEndToEnd(t *testing.T) {
	fake, _ := semantictest.Server(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := newTestServerWith(t, func(svc *service.Service) {
		client := semantic.NewClient(fake.URL+"/v1", "test-key", "")
		embedder, err := semantic.Start(ctx, svc.Index.DB, svc.Vault, client)
		if err != nil {
			t.Fatal(err)
		}
		svc.Semantic = embedder
		svc.Index.Notify = embedder.Notify
	})

	var health service.Health
	doJSON(t, "GET", ts.URL+"/api/v1/health", nil, http.StatusOK, &health)
	if !health.SemanticSearch {
		t.Fatal("health should report semantic search on")
	}

	// Documents created through the API get embedded via the index hook.
	var created service.Document
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{
		"type": "note", "title": "Cluster rollout",
		"markdown": "# Cluster rollout\n\nkubernetes cluster rollout after the ingress upgrade\n",
	}, http.StatusCreated, &created)
	doJSON(t, "POST", ts.URL+"/api/v1/documents", map[string]any{
		"type": "note", "title": "Lunch", "markdown": "# Lunch\n\ntacos with the team\n",
	}, http.StatusCreated, nil)

	deadline := time.Now().Add(10 * time.Second)
	var status service.SemanticStatus
	for time.Now().Before(deadline) {
		doJSON(t, "GET", ts.URL+"/api/v1/semantic/status", nil, http.StatusOK, &status)
		if status.Documents == 2 && status.Pending == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status.Documents != 2 || !status.Enabled || status.Model != semantic.DefaultModel {
		t.Fatalf("status = %+v", status)
	}

	var hits []service.SearchResult
	doJSON(t, "GET", ts.URL+"/api/v1/search?mode=semantic&q=kubernetes+rollout", nil, http.StatusOK, &hits)
	if len(hits) != 2 || hits[0].Path != created.Path || hits[0].Score <= hits[1].Score {
		t.Errorf("semantic hits = %+v", hits)
	}
	if hits[0].Title != "Cluster rollout" || hits[0].Type != "note" {
		t.Errorf("hit should carry index metadata: %+v", hits[0])
	}

	var related []service.SearchResult
	doJSON(t, "GET", ts.URL+"/api/v1/related?path="+created.Path, nil, http.StatusOK, &related)
	if len(related) != 1 || related[0].Title != "Lunch" {
		t.Errorf("related = %+v", related)
	}
}
