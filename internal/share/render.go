// Share-page rendering: goldmark (GFM, raw HTML escaped — these pages are
// public) inside a self-contained template. Wikilinks render as plain styled
// text (their targets are private), callout markers become bold titles, and
// a <base> tag makes the document's relative attachment references resolve
// through the share's own gated file route.
package share

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/jclement/quire/internal/vault"
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM, callouts{}),
)

var (
	wikilinkRe = regexp.MustCompile(`\[\[([^\[\]|]+)(?:\|([^\[\]]+))?\]\]`)
	fenceRe    = regexp.MustCompile("^\\s*(```|~~~)")
)

// preprocess adapts quire-flavored markdown for a public page, skipping
// fenced code blocks so code samples stay verbatim. Callouts are handled in
// the AST (see callouts.go), not here.
func preprocess(markdown string) string {
	lines := strings.Split(markdown, "\n")
	inFence := false
	for i, line := range lines {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		lines[i] = wikilinkRe.ReplaceAllStringFunc(line, func(link string) string {
			parts := wikilinkRe.FindStringSubmatch(link)
			display := parts[2]
			if display == "" {
				display = parts[1]
			}
			// Rendered as emphasis — a reference, not a navigable link.
			return "*" + strings.TrimSpace(display) + "*"
		})
	}
	return strings.Join(lines, "\n")
}

var pageTemplate = template.Must(template.New("share").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<base href="/s/{{.Token}}/">
<title>{{.Title}}</title>
<style>
:root { color-scheme: light dark;
  --bg: #fdfdfc; --fg: #1f2328; --muted: #6a737d; --border: #e4e4e0;
  --raised: #f4f4f1; --accent: #4662d7;
  --ok: #1a7f37; --warn: #9a6700; --danger: #c0453e; }
@media (prefers-color-scheme: dark) { :root {
  --bg: #16181d; --fg: #d6dae0; --muted: #8b939e; --border: #2c313a;
  --raised: #1e2229; --accent: #7d95f2;
  --ok: #4cae6a; --warn: #d9a53f; --danger: #e5695f; } }
* { box-sizing: border-box; }
body { margin: 0; background: var(--bg); color: var(--fg);
  font: 16px/1.65 ui-sans-serif, -apple-system, "Segoe UI", sans-serif; }
main { max-width: 44rem; margin: 0 auto; padding: 2.5rem 1.25rem 4rem; }
h1, h2, h3, h4 { line-height: 1.25; margin: 1.6em 0 .5em; }
h1 { font-size: 1.7rem; margin-top: .5em; }
h2 { font-size: 1.25rem; border-bottom: 1px solid var(--border); padding-bottom: .25em; }
a { color: var(--accent); }
em { color: var(--accent); font-style: normal; }
img { max-width: 100%; border-radius: 6px; }
code { background: var(--raised); border: 1px solid var(--border); border-radius: 4px;
  padding: .1em .35em; font: .85em ui-monospace, "SF Mono", monospace; }
pre { background: var(--raised); border: 1px solid var(--border); border-radius: 8px;
  padding: .85rem 1rem; overflow-x: auto; }
pre code { background: none; border: none; padding: 0; }
blockquote { margin: 1em 0; padding: .1em 1em; border-left: 3px solid var(--border); color: var(--muted); }

/* Callouts: the whole panel carries the type's colour, with a stronger
   left edge and title. Tints are mixed against the page background so the
   same rule works in light and dark. */
.callout { margin: 1em 0; padding: .1em 1rem; border-radius: 6px;
  border: 1px solid var(--cal); border-left-width: 3px;
  background: color-mix(in oklab, var(--cal) 9%, var(--bg));
  --cal: var(--accent); }
.callout .callout-title { font-weight: 600; color: var(--cal); margin: .7em 0 .2em; }
.callout > :last-child { margin-bottom: .7em; }
.callout[data-callout="note"], .callout[data-callout="info"] { --cal: var(--accent); }
.callout[data-callout="tip"], .callout[data-callout="success"] { --cal: var(--ok); }
.callout[data-callout="warning"], .callout[data-callout="question"] { --cal: var(--warn); }
.callout[data-callout="danger"] { --cal: var(--danger); }
.callout[data-callout="example"] { --cal: var(--muted); }

/* A shared note is something people print. Keep the ink sensible, keep
   blocks whole, and spell out link targets that a page can't click. */
@media print {
  :root { --bg: #ffffff; --fg: #1f2328; --muted: #5b6169; --border: #d7d7d2;
    --raised: #f6f6f4; --accent: #3b4ea8; --ok: #1a7f37; --warn: #8a5a00;
    --danger: #b3322b; }
  body { background: #fff; }
  main { max-width: none; padding: 0; }
  footer { border-top: 1px solid var(--border); }
  @page { margin: 18mm 16mm; }
  pre, .callout, blockquote, table, img { break-inside: avoid; }
  h1, h2, h3, h4 { break-after: avoid; }
  p { orphans: 3; widows: 3; }
  pre { white-space: pre-wrap; word-break: break-word; }
  a[href^="http"]::after { content: " (" attr(href) ")"; font-size: .8em; color: var(--muted); }
}
table { border-collapse: collapse; display: block; overflow-x: auto; }
th, td { border: 1px solid var(--border); padding: .35rem .7rem; text-align: left; }
th { background: var(--raised); }
ul.contains-task-list { list-style: none; padding-left: .25rem; }
input[type=checkbox] { accent-color: var(--accent); margin-right: .5em; }
hr { border: none; border-top: 1px solid var(--border); margin: 2rem 0; }
footer { max-width: 44rem; margin: 0 auto; padding: 0 1.25rem 2rem;
  color: var(--muted); font-size: .8rem; border-top: 1px solid var(--border);
  padding-top: 1rem; }
</style>
</head>
<body>
<main>{{.Body}}</main>
<footer>Shared read-only · quire</footer>
</body>
</html>
`))

// renderPage produces the complete share page HTML for a document.
func renderPage(title, token, markdown string) ([]byte, error) {
	_, body, _ := vault.SplitFrontmatter([]byte(markdown))

	var rendered bytes.Buffer
	if err := md.Convert([]byte(preprocess(string(body))), &rendered); err != nil {
		return nil, fmt.Errorf("rendering markdown: %w", err)
	}

	var page bytes.Buffer
	err := pageTemplate.Execute(&page, struct {
		Title string
		Token string
		Body  template.HTML // goldmark output with raw HTML escaped (safe default)
	}{Title: title, Token: token, Body: template.HTML(rendered.String())})
	if err != nil {
		return nil, fmt.Errorf("rendering share page: %w", err)
	}
	return page.Bytes(), nil
}
