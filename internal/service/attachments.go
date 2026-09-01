// Attachment storage: pasted/dropped files land in the vault under
// attachments/YYYY/MM/ with collision-resistant names, and the caller gets a
// ready-to-insert markdown reference. Files are user content — they live in
// the vault, get backed up with it, and are never silently deleted.
package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/jclement/quire/internal/vault"
)

// maxAttachmentSize guards the upload endpoint; 50MB covers phone photos and
// screen recordings without letting a runaway request eat the disk.
const maxAttachmentSize = 50 << 20

// Attachment is the upload response: the vault path and the markdown to
// insert. References are vault-relative so they stay meaningful to external
// editors; the SPA rewrites them to /api/v1/files/ URLs when rendering.
type Attachment struct {
	Path     string `json:"path"`
	Markdown string `json:"markdown"`
}

// SaveAttachment stores an uploaded file and returns its reference.
func (s *Service) SaveAttachment(originalName string, r io.Reader) (Attachment, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxAttachmentSize+1))
	if err != nil {
		return Attachment{}, fmt.Errorf("reading upload: %w", err)
	}
	if len(data) > maxAttachmentSize {
		return Attachment{}, fmt.Errorf("attachment exceeds the %dMB limit", maxAttachmentSize>>20)
	}
	if len(data) == 0 {
		return Attachment{}, fmt.Errorf("empty upload")
	}

	ext := strings.ToLower(path.Ext(originalName))
	stem := vault.Slugify(strings.TrimSuffix(path.Base(originalName), ext))
	if stem == "" {
		stem = "file"
	}

	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return Attachment{}, fmt.Errorf("generating attachment name: %w", err)
	}

	rel := fmt.Sprintf("attachments/%s/%s-%s%s",
		s.Now().Format("2006/01"), stem, hex.EncodeToString(suffix), ext)
	if _, err := s.Vault.Write(rel, data, ""); err != nil {
		return Attachment{}, err
	}

	display := path.Base(originalName)
	if isImageExt(ext) {
		return Attachment{Path: rel, Markdown: fmt.Sprintf("![%s](%s)", display, rel)}, nil
	}
	return Attachment{Path: rel, Markdown: fmt.Sprintf("[%s](%s)", display, rel)}, nil
}

// ReadAttachment streams a vault file for download; only non-markdown files
// under the vault are served this way (documents go through the documents
// API where structure and conflict handling live).
func (s *Service) ReadAttachment(rel string) ([]byte, error) {
	f, err := s.Vault.Read(rel)
	if err != nil {
		return nil, err
	}
	return f.Raw, nil
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".avif", ".heic":
		return true
	}
	return false
}
