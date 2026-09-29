package service

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestConcurrentEditsToOneNoteAllLand: an agent creating tasks while the
// browser appends to the same daily note is the ordinary case, not an edge.
// Every one of these edits is re-derivable from the current file, so none of
// them may be lost to another — and none may fail just for losing a race.
func TestConcurrentEditsToOneNoteAllLand(t *testing.T) {
	s := newTestService(t)

	const each = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 3*each)
	for i := range each {
		wg.Add(3)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.CreateTaskWith(TaskSpec{Text: fmt.Sprintf("agent task %d", i)})
			errs <- err
		}()
		go func() {
			defer wg.Done()
			<-start
			_, err := s.CreateTask(fmt.Sprintf("quick task %d", i), "", "")
			errs <- err
		}()
		go func() {
			defer wg.Done()
			<-start
			_, err := s.AppendToDocument("daily/2026-09-01.md", fmt.Sprintf("appended line %d", i), "")
			errs <- err
		}()
	}
	// The daily note must exist for the appends; creating it here also
	// means the task creators race on the write, not only on EnsureDaily.
	if _, err := s.EnsureDaily("2026-09-01"); err != nil {
		t.Fatal(err)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent edit failed: %v", err)
		}
	}

	doc, err := s.GetDaily("2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	for i := range each {
		for _, want := range []string{
			fmt.Sprintf("- [ ] agent task %d\n", i),
			fmt.Sprintf("- [ ] quick task %d\n", i),
			fmt.Sprintf("appended line %d\n", i),
		} {
			if !strings.Contains(doc.Markdown, want) {
				t.Errorf("lost %q", strings.TrimSpace(want))
			}
		}
	}
}

// TestConcurrentDailyCreation: the first capture of the day races to create
// the note. The loser must use the note the winner made, not fail.
func TestConcurrentDailyCreation(t *testing.T) {
	s := newTestService(t)
	const creators = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range creators {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := s.EnsureDaily("2026-09-01"); err != nil {
				t.Errorf("creator %d: %v", i, err)
			}
		}()
	}
	close(start)
	wg.Wait()
}

// outsideWriteOnce arranges for the next write to path to lose its
// compare-and-swap: just before it, rewrite applies an edit straight to the
// vault, as the editor or vim would. Returns how many writes were attempted.
func outsideWriteOnce(t *testing.T, s *Service, path string, rewrite func(string) string) *int {
	t.Helper()
	attempts := 0
	beforeWriteHook = func(p string) {
		if p != path {
			return
		}
		attempts++
		if attempts != 1 {
			return
		}
		f, err := s.Vault.Read(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Vault.Write(path, []byte(rewrite(string(f.Raw))), f.SHA256); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { beforeWriteHook = nil })
	return &attempts
}

// TestLostRaceIsReapplied: the retry loop's reason to exist. An append that
// loses to an outside write is worked out again on top of it — both land.
func TestLostRaceIsReapplied(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("notes/n.md", "# N\n", ""); err != nil {
		t.Fatal(err)
	}
	attempts := outsideWriteOnce(t, s, "notes/n.md", func(raw string) string { return raw + "typed in vim\n" })

	doc, err := s.AppendToDocument("notes/n.md", "from the agent", "")
	if err != nil {
		t.Fatal(err)
	}
	if *attempts != 2 {
		t.Errorf("write attempts = %d, want 2 (one lost, one re-applied)", *attempts)
	}
	if want := "# N\ntyped in vim\nfrom the agent\n"; doc.Markdown != want {
		t.Errorf("after re-apply:\ngot  %q\nwant %q", doc.Markdown, want)
	}
}

// TestToggleThatLostARaceDoesNotUndoIt: the user ticks a task in the editor
// and saves while clicking its checkbox in Today. The click loses the race;
// re-applied as a flip it would reopen the task the save just completed.
func TestToggleThatLostARaceDoesNotUndoIt(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("notes/t.md", "- [ ] pay the bill\n", ""); err != nil {
		t.Fatal(err)
	}
	doc, _ := s.GetDocument("notes/t.md")
	ticked := "- [x] pay the bill ✅ 2026-09-01\n"
	attempts := outsideWriteOnce(t, s, "notes/t.md", func(string) string { return ticked })

	task, err := s.ToggleTask(doc.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !task.Done {
		t.Errorf("toggle reopened the task: %+v", task)
	}
	if *attempts != 1 {
		t.Errorf("write attempts = %d, want 1: the target state was already reached", *attempts)
	}
	after, _ := s.Vault.Read("notes/t.md")
	if string(after.Raw) != ticked {
		t.Errorf("file = %q, want the editor's save untouched", after.Raw)
	}
}

// TestConcurrentCompletesOfARepeatingTask: an agent's complete_task times
// out and it retries while the first is still running. Exactly one
// completion and one next occurrence, however many arrive.
func TestConcurrentCompletesOfARepeatingTask(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("notes/chores.md", "- [ ] change the filter 📅 2026-09-01 🔁 every month\n", ""); err != nil {
		t.Fatal(err)
	}
	doc, _ := s.GetDocument("notes/chores.md")
	id := doc.Tasks[0].ID

	const agents = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range agents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			task, err := s.CompleteTask(id)
			if err != nil {
				t.Errorf("agent %d: %v", i, err)
				return
			}
			if !task.Done {
				t.Errorf("agent %d got an open task back: %+v", i, task)
			}
		}()
	}
	close(start)
	wg.Wait()

	after, _ := s.Vault.Read("notes/chores.md")
	want := "- [x] change the filter 📅 2026-09-01 🔁 every month ✅ 2026-09-01\n" +
		"- [ ] change the filter 📅 2026-10-01 🔁 every month\n"
	if string(after.Raw) != want {
		t.Errorf("after %d concurrent completes:\ngot  %q\nwant %q", agents, after.Raw, want)
	}
}

// TestConcurrentLinksAllLand: linking two people to a meeting at once used
// to read the list outside the edit, so each re-applied its own stale list
// and one link vanished.
func TestConcurrentLinksAllLand(t *testing.T) {
	s := newTestService(t)
	if _, err := s.UpdateDocument("meetings/m.md", "---\npeople: []\n---\n# M\n", ""); err != nil {
		t.Fatal(err)
	}
	const people = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range people {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := s.LinkEntity("meetings/m.md", "people", fmt.Sprintf("Person %d", i)); err != nil {
				t.Errorf("link %d: %v", i, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	doc, _ := s.GetDocument("meetings/m.md")
	for i := range people {
		if want := fmt.Sprintf("[[Person %d]]", i); !strings.Contains(doc.Markdown, want) {
			t.Errorf("lost %s:\n%s", want, doc.Markdown)
		}
	}
}
