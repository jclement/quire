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
