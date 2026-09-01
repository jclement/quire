package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jclement/quire/internal/index"
	"github.com/jclement/quire/internal/service"
	"github.com/jclement/quire/internal/vault"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	v, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	svc := service.New(v, &index.Index{DB: db, Vault: v})
	svc.Now = func() time.Time { return time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local) }

	s := &Server{Service: svc, Events: NewBroadcaster(), Version: "test"}
	mux := http.NewServeMux()
	s.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// doJSON issues a request and decodes the envelope's data into out.
func doJSON(t *testing.T, method, url string, body any, wantStatus int, out any) {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reqBody = bytes.NewReader(raw)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != wantStatus {
		var raw bytes.Buffer
		_, _ = raw.ReadFrom(res.Body)
		t.Fatalf("%s %s = %d, want %d — body: %s", method, url, res.StatusCode, wantStatus, raw.String())
	}
	if out != nil {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDocumentLifecycle(t *testing.T) {
	ts := newTestServer(t)

	var doc service.Document
	doJSON(t, "POST", ts.URL+"/api/v1/documents",
		map[string]string{"type": "person", "title": "Sarah Chen"}, http.StatusCreated, &doc)
	if doc.Path != "people/sarah-chen.md" {
		t.Fatalf("path = %q", doc.Path)
	}

	// Update with correct base succeeds; stale base conflicts.
	var updated service.Document
	doJSON(t, "PUT", ts.URL+"/api/v1/documents/"+doc.Path,
		map[string]string{"markdown": "# Sarah Chen\n\nVP Infra at [[Acme]].\n", "base_sha256": doc.SHA256},
		http.StatusOK, &updated)
	doJSON(t, "PUT", ts.URL+"/api/v1/documents/"+doc.Path,
		map[string]string{"markdown": "clobber", "base_sha256": doc.SHA256},
		http.StatusConflict, nil)

	var got service.Document
	doJSON(t, "GET", ts.URL+"/api/v1/documents/"+doc.Path, nil, http.StatusOK, &got)
	if !strings.Contains(got.Markdown, "VP Infra") {
		t.Errorf("markdown = %q", got.Markdown)
	}
	if len(got.Links) != 1 || got.Links[0].Raw != "Acme" || got.Links[0].Target != nil {
		t.Errorf("links = %+v (Acme should be dangling)", got.Links)
	}

	var list []service.DocMeta
	doJSON(t, "GET", ts.URL+"/api/v1/documents?type=person", nil, http.StatusOK, &list)
	if len(list) != 1 {
		t.Errorf("list = %+v", list)
	}

	doJSON(t, "GET", ts.URL+"/api/v1/documents/people/nope.md", nil, http.StatusNotFound, nil)

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/documents/"+doc.Path, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("delete = %d", res.StatusCode)
	}
}

func TestTraversalRejected(t *testing.T) {
	ts := newTestServer(t)
	// Literal dot-segments (encoded ones are normalized or rejected by the
	// mux before reaching handlers).
	res, err := http.Get(ts.URL + "/api/v1/files/attachments/%2e%2e/secret")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode == http.StatusOK {
		t.Errorf("traversal returned 200")
	}
}

func TestTasksAndToday(t *testing.T) {
	ts := newTestServer(t)

	var task service.Task
	doJSON(t, "POST", ts.URL+"/api/v1/tasks",
		map[string]string{"text": "Buy cake", "due": "2026-09-01"}, http.StatusCreated, &task)

	var today service.TodayPayload
	doJSON(t, "GET", ts.URL+"/api/v1/today", nil, http.StatusOK, &today)
	if len(today.DueToday) != 1 || today.DueToday[0].Text != "Buy cake" {
		t.Errorf("due today = %+v", today.DueToday)
	}
	if today.Daily == nil {
		t.Errorf("daily note should exist after capture")
	}

	var toggled service.Task
	doJSON(t, "POST", ts.URL+"/api/v1/tasks/"+task.ID+"/toggle", nil, http.StatusOK, &toggled)
	if !toggled.Done {
		t.Errorf("toggled = %+v", toggled)
	}

	var logbook []service.Task
	doJSON(t, "GET", ts.URL+"/api/v1/tasks?view=logbook", nil, http.StatusOK, &logbook)
	if len(logbook) != 1 {
		t.Errorf("logbook = %+v", logbook)
	}

	doJSON(t, "GET", ts.URL+"/api/v1/tasks?view=bogus", nil, http.StatusBadRequest, nil)
}

func TestSearchEndpoint(t *testing.T) {
	ts := newTestServer(t)
	doJSON(t, "POST", ts.URL+"/api/v1/documents",
		map[string]string{"type": "note", "title": "Kubernetes upgrade", "markdown": "# Kubernetes upgrade\n\ncluster notes #infra\n"},
		http.StatusCreated, nil)

	var hits []service.SearchResult
	doJSON(t, "GET", ts.URL+"/api/v1/search?q=cluster", nil, http.StatusOK, &hits)
	if len(hits) != 1 || !strings.Contains(hits[0].Snippet, "<mark>") {
		t.Errorf("hits = %+v", hits)
	}
}

func TestDailyEndpoints(t *testing.T) {
	ts := newTestServer(t)

	doJSON(t, "GET", ts.URL+"/api/v1/daily/2026-09-01", nil, http.StatusNotFound, nil)

	var doc service.Document
	doJSON(t, "POST", ts.URL+"/api/v1/daily/2026-09-01", nil, http.StatusOK, &doc)
	if doc.Path != "daily/2026-09-01.md" || doc.Type != "daily" {
		t.Errorf("daily = %+v", doc.DocMeta)
	}
	// Idempotent.
	doJSON(t, "POST", ts.URL+"/api/v1/daily/2026-09-01", nil, http.StatusOK, &doc)
	doJSON(t, "GET", ts.URL+"/api/v1/daily/2026-09-01", nil, http.StatusOK, &doc)
}

func TestAttachmentUploadAndServe(t *testing.T) {
	ts := newTestServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "Screen Shot 2026.png")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(fw, "fake-png-bytes")
	mw.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/attachments", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("upload = %d", res.StatusCode)
	}
	var envelope struct {
		Data service.Attachment `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	att := envelope.Data
	if !strings.HasPrefix(att.Path, "attachments/2026/09/screen-shot-2026-") {
		t.Errorf("attachment path = %q", att.Path)
	}
	if !strings.HasPrefix(att.Markdown, "![Screen Shot 2026.png](attachments/") {
		t.Errorf("markdown ref = %q", att.Markdown)
	}

	got, err := http.Get(ts.URL + "/api/v1/files/" + att.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK || got.Header.Get("Content-Type") != "image/png" {
		t.Errorf("serve = %d %s", got.StatusCode, got.Header.Get("Content-Type"))
	}
}

func TestHealth(t *testing.T) {
	ts := newTestServer(t)
	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	doJSON(t, "GET", ts.URL+"/api/v1/health", nil, http.StatusOK, &health)
	if health.Status != "ok" || health.Version != "test" {
		t.Errorf("health = %+v", health)
	}
}
