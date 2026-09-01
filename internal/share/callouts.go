// Obsidian-style callouts for the server-rendered share page.
//
// A goldmark extension rather than string surgery: the parser hands us the
// blockquote AST, we tag the ones that open with `[!type]`, and a custom
// renderer emits a styled panel. Doing it in the AST means the title and body
// still go through goldmark's normal escaping — the share page is public, so
// injecting raw HTML was never an option.
package share

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// calloutTypes are the eight the app supports; anything else falls back to
// "note", matching Obsidian's behaviour.
var calloutTypes = map[string]bool{
	"note": true, "info": true, "tip": true, "warning": true,
	"danger": true, "question": true, "success": true, "example": true,
}

// calloutOpener matches the `[!warning] Optional title` that starts a callout
// blockquote. The +/- fold markers are accepted and ignored (nothing folds on
// a printed page).
var calloutOpener = regexp.MustCompile(`^\[!(\w+)\][+-]?\s*(.*)$`)

const (
	calloutKindAttr  = "data-callout"
	calloutTitleAttr = "data-callout-title"
)

// callouts is the goldmark extension bundling the transformer and renderer.
type callouts struct{}

func (c callouts) Extend(m goldmark.Markdown) {
	// A *paragraph* transformer, not an AST one: paragraph transformers run
	// while blocks are still being parsed, so dropping the `[!type]` marker
	// line here means it never reaches inline parsing. An AST transformer
	// runs after inlines are built, where editing Lines() no longer removes
	// the already-parsed marker text.
	m.Parser().AddOptions(parser.WithParagraphTransformers(
		util.Prioritized(calloutTransformer{}, 900)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(calloutRenderer{}, 100)))
}

type calloutTransformer struct{}

// Transform tags a blockquote that opens with `[!type]` and strips the marker
// line, which becomes the panel's title instead.
func (calloutTransformer) Transform(para *ast.Paragraph, reader text.Reader, _ parser.Context) {
	quote, ok := para.Parent().(*ast.Blockquote)
	// Only the paragraph that opens the quote can declare the callout.
	if !ok || quote.FirstChild() != para || para.Lines().Len() == 0 {
		return
	}
	if _, already := quote.AttributeString(calloutKindAttr); already {
		return
	}

	// The segment keeps its trailing newline, and Go's `$` anchors at end of
	// text (not before a final newline as in Perl), so trim first.
	first := para.Lines().At(0)
	line := strings.TrimRight(string(first.Value(reader.Source())), "\r\n")
	match := calloutOpener.FindStringSubmatch(line)
	if match == nil {
		return
	}

	kind := strings.ToLower(match[1])
	if !calloutTypes[kind] {
		kind = "note"
	}
	title := strings.TrimSpace(match[2])
	if title == "" {
		title = strings.ToUpper(kind[:1]) + kind[1:]
	}
	quote.SetAttributeString(calloutKindAttr, []byte(kind))
	quote.SetAttributeString(calloutTitleAttr, []byte(title))

	// Drop the marker line; if it was the whole paragraph, drop the paragraph
	// so the panel isn't left with an empty first block.
	lines := para.Lines()
	if lines.Len() == 1 {
		quote.RemoveChild(quote, para)
		return
	}
	trimmed := text.NewSegments()
	for i := 1; i < lines.Len(); i++ {
		trimmed.Append(lines.At(i))
	}
	para.SetLines(trimmed)
}

type calloutRenderer struct{}

func (r calloutRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
}

// renderBlockquote emits a callout panel for tagged blockquotes and an
// ordinary <blockquote> for everything else.
func (calloutRenderer) renderBlockquote(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	kind, tagged := node.AttributeString(calloutKindAttr)
	if !tagged {
		if entering {
			_, _ = w.WriteString("<blockquote>\n")
		} else {
			_, _ = w.WriteString("</blockquote>\n")
		}
		return ast.WalkContinue, nil
	}

	if !entering {
		_, _ = w.WriteString("</div>\n")
		return ast.WalkContinue, nil
	}
	title, _ := node.AttributeString(calloutTitleAttr)
	_, _ = w.WriteString(`<div class="callout" data-callout="`)
	_, _ = w.Write(util.EscapeHTML(kind.([]byte)))
	_, _ = w.WriteString(`"><p class="callout-title">`)
	_, _ = w.Write(util.EscapeHTML(title.([]byte)))
	_, _ = w.WriteString("</p>\n")
	return ast.WalkContinue, nil
}
