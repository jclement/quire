// `quire backup`: one tar.gz holding everything that matters — the vault
// plus auth.db (snapshotted via VACUUM INTO so a live server can't hand us a
// torn copy) and config.yaml. The index is deliberately excluded: it is
// rebuildable by design.
package main

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/jclement/quire/internal/config"
)

func runBackup(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	out := fmt.Sprintf("quire-backup-%s.tar.gz", time.Now().Format("20060102-150405"))
	if len(args) > 0 {
		out = args[0]
	}
	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("creating %s: %w", out, err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	files := 0
	addFile := func(diskPath, tarPath string) error {
		info, err := os.Stat(diskPath)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = tarPath
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		src, err := os.Open(diskPath)
		if err != nil {
			return err
		}
		defer src.Close()
		if _, err := io.Copy(tw, src); err != nil {
			return err
		}
		files++
		return nil
	}

	// The vault, wholesale.
	err = filepath.WalkDir(cfg.VaultDir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(cfg.DataDir, p)
		if err != nil {
			return err
		}
		return addFile(p, filepath.ToSlash(rel))
	})
	if err != nil {
		return fmt.Errorf("archiving vault: %w", err)
	}

	// auth.db via VACUUM INTO — a consistent snapshot even mid-write.
	authPath := filepath.Join(cfg.StateDir(), "auth.db")
	if _, err := os.Stat(authPath); err == nil {
		snapshot := filepath.Join(os.TempDir(), fmt.Sprintf("quire-auth-snapshot-%d.db", os.Getpid()))
		defer os.Remove(snapshot)
		db, err := sql.Open("sqlite", authPath)
		if err != nil {
			return fmt.Errorf("opening auth.db: %w", err)
		}
		_, err = db.Exec("VACUUM INTO ?", snapshot)
		db.Close()
		if err != nil {
			return fmt.Errorf("snapshotting auth.db: %w", err)
		}
		if err := addFile(snapshot, ".quire/auth.db"); err != nil {
			return fmt.Errorf("archiving auth.db: %w", err)
		}
	}

	if cfgPath := filepath.Join(cfg.StateDir(), "config.yaml"); fileExists(cfgPath) {
		if err := addFile(cfgPath, ".quire/config.yaml"); err != nil {
			return fmt.Errorf("archiving config: %w", err)
		}
	}

	fmt.Printf("backup written: %s (%d files)\nrestore: extract into an empty data dir and run `quire reindex`\n", out, files)
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
