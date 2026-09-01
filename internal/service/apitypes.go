// The wire contract: every shape the HTTP API and MCP tools return lives
// here, and nowhere else. `mise run gen` runs tygo over THIS FILE ONLY to
// produce web/src/api/generated.ts, so the frontend's types cannot drift
// from the server's — a field renamed here fails the frontend typecheck.
//
// Keep this file to plain data: no methods, no unexported fields, no types
// the browser never sees.
package service

// DocMeta is a document's listing metadata.
type DocMeta struct {
	Path   string   `json:"path"`
	Type   string   `json:"type" tstype:"DocType"`
	Title  string   `json:"title"`
	Mtime  string   `json:"mtime"`
	SHA256 string   `json:"sha256"`
	Tags   []string `json:"tags"`
}

// Link is a wikilink with its resolution ("" target = dangling → null).
type Link struct {
	Target  *string `json:"target"`
	Raw     string  `json:"raw"`
	Display string  `json:"display"`
}

// Document is the full read payload.
type Document struct {
	// Embedded: DocMeta's fields marshal inline, so TS extends rather than nests.
	DocMeta     `tstype:",extends,required"`
	Markdown    string         `json:"markdown"`
	Frontmatter map[string]any `json:"frontmatter"`
	Links       []Link         `json:"links"`
	Backlinks   []DocMeta      `json:"backlinks"`
	Tasks       []Task         `json:"tasks"`
}

// Task is the API task shape.
type Task struct {
	ID          string   `json:"id"`
	DocPath     string   `json:"doc_path"`
	DocTitle    string   `json:"doc_title"`
	Line        int      `json:"line"`
	Text        string   `json:"text"`
	Done        bool     `json:"done"`
	Due         *string  `json:"due"`
	Defer       *string  `json:"defer"`
	Priority    int      `json:"priority"`
	Waiting     bool     `json:"waiting"`
	Recur       *string  `json:"recur"`
	Project     *string  `json:"project"`
	Tags        []string `json:"tags"`
	CompletedOn *string  `json:"completed_on"`
}

// SearchResult is one search hit.
type SearchResult struct {
	Path    string `json:"path"`
	Type    string `json:"type" tstype:"DocType | \"task\""`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// Health is the unauthenticated liveness payload (also feeds the update
// check and the version shown in Settings).
type Health struct {
	Status          string `json:"status"`
	Version         string `json:"version"`
	UpdateAvailable bool   `json:"update_available"`
}

// ShareInfo is a share link as the API presents it. It lives here rather
// than in internal/share so every response shape stays in one package —
// that package is what generates the frontend's TypeScript types.
type ShareInfo struct {
	Token     string `json:"token"`
	DocPath   string `json:"doc_path"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	// Always sent (empty string = unset) rather than omitted: a key that
	// sometimes vanishes can't be typed honestly on the client.
	ExpiresAt    string `json:"expires_at"`
	RevokedAt    string `json:"revoked_at"`
	ViewCount    int64  `json:"view_count"`
	LastViewedAt string `json:"last_viewed_at"`
}

// Birthday is an upcoming birthday surfaced on Today (within the next week
// — the single-dad review's "1-week and day-of" requirement).
type Birthday struct {
	Path      string `json:"path"`
	Title     string `json:"title"`
	Date      string `json:"date"` // this year's occurrence, YYYY-MM-DD
	DaysUntil int    `json:"days_until"`
	Age       *int   `json:"age"` // when the birthday year is known
}

// TodayPayload is the composed home-screen (and MCP `today` tool) response.
type TodayPayload struct {
	Date      string     `json:"date"`
	Daily     *Document  `json:"daily"`
	Meetings  []DocMeta  `json:"meetings"`
	Overdue   []Task     `json:"overdue"`
	DueToday  []Task     `json:"due_today"`
	Available []Task     `json:"available"`
	Waiting   []Task     `json:"waiting"`
	Birthdays []Birthday `json:"birthdays"`
	Recent    []DocMeta  `json:"recent"`
}

// Attachment is the upload response: the vault path and the markdown to
// insert. References are vault-relative so they stay meaningful to external
// editors; the SPA rewrites them to /api/v1/files/ URLs when rendering.
type Attachment struct {
	Path     string `json:"path"`
	Markdown string `json:"markdown"`
}

// RenameResult reports what a rename touched.
type RenameResult struct {
	Document  Document `json:"document"`
	Rewritten []string `json:"rewritten"` // docs whose links were updated
}

// CalendarDoc is a document touched on a day.
type CalendarDoc struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Type  string `json:"type" tstype:"DocType"`
}

// CalendarDay is one cell of the month grid.
type CalendarDay struct {
	Date string `json:"date"` // YYYY-MM-DD
	// HasDaily reports whether a daily note exists for this date (whether or
	// not it was touched today — it is the day's anchor).
	HasDaily  bool          `json:"has_daily"`
	Touched   []CalendarDoc `json:"touched"`  // documents modified that day
	Meetings  []CalendarDoc `json:"meetings"` // meetings scheduled that day
	Completed int           `json:"completed_tasks"`
}

// CalendarMonth is the month payload.
type CalendarMonth struct {
	Month string        `json:"month"` // YYYY-MM
	Prev  string        `json:"prev"`
	Next  string        `json:"next"`
	Days  []CalendarDay `json:"days"` // every day of the month, in order
}

// AgentGuidanceResponse is the owner's MCP guidance and where it is stored.
type AgentGuidanceResponse struct {
	// Path is the vault document holding it, so the UI can link to it.
	Path string `json:"path"`
	Text string `json:"text"`
}
