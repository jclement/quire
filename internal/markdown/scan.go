// Package markdown is quire's one markdown grammar on the server: a
// line-oriented scanner that extracts the structure the index needs
// (wikilinks, tags, tasks, title) without ever modifying content. The SPA
// renders markdown client-side; this package only *reads*.
package markdown

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/jclement/quire/internal/vault"
)

// Link is a wikilink occurrence: Raw is the target text as written
// ("Sarah Chen"), Display the alias if given.
type Link struct {
	Raw     string
	Display string
	Line    int // 1-based line in the full document
}

// Task is a checkbox line plus its parsed metadata. The emoji grammar is
// Obsidian-Tasks-compatible: 📅 due, 🛫 defer, ✅ completed-on, ⏳ waiting,
// ⏫/🔼/🔽 priority.
type Task struct {
	ID          string // content-derived: hash(docPath + normalized text)
	Line        int    // 1-based; a hint, not identity
	Text        string // task text with metadata stripped, for display
	RawText     string // everything after the checkbox, verbatim
	Done        bool
	Due         string // YYYY-MM-DD or ""
	Defer       string
	CompletedOn string
	Priority    int // 0 none, 1 high, 2 medium, 3 low
	Waiting     bool
	Recur       string // recurrence spec, e.g. "every year" / "every 3 months when done"
	Tags        []string
	Links       []Link // wikilinks inside the task text
}

// Doc is everything the scanner extracts from one document.
type Doc struct {
	Title string
	Links []Link
	Tags  []string
	Tasks []Task
	Body  string // body text without frontmatter, for FTS
}

var (
	wikilinkRe = regexp.MustCompile(`\[\[([^\[\]|]+)(?:\|([^\[\]]+))?\]\]`)
	// Tags: start-of-line or whitespace, then #word (letters first). This
	// deliberately misses tags inside inline code — an accepted edge.
	tagRe  = regexp.MustCompile(`(?:^|\s)#([\p{L}][\p{L}\p{N}_/-]*)`)
	taskRe = regexp.MustCompile(`^\s*[-*+] \[([ xX])\] (.+)$`)
	// Recurrence: "🔁 every year", "🔁 every 3 months when done". The spec
	// vocabulary is deliberately tiny — see DESIGN.md "Tasks".
	recurRe = regexp.MustCompile(`🔁\s*(every(?:\s+\d+)?\s+(?:day|week|month|year)s?(?:\s+when\s+done)?)`)
	h1Re    = regexp.MustCompile(`^# (.+)$`)
	dateRe  = regexp.MustCompile(`^\s*(\d{4}-\d{2}-\d{2})`)
	fence   = regexp.MustCompile("^\\s*(```|~~~)")
)

// Scan parses a document's raw bytes. docPath seeds task IDs; bodyOffset
// handling (frontmatter) is internal — reported line numbers are 1-based
// positions in the full raw file, so writers can address exact lines.
func Scan(docPath string, raw []byte) Doc {
	_, body, hasFM := vault.SplitFrontmatter(raw)
	// Line numbers must count the frontmatter lines we skipped.
	offset := 0
	if hasFM {
		offset = strings.Count(string(raw[:len(raw)-len(body)]), "\n")
	}

	var doc Doc
	doc.Body = string(body)
	tagSet := map[string]struct{}{}

	// Frontmatter carries the entity graph — `company: "[[Acme]]"`,
	// `people: ["[[Sarah Chen]]"]` — so its wikilinks are real links. Without
	// this, linking a meeting to its attendees in frontmatter produced no
	// backlink at all.
	if hasFM {
		for i, line := range strings.Split(string(raw[:len(raw)-len(body)]), "\n") {
			for _, lm := range wikilinkRe.FindAllStringSubmatch(line, -1) {
				doc.Links = append(doc.Links, Link{
					Raw:     strings.TrimSpace(lm[1]),
					Display: strings.TrimSpace(lm[2]),
					Line:    i + 1,
				})
			}
		}
	}

	inFence := false
	for i, line := range strings.Split(doc.Body, "\n") {
		lineNo := offset + i + 1
		if fence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if doc.Title == "" {
			if m := h1Re.FindStringSubmatch(line); m != nil {
				doc.Title = strings.TrimSpace(m[1])
			}
		}
		if m := taskRe.FindStringSubmatch(line); m != nil {
			doc.Tasks = append(doc.Tasks, parseTask(docPath, lineNo, m[1] != " ", m[2]))
			// Task links/tags also count as document links/tags; fall through.
		}
		for _, lm := range wikilinkRe.FindAllStringSubmatch(line, -1) {
			doc.Links = append(doc.Links, Link{Raw: strings.TrimSpace(lm[1]), Display: strings.TrimSpace(lm[2]), Line: lineNo})
		}
		for _, tm := range tagRe.FindAllStringSubmatch(line, -1) {
			tagSet[strings.ToLower(tm[1])] = struct{}{}
		}
	}
	for t := range tagSet {
		doc.Tags = append(doc.Tags, t)
	}
	return doc
}

