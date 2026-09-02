package mail

import (
	"strings"
	"testing"

	"github.com/jclement/quire/internal/service"
)

func strPtr(s string) *string { return &s }
func intPtr(n int) *int       { return &n }

func TestBuildDigest(t *testing.T) {
	// A quiet day sends nothing.
	if _, empty := BuildDigest(service.TodayPayload{Date: "2026-09-01"}, "https://q.example"); !empty {
		t.Errorf("quiet day should be empty")
	}

	payload := service.TodayPayload{
		Date:     "2026-09-01",
		Meetings: []service.DocMeta{{Title: "Acme Sync"}},
		Overdue:  []service.Task{{Text: "Chase legal"}},
		DueToday: []service.Task{{Text: "Sign Maya's form", Due: strPtr("2026-09-01")}},
		Birthdays: []service.Birthday{
			{Title: "Sarah Chen", DaysUntil: 0, Age: intPtr(41)},
		},
	}
	msg, empty := BuildDigest(payload, "https://q.example")
	if empty {
		t.Fatal("digest empty with content")
	}
	if msg.Subject != "quire — Tuesday, 1 September 2026" {
		t.Errorf("subject = %q", msg.Subject)
	}
	for _, want := range []string{"Acme Sync", "Chase legal", "Sign Maya's form — today", "🎂 Sarah Chen turns 41 — today!"} {
		if !strings.Contains(msg.Text, want) {
			t.Errorf("text missing %q:\n%s", want, msg.Text)
		}
		if !strings.Contains(msg.HTML, "Sarah Chen") {
			t.Errorf("html missing content")
		}
	}
	// HTML must escape content (a task named with markup must not inject).
	hostile := service.TodayPayload{Date: "2026-09-01", Overdue: []service.Task{{Text: "<script>alert(1)</script>"}}}
	msg, _ = BuildDigest(hostile, "https://q.example")
	if strings.Contains(msg.HTML, "<script>") {
		t.Errorf("HTML injection: %s", msg.HTML)
	}
}

func TestSampleAlwaysHasContent(t *testing.T) {
	quiet := service.TodayPayload{Date: "2026-09-02"}
	msg := BuildSample(quiet, "https://q.example")
	if !strings.HasPrefix(msg.Subject, "[test] ") || !strings.Contains(msg.HTML, "sample day") {
		t.Errorf("a quiet day should send a labelled sample: %q", msg.Subject)
	}
	if !strings.Contains(msg.HTML, "Book the dentist") || !strings.Contains(msg.HTML, "https://q.example/doc/notes/life-admin.md") {
		t.Errorf("sample rows should link into the app: %s", msg.HTML)
	}
	busy := service.TodayPayload{Date: "2026-09-02", DueToday: []service.Task{{Text: "real thing", DocPath: "notes/x.md"}}}
	msg = BuildSample(busy, "https://q.example")
	if strings.Contains(msg.HTML, "sample day") || !strings.Contains(msg.HTML, "real thing") {
		t.Errorf("a real day should be sent as itself: %s", msg.Subject)
	}
	if strings.Contains(msg.HTML, "<!--note-->") {
		t.Error("the note placeholder must be replaced")
	}
}
