package index

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jclement/quire/internal/vault"
)

func newDecisionIndex(t *testing.T) *Index {
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
	ix := &Index{DB: db, Vault: v}
	for path, content := range map[string]string{
		"people/sarah-chen.md":  "---\narea: work\n---\n# Sarah Chen\n",
		"people/dan-roe.md":     "# Dan Roe\n",
		"projects/apollo.md":    "---\narea: work\n---\n# Project Apollo\n",
		"companies/acme.md":     "# Acme\n",
		"notes/garden.md":       "---\narea: home\n---\n# Garden\n\n## Decisions\n\n- Plant tomatoes, not peppers\n",
		"templates/meeting.md":  "# {{title}}\n\n## Decisions\n\n- Template bullet is not a decision\n",
		"templates/decision.md": "---\ntags: [decision]\n---\n# {{title}}\n",
		// A meeting: its attendees and project are what every decision in
		// it is about, whether or not the bullet repeats them.
		"meetings/2026-09-01-apollo.md": "---\ndate: 2026-09-01T14:00\npeople: [\"[[Sarah Chen]]\"]\nproject: \"[[Project Apollo]]\"\n---\n" +
			"# Apollo sync\n\n## Decisions\n\n- Ship on the 12th\n- [[Dan Roe]] owns the rollback plan\n\n## Action items\n\n- [ ] Tell [[Acme]]\n",
		// A record, with its own singular Decision section: one row, not two.
		"notes/adopt-postgres.md": "---\ntags: [decision]\ndate: 2026-08-15\nproject: \"[[Project Apollo]]\"\n---\n" +
			"# Adopt Postgres\n\n## Context\n\n- outgrew SQLite\n\n## Decision\n\n- Move to Postgres, with [[Acme]] hosting\n",
		"daily/2026-09-03.md": "# 2026-09-03\n\n### decisions\n\n- Skip the offsite\n",
	} {
		if _, err := v.Write(path, []byte(content), ""); err != nil {
			t.Fatal(err)
		}
	}
	// The garden note has no date of its own, so its decision is dated by
	// the file — pinned here so the order is deterministic.
	garden, err := v.Read("notes/garden.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.WriteKeepingModTime("notes/garden.md", garden.Raw, garden.SHA256, time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := ix.FullScan(); err != nil {
		t.Fatal(err)
	}
	return ix
}

type decisionSummary struct {
	Kind, Path, Text, Date string
	Entities               []string
}

func summarize(rows []DecisionRow) []decisionSummary {
	var out []decisionSummary
	for _, r := range rows {
		var ents []string
		for _, e := range r.Entities {
			ents = append(ents, e.Path)
		}
		out = append(out, decisionSummary{r.Kind, r.DocPath, r.Text, r.Date, ents})
	}
	return out
}

func TestDecisionLog(t *testing.T) {
	ix := newDecisionIndex(t)
	rows, err := ix.Decisions(DecisionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	got := summarize(rows)
	want := []decisionSummary{
		{DecisionInline, "daily/2026-09-03.md", "Skip the offsite", "2026-09-03", nil},
		{DecisionInline, "meetings/2026-09-01-apollo.md", "Ship on the 12th", "2026-09-01",
			[]string{"projects/apollo.md", "people/sarah-chen.md"}},
		{DecisionInline, "meetings/2026-09-01-apollo.md", "[[Dan Roe]] owns the rollback plan", "2026-09-01",
			[]string{"projects/apollo.md", "people/dan-roe.md", "people/sarah-chen.md"}},
		{DecisionRecord, "notes/adopt-postgres.md", "Adopt Postgres", "2026-08-15",
			[]string{"projects/apollo.md", "companies/acme.md"}},
		// No date of its own: the file's day, as a last resort.
		{DecisionInline, "notes/garden.md", "Plant tomatoes, not peppers", "2026-07-01", nil},
	}
	// Templates, and the record's own singular Decision section, are absent.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decisions =\n%+v\nwant\n%+v", got, want)
	}
}