// Metadata markers. ⏳ takes an optional following [[person]]; the rest take
// an optional following date.
const (
	markDue      = "📅"
	markDefer    = "🛫"
	markDone     = "✅"
	markWaiting  = "⏳"
	markPrioHigh = "⏫"
	markPrioMed  = "🔼"
	markPrioLow  = "🔽"
)

func parseTask(docPath string, line int, done bool, rawText string) Task {
	t := Task{Line: line, Done: done, RawText: rawText}

	text := rawText
	if m := recurRe.FindStringSubmatch(text); m != nil {
		t.Recur = strings.Join(strings.Fields(m[1]), " ")
		text = strings.Replace(text, m[0], "", 1)
	}
	text = extractDated(text, markDue, &t.Due)
	text = extractDated(text, markDefer, &t.Defer)
	text = extractDated(text, markDone, &t.CompletedOn)
	if strings.Contains(text, markWaiting) {
		t.Waiting = true
		text = strings.Replace(text, markWaiting, "", 1)
	}
	switch {
	case strings.Contains(text, markPrioHigh):
		t.Priority, text = 1, strings.Replace(text, markPrioHigh, "", 1)
	case strings.Contains(text, markPrioMed):
		t.Priority, text = 2, strings.Replace(text, markPrioMed, "", 1)
	case strings.Contains(text, markPrioLow):
		t.Priority, text = 3, strings.Replace(text, markPrioLow, "", 1)
	}

	for _, tm := range tagRe.FindAllStringSubmatch(text, -1) {
		t.Tags = append(t.Tags, strings.ToLower(tm[1]))
	}
	for _, lm := range wikilinkRe.FindAllStringSubmatch(text, -1) {
		t.Links = append(t.Links, Link{Raw: strings.TrimSpace(lm[1]), Display: strings.TrimSpace(lm[2]), Line: line})
	}

	t.Text = strings.Join(strings.Fields(text), " ")
	t.ID = TaskID(docPath, t.Text)
	return t
}

// extractDated removes `marker [date]` from text, storing the date (if any)
// into dst. The marker is removed even without a date so it never lingers in
// display text.
func extractDated(text, marker string, dst *string) string {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return text
	}
	after := text[idx+len(marker):]
	if m := dateRe.FindStringSubmatch(after); m != nil {
		*dst = m[1]
		cut := strings.Index(after, m[1]) + len(m[1])
		return text[:idx] + after[cut:]
	}
	return text[:idx] + after
}

// TaskID derives the stable content hash for a task: the normalized display
// text scoped by document. Line numbers are excluded on purpose — edits above
// a task must not orphan it. See DESIGN.md decision 8.
func TaskID(docPath, normalizedText string) string {
	sum := sha256.Sum256([]byte(docPath + "\x00" + normalizedText))
	return hex.EncodeToString(sum[:8])
}
