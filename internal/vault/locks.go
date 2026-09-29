// Per-path locking: what makes Write's compare-and-swap one step rather
// than a check followed, some time later, by a swap.
package vault

import (
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

// PathLocks hands out one mutex per vault path, so work on different files
// never waits on each other. The zero value is ready to use.
//
// Entries are never freed: one small mutex per file ever touched is bounded
// by the size of the vault, and freeing them safely would need reference
// counting for no real gain.
type PathLocks struct {
	locks sync.Map // path → *sync.Mutex
}

// Lock blocks until path is free and returns the function that frees it.
//
// The key is the path folded the way macOS's filesystem folds names — case
// and Unicode normalization — so every spelling of one file shares one lock.
// On a case-sensitive filesystem that over-serialises two distinct files
// differing only in case, which costs nothing worth measuring.
func (p *PathLocks) Lock(path string) (unlock func()) {
	key := strings.ToLower(norm.NFC.String(path))
	value, _ := p.locks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
