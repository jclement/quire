// Package vault is the only package that reads or writes the user's files.
// Everything here obeys the fidelity rules in DESIGN.md: atomic writes,
// compare-and-swap on content hash, and never touching bytes an operation
// didn't semantically change.
package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Vault provides access to the markdown tree rooted at Dir.
type Vault struct {
	Dir string
}

// File is a raw vault file plus the metadata every caller needs.
type File struct {
	Path    string // vault-relative, forward slashes
	Raw     []byte
	SHA256  string
	ModTime time.Time
}

// ErrConflict is returned by Write when the on-disk content no longer matches
// the caller's base hash — someone (an editor, git pull, another tab) changed
// the file underneath them.
var ErrConflict = errors.New("file changed on disk")

// ErrNotFound is returned when a vault path does not exist.
var ErrNotFound = errors.New("not found")

// New returns a Vault rooted at dir, creating the standard entity directories
// so type inference and new-document placement always have a home.
func New(dir string) (*Vault, error) {
	for _, sub := range []string{"notes", "people", "companies", "projects", "meetings", "daily", "attachments"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("creating vault dir %s: %w", sub, err)
		}
	}
	return &Vault{Dir: dir}, nil
}

// abs converts a validated vault-relative path to an absolute one.
func (v *Vault) abs(rel string) string {
	return filepath.Join(v.Dir, filepath.FromSlash(rel))
}

// Read returns the file at the vault-relative path.
func (v *Vault) Read(rel string) (File, error) {
	if err := ValidatePath(rel); err != nil {
		return File{}, err
	}
	raw, err := os.ReadFile(v.abs(rel))
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, fmt.Errorf("%s: %w", rel, ErrNotFound)
	}
	if err != nil {
		return File{}, fmt.Errorf("reading %s: %w", rel, err)
	}
	info, err := os.Stat(v.abs(rel))
	if err != nil {
		return File{}, fmt.Errorf("stat %s: %w", rel, err)
	}
	return File{Path: rel, Raw: raw, SHA256: HashBytes(raw), ModTime: info.ModTime()}, nil
}

// Write atomically replaces the file at rel with content, but only if the
// current on-disk hash matches baseSHA (compare-and-swap). baseSHA "" means
// the caller expects to create a new file; if one already exists that is a
// conflict too. Returns the new file state.
func (v *Vault) Write(rel string, content []byte, baseSHA string) (File, error) {
	if err := ValidatePath(rel); err != nil {
		return File{}, err
	}
	current, err := v.Read(rel)
	switch {
	case err == nil:
		if current.SHA256 != baseSHA {
			return File{}, fmt.Errorf("%s: %w", rel, ErrConflict)
		}
	case errors.Is(err, ErrNotFound):
		if baseSHA != "" {
			return File{}, fmt.Errorf("%s: %w", rel, ErrConflict)
		}
	default:
		return File{}, err
	}
	if err := v.writeAtomic(rel, content); err != nil {
		return File{}, err
	}
	return v.Read(rel)
}

// writeAtomic writes via a same-directory temp file + fsync + rename, so a
// crash never leaves a half-written document and the rename stays atomic
// (cross-device renames aren't).
func (v *Vault) writeAtomic(rel string, content []byte) error {
	dst := v.abs(rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("creating parent of %s: %w", rel, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".quire-write-*")
	if err != nil {
		return fmt.Errorf("temp file for %s: %w", rel, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("fsync %s: %w", rel, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp for %s: %w", rel, err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("renaming into %s: %w", rel, err)
	}
	return nil
}

// Delete removes the file at rel.
func (v *Vault) Delete(rel string) error {
	if err := ValidatePath(rel); err != nil {
		return err
	}
	if err := os.Remove(v.abs(rel)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", rel, ErrNotFound)
		}
		return fmt.Errorf("deleting %s: %w", rel, err)
	}
	return nil
}

// WalkMarkdown calls fn for every .md file in the vault (vault-relative path
// plus os.FileInfo), skipping dotfiles and dot-directories.
func (v *Vault) WalkMarkdown(fn func(rel string, info fs.FileInfo) error) error {
	return filepath.WalkDir(v.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") && p != v.Dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(name, ".md") {
			return nil
		}
		rel, err := filepath.Rel(v.Dir, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(filepath.ToSlash(rel), info)
	})
}

// Exists reports whether rel exists in the vault.
func (v *Vault) Exists(rel string) bool {
	if err := ValidatePath(rel); err != nil {
		return false
	}
	_, err := os.Stat(v.abs(rel))
	return err == nil
}

// HashBytes returns the hex sha256 used as the vault's content hash.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
