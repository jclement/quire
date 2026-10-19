package index

import "testing"

// A task read is several statements — the tasks, then their projects and
// who they wait on — and with a read pool each could land on a connection
// with a different view. A reindex between them paired a task's old text
// with links already deleted, and the task came back without its project.
// Every task read now runs in one read transaction; this holds one open
// across a reindex and checks the old task still has its project.
// (Reproduced first by the review of the read pool.)
func TestTaskReadIsOneSnapshot(t *testing.T) {
	ix := newTestIndex(t)
	var seq int
	var name, dbPath string
	if err := ix.DB.QueryRow("PRAGMA database_list").Scan(&seq, &name, &dbPath); err != nil {
		t.Fatal(err)
	}
	read, err := OpenReader(dbPath, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	ix.Read = read

	path := "notes/review.md"
	if _, err := ix.Vault.Write(path, []byte("- [ ] Send [[Project Apollo]] report\n"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.IndexFile(path); err != nil {
		t.Fatal(err)
	}

	tx, err := ix.reader().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	rows, err := tx.Query(taskSelect+" WHERE t.doc_path = ?", path)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	// The file is rewritten and reindexed after the task SELECT and before
	// the project lookup.
	f, err := ix.Vault.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Vault.Write(path, []byte("- [ ] Replacement task\n"), f.SHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.IndexFile(path); err != nil {
		t.Fatal(err)
	}

	tasks, err := ix.collectTasks(tx, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Text != "Send [[Project Apollo]] report" {
		t.Fatalf("tasks = %+v", tasks)
	}
	if tasks[0].ProjectPath != "projects/apollo.md" {
		t.Fatalf("the task lost its project across a reindex: %q", tasks[0].ProjectPath)
	}

	// And the next read sees the new state, whole.
	after, err := ix.TaskAt(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	if after.Text != "Replacement task" || after.ProjectPath != "" {
		t.Fatalf("after the reindex: %+v", after)
	}
}
