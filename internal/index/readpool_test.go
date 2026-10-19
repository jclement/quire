package index

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jclement/quire/internal/vault"
)

// TestReadPoolSeesEveryCommittedWrite runs readers on the pool while the
// writer re-indexes a file over and over: no read may fail (SQLITE_BUSY
// above all), and none may see a version older than the last write that
// committed before it started — read-your-writes is what lets a handler
// answer with the document it just saved.
func TestReadPoolSeesEveryCommittedWrite(t *testing.T) {
	v, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ix, _, err := OpenIndex(filepath.Join(t.TempDir(), "index.db"), v)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	if ix.Read == nil {
		t.Fatal("OpenIndex should open a read pool")
	}

	const path = "notes/counter.md"
	write := func(n int) {
		t.Helper()
		f, _ := v.Read(path)
		content := fmt.Sprintf("# Version %d\n\n- [ ] task %d\n", n, n)
		if _, err := v.Write(path, []byte(content), f.SHA256); err != nil {
			t.Fatal(err)
		}
		if _, err := ix.IndexFile(path); err != nil {
			t.Fatal(err)
		}
	}
	write(0)

	const writes = 60
	var committed atomic.Int64
	var wg sync.WaitGroup
	done := make(chan struct{})
	for r := range 8 {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				floor := committed.Load()
				var seen int64
				if r%2 == 0 {
					row, err := ix.GetDocMeta(path)
					if err != nil {
						t.Errorf("reader %d: %v", r, err)
						return
					}
					seen, _ = strconv.ParseInt(strings.TrimPrefix(row.Title, "Version "), 10, 64)
				} else {
					tasks, err := ix.Tasks(ViewInbox, "2026-09-01", "")
					if err != nil {
						t.Errorf("reader %d: %v", r, err)
						return
					}
					if len(tasks) != 1 {
						t.Errorf("reader %d: %d inbox tasks, want 1", r, len(tasks))
						return
					}
					seen, _ = strconv.ParseInt(strings.TrimPrefix(tasks[0].Text, "task "), 10, 64)
				}
				if seen < floor {
					t.Errorf("reader %d saw version %d after %d committed", r, seen, floor)
					return
				}
			}
		})
	}
	for n := 1; n <= writes; n++ {
		write(n)
		committed.Store(int64(n))
	}
	close(done)
	wg.Wait()

	if _, err := ix.Read.Exec("DELETE FROM documents"); err == nil {
		t.Error("the read pool must refuse writes")
	}
}
