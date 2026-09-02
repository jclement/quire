// Small helpers shared by the chunker and embedder.
package semantic

import (
	"path"
	"strings"

	"github.com/jclement/quire/internal/vault"
)

func stripFrontmatter(raw []byte) []byte {
	if _, body, ok := vault.SplitFrontmatter(raw); ok {
		return body
	}
	return raw
}

func splitLines(s string) []string { return strings.Split(s, "\n") }

func stem(rel string) string {
	base := path.Base(rel)
	return strings.TrimSuffix(base, path.Ext(base))
}
