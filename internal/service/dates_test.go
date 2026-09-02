package service

import (
	"strings"
	"testing"
	"time"
)

// TestParseWhen: the API used to write whatever string it was handed
// straight into the markdown, so `{"due":"today"}` produced "📅 today" — a
// task no view could ever surface, with no error to explain it.
func TestParseWhen(t *testing.T) {
	// A Tuesday, so weekday arithmetic is checkable in both directions.
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"2026-12-25", "2026-12-25"},
		{"today", "2026-09-01"},
		{"TODAY", "2026-09-01"},
		{"  today  ", "2026-09-01"},
		{"tomorrow", "2026-09-02"},
		{"tom", "2026-09-02"},
		{"yesterday", "2026-08-31"},
		{"wed", "2026-09-02"},       // tomorrow
		{"wednesday", "2026-09-02"}, // long form works too
		{"thurs", "2026-09-03"},     // any real prefix works
		{"mon", "2026-09-07"},       // next week
		{"tue", "2026-09-08"},       // today is Tuesday: means the next one
		{"+3d", "2026-09-04"},
		{"+0d", "2026-09-01"},
		{"+30d", "2026-10-01"},
	} {
		got, err := ParseWhen(tc.in, now)
		if err != nil {
			t.Errorf("ParseWhen(%q) errored: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseWhen(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// Anything else must fail rather than reach the file.
	for _, bad := range []string{
		"nextish week", "2026-13-45", "someday", "friday-ish", "+d", "2026/09/01",
	} {
		if got, err := ParseWhen(bad, now); err == nil {
			t.Errorf("ParseWhen(%q) = %q, want an error", bad, got)
		}
	}
}

// TestCreateTaskResolvesDates is the regression: a natural date must land in
// the markdown as ISO, and a nonsense one must not land at all.
func TestCreateTaskResolvesDates(t *testing.T) {
	svc := newTestService(t)

	task, err := svc.CreateTask("Book the dentist", "today", "")
	if err != nil {
		t.Fatal(err)
	}
	if task.Due == nil || *task.Due != svc.today() {
		t.Fatalf("due = %v, want %s", task.Due, svc.today())
	}

	doc, err := svc.GetDocument(task.DocPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "📅 " + svc.today(); !strings.Contains(doc.Markdown, want) {
		t.Errorf("markdown %q does not contain %q", doc.Markdown, want)
	}
	if strings.Contains(doc.Markdown, "📅 today") {
		t.Error("the literal word 'today' reached the markdown")
	}

	if _, err := svc.CreateTask("Nonsense", "someday", ""); err == nil {
		t.Error("an unparseable due date should be rejected, not written")
	}
}
