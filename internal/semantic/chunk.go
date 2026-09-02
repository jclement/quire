// Chunking: a note becomes a handful of passages, one per heading section
// (long sections split at paragraph breaks), each prefixed with the note's
// title and section heading so the vector knows what it is about. Chunks
// are hashed individually, so editing one section re-embeds one chunk.
package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Chunk is one embeddable passage of a document.
type Chunk struct {
	// Heading is the section's heading text ("" for the preamble).
	Heading string
	// Text is what gets embedded: title, heading, and the section body.
	Text string
}

// targetChunkChars is where a section is split; ~1500 chars is roughly 350
// tokens, small enough that one passage is about one thing.
const targetChunkChars = 1500

// minChunkChars: a section shorter than this is folded into its neighbour
// so a page of one-line headings doesn't become twenty near-empty vectors.
const minChunkChars = 80

// maxChunksPerDoc bounds the cost of a giant note.
const maxChunksPerDoc = 40

// Chunks splits markdown (frontmatter stripped) into passages.
func Chunks(title string, raw []byte) []Chunk {
	body := string(stripFrontmatter(raw))
	sections := splitSections(body)
	var out []Chunk
	for _, sec := range sections {
		for _, piece := range splitLong(sec.text) {
			piece = strings.TrimSpace(piece)
			if piece == "" {
				continue
			}
			out = append(out, Chunk{Heading: sec.heading, Text: prefix(title, sec.heading) + piece})
			if len(out) >= maxChunksPerDoc {
				return out
			}
		}
	}
	if len(out) == 0 && strings.TrimSpace(title) != "" {
		// A note with only a title still deserves to be found by it.
		out = append(out, Chunk{Text: title})
	}
	return out
}

type section struct {
	heading string
	text    string
}

func splitSections(body string) []section {
	var out []section
	current := section{}
	var buf strings.Builder
	inFence := false
	flush := func() {
		text := strings.TrimSpace(buf.String())
		buf.Reset()
		if text == "" && current.heading == "" {
			return
		}
		// Fold a tiny section into the previous one.
		if len(out) > 0 && len(text) < minChunkChars {
			out[len(out)-1].text += "\n\n" + current.heading + "\n" + text
			return
		}
		out = append(out, section{heading: current.heading, text: text})
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") || strings.HasPrefix(strings.TrimSpace(line), "~~~") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "#") {
			hashes := strings.TrimLeft(line, "#")
			if len(line)-len(hashes) <= 6 && strings.HasPrefix(hashes, " ") {
				flush()
				current = section{heading: strings.TrimSpace(hashes)}
				continue
			}
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	flush()
	return out
}

// splitLong cuts text at paragraph breaks so no piece is much over the
// target; a single enormous paragraph is cut on the nearest sentence end.
func splitLong(text string) []string {
	if len(text) <= targetChunkChars {
		return []string{text}
	}
	var out []string
	var cur strings.Builder
	for _, para := range strings.Split(text, "\n\n") {
		if cur.Len()+len(para) > targetChunkChars && cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
		for len(para) > targetChunkChars {
			cut := strings.LastIndexAny(para[:targetChunkChars], ".!?\n")
			if cut < targetChunkChars/2 {
				cut = targetChunkChars
			} else {
				cut++
			}
			out = append(out, para[:cut])
			para = para[cut:]
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(para)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func prefix(title, heading string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(title)
	}
	// "Ops › Ops" says nothing: an H1 that repeats the title is skipped.
	if heading != "" && !strings.EqualFold(heading, title) {
		if b.Len() > 0 {
			b.WriteString(" › ")
		}
		b.WriteString(heading)
	}
	if b.Len() == 0 {
		return ""
	}
	b.WriteString("\n\n")
	return b.String()
}

// Fingerprint identifies a chunk's text under a model, so an unchanged
// passage is never re-embedded.
func Fingerprint(model, text string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + text))
	return hex.EncodeToString(sum[:])
}
