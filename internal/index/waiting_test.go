package index

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jclement/quire/internal/vault"
)

// waitingIndex is a vault built for the who-rule: people and a company,
// a project that is neither, and waiting tasks linking them in every order.
func waitingIndex(t *testing.T, files map[string]string) *Index {
	t.Helper()
	v, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for path, content := range files {
		if _, err := v.Write(path, []byte(content), ""); err != nil {
			t.Fatal(err)
		}
	}
	ix := &Index{DB: db, Vault: v}
	if err := ix.FullScan(); err != nil {
		t.Fatal(err)
	}
	return ix
}

func TestWaitingOnResolution(t *testing.T) {
	ix := waitingIndex(t, map[string]string{
		"people/frances-bagley.md": "---\ntype: person\n---\n# Frances Bagley\n\n- [ ] Intro to their CFO ⏳ 2026-09-25\n",
		"companies/acme.md":        "# Acme\n",
		"projects/soc2.md":         "---\nstatus: active\n---\n# SOC2\n",
		"meetings/2026-09-01-sync.md": "---\npeople: [\"[[Frances Bagley]]\"]\n---\n# Sync\n\n" +
			"- [ ] Evidence for [[SOC2]] from [[Frances Bagley]] ⏳ 2026-09-20\n" +
			"- [ ] Signed MSA [[Acme]] then [[Frances Bagley]] ⏳ 2026-09-10\n" +
			"- [ ] Numbers for [[SOC2]] ⏳\n" +
			"- [ ] Reply from [[Nobody Written]] ⏳ 2026-09-01\n" +
			"- [ ] Something owed, nobody named ⏳\n",
	})
	rows, err := ix.Tasks(ViewWaiting, "2026-09-29", "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]WaitingOn{}
	var order []string
	for _, r := range rows {
		got[r.Text] = r.WaitingOn
		order = append(order, r.WaitingSince)
	}
	cases := []struct {
		text string
		want WaitingOn
	}{
		// A person beats an earlier non-person link.
		{"Evidence for [[SOC2]] from [[Frances Bagley]]", WaitingOn{"Frances Bagley", "people/frances-bagley.md", "person"}},
		// Among people and companies, the first on the line wins.
		{"Signed MSA [[Acme]] then [[Frances Bagley]]", WaitingOn{"Acme", "companies/acme.md", "company"}},
		// No person at all: fall back to the first link.
		{"Numbers for [[SOC2]]", WaitingOn{"SOC2", "projects/soc2.md", "project"}},
		// Dangling: the name as written, no path.
		{"Reply from [[Nobody Written]]", WaitingOn{"Nobody Written", "", ""}},
		// Frontmatter people are not "who": the line names nobody.
		{"Something owed, nobody named", WaitingOn{}},
		// On a person's own page, an unnamed wait is theirs.
		{"Intro to their CFO", WaitingOn{"Frances Bagley", "people/frances-bagley.md", "person"}},
	}
	for _, c := range cases {
		if got[c.text] != c.want {
			t.Errorf("%q: waiting on %+v, want %+v", c.text, got[c.text], c.want)
		}
	}
	// Oldest first; bare ⏳ (no age) trails.
	want := []string{"2026-09-01", "2026-09-10", "2026-09-20", "2026-09-25", "", ""}
	if len(order) != len(want) {
		t.Fatalf("waiting order = %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("waiting order = %v, want %v", order, want)
		}
	}

	// The person rollup keeps only what the rule assigns to Frances: the
	// MSA links her but is owed by Acme.
	mine, err := ix.WaitingOnDoc("people/frances-bagley.md")
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, r := range mine {
		texts = append(texts, r.Text)
	}
	if len(texts) != 2 || texts[0] != "Evidence for [[SOC2]] from [[Frances Bagley]]" || texts[1] != "Intro to their CFO" {
		t.Errorf("waiting on Frances = %v", texts)
	}
}

// Someday is a tag: it takes a task out of the inbox and today, and into
// its own list — until a due date makes it real again.
func TestSomedayView(t *testing.T) {
	ix := waitingIndex(t, map[string]string{
		"daily/2026-09-01.md": "- [ ] Learn the cello #someday\n" +
			"- [ ] Plain inbox item\n" +
			"- [ ] Parked with a tickler #someday 🛫 2026-09-10\n" +
			"- [ ] Parked for later #someday 🛫 2026-12-01\n" +
			"- [ ] Parked but due #someday 📅 2026-09-20\n",
	})
	texts := func(view TaskView) []string {
		t.Helper()
		rows, err := ix.Tasks(view, "2026-09-29", "")
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rows {
			out = append(out, r.Text)
		}
		return out
	}
	if got := texts(ViewInbox); len(got) != 1 || got[0] != "Plain inbox item" {
		t.Errorf("inbox = %v", got)
	}
	if got := texts(ViewSomeday); len(got) != 2 || got[0] != "Learn the cello #someday" || got[1] != "Parked with a tickler #someday" {
		t.Errorf("someday = %v", got)
	}
	// Today: the deferred-to-now someday stays out; the overdue one is in.
	if got := texts(ViewToday); len(got) != 1 || got[0] != "Parked but due #someday" {
		t.Errorf("today = %v", got)
	}
}

// A v4 index keeps its embeddings through the upgrade, and every document
// is marked for re-reading so the new columns fill.
func TestMigrateV4ToV5(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"CREATE TABLE documents (path TEXT PRIMARY KEY, size INTEGER, sha256 TEXT)",
		"CREATE TABLE tasks (id TEXT PRIMARY KEY)",
		"CREATE TABLE task_links (task_id TEXT, target_norm TEXT)",
		"CREATE TABLE embeddings (path TEXT)",
		"INSERT INTO documents VALUES ('a.md', 10, 'abc')",
		"INSERT INTO embeddings VALUES ('a.md')",
		"PRAGMA user_version = 4",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	db, needsReindex, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if needsReindex {
		t.Error("a column migration should not ask for a rebuild")
	}
	var size, embeddings int
	if err := db.QueryRow("SELECT size FROM documents").Scan(&size); err != nil || size != -1 {
		t.Errorf("document not marked stale: size=%d err=%v", size, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&embeddings); err != nil || embeddings != 1 {
		t.Errorf("embeddings lost: %d %v", embeddings, err)
	}
	if _, err := db.Exec("SELECT waiting_since FROM tasks; SELECT ord, target_raw FROM task_links"); err != nil {
		t.Errorf("new columns missing: %v", err)
	}
}
