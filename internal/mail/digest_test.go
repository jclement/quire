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
	if msg.Subject != "quire — 2026-09-01" {
		t.Errorf("subject = %q", msg.Subject)
	}
	for _, want := range []string{"Acme Sync", "Chase legal", "Sign Maya's form (due 2026-09-01)", "🎂 Sarah Chen — today! (turns 41)"} {
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
