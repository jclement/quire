package service

import (
	"strings"
	"testing"
)

func TestAppendUnderHeading(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			"into the section, above what follows it",
			"# Day\n\n## Focus\n\n- a\n\n## Captured\n\n- [ ] first\n\n## Log\n\n- later\n",
			"# Day\n\n## Focus\n\n- a\n\n## Captured\n\n- [ ] first\n- [ ] new\n\n## Log\n\n- later\n",
		},
		{
			"section is last: still inside it, not after trailing blanks",
			"# Day\n\n## Captured\n\n- [ ] first\n\n",
			"# Day\n\n## Captured\n\n- [ ] first\n- [ ] new\n",
		},
		{
			"no such heading: plain append",
			"# Day\n\nsome prose\n",
			"# Day\n\nsome prose\n- [ ] new\n",
		},
		{
			"a deeper heading does not close the section",
			"# Day\n\n## Captured\n\n### Sub\n\n- a\n\n## Log\n\n- b\n",
			"# Day\n\n## Captured\n\n### Sub\n\n- a\n- [ ] new\n\n## Log\n\n- b\n",
		},
		{
			"a heading inside a code fence is not a heading",
			"# Day\n\n## Captured\n\n```\n## Log\n```\n\n## Log\n\n- b\n",
			"# Day\n\n## Captured\n\n```\n## Log\n```\n- [ ] new\n\n## Log\n\n- b\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := appendUnderHeading(tc.in, captureHeading, "- [ ] new"); got != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestCaptureNoteFilesProseInTheDay(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.InstallStarterTemplates(); err != nil {
		t.Fatal(err)
	}
	doc, err := svc.CaptureNote("  an idea worth\nkeeping  ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Markdown, "- an idea worth keeping") {
		t.Errorf("captured note missing or unfolded:\n%s", doc.Markdown)
	}
	// It lands in the day's Captured section, not at the end of the file.
	captured := strings.Index(doc.Markdown, "## Captured")
	idea := strings.Index(doc.Markdown, "an idea worth keeping")
	if captured < 0 || idea < captured {
		t.Errorf("note should sit under Captured:\n%s", doc.Markdown)
	}
	// A task captured after it joins the same section rather than the end.
	if _, err := svc.CreateTask("and a real action", "", ""); err != nil {
		t.Fatal(err)
	}
	again, err := svc.GetDaily(svc.today())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(again.Markdown, "- [ ] and a real action") {
		t.Errorf("task missing:\n%s", again.Markdown)
	}
	// Both captures sit under Captured, in the order they were made.
	head := strings.Index(again.Markdown, "## Captured")
	idea = strings.Index(again.Markdown, "an idea worth keeping")
	action := strings.Index(again.Markdown, "and a real action")
	if head < 0 || idea < head || action < idea {
		t.Errorf("captures should stack under Captured:\n%s", again.Markdown)
	}
	// And nothing landed under the earlier sections.
	if log := strings.Index(again.Markdown, "## Log"); log > head {
		t.Errorf("Captured should still precede Log:\n%s", again.Markdown)
	}
	if _, err := svc.CaptureNote("   "); err == nil {
		t.Error("empty capture should be refused")
	}
}
