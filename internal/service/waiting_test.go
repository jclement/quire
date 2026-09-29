// Waiting-for: the ⏳ date, who a wait is owed by, and how the views group
// and age it.
package service

import (
	"strings"
	"testing"
	"time"
)

func TestMarkingWaitingStampsAndClearsTheDate(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"notes/w.md": "# W\n\n- [ ] SOC evidence from [[Frances Bagley]] 📅 2026-09-30\n",
	})
	doc, _ := svc.GetDocument("notes/w.md")

	yes := true
	marked, err := svc.EditTask(doc.Tasks[0].ID, TaskEdit{Waiting: &yes})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := svc.Vault.Read("notes/w.md")
	if !strings.Contains(string(f.Raw), "📅 2026-09-30 ⏳ 2026-09-01") {
		t.Errorf("marking should stamp today:\n%s", f.Raw)
	}
	if marked.WaitingFor == nil || marked.WaitingFor.Since == nil || *marked.WaitingFor.Since != "2026-09-01" {
		t.Errorf("waiting_for = %+v", marked.WaitingFor)
	}

	// Marking again keeps the original date: the wait did not restart.
	svc.Now = func() time.Time { return fixedNow.AddDate(0, 0, 3) }
	if _, err := svc.EditTask(marked.ID, TaskEdit{Waiting: &yes}); err != nil {
		t.Fatal(err)
	}
	f, _ = svc.Vault.Read("notes/w.md")
	if !strings.Contains(string(f.Raw), "⏳ 2026-09-01") {
		t.Errorf("re-marking restarted the clock:\n%s", f.Raw)
	}

	no := false
	cleared, err := svc.EditTask(marked.ID, TaskEdit{Waiting: &no})
	if err != nil {
		t.Fatal(err)
	}
	f, _ = svc.Vault.Read("notes/w.md")
	if strings.Contains(string(f.Raw), "⏳") || strings.Contains(string(f.Raw), "2026-09-01") {
		t.Errorf("unmarking should drop marker and date:\n%s", f.Raw)
	}
	if !strings.Contains(string(f.Raw), "📅 2026-09-30") || cleared.WaitingFor != nil {
		t.Errorf("unmarking disturbed the rest: %s / %+v", f.Raw, cleared.WaitingFor)
	}
}

func TestCreateTaskWaitingOnAName(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"people/frances-bagley.md": "---\ntype: person\naliases: [Frances]\n---\n# Frances Bagley\n",
	})

	// An alias resolves, and the who is written into the slot after ⏳,
	// under the document's title; the task's words are left alone.
	task, err := svc.CreateTaskWith(TaskSpec{Text: "Get SOC evidence", WaitingOn: "frances"})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := svc.Vault.Read(task.DocPath)
	if task.Text != "Get SOC evidence" || !strings.Contains(string(f.Raw), "- [ ] Get SOC evidence ⏳ 2026-09-01 [[Frances Bagley]]") {
		t.Errorf("text = %q, file:\n%s", task.Text, f.Raw)
	}
	w := task.WaitingFor
	if !task.Waiting || w == nil || w.Since == nil || *w.Since != "2026-09-01" ||
		w.On == nil || *w.On != "Frances Bagley" || w.OnPath == nil || *w.OnPath != "people/frances-bagley.md" {
		t.Errorf("waiting_for = %+v", w)
	}

	// A name with no page yet is still a who — it lands on the Unwritten list.
	unwritten, err := svc.CreateTaskWith(TaskSpec{Text: "Quote", WaitingOn: "Dan Roe"})
	if err != nil {
		t.Fatal(err)
	}
	if unwritten.WaitingFor.On == nil || *unwritten.WaitingFor.On != "Dan Roe" || unwritten.WaitingFor.OnPath != nil {
		t.Errorf("unwritten who = %+v", unwritten.WaitingFor)
	}
}

