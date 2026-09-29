// Task → note: the inbox's escape hatch for a checkbox that was never an
// action. A paragraph of UX feedback captured as "- [ ]" sits in the inbox
// for weeks because there is nothing to do and no other way out; this
// files it as a note and leaves a link where the task was.
package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jclement/quire/internal/vault"
)

// noteTitleWords is how many words of a task become its note's default
// title: enough to recognise it in a list, short enough to be a name.
const noteTitleWords = 8

var (
	// titleLinkRe matches a wikilink, capturing target and alias.
	titleLinkRe = regexp.MustCompile(`\[\[([^\[\]|]+)(?:\|([^\[\]]+))?\]\]`)
	// titleTagRe matches a #tag with the space before it.
	titleTagRe = regexp.MustCompile(`(?:^|\s)#[\p{L}][\p{L}\p{N}_/-]*`)
)

// NoteTitleFrom proposes a title from a task's text: its first eight words
// with links read as their display text and tags and inline markup
// dropped, trailing punctuation trimmed. The web dialog proposes the same
// (web/src/lib/noteTitle.ts) and lets the owner edit it.
func NoteTitleFrom(text string) string {
	text = titleLinkRe.ReplaceAllStringFunc(text, func(link string) string {
		m := titleLinkRe.FindStringSubmatch(link)
		if m[2] != "" {
			return m[2]
		}
		return m[1]
	})
	text = titleTagRe.ReplaceAllString(text, "")
	text = strings.NewReplacer("**", "", "`", "", "==", "").Replace(text)
	words := strings.Fields(text)
	if len(words) > noteTitleWords {
		words = words[:noteTitleWords]
	}
	return strings.TrimRight(strings.Join(words, " "), ".,;:!?—-")
}

// TaskToNote files the task with id as a note: a new note document titled
// title (NoteTitleFrom the text when blank) whose body is the task's text,
// filed under the source document's area (area when the source has none —
// daily notes never do), and the task's line replaced by a plain bullet
// linking it. Returns the new note.
func (s *Service) TaskToNote(id, title, area string) (Document, error) {
	row, err := s.Index.TaskByID(id)
	if err != nil {
		return Document{}, fmt.Errorf("task %s: %w", id, vault.ErrNotFound)
	}
	if title = strings.TrimSpace(title); title == "" {
		title = NoteTitleFrom(row.Text)
	}
	if title == "" {
		return Document{}, fmt.Errorf("%w: the note needs a title", ErrValidation)
	}
	// Find the line before creating anything, so a task that has moved or
	// changed does not leave an orphan note behind.
	f, err := s.Vault.Read(row.DocPath)
	if err != nil {
		return Document{}, err
	}
	lines := strings.Split(string(f.Raw), "\n")
	lineIdx := findTaskLine(lines, row)
	if lineIdx < 0 {
		return Document{}, fmt.Errorf("task %s: source line not found (file changed); reindex and retry", id)
	}
	if source, err := s.Index.GetDocMeta(row.DocPath); err == nil && source.Area != "" {
		area = source.Area
	}

	note, err := s.CreateDocumentIn(vault.TypeNote, title, "# "+title+"\n\n"+row.Text+"\n", area)
	if err != nil {
		return Document{}, err
	}
	// The bullet keeps the task's indentation and list marker, so a task
	// nested under another item stays nested.
	bullet := "- "
	if m := checkboxRe.FindStringSubmatch(lines[lineIdx]); m != nil {
		bullet = m[1]
	}
	lines[lineIdx] = bullet + s.linkTo(note)
	if _, err := s.UpdateDocument(row.DocPath, strings.Join(lines, "\n"), f.SHA256); err != nil {
		// Undo the half that landed: a note nobody links to, with the task
		// still in the inbox, is a duplicate in waiting.
		_ = s.DeleteDocument(note.Path)
		return Document{}, err
	}
	// Re-read so the note's backlinks include the line just written.
	return s.GetDocument(note.Path)
}

// linkTo is the wikilink that reaches doc: its title, unless the title is
// shared — a duplicate gets a suffixed path, and which of the two a bare
// [[Title]] reaches is an accident of path order — in which case the path,
// with the title as display text.
func (s *Service) linkTo(doc Document) string {
	unsuffixed := doc.Path == vault.NewDocPath(vault.TypeNote, doc.Title, s.Now())
	if unsuffixed && s.Index.ResolveLink(doc.Title) == doc.Path {
		return "[[" + doc.Title + "]]"
	}
	return "[[" + strings.TrimSuffix(doc.Path, ".md") + "|" + doc.Title + "]]"
}
