package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jclement/quire/internal/calendar"
	"github.com/jclement/quire/internal/service"
)

const apiTestICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:test\r\n" +
	"BEGIN:VEVENT\r\nUID:review@example.com\r\nDTSTAMP:20260801T000000Z\r\n" +
	"DTSTART:20260901T150000\r\nDTEND:20260901T160000\r\nSUMMARY:Design review\r\n" +
	"ATTENDEE;CN=Sarah Chen:mailto:sarah@acme.example\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

// TestCalendarRoutes walks the feature's REST surface: the secret goes in
// once and never comes back out, the agenda arrives, a note is made once.
func TestCalendarRoutes(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(apiTestICS))
	}))
	t.Cleanup(feed.Close)
	secretURL := feed.URL + "/cal/Zk9-secret-token/basic.ics"

	ts := newTestServerWith(t, func(svc *service.Service) {
		svc.Feeds = calendar.NewFetcher(calendar.OpenStore(filepath.Join(t.TempDir(), "feeds.json")))
	})

	for _, call := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/v1/calendar/feeds", `{"url":"` + secretURL + `"}`, http.StatusCreated},
		{"GET", "/api/v1/calendar/feeds", "", http.StatusOK},
		{"POST", "/api/v1/calendar/refresh", "", http.StatusOK},
	} {
		res := rawRequest(t, call.method, ts.URL+call.path, call.body)
		if res.status != call.status {
			t.Fatalf("%s %s = %d: %s", call.method, call.path, res.status, res.body)
		}
		if strings.Contains(res.body, "Zk9-secret-token") {
			t.Errorf("%s %s returned the feed secret: %s", call.method, call.path, res.body)
		}
	}

	var feeds []service.CalendarFeed
	doJSON(t, "GET", ts.URL+"/api/v1/calendar/feeds", nil, http.StatusOK, &feeds)
	if len(feeds) != 1 || feeds[0].Error != "" || feeds[0].LastSuccess == nil || feeds[0].Events != 1 {
		t.Fatalf("feeds = %+v", feeds)
	}

	var today service.TodayPayload
	doJSON(t, "GET", ts.URL+"/api/v1/today", nil, http.StatusOK, &today)
	if len(today.Events) != 1 || today.Events[0].Title != "Design review" {
		t.Fatalf("today's events = %+v", today.Events)
	}

	body := map[string]string{"uid": "review@example.com", "date": "2026-09-01"}
	var doc service.Document
	doJSON(t, "POST", ts.URL+"/api/v1/calendar/events/note", body, http.StatusCreated, &doc)
	var again service.Document
	doJSON(t, "POST", ts.URL+"/api/v1/calendar/events/note", body, http.StatusOK, &again)
	if again.Path != doc.Path {
		t.Errorf("second press opens the same note: %s vs %s", again.Path, doc.Path)
	}

	var prep service.MeetingPrep
	doJSON(t, "GET", ts.URL+"/api/v1/meeting-prep?path="+doc.Path, nil, http.StatusOK, &prep)
	if prep.Title != "Design review" || prep.EventUID != "review@example.com" {
		t.Errorf("prep = %+v", prep)
	}
	doJSON(t, "GET", ts.URL+"/api/v1/meeting-prep", nil, http.StatusBadRequest, nil)
	doJSON(t, "GET", ts.URL+"/api/v1/meeting-prep?event_uid=nope@example.com", nil, http.StatusNotFound, nil)

	doJSON(t, "DELETE", ts.URL+"/api/v1/calendar/feeds/"+feeds[0].ID, nil, http.StatusNoContent, nil)
	doJSON(t, "GET", ts.URL+"/api/v1/today", nil, http.StatusOK, &today)
	if len(today.Events) != 0 {
		t.Errorf("a removed feed's events vanish at once: %+v", today.Events)
	}
	doJSON(t, "POST", ts.URL+"/api/v1/calendar/feeds", map[string]string{"url": "ftp://nope"}, http.StatusBadRequest, nil)
}

type rawResponse struct {
	status int
	body   string
}

func rawRequest(t *testing.T, method, url, body string) rawResponse {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return rawResponse{status: res.StatusCode, body: string(raw)}
}
