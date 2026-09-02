package service

import (
	"testing"
	"time"

	"github.com/jclement/quire/internal/vault"
)

func TestCalendarMonth(t *testing.T) {
	s := newTestService(t) // clock pinned to 2026-09-01

	// A daily note, a meeting dated today, and a completed task.
	if _, err := s.EnsureDaily("2026-09-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDocument(vault.TypeMeeting, "Acme Sync", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDocument("notes/log.md", "- [x] Shipped it ✅ 2026-09-01\n", ""); err != nil {
		t.Fatal(err)
	}

	// "Touched" comes from the indexed mtime, which is a real wall-clock
	// time, while the service clock is pinned to 2026-09-01. Left alone the
	// two agree only when the suite happens to run on that date in the
	// runner's timezone — green in MDT, red in UTC. Reindexing cannot fix it
	// either: IndexFile short-circuits on an unchanged sha256, so touching
	// the files would never reach the index. Stamp the index directly, which
	// is the thing the calendar actually reads.
	stamp := time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local).Unix()
	if _, err := s.Index.DB.Exec("UPDATE documents SET mtime = ?", stamp); err != nil {
		t.Fatal(err)
	}

	month, err := s.Calendar("")
	if err != nil {
		t.Fatal(err)
	}
	if month.Month != "2026-09" || month.Prev != "2026-08" || month.Next != "2026-10" {
		t.Errorf("month nav = %+v", month)
	}
	if len(month.Days) != 30 {
		t.Fatalf("September has %d days", len(month.Days))
	}
	if month.Days[0].Date != "2026-09-01" || month.Days[29].Date != "2026-09-30" {
		t.Errorf("day range = %s … %s", month.Days[0].Date, month.Days[29].Date)
	}

	day := month.Days[0]
	if !day.HasDaily {
		t.Errorf("daily note not detected")
	}
	if len(day.Meetings) != 1 || day.Meetings[0].Title != "Acme Sync" {
		t.Errorf("meetings = %+v", day.Meetings)
	}
	if day.Completed != 1 {
		t.Errorf("completed tasks = %d", day.Completed)
	}
	// Everything written above was touched today.
	if len(day.Touched) < 3 {
		t.Errorf("touched = %+v", day.Touched)
	}
	// Empty days still appear, with non-nil slices so JSON is [] not null.
	if month.Days[15].HasDaily || month.Days[15].Touched == nil || month.Days[15].Meetings == nil {
		t.Errorf("empty day = %+v", month.Days[15])
	}

	// An explicit month with no activity is still a full, valid grid.
	feb, err := s.Calendar("2026-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(feb.Days) != 28 || feb.Prev != "2026-01" || feb.Next != "2026-03" {
		t.Errorf("february = %d days, prev %s next %s", len(feb.Days), feb.Prev, feb.Next)
	}

	if _, err := s.Calendar("nonsense"); err == nil {
		t.Errorf("invalid month accepted")
	}
}

func TestCalendarLeapYearAndBoundaries(t *testing.T) {
	s := newTestService(t)
	s.Now = func() time.Time { return time.Date(2028, 2, 10, 9, 0, 0, 0, time.Local) }

	month, err := s.Calendar("2028-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(month.Days) != 29 {
		t.Errorf("Feb 2028 (leap) = %d days", len(month.Days))
	}
	// December must roll the year over.
	dec, err := s.Calendar("2028-12")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Next != "2029-01" || dec.Prev != "2028-11" {
		t.Errorf("december nav = prev %s next %s", dec.Prev, dec.Next)
	}
}
