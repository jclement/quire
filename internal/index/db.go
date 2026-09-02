// Package index maintains index.db — the rebuildable SQLite index over the
// vault. Nothing in here is ever a source of truth: schema changes just bump
// schemaVersion, which nukes and rebuilds the whole database on next start
// (that's why index.db needs no migration files, unlike auth.db).
package index

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// schemaVersion is stored in PRAGMA user_version. Bump on any schema change.
const schemaVersion = 4 // v4: documents.area_explicit / area_from (inheritance)

const schema = `
CREATE TABLE documents (
	path             TEXT PRIMARY KEY,
	type             TEXT NOT NULL,
	title            TEXT NOT NULL,
	mtime            INTEGER NOT NULL,
	size             INTEGER NOT NULL,
	sha256           TEXT NOT NULL,
	frontmatter_json TEXT NOT NULL DEFAULT '{}',
	-- area is the effective area the document files under: its own
	-- frontmatter area: value (lowercased) when set, otherwise the one it
	-- inherits through its entity links (see areas.go); '' when neither,
	-- and always '' for daily notes, which belong to every area.
	area             TEXT NOT NULL DEFAULT '',
	-- area_explicit is the frontmatter value alone; area_from is the path
	-- of the document the effective area was inherited from ('' when the
	-- area is explicit or absent).
	area_explicit    TEXT NOT NULL DEFAULT '',
	area_from        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX documents_area ON documents(area);

-- One row per name a document answers to: lowered title, filename stem,
-- slug, aliases, and the path itself. Wikilink resolution is a join through
-- this table at query time, so links resolve correctly the moment their
-- target appears — no stale resolved-path columns.
CREATE TABLE docnames (
	path TEXT NOT NULL,
	name TEXT NOT NULL
);
CREATE INDEX docnames_name ON docnames(name);
CREATE INDEX docnames_path ON docnames(path);

CREATE TABLE links (
	src_path    TEXT NOT NULL,
	target_norm TEXT NOT NULL, -- lowered raw target, joins docnames.name
	target_raw  TEXT NOT NULL,
	display     TEXT NOT NULL DEFAULT '',
	line        INTEGER NOT NULL
);
CREATE INDEX links_src ON links(src_path);
CREATE INDEX links_target ON links(target_norm);

CREATE TABLE tags (
	path TEXT NOT NULL,
	tag  TEXT NOT NULL
);
CREATE INDEX tags_tag ON tags(tag);
CREATE INDEX tags_path ON tags(path);

CREATE TABLE tasks (
	id           TEXT PRIMARY KEY,
	doc_path     TEXT NOT NULL,
	line         INTEGER NOT NULL,
	text         TEXT NOT NULL,
	raw_text     TEXT NOT NULL,
	done         INTEGER NOT NULL,
	due          TEXT NOT NULL DEFAULT '',
	defer_date   TEXT NOT NULL DEFAULT '',
	completed_on TEXT NOT NULL DEFAULT '',
	priority     INTEGER NOT NULL DEFAULT 0,
	waiting      INTEGER NOT NULL DEFAULT 0,
	recur        TEXT NOT NULL DEFAULT '',
	project_norm TEXT NOT NULL DEFAULT '', -- joins docnames for the project
	tags_json    TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX tasks_doc ON tasks(doc_path);
CREATE INDEX tasks_done_due ON tasks(done, due);

-- Wikilinks inside task text ("Chase [[Dan Roe]]"), used to roll tasks up
-- onto person/project pages via the docnames join.
CREATE TABLE task_links (
	task_id     TEXT NOT NULL,
	target_norm TEXT NOT NULL
);
CREATE INDEX task_links_target ON task_links(target_norm);

CREATE VIRTUAL TABLE fts USING fts5(
	path UNINDEXED, title, body, tags,
	tokenize = 'porter unicode61'
);
`

// Open opens (or creates) index.db at path, recreating the schema from
// scratch whenever the stored version doesn't match. Returns needsReindex
// when the caller must run a full vault scan to repopulate.
func Open(path string) (db *sql.DB, needsReindex bool, err error) {
	// A single connection avoids SQLITE_BUSY entirely: the indexer goroutine
	// and request handlers share it, and personal-vault query volume doesn't
	// need parallelism.
	db, err = sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, false, fmt.Errorf("opening index db: %w", err)
	}
	db.SetMaxOpenConns(1)

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return nil, false, fmt.Errorf("reading schema version: %w", err)
	}
	if version == schemaVersion {
		return db, false, nil
	}
	// v3 → v4 adds columns; everything else in the file (embeddings above
	// all, which cost money to rebuild) stays.
	if version == 3 {
		if err := migrateV3ToV4(db); err == nil {
			return db, false, nil
		}
	}

	// Wrong or zero version: drop everything and rebuild. index.db is
	// disposable by design (DESIGN.md decision 2).
	if err := recreate(db); err != nil {
		return nil, false, err
	}
	return db, true, nil
}

func migrateV3ToV4(db *sql.DB) error {
	for _, stmt := range []string{
		"ALTER TABLE documents ADD COLUMN area_explicit TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE documents ADD COLUMN area_from TEXT NOT NULL DEFAULT ''",
		"UPDATE documents SET area_explicit = area",
		"PRAGMA user_version = 4",
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrating index to v4: %w", err)
		}
	}
	return nil
}

func recreate(db *sql.DB) error {
	rows, err := db.Query(`SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return fmt.Errorf("listing tables: %w", err)
	}
	var drops []string
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			return err
		}
		drops = append(drops, fmt.Sprintf("DROP %s IF EXISTS %q", typ, name))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, stmt := range drops {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("dropping old schema: %w", err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("creating schema: %w", err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("setting schema version: %w", err)
	}
	return nil
}