// The review's case: the line already links someone, and waiting_on names
// someone else. The named who must win, and re-assigning must move it —
// keeping the id and the date the wait began.
func TestEditTaskWaitingOnOverridesTheLine(t *testing.T) {
	svc := newTestService(t)
	writeVault(t, svc, map[string]string{
		"people/frances-bagley.md": "---\ntype: person\n---\n# Frances Bagley\n",
		"companies/acme.md":        "# Acme\n",
		"notes/w.md":               "# W\n\n- [ ] Ask [[Frances Bagley]] for the CFO intro\n",
	})
	doc, _ := svc.GetDocument("notes/w.md")
	dan := "Dan Roe"
	task, err := svc.EditTask(doc.Tasks[0].ID, TaskEdit{WaitingOn: &dan})
	if err != nil {
		t.Fatal(err)
	}
	if task.WaitingFor == nil || task.WaitingFor.On == nil || *task.WaitingFor.On != "Dan Roe" {
		t.Fatalf("waiting on = %+v", task.WaitingFor)
	}
	if task.ID != doc.Tasks[0].ID || task.Text != "Ask [[Frances Bagley]] for the CFO intro" {
		t.Errorf("naming who changed the task: %q %s", task.Text, task.ID)
	}

	// Re-assign a few days later: the slot moves, the date stays.
	svc.Now = func() time.Time { return fixedNow.AddDate(0, 0, 3) }
	acme := "Acme"
	moved, err := svc.EditTask(task.ID, TaskEdit{WaitingOn: &acme})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := svc.Vault.Read("notes/w.md")
	if !strings.Contains(string(f.Raw), "- [ ] Ask [[Frances Bagley]] for the CFO intro ⏳ 2026-09-01 [[Acme]]\n") {
		t.Errorf("re-assign should replace the who in place:\n%s", f.Raw)
	}
	if moved.ID != task.ID || *moved.WaitingFor.OnPath != "companies/acme.md" {
		t.Errorf("moved = %s %+v", moved.ID, moved.WaitingFor)
	}

	// Unmarking takes marker, date and who together.
	no := false
	if _, err := svc.EditTask(moved.ID, TaskEdit{Waiting: &no}); err != nil {
		t.Fatal(err)
	}
	f, _ = svc.Vault.Read("notes/w.md")
	if !strings.Contains(string(f.Raw), "- [ ] Ask [[Frances Bagley]] for the CFO intro\n") {
		t.Errorf("unmarking left something:\n%s", f.Raw)
	}
}

// A ⏳ date in the future (typed ahead, or a clock skew) is a wait that has
// not started, not a negative one.
func TestFutureWaitingDateIsZeroDays(t *testing.T) {
	since := "2026-09-05"
	tasks := ageTasks([]Task{{Waiting: true, WaitingFor: &WaitingFor{Since: &since}}}, "2026-09-01")
	if d := tasks[0].WaitingFor.Days; d == nil || *d != 0 {
		t.Errorf("days = %v", d)
	}
}

func TestWaitingGroupsAndStaleness(t *testing.T) {
	svc := newTestService(t) // today is 2026-09-01
	writeVault(t, svc, map[string]string{
		"people/frances-bagley.md": "---\ntype: person\n---\n# Frances Bagley\n",
		"companies/acme.md":        "# Acme\n",
		"projects/soc2.md":         "# SOC2\n",
		"notes/w.md": "# W\n\n" +
			"- [ ] Evidence [[Frances Bagley]] ⏳ 2026-08-25\n" + // 7 days: at the threshold, not over
			"- [ ] Pen test [[Frances Bagley]] ⏳ 2026-08-20\n" + // 12 days: stale
			"- [ ] MSA [[Acme]] ⏳ 2026-08-31\n" +
			"- [ ] Numbers for [[SOC2]] ⏳ 2026-08-01\n" +
			"- [ ] Something ⏳\n",
	})
	groups, err := svc.WaitingGroups("")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}
	// People and companies first (oldest wait first), then other names,
	// then the unassigned.
	want := []string{"Frances Bagley", "Acme", "SOC2", ""}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("groups = %q, want %q", names, want)
	}
	frances := groups[0]
	if frances.Tasks[0].Text != "Pen test [[Frances Bagley]]" {
		t.Errorf("oldest first within a group: %+v", frances.Tasks)
	}
	pen, evidence := frances.Tasks[0].WaitingFor, frances.Tasks[1].WaitingFor
	if *pen.Days != 12 || !pen.Stale || *evidence.Days != StaleWaitingDays || evidence.Stale {
		t.Errorf("ages: pen %d/%v evidence %d/%v", *pen.Days, pen.Stale, *evidence.Days, evidence.Stale)
	}
	if frances.Stale != 1 || frances.Path == nil || *frances.Path != "people/frances-bagley.md" {
		t.Errorf("group = %+v", frances)
	}
	if bare := groups[3].Tasks[0].WaitingFor; bare.Days != nil || bare.Stale {
		t.Errorf("a bare ⏳ has no age: %+v", bare)
	}

	// The person page carries what is owed by them.
	doc, err := svc.GetDocument("people/frances-bagley.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.WaitingOn) != 2 || doc.WaitingOn[0].WaitingFor.Days == nil {
		t.Errorf("person waiting_on = %+v", doc.WaitingOn)
	}

	// The weekly review picks out the stale ones.
	week, err := svc.WeekReview("", "")
	if err != nil {
		t.Fatal(err)
	}
	var stale []string
	for _, task := range week.StaleWaiting {
		stale = append(stale, task.Text)
	}
	if strings.Join(stale, "|") != "Numbers for [[SOC2]]|Pen test [[Frances Bagley]]" {
		t.Errorf("stale waiting = %q", stale)
	}
}
