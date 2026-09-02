// Excalidraw drawings. A drawing is two vault files side by side under
// attachments/: the `.excalidraw` scene (JSON, what the editor reopens) and a
// `.excalidraw.svg` render of it. The note embeds the SVG as an ordinary
// image, so share pages, print, PDF export, vim and every other markdown
// viewer show the picture without knowing what Excalidraw is; only the app
// knows the SVG has a source next to it and offers "Edit drawing".
package service

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/jclement/quire/internal/vault"
)

// DrawingExt is the scene file's extension; the render adds ".svg" to it.
const DrawingExt = ".excalidraw"

// emptyScene is what a new drawing opens with — Excalidraw's own file shape,
// so the file is valid for excalidraw.com too.
const emptyScene = `{"type":"excalidraw","version":2,"source":"quire","elements":[],"appState":{"viewBackgroundColor":"#ffffff"},"files":{}}` + "\n"

// emptyRender is the placeholder picture a fresh drawing shows until it is
// first saved; it makes the embed visible (and clickable) straight away.
const emptyRender = `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="120" viewBox="0 0 320 120">` +
	`<rect width="320" height="120" rx="6" fill="#f8fafc" stroke="#cbd5e1" stroke-dasharray="6 4"/>` +
	`<text x="160" y="66" text-anchor="middle" font-family="sans-serif" font-size="14" fill="#64748b">Empty drawing</text></svg>` + "\n"

// scriptTag catches the only thing that could make an SVG dangerous when a
// viewer opens it outside the app's CSP. Case-insensitive; the files handler
// adds a CSP on top, this is the belt to its braces.
var scriptTag = regexp.MustCompile(`(?i)<\s*script|\bon[a-z]+\s*=|javascript:`)

// IsDrawingRender reports whether rel is the SVG half of a drawing.
func IsDrawingRender(rel string) bool {
	return strings.HasSuffix(strings.ToLower(rel), DrawingExt+".svg")
}

// DrawingSourceFor maps a render path to its scene path.
func DrawingSourceFor(renderRel string) string {
	return strings.TrimSuffix(renderRel, ".svg")
}

// CreateDrawing writes an empty scene and its placeholder render and returns
// the markdown that embeds it.
func (s *Service) CreateDrawing(title string) (Drawing, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Drawing"
	}
	stem := vault.Slugify(title)
	if stem == "" {
		stem = "drawing"
	}
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return Drawing{}, fmt.Errorf("generating drawing name: %w", err)
	}
	rel := fmt.Sprintf("attachments/%s/%s-%s%s",
		s.Now().Format("2006/01"), stem, hex.EncodeToString(suffix), DrawingExt)
	if _, err := s.Vault.Write(rel, []byte(emptyScene), ""); err != nil {
		return Drawing{}, err
	}
	if _, err := s.Vault.Write(rel+".svg", []byte(emptyRender), ""); err != nil {
		return Drawing{}, err
	}
	return Drawing{
		Path:     rel,
		SVGPath:  rel + ".svg",
		Markdown: fmt.Sprintf("![%s](%s)", title, rel+".svg"),
	}, nil
}

// SaveDrawing replaces a drawing's scene and render. The scene must be an
// Excalidraw document and the render a script-free SVG; both are size-capped
// like any upload. Last writer wins — a drawing is edited in one modal at a
// time, and the scene carries no merge-able structure worth a CAS dance.
func (s *Service) SaveDrawing(rel string, scene, svg []byte) (Drawing, error) {
	if err := vault.ValidatePath(rel); err != nil {
		return Drawing{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if path.Ext(rel) != DrawingExt {
		return Drawing{}, fmt.Errorf("%w: %q is not a %s file", ErrValidation, rel, DrawingExt)
	}
	if len(scene) > maxAttachmentSize || len(svg) > maxAttachmentSize {
		return Drawing{}, fmt.Errorf("%w: drawing exceeds the %dMB limit", ErrValidation, maxAttachmentSize>>20)
	}
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(scene, &head); err != nil || head.Type != "excalidraw" {
		return Drawing{}, fmt.Errorf("%w: scene is not an Excalidraw document", ErrValidation)
	}
	trimmed := bytes.TrimSpace(svg)
	if !bytes.HasPrefix(trimmed, []byte("<svg")) && !bytes.HasPrefix(trimmed, []byte("<?xml")) {
		return Drawing{}, fmt.Errorf("%w: render is not an SVG", ErrValidation)
	}
	if scriptTag.Match(svg) {
		return Drawing{}, fmt.Errorf("%w: render contains script", ErrValidation)
	}
	// Only an existing drawing can be saved to: the create step picks the
	// path, so a client cannot scatter files across the vault by name.
	if _, err := s.Vault.Read(rel); err != nil {
		if errors.Is(err, vault.ErrNotFound) {
			return Drawing{}, fmt.Errorf("%w: drawing %q does not exist; create it first", ErrValidation, rel)
		}
		return Drawing{}, err
	}
	if err := s.overwrite(rel, scene); err != nil {
		return Drawing{}, err
	}
	if err := s.overwrite(rel+".svg", svg); err != nil {
		return Drawing{}, err
	}
	return Drawing{Path: rel, SVGPath: rel + ".svg", Markdown: ""}, nil
}

// overwrite writes rel regardless of what is there, going through Write so
// the atomic-rename and watcher behaviour is the same as every other save.
func (s *Service) overwrite(rel string, data []byte) error {
	base := ""
	if current, err := s.Vault.Read(rel); err == nil {
		base = current.SHA256
	}
	_, err := s.Vault.Write(rel, data, base)
	return err
}