func TestDecisionFilters(t *testing.T) {
	ix := newDecisionIndex(t)
	texts := func(f DecisionFilter) []string {
		t.Helper()
		rows, err := ix.Decisions(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rows {
			out = append(out, r.Text)
		}
		return out
	}

	// Entity: linked from the bullet, inherited from the meeting's
	// frontmatter, or made in the entity's own page.
	if got := texts(DecisionFilter{Entity: "people/dan-roe.md"}); !reflect.DeepEqual(got, []string{"[[Dan Roe]] owns the rollback plan"}) {
		t.Errorf("Dan's decisions = %v", got)
	}
	if got := texts(DecisionFilter{Entity: "projects/apollo.md"}); len(got) != 3 {
		t.Errorf("Apollo's decisions = %v, want the two meeting bullets and the record", got)
	}
	if got := texts(DecisionFilter{Entity: "notes/garden.md"}); !reflect.DeepEqual(got, []string{"Plant tomatoes, not peppers"}) {
		t.Errorf("decisions made in the garden note = %v", got)
	}
	// Text matches the decision or its source's title, every word.
	if got := texts(DecisionFilter{Text: "apollo rollback"}); !reflect.DeepEqual(got, []string{"[[Dan Roe]] owns the rollback plan"}) {
		t.Errorf("text filter = %v", got)
	}
	// Area: work gets the meeting (inherited via Sarah/Apollo) plus the
	// daily note, which belongs to every area; not the home garden.
	for _, text := range texts(DecisionFilter{Area: "home"}) {
		if text == "Ship on the 12th" {
			t.Errorf("a work decision leaked into home: %v", text)
		}
	}
	if got := texts(DecisionFilter{Area: "home"}); !reflect.DeepEqual(got, []string{"Skip the offsite", "Plant tomatoes, not peppers"}) {
		t.Errorf("home decisions = %v", got)
	}
}

// Rewriting a document replaces its decisions rather than adding to them.
func TestDecisionsFollowEdits(t *testing.T) {
	ix := newDecisionIndex(t)
	before, err := ix.Vault.Read("notes/garden.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Vault.Write("notes/garden.md", []byte("# Garden\n\n## Decisions\n\n- Plant beans\n"), before.SHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.IndexFile("notes/garden.md"); err != nil {
		t.Fatal(err)
	}
	rows, err := ix.Decisions(DecisionFilter{Entity: "notes/garden.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Text != "Plant beans" {
		t.Errorf("after edit = %+v", summarize(rows))
	}
	if err := ix.Remove("notes/garden.md"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := ix.DB.QueryRow("SELECT COUNT(*) FROM decisions WHERE doc_path = 'notes/garden.md'").Scan(&n); err != nil || n != 0 {
		t.Errorf("removed document left %d decisions (%v)", n, err)
	}
}

// oldIndex writes a minimal index at the given version — just the tables
// the upgrade steps touch, plus embeddings, which must survive them.
func oldIndex(t *testing.T, version int, extra ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		"CREATE TABLE documents (path TEXT PRIMARY KEY, size INTEGER, sha256 TEXT)",
		"CREATE TABLE tasks (id TEXT PRIMARY KEY)",
		"CREATE TABLE task_links (task_id TEXT, target_norm TEXT)",
		"CREATE TABLE embeddings (path TEXT)",
		"INSERT INTO documents VALUES ('a.md', 10, 'abc')",
		"INSERT INTO embeddings VALUES ('a.md')",
	}
	if version >= 5 {
		stmts = append(stmts,
			"ALTER TABLE tasks ADD COLUMN waiting_since TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE tasks ADD COLUMN waiting_on_raw TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE task_links ADD COLUMN target_raw TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE task_links ADD COLUMN ord INTEGER NOT NULL DEFAULT -1")
	}
	stmts = append(append(stmts, extra...), fmt.Sprintf("PRAGMA user_version = %d", version))
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// An index from before decisions existed upgrades in place, every step in
// order — v4 through v5's waiting columns to v6's decision tables — with
// every file marked for a re-read and the embeddings, which cost money to
// rebuild, intact.
func TestOpenUpgradesToV6KeepingEmbeddings(t *testing.T) {
	for _, from := range []int{4, 5} {
		t.Run(fmt.Sprintf("from v%d", from), func(t *testing.T) {
			db, _, err := Open(oldIndex(t, from))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var version, embeddings, size, decisions int
			_ = db.QueryRow("PRAGMA user_version").Scan(&version)
			if version != schemaVersion || schemaVersion != 6 {
				t.Errorf("upgraded to v%d, want v6", version)
			}
			if err := db.QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&embeddings); err != nil || embeddings != 1 {
				t.Errorf("embeddings lost: %d, %v", embeddings, err)
			}
			if err := db.QueryRow("SELECT size FROM documents").Scan(&size); err != nil || size != -1 {
				t.Errorf("document not marked for re-read: size %d, %v", size, err)
			}
			if err := db.QueryRow("SELECT COUNT(*) FROM decisions JOIN decision_links ON decision_id = id").Scan(&decisions); err != nil {
				t.Errorf("decision tables missing: %v", err)
			}
			if _, err := db.Exec("SELECT waiting_since, waiting_on_raw FROM tasks"); err != nil {
				t.Errorf("v5 columns missing: %v", err)
			}
		})
	}
}

// A v5 → v6 step that fails rolls back whole and is reported: the file
// stays a readable v5 with its embeddings, never rebuilt from scratch.
func TestMigrateV5ToV6FailureRollsBack(t *testing.T) {
	// decision_links already exists, so the step fails after decisions was
	// created — which the rollback must undo.
	path := oldIndex(t, 5, "CREATE TABLE decision_links (x TEXT)")
	if _, _, err := Open(path); err == nil {
		t.Fatal("a failed migration should be reported")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, embeddings int
	_ = db.QueryRow("PRAGMA user_version").Scan(&version)
	_ = db.QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&embeddings)
	if version != 5 || embeddings != 1 {
		t.Errorf("after rollback: version %d, embeddings %d", version, embeddings)
	}
	if _, err := db.Exec("SELECT id FROM decisions"); err == nil {
		t.Error("the decisions table should have been rolled back")
	}
}

func TestSearchIsDecision(t *testing.T) {
	ix := newDecisionIndex(t)
	hits, err := ix.Search("is:decision rollback", 10, "2026-09-29")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Type != "decision" || hits[0].Path != "meetings/2026-09-01-apollo.md" ||
		hits[0].Snippet != "2026-09-01 · Apollo sync" {
		t.Errorf("hits = %+v", hits)
	}
	hits, err = ix.Search("is:decision after:2026-09-02", 10, "2026-09-29")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Title != "Skip the offsite" {
		t.Errorf("dated hits = %+v", hits)
	}
}
