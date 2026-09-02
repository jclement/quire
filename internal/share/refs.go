// Which files a shared document actually references. This is the
// authorization list for /s/{token}/{path}: a share is a window onto one
// document, so it may serve the attachments that document embeds or links
// and nothing else.
//
// It parses the same goldmark AST the page renders from, deliberately, so
// the two agree by construction. A substring scan of the raw markdown would
// not: mentioning "attachments/private/pay.pdf" in a sentence, or having it
// appear inside an unrelated URL, would silently publish that file.
package share

import (
	"net/url"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/jclement/quire/internal/vault"
)

// referencedFiles returns the set of vault-relative paths the document
// embeds or links. Only same-vault destinations are included: anything with
// a scheme or authority is somebody else's server, and anything that fails
// vault path validation could not be served anyway.
func referencedFiles(markdown string) map[string]bool {
	_, body, _ := vault.SplitFrontmatter([]byte(markdown))
	source := []byte(preprocess(string(body)))
	doc := md.Parser().Parse(text.NewReader(source))

	refs := map[string]bool{}
	add := func(dest []byte) {
		rel := normalizeRef(string(dest))
		if rel == "" {
			return
		}
		if err := vault.ValidatePath(rel); err != nil {
			return
		}
		refs[rel] = true
	}

	// Images and links are the only nodes that can name a file. Code spans
	// and fenced blocks are not link nodes, so a path quoted in an example
	// never lands here — which is the entire point.
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Image:
			add(node.Destination)
		case *ast.Link:
			add(node.Destination)
		}
		return ast.WalkContinue, nil
	})
	return refs
}

// normalizeRef turns a markdown destination into a vault-relative path, or
// "" if it does not name one. Percent-escapes are decoded because that is
// how a browser will ask for the file.
func normalizeRef(dest string) string {
	dest = strings.TrimSpace(dest)
	if dest == "" || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "//") {
		return ""
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return ""
	}
	// The share page sets <base href="/s/{token}/">, so destinations resolve
	// relative to the share root; a leading slash means the same thing here.
	return strings.TrimPrefix(u.Path, "/")
}
