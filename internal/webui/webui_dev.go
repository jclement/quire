//go:build !prod

// Development build (no embedded frontend): in dev the browser talks to the
// Vite server, which proxies API calls here — so this handler only needs to
// point a lost visitor at the right port. Production builds use -tags prod.
package webui

import (
	"fmt"
	"net/http"
)

// Handler explains where the dev UI actually lives.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><title>quire (dev)</title>
<body style="font-family: system-ui; margin: 4rem auto; max-width: 32rem">
<h1>quire — dev build</h1>
<p>This binary was built without the embedded UI. In development, open the
Vite server instead (usually <a href="http://localhost:5173">localhost:5173</a>,
started by <code>mise run dev</code>). For a self-contained binary, build with
<code>mise run build</code>.</p>`)
	})
}
