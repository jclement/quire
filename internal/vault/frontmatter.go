// Frontmatter handling. The prime rule (DESIGN.md "Fidelity rules"): we never
// round-trip a document through a YAML re-serializer. Reads parse a copy;
// writes edit the raw block line-by-line so key order, comments, and quoting
// styles survive untouched.
package vault

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
)

var fmDelimiter = []byte("---")

// SplitFrontmatter splits raw into the YAML block (without delimiters) and
// the body. A document has frontmatter only if it starts with a `---` line
// closed by another. Returns ok=false (and body=raw) otherwise.
func SplitFrontmatter(raw []byte) (yamlBlock, body []byte, ok bool) {
	rest, found := bytes.CutPrefix(raw, append(fmDelimiter, '\n'))
	if !found {
		return nil, raw, false
	}
	// The closing delimiter must sit on its own line.
	if after, closed := bytes.CutPrefix(rest, append(fmDelimiter, '\n')); closed {
		return nil, after, true // empty frontmatter block
	}
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		// Closing delimiter at EOF without trailing newline.
		if bytes.HasSuffix(rest, []byte("\n---")) {
			return rest[:len(rest)-len("\n---")], nil, true
		}
		return nil, raw, false
	}
	return rest[:end], rest[end+len("\n---\n"):], true
}

// ParseFrontmatter returns the document's frontmatter as a map (nil if none
// or unparseable — broken YAML must never break indexing; the file is just a
// note with odd leading text).
func ParseFrontmatter(raw []byte) map[string]any {
	block, _, ok := SplitFrontmatter(raw)
	if !ok || len(block) == 0 {
		return nil
	}
	var m map[string]any
	if err := yaml.Unmarshal(block, &m); err != nil {
		return nil
	}
	return m
}

// BuildDoc assembles a brand-new document from frontmatter fields and a body.
// Only used at creation time — existing documents are never rebuilt this way.
// fields preserves the given order.
func BuildDoc(fields [][2]string, body string) []byte {
	var b bytes.Buffer
	if len(fields) > 0 {
		b.WriteString("---\n")
		for _, kv := range fields {
			fmt.Fprintf(&b, "%s: %s\n", kv[0], kv[1])
		}
		b.WriteString("---\n")
	}
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.Bytes()
}

// RemoveFrontmatterKey deletes a top-level key, leaving every other line
// (including comments) untouched. Removing the last key leaves an empty
// block rather than guessing whether the user wants the delimiters gone.
func RemoveFrontmatterKey(raw []byte, key string) []byte {
	block, body, ok := SplitFrontmatter(raw)
	if !ok {
		return raw
	}
	prefix := key + ":"
	var kept []string
	for _, line := range strings.Split(string(block), "\n") {
		if strings.HasPrefix(line, prefix) {
			continue
		}
		kept = append(kept, line)
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	if joined := strings.Join(kept, "\n"); strings.TrimSpace(joined) != "" {
		b.WriteString(strings.TrimSuffix(joined, "\n"))
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	b.Write(body)
	return b.Bytes()
}

// SetFrontmatterKey surgically sets a top-level scalar key in the document's
// frontmatter, replacing the key's line if present (preserving everything
// else byte-for-byte) or appending it just before the closing delimiter.
// Documents without frontmatter gain a minimal block. Nested keys are out of
// scope — quire's own schema is flat scalars and inline lists.
func SetFrontmatterKey(raw []byte, key, value string) []byte {
	block, _, ok := SplitFrontmatter(raw)
	newLine := key + ": " + value
	if !ok {
		return append([]byte("---\n"+newLine+"\n---\n"), raw...)
	}

	lines := strings.Split(string(block), "\n")
	prefix := key + ":"
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = newLine
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, newLine)
	}
	newBlock := strings.Join(lines, "\n")

	// Reassemble around the original body bytes.
	_, body, _ := SplitFrontmatter(raw)
	var b bytes.Buffer
	b.WriteString("---\n")
	b.WriteString(strings.TrimSuffix(newBlock, "\n"))
	b.WriteString("\n---\n")
	b.Write(body)
	return b.Bytes()
}
