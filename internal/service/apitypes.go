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
	// Area is the document's frontmatter area:, lowercased; "" when
	// unclassified. Daily notes are always "".
	Area string `json:"area"`
	// AreaFrom is the document the area was inherited from, "" when explicit.
	AreaFrom string `json:"area_from"`
}

// AreaCount is one area (work, personal, …) with its document count. Defined
// areas (Settings) carry their colour and come first; an area only found in
// frontmatter is listed too, in neutral, so nothing disappears.
type AreaCount struct {
	Area    string `json:"area"`
	Count   int    `json:"count"`
	Color   string `json:"color"`
	Defined bool   `json:"defined"`
}

// AreaDef is a defined area as edited in Settings.
type AreaDef struct {
	Name  string `json:"name"`
	Color string `json:"color"`
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
	// OpenTasks are open tasks elsewhere in the vault that name this
	// document — "what am I still owed about Acme". Entity documents only
	// (person, company, project); empty for everything else.
	OpenTasks []Task `json:"open_tasks"`
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
	// Score is cosine similarity for semantic results; absent for full-text.
	Score float64 `json:"score,omitempty"`
}

// Health is the unauthenticated liveness payload (also feeds the update
// check and the version shown in Settings).
type Health struct {
	Status          string `json:"status"`
	Version         string `json:"version"`
	UpdateAvailable bool   `json:"update_available"`
	// SemanticSearch is whether an embeddings key is configured — the UI
	// shows the Semantic toggle only when it is.
	SemanticSearch bool `json:"semantic_search"`
	// Git is whether the vault is git-backed (on by default; QUIRE_GIT=false
	// turns it off), so the UI can say whether a deletion is recoverable.
	Git bool `json:"git"`
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

// SemanticStatus is the embeddings pipeline as Settings shows it.
type SemanticStatus struct {
	Enabled   bool   `json:"enabled"`
	Model     string `json:"model"`
	Documents int    `json:"documents"`
	Pending   int    `json:"pending"`
	// LastError is the most recent embedding failure, "" when healthy.
	LastError string `json:"last_error"`
}

// WeekPayload is the weekly review: what landed, what slipped, what is
// still owed, and what has gone quiet — composed from the index rather
// than remembered, and sitting above the week's own note.
type WeekPayload struct {
	Week  string `json:"week"`
	Start string `json:"start"`
	End   string `json:"end"`
	Prev  string `json:"prev"`
	Next  string `json:"next"`
	// Note is the week's own document, nil until it is written.
	Note *Document `json:"note"`
	// Completed is everything finished inside the week, newest first.
	Completed []Task `json:"completed"`
	// Slipped is still open and was due before the week ended.
	Slipped []Task `json:"slipped"`
	// Waiting is delegated work still outstanding.
	Waiting []Task `json:"waiting"`
	// Stalled are active projects with no open task anywhere.
	Stalled []DocMeta `json:"stalled"`
	// Recurrence lists repeating tasks that quietly stopped repeating.
	Recurrence []RecurrenceProblem `json:"recurrence"`
	// Meetings held in the week, and documents touched in it.
	Meetings []DocMeta `json:"meetings"`
	Touched  []DocMeta `json:"touched"`
}

// RecurrenceProblem is a repeating task that has stopped repeating: either
// completed with no next occurrence, or written with a 🔁 spec the grammar
// does not understand. Both fail silently, which is why they are reported.
type RecurrenceProblem struct {
	Task Task `json:"task"`
	// Reason is "stopped" or "unparsed".
	Reason string `json:"reason"`
}

// Unwritten is a name the vault keeps referring to that has no document —
// a to-do list for the graph, which writes itself.
type Unwritten struct {
	Name    string    `json:"name"`
	Refs    int       `json:"refs"`
	Sources []DocMeta `json:"sources"`
}

// TimezoneInfo is the configured zone, what it resolves to, and the
// server's idea of now in it — so Settings can show "today is …".
type TimezoneInfo struct {
	Timezone  string `json:"timezone"`
	Effective string `json:"effective"`
	Now       string `json:"now"`
}

// EmailStatus is what Settings shows about email: whether SMTP and a digest
// recipient are configured, and when the digest goes out.
type EmailStatus struct {
	Configured bool   `json:"configured"`
	From       string `json:"from"`
	DigestTo   string `json:"digest_to"`
	DigestTime string `json:"digest_time"`
}

// Drawing is an Excalidraw drawing: the scene file and its SVG render.
type Drawing struct {
	// Path is the .excalidraw scene, the file the editor reopens.
	Path string `json:"path"`
	// SVGPath is the render the note embeds.
	SVGPath string `json:"svg_path"`
	// Markdown embeds the render; empty on a save (the note already has it).
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

// TokenInfo is an API token as the management UI sees it. The token itself
// is never here: only its 8-char display prefix, because the plaintext is
// shown once at creation and only its hash is stored.
type TokenInfo struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Prefix string   `json:"prefix"`
	Scopes []string `json:"scopes"`
	// Always sent (empty string = unset) so the client can type them
	// honestly — see ShareInfo.
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	RevokedAt  string `json:"revoked_at"`
	LastUsedAt string `json:"last_used_at"`
}

// NewToken is the create response — the only time the plaintext exists
// outside the caller's clipboard.
type NewToken struct {
	Token TokenInfo `json:"token"`
	// Plaintext is shown once and never retrievable again.
	Plaintext string `json:"plaintext"`
}

// ConnectedApp is an OAuth client that has been granted access: what is
// attached to this vault, with what scopes, and whether its grant is live.
type ConnectedApp struct {
	ClientID    string   `json:"client_id"`
	Name        string   `json:"name"`
	Scopes      []string `json:"scopes"`
	ConsentedAt string   `json:"consented_at"`
	LastUsedAt  string   `json:"last_used_at"`
	ActiveGrant bool     `json:"active_grant"`
}

// TagCount is a tag and how many documents carry it — the tags page.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// AuditEntry is one recorded agent action. Human edits from the owner's own
// browser session are not audited (they would drown the log in autosaves);
// everything an API token or OAuth client does is.
type AuditEntry struct {
	ID        int64  `json:"id"`
	At        string `json:"at"`
	Principal string `json:"principal"`
	// Action is the MCP tool name, or "METHOD /path" for a REST write.
	Action string `json:"action"`
	// Path is the document the action touched, when there was one.
	Path string `json:"path"`
	// Detail is a short human-readable summary (a title, a task's text).
	Detail string `json:"detail"`
	OK     bool   `json:"ok"`
}

// TemplateInfo describes one template for pickers.
type TemplateInfo struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	For         string `json:"for" tstype:"DocType"`
	Description string `json:"description"`
	// Default reports that this is the type's automatic template
	// (templates/<type>.md) rather than a named alternative.
	Default bool `json:"default"`
}
