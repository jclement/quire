package vault

import (
	"testing"
	"time"
)

// TestPathLocksMatchTheFilesystem: on case-insensitive, normalization-
// insensitive APFS, "Café.md" (composed), "Café.md" (decomposed) and
// "café.md" are one file, so they must be one lock — otherwise two writers
// naming it differently race exactly as if there were no lock at all.
func TestPathLocksMatchTheFilesystem(t *testing.T) {
	var locks PathLocks
	for _, variant := range []string{"notes/café.md", "notes/Café.md", "NOTES/CAFÉ.md"} {
		unlock := locks.Lock("notes/Café.md")
		acquired := make(chan func())
		go func() { acquired <- locks.Lock(variant) }()
		select {
		case other := <-acquired:
			other()
			unlock()
			t.Errorf("%q locked while %q was held", variant, "notes/Café.md")
		case <-time.After(50 * time.Millisecond):
			unlock()
			(<-acquired)()
		}
	}
}
