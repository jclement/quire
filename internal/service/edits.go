// Re-derivable edits: appending a line, ticking a checkbox, setting one
// frontmatter key. Each is computed from whatever the file says right now,
// so losing a compare-and-swap does not mean the caller's intent is stale —
// it means the edit should be worked out again against the new content.
// Whole-document saves from the editor are the opposite case and never come
// through here: their content IS the user's stale view, and a conflict must
// reach them.
package service

import (
	"errors"

	"github.com/jclement/quire/internal/vault"
)

// maxEditAttempts bounds how often a re-derivable edit is re-applied after
// losing a race. Edits inside this process queue on the path lock and never
// lose to each other; what is left is an editor save or an outside program
// landing between the read and the write, which does not happen three times
// in a row unless something is rewriting the file in a loop — and then
// failing is the right answer.
const maxEditAttempts = 3

// beforeWriteHook, when set, runs in UpdateDocument between an edit's read
// and its write. Tests only: it is how a test lands an outside write in
// exactly the window a re-derivable edit has to survive.
var beforeWriteHook func(path string)

// reapplying runs edit holding path's edit lock, running it again from a
// fresh read when its write loses a compare-and-swap. edit must read the file
// itself; a closure over content read outside it would re-apply stale bytes.
func reapplying[T any](s *Service, path string, edit func() (T, error)) (T, error) {
	defer s.edits.Lock(path)()
	var result T
	var err error
	for range maxEditAttempts {
		result, err = edit()
		if !errors.Is(err, vault.ErrConflict) {
			return result, err
		}
	}
	return result, err
}
