// Attachment storage: pasted/dropped files land in the vault under
// attachments/YYYY/MM/ with collision-resistant names, and the caller gets a
// ready-to-insert markdown reference. Files are user content — they live in
// the vault, get backed up with it, and are never silently deleted.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/jclement/quire/internal/vault"
	"github.com/jclement/quire/internal/vision"
)

// describeTimeout bounds the vision call. Someone is watching a placeholder
// where their screenshot should be, so a slow model must lose the race
// rather than hold the paste open.
const describeTimeout = 25 * time.Second

// maxAttachmentSize guards the upload endpoint; 50MB covers phone photos and
// screen recordings without letting a runaway request eat the disk.
const maxAttachmentSize = 50 << 20

// SaveAttachment stores an uploaded file and returns its reference.
func (s *Service) SaveAttachment(ctx context.Context, originalName string, r io.Reader) (Attachment, error) {
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
		alt := display
		if described := s.describeImage(ctx, data, ext); described != "" {
			alt = described
		}
		return Attachment{Path: rel, Markdown: fmt.Sprintf("![%s](%s)", alt, rel)}, nil
	}
	return Attachment{Path: rel, Markdown: fmt.Sprintf("[%s](%s)", display, rel)}, nil
}

// VisionEnabled reports whether screenshots get described on upload.
func (s *Service) VisionEnabled() bool { return s.Vision != nil }

// describeImage asks the vision model for alt text, returning "" for every
// reason not to: vision off, a format no API accepts, an image too large, a
// call that failed or ran long. An image that arrives labelled with its
// filename is a small loss; a paste that fails because a third party had a
// bad minute is a much larger one, so this never returns an error.
func (s *Service) describeImage(ctx context.Context, data []byte, ext string) string {
	if s.Vision == nil || !vision.Describable(ext) || len(data) > vision.MaxBytes {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, describeTimeout)
	defer cancel()
	described, err := s.Vision.Describe(ctx, data, vision.MIMEType(ext))
	if err != nil {
		slog.Warn("describing attachment", "err", err)
		return ""
	}
	return described
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
