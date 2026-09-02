# quire — design

A self-hosted personal knowledge & work hub: one Go binary, markdown on disk as the
canonical store, an opinionated web UI, and an agent-operable API/MCP surface.

> Markdown owns the knowledge. The application owns the experience.

Positioning (one breath): quire is for the person who runs their day — and their life —
through notes, meetings, commitments, and people; who wants Obsidian's files, Things'
task discipline, and a personal CRM's memory in one self-hosted app that an AI agent can
operate through the same API a human uses. Built for N=1 (Jeff) with conviction.

This document synthesizes four design reviews (CTO, PM, power-user developer, single-dad
life-admin) done 2026-09-01; the full briefs' conclusions are folded in below.

## The three bets

1. **Fidelity earns trust.** The app never reformats a file it didn't semantically
   change. Byte-identical round-trips, surgical frontmatter edits, atomic writes,
   conflicts never silently lost. One violation and the user retreats to plain files.
2. **Keyboard + capture earn daily use.** Linear-style single-key commands on desktop,
   sub-8-second capture on mobile. Capture never requires filing; filing is a
   later-me problem (the Inbox).
3. **The join is the moat.** Meetings mint real tasks with provenance; person pages
   assemble themselves from your writing; an agent answers "what am I waiting for from
   Acme?" over MCP. Nobody else combines your-files + entity-aware + agent-operable.

## Architecture

Single Go binary (`quire`) serving: REST API under `/api/v1/`, MCP (Streamable HTTP) at
`/mcp`, share pages at `/s/*` (server-rendered, later), and the embedded React SPA for
everything else. Modular monolith:

```
main.go                    # cmd dispatch: serve | reindex | doctor | version
internal/
  config/    # env + yaml config, mode selection
  vault/     # filesystem: read/write/rename markdown + attachments, atomic writes,
             # frontmatter round-trip. The ONLY package that writes user files.
  markdown/  # goldmark parsing: wikilinks, tags, tasks, callouts — one grammar for
             # indexer, renderer, and API
  index/     # SQLite index.db: schema, indexer pipeline, fsnotify watcher, FTS5
  task/      # task extraction, views (inbox/today/upcoming), write-back
  service/   # the one service layer under REST + MCP; enforces scopes
  api/       # HTTP handlers, envelope, SSE events — no SQL, no fs
  mcp/       # MCP tools — thin wrappers over service
  auth/      # auth.db: modes, sessions, passkeys, bearer tokens
  webui/     # go:embed of web/dist + SPA fallback; dev-mode proxy behind build tag
web/         # Vite + React 19 + TS + Tailwind v4 SPA
```

## Storage

Data dir (`QUIRE_DATA_DIR`, Docker `/data`): user content under `vault/`, app state
under `.quire/` — never inside the vault. Zero droppings among the user's files.

```
data/
  vault/
    notes/  people/  companies/  projects/  meetings/  daily/  attachments/
  .quire/
    index.db   # rebuildable, always. `quire reindex` drops and rebuilds; deleting it
               # must never lose anything.
    auth.db    # NOT rebuildable: passkeys, sessions, tokens, recovery codes
    config.yaml
```

**Identity is the vault-relative path** (`people/sarah-chen.md`). No UUIDs in
frontmatter — they're metadata churn and die on external renames. `type` is inferred
from the directory; frontmatter may override. Wikilinks resolve Obsidian-style: exact
path → filename → alias, case-insensitive. Rename in the UI rewrites inbound links
(showing the affected files first); external renames leave dangling backlinks that
`quire doctor` lists.

**Frontmatter schemas** (all fields optional; only non-defaults written):

- person: `aliases, company, email, phone, title, birthday, role, tags`
- company: `domain, website, tags`
- project: `status (active|waiting|someday|completed|archived), due, company, people, tags`
- meeting: `date, people, project, company, tags`
- daily: none (identity is `daily/YYYY-MM-DD.md`)
- note: `title (defaults to H1/filename), tags`

Dates are ISO 8601 local time (a personal tool; UTC in frontmatter is hostile to grep).

### Fidelity rules (sacred)

- Round-trip guarantee: open + save with no edits = byte-identical. CI-tested.
- Frontmatter edited **by key**, preserving key order, comments, quoting, and unknown
  keys. Never parse-and-dump through a re-serializing YAML writer.
- Task toggle touches exactly one line: `[ ]` → `[x]` (plus optional `✅ YYYY-MM-DD`).
- No app-maintained `updated:` timestamps or other volatile churn. mtime and git know.
- LF preserved, trailing-newline presence preserved, atomic writes
  (same-dir temp + fsync + rename).
- Files quire can't parse (broken YAML, unknown extensions) are indexed best-effort or
  skipped — never "fixed", renamed, or quarantined.
- Git test: after a week of use, `git diff` on the vault shows only lines the user
  semantically changed.

## Index pipeline

`modernc.org/sqlite` (pure Go, ships FTS5, keeps `CGO_ENABLED=0`), WAL mode.
Everything in index.db is derivable from a full parse of `vault/`.

Schema (core): `documents(path PK, type, title, mtime, size, sha256, frontmatter_json)`,
`links(src_path, target_path, target_raw, display, position)`, `tags(path, tag)`,
`tasks(...)`, `fts` (FTS5, porter unicode61), `attachments(...)`.

Watcher: fsnotify over `vault/`, 300ms debounce keyed by path, one indexer goroutine
(serialized, no lock games). Startup full-scan diffs `(mtime, size)` and re-hashes on
mismatch, catching changes made while quire was down. `git pull` bursts are absorbed by
the debounce + rescan, not per-event thrash. The app's own writes are ignored via a
recently-written `(path, sha256)` set. Each index update publishes an SSE event on
`/api/v1/events` so open tabs refresh live.

## Tasks

Tasks are markdown checkboxes that are also real tasks — bidirectional sync that never
lies is behavior #1. Inline grammar (Obsidian Tasks-compatible):

```markdown
- [ ] Send Sarah the diagram 📅 2026-09-02 🛫 2026-09-01 ⏫ #apollo @[[Sarah Chen]]
```

`📅` due, `🛫` start/defer, `⏫`/`🔼`/`🔽` priority, `⏳` waiting, `✅` completion date.
Task ID = `sha256(doc_path + normalized_text)[:16]` — content-derived, line numbers are
hints. Rewording a task recreates it; accepted trade-off (revisit if recurrence needs
stable IDs). Provenance is free: every task knows its source document.

Defer dates actually hide things: Today shows only *available* tasks. Views: Inbox
(no date, no project), Today (due/overdue/available), Upcoming, Waiting, Logbook.
Recurrence (v0.2) must support lead time ("surface 3 weeks before due") and
repeat-after-completion — that's the life-admin (renewals) requirement, and it's the
reason `due` and `defer` are separate fields from day one.

## API

House envelope: `{"data":…}` / `{"error":{"code","message"}}`; `/api/v1/` path
versioning; session cookie or `Authorization: Bearer sk_…`. `GET /api/v1/health` is
unauthenticated: `{status, version, update_available}`.

Core resources: `documents` (CRUD by path; read returns raw markdown + parsed structure:
frontmatter JSON, links, tasks), `search` (one filter grammar shared with the UI:
`type:meeting company:acme tag:x is:task due:today after:2026-08-01`), `tasks`
(views + toggle by task ID), `today` (the composed Today payload), `attachments`
(upload → collision-resistant path), `render` (server-side markdown → HTML so previews
and share pages agree), `events` (SSE).

**Every document write carries `base_sha256`**; mismatch → `409 CONFLICT` with current
content. The editor autosaves on idle with the same check, so conflict windows are
seconds wide. Conflicts surface a reload/overwrite/copy-mine dialog — no auto-merge.

Scopes are coarse: `read`, `write`, `tasks`, `share`. Tokens: `sk_` + 32 random bytes,
SHA-256 stored, 8-char prefix displayed, expiry/revocation/`last_used_at`.

Settings is where access is audited and withdrawn: passkeys, API tokens, connected
OAuth apps and live share links, each revocable. Disconnecting an app deletes the
client record *and* revokes its outstanding tokens and codes, so a reconnect must
pass consent again. Revoked tokens stay listed — an audit trail, like shares.

## MCP

Official `modelcontextprotocol/go-sdk`, Streamable HTTP at `/mcp`, authenticated by
bearer token or an OAuth access token — same middleware, same scopes. Tools are thin
wrappers over the same service layer as REST, so permissions cannot drift:

`search`, `get_document`, `create_note`, `update_document` (with `base_sha256`
precondition), `append_to_document` (targeted section append — agents must not rewrite
whole files), `create_person/project/meeting`, `list_tasks`, `create_task` (natural
dates parsed server-side, resolved ISO echoed back; ambiguous name resolution returns
candidates, never guesses), `complete_task`, `today` (the flagship composed call), and
`person_context` (the rollup a person page shows, as JSON — meeting prep in one call).

Agent guardrails: read-only tokens are the default posture; no delete tool; every
API/MCP write is audit-logged (principal, tool, path, when) — `audit_log` in auth.db,
surfaced in Settings. The owner's own browser session is not audited on purpose: the
log answers "what did the agents do", and the human's autosaves would drown it. This
claim was in this document for two releases before anything recorded a row; it is
true now, with tests for the middleware path and the MCP path.

Every tool carries MCP annotations (`readOnlyHint`, `destructiveHint`,
`idempotentHint`) so a client can run reads freely, retry additive writes, and ask
before a full-document replace. Descriptions are written as onboarding — when to
reach for this tool rather than that one — because the description *is* the
integration.

**Tools are registered per request against the caller's scopes.** The auth
middleware attaches the principal to the request context and internal/mcp builds
that request's server from it: read scope yields the five read tools, tasks yields
the task tools, write yields the document writers. This replaced a single blanket
scope check on /mcp, which was a real privilege escalation — a token scoped only to
`tasks` was refused a document write over REST (403) and could make the identical
write through `create_document`. Registering rather than rejecting also means
`tools/list` tells the truth, so an agent never picks a tool that will refuse it.

## Getting to it from outside

quire listens on one plain-HTTP port and terminates no TLS. Exposure is somebody
else's job: a Tailscale sidecar, a Cloudflare Tunnel, or an ordinary reverse proxy.
The app is told one thing about the outside world — `base_url` — and every URL it
hands out (share links, the WebAuthn RP ID, OAuth issuer and endpoint metadata) is
built from it. It is never inferred from the `Host` header, which an attacker
controls.

**The whole app is designed to be safely exposed**, which is why there is no
split-surface gate any more. `deploy/` carries three working compose samples
(tailnet-only, tailnet+funnel, Cloudflare Tunnel);
`internal/auth/exposure_test.go` is the fail-closed inventory asserting that
claim route by route. Every `/api/*` and `/mcp` request is authenticated
in-process by `auth.Middleware`; the only routes readable without a credential are
`/s/*` share pages (that is their purpose — the link is the secret), the SPA shell
(which holds no data; its API calls are checked), `/api/v1/health`, and the auth
endpoints that gate their own flows.

**This replaced tsnet, and the reason is worth recording.** v0.1.0 embedded
`tailscale.com/tsnet`, authenticated requests by tailnet identity (WhoIs), and used
Funnel to publish share pages while keeping everything else private. That split — one
listener serving two populations with different rights — was the entire source of a
critical vulnerability: Funnel proxies connections through Tailscale's ingress, so
public requests arrive carrying a *tailnet* source address that WhoIs resolves
happily, and the gate served the whole vault to anonymous callers. The fix
(`ipn.FunnelConn` + an address-range check, failing closed) worked, but the lesson
was that a per-request trust decision derived from network topology is a hard thing
to get right and an easy thing to get wrong silently. App-level auth on every route
is one decision, testable without a network.

Removing tsnet also dropped 318 of 819 Go packages (196 of them Tailscale's) and 23 MB
of binary — 65.5 MB to 41.7 MB, a third of the artifact. A sidecar gives back the same
TLS and MagicDNS, with the trust boundary at the proxy where it can be reasoned about.

## Importing an existing vault

quire indexes whatever markdown it is pointed at; there is no import step and
nothing is rewritten on the way in. Two rules make an existing library work
rather than merely survive:

- **Type inference is case-insensitive on the top-level directory**, because
  vaults that predate quire commonly use `People/` and `Projects/`. Matching
  only the lowercase form typed everything as a plain note and the entity
  model silently did nothing. A directory named differently (`Meeting Notes/`)
  stays a note — guessing at intent is worse than honouring an explicit
  frontmatter `type:`, which always wins.
- **A wikilink resolves to the page it names**, so `[[Page#Heading]]` and
  `[[Page^block]]` reach Page. Anchors used to resolve to nothing: `doctor`
  called them dangling and the target grew no backlink. The anchor is stripped
  for link resolution only — document *names* keep their `#`, or a page titled
  "C# Notes" would become unlinkable.

## The editor buffer

Read mode and the editor render one buffer, and while nothing is unsaved that
buffer *is* the server's version — `text = clean ? doc.markdown : publishedText`,
derived rather than stored. That matters more than it looks: the file changes
underneath the app routinely (a rendered task checkbox rewrites one line; vim
or Obsidian rewrites the whole thing and the watcher pushes an SSE event), and
a second copy of the text is a second thing that can go stale.

It previously *was* a stored copy, refreshed by a setState during render. The
helper deciding whether to refresh had unit tests and they passed; the update
was nonetheless dropped on a subsequent render, so an external edit refetched
correctly and still showed the old text on screen. Deriving removes the class
of bug rather than the instance — there is no copy left to lose.

Unsaved edits are never overwritten: while the buffer is dirty it holds the
user's text, and a genuine divergence surfaces as a 409 on the next save with
an explicit keep-mine / take-disk choice.

**A conflict freezes writing**, and that has to include blur and Cmd+S, not
just the idle autosave. Clicking "Keep mine" blurs the editor; before the
freeze covered that path, the blur fired an ordinary save carrying the same
stale base sha, which conflicted again and swallowed the user's choice — so
the button appeared to do nothing and the other version won. Only an explicit
override (keepMine, takeDisk) may write while a conflict is unresolved.

## Testing

Three layers, each for what only it can see:

- **Go unit and integration tests** — the service layer, the index, the
  markdown scanner, and every authorization decision. `internal/auth/exposure_test.go`
  is the fail-closed route inventory: anything touching the vault must be
  Protected(), and a new route that is not listed fails the suite rather than
  being assumed safe.
- **Frontend unit tests** (bun) — pure logic in `web/src/lib`.
- **Browser end-to-end** (Playwright, `mise run test:e2e`) — runs against
  real built binaries serving the embedded SPA. Two instances, because they
  need incompatible configurations: one with auth off, so every spec is one
  navigation from the thing it tests, and one in passkey mode on a
  non-loopback address for the WebAuthn ceremonies and the bootstrap
  enrollment gate. The ceremonies use Chromium's virtual authenticator, so
  they need no test-only bypass in the product. This layer
  exists for the seam between browser, SPA and Go, which is where several of
  this project's real bugs have lived: a document rendering two frontmatter
  blocks, a checkbox that updated the file but not the view, a CSP that
  silently blocked the bundled fonts, and `due: "today"` reaching the
  markdown as the literal word.

The last of those is the pattern worth naming: **the browser has repeatedly
found what a green Go suite could not.** Unit tests assert the behaviour you
thought about; the browser asserts the behaviour the user gets.

## Areas

An area is `area:` in frontmatter, lowercased on read. Discovered from the vault
and merged with two seeds (`work`, `personal`) so the Nirvana split exists on day
one; a third area is just frontmatter. `documents.area` is a column (schema v3)
so every listing, the task views (through their existing documents join) and
Today narrow with one `AND d.area = ?`. "Unclassified" is `area = '' AND type !=
'daily'`: daily notes are the capture spine and belong to every area, never to
none. The switcher is a per-device preference like the theme, and a typed
`area:` in the search grammar outranks it so a URL stays a whole query.

Chosen over fixed areas (a third one would need a code change) and over
directory-based areas (`work/people/` collides with the type directories and
breaks the imported-vault story). Decision made with the owner, 2026-09-02.

## Templates

`templates/` is a document type of its own, kept out of everyday listings and
search unless asked for by type. Two conventions and nothing else:
`templates/<type>.md` is that type's default and applies to any body-less
creation of that type (so `templates/daily.md` shapes every new day, with
`{{date}}` bound to the note's own day rather than to now); any other file
declares `for: <type>` and is offered by name. A template's remaining
frontmatter is copied in order into the new document, so a decision template
can carry `tags: [decision]` — which is how PM/dev/CTO document kinds (decision
records, incident reviews, 1:1s) exist without a new first-class type each.

The starter set is installed only from Settings, never automatically, and never
over an existing file: quire does not drop files into a vault unasked, and a
user's edited template must survive a re-install.

## Journal and tags

The journal (`/journal`) is a paged, newest-first read of daily notes, driven by
`GET /api/v1/daily?before=`. Daily paths sort lexically as dates, so "the ten notes
before this date" is one indexed query with no date parsing. Each day renders
through the same Markdown component as a document page, so task toggles work in
place — which exposed that a toggle resolves its task id through the *index* while
the page reads the *file*: after an external edit there is a debounce-wide window
where the file is ahead of the index and a toggle 404s. It self-heals in under a
second and is not worth a fallback scan, but the E2E test waits for the index
rather than the DOM for exactly that reason.

Tags are one concept with two spellings — `#tag` in prose, `tags:` in frontmatter —
merged at index time. `#tag` in prose renders as a link to the tag search; a purely
numeric `#123` is not a tag, matching Obsidian.

## Sharing

Shares live in auth.db (grants, not derivable from the vault): 16-char random token →
one document, optional expiry, soft revocation, view counts. `/s/<token>` renders a
standalone server-side page (goldmark, raw HTML escaped, wikilinks flattened to styled
text — their targets are private, callout markers become bold titles) under a strict
CSP (`default-src 'none'`; the page carries no JavaScript). Attachments are served
through the share only if the shared document **references** them — membership in the
goldmark AST's link/image set, not a substring scan of the markdown, which used to mean
that merely naming a file in prose published it. Markdown never serves through the file
route. Revoked/expired/nonexistent are indistinguishable 404s.
Share URLs are built from `base_url`, so they are only correct if it is.

## OAuth 2.1 for remote MCP

A built-in authorization server (internal/oauth) lets hosted clients (claude.ai
connectors, Claude Desktop) connect by URL alone: RFC 8414 metadata, RFC 7591 dynamic
client registration (capped unconsented registrations), PKCE-S256-only code flow,
1h access tokens + 30d rotating refresh tokens with reuse detection, RFC 7009
revocation. Public clients only. A 401 from /mcp carries the WWW-Authenticate
resource-metadata challenge — that header is the whole discovery mechanism.

Consent is the owner's, proved by a passkey session (or the loopback-only auth-none
listener, which nobody remote can reach by construction). Anyone else who lands on
/oauth/authorize is told to sign in rather than shown a form they could approve.
The consequence is worth stating plainly: **`token-only` mode cannot complete an
OAuth flow**, because it has no browser-shaped credential — quire warns about this
at startup. Run `passkey` if you want claude.ai connectors.

## Email

The provider abstraction is SMTP itself (every transactional provider exposes it);
internal/mail wraps wneessen/go-mail behind a Sender interface so an API transport
can slot in later. One consumer today: the morning digest (meetings, birthdays,
overdue, due, waiting) at QUIRE_DIGEST_TIME — quiet days send nothing.

## Printing / PDF

PDF export is the browser's, not the server's. Anything that renders Mermaid
needs a browser engine: embedding headless Chrome would add hundreds of
megabytes and a browser's attack surface to a distroless image, and pure-Go
PDF libraries cannot execute Mermaid's JavaScript at all. Printing the page
we already render gets diagrams, images, highlighted code, callouts and
tables for free, because it *is* the rendered DOM.

Three things make that produce a good PDF rather than a screenshot of an app:
print CSS hides the chrome and controls fragmentation (nothing breaks inside
a code block, callout, table row or image; headings never end a page; external
link targets are spelled out since paper has no hrefs); the light palette is
restated under `@media print` so a dark-mode session doesn't print white on
black; and Mermaid diagrams are re-rendered in their light theme first.
Cmd/Ctrl+P is intercepted so that last step can finish — `beforeprint` cannot
be awaited, so the diagram swap has to happen before the dialog opens. Share
pages carry their own copy of the same rules, since a recipient prints too.

Callout panels are tinted across their whole background rather than marked
with a left edge alone. That forces all eight types to have distinct hues
(note/info, tip/success and warning/question would otherwise be identical
panels), derived from the existing semantic tokens.

## Agent guidance

The MCP server's `instructions` are composed at session start from built-in
working rules plus the owner's own guidance, stored as `AGENTS.md` in the
vault. Vault-as-storage (rather than a settings row) is deliberate and follows
the product's premise: it is editable in the app *and* in vim, greppable, and
versioned by the vault's git repo. The server is constructed per MCP session,
so edited guidance reaches the next client without a restart.

## Frontend types are generated

`internal/service/apitypes.go` holds every wire shape, and `mise run gen`
(tygo) derives `web/src/api/generated.ts` from that file alone; `types.ts`
re-exports it. `mise run lint` fails when the checked-in output is stale, so a
renamed Go field breaks the build instead of surfacing as a runtime bug.
Pointers generate as `T | null` because Go marshals nil as `null`, not as an
absent key — getting that backwards is exactly the drift this prevents. Request
bodies (TaskEdit) stay hand-written: "key omitted = leave unchanged" is not a
shape generation can express.

## Auth modes

```yaml
auth:
  mode: passkey   # passkey | none | token-only
```

- `none` — local/dev mode. Singleton user, all scopes. **Refuses to start unless bound
  to loopback**; no override. That invariant is the whole safety story — the
  magic-cookie-URL idea is rejected (no real boundary on a single-user machine; URL
  secrets leak into history/screenshots). `mise run dev` and bare `quire serve` with no
  config default to this, print the URL.
- `passkey` — the mode for any human-facing install. `go-webauthn/webauthn`; server-side sessions
  (32-byte token, HttpOnly, SameSite=Lax, Secure off-localhost, 30-day sliding).
  Bootstrap: first visit registers the first passkey and prints 8 single-use recovery
  codes (argon2id-hashed). Recovery login forces new passkey registration and burns the
  code. **Claiming is gated**: on a non-loopback listener the first registration
  requires an 80-bit enrollment code minted at startup and printed to the log,
  because an unclaimed instance on a reachable address otherwise belongs to
  whoever loads it first — and `/api/v1/auth/status` advertises that it is
  unclaimed. Loopback listeners skip the gate; there is no remote to race.
  RP ID binds to `base_url`'s hostname — passkeys registered at one hostname do
  not work at another, so changing how you expose quire means re-registering.
- `token-only` — headless boxes: bearer tokens only, no browser login.

## Frontend

React 19 + TS + Tailwind v4 (`@theme` tokens, no config file), TanStack Router + Query,
CodeMirror 6, Lucide, Inter + JetBrains Mono. Dense-technical house style; calm, not
enterprise. Light/dark/system with pre-paint class script. Bundle budget ~500KB gz on
the critical path — Mermaid, syntax highlighter, lightbox lazy-loaded.

**Keyboard model: Linear/Gmail-style single keys, not modal vim.** Outside text inputs,
single keys are commands; inside editors, commands need modifiers. Focus is never
nowhere; `Escape` always goes one level out. Day-one keys: `Cmd+K` palette (`>`
commands, `#` tags, `@` people), `j/k` list movement, `Enter` open, `x` toggle task,
`e` edit, `Cmd+Enter` save+exit, `c` capture, `g` chords (`g t` Today, `g i` Inbox,
`g d` daily…), `/` search, `Cmd+[`/`Cmd+]` history, `s` snooze (typed natural dates),
`?` cheat sheet.

**Editor details that matter:** complete list continuation (Enter continues, empty item
exits, Tab indents), `[[` autocomplete <50ms with create-if-missing, `#` tag
autocomplete, paste-image inserts placeholder immediately and resolves in place (no
dialog ever), `Cmd+L` checkbox toggle, live-preview decorations that reveal raw source
when the cursor enters, frontmatter folded to a strip, no smart quotes, no autosave
churn (idle-debounced single write per burst).

**Views:** Today (home, not customizable: daily note + meetings + overdue/due/available
tasks + waiting + recent), Inbox, task views, per-type lists, document page
(read/edit; split view desktop-later), person/project/company/meeting pages with
SQL-driven rollups (backlinks, open tasks, recent meetings, last-interaction).

**Mobile is capture + glance + read**, and says so proudly: Today, quick capture
(≤3 taps, keyboard up, no required fields, no type-picker — everything lands in Inbox),
task list (check/snooze one-thumbed), and gorgeous read view. Bottom nav, FAB, 44px
targets, PWA-installable. Serious editing is a desktop activity.

Life-admin requirements folded in: person pages carry `role`/`birthday` and surface
last-interaction staleness; birthdays feed Today; capture-photo→task is the flagship
mobile gesture (v0.2); hard-dated tasks visually distinct from soft-recurring ones so
overdue-soft never screams.

## Build & release

`mise` implements the house contract (`setup/dev/test/lint/fmt/check/build`). Dev:
Go on `127.0.0.1:8321`, Vite on `:5173` proxying `/api` + `/mcp`. Prod: `web/dist`
embedded via `go:embed` (a `dev` build tag swaps in a proxy so the binary runs without
a frontend build). CI (GitHub Actions) runs `mise run check` on push/PR. Release on
tag: frontend build → GoReleaser → linux/darwin × amd64/arm64 binaries
(`CGO_ENABLED=0`), multi-arch `ghcr.io/jclement/quire` Docker image, Homebrew tap,
cosign signing — per the house `shipping` pattern.

## Key decisions

1. **Path as identity, no UUIDs** — grep-able vault, no metadata churn; rename is a UI
   operation that rewrites links with consent.
2. **Two databases** — index.db is disposable by construction (`quire reindex` proves
   it); auth.db holds the only state that can't be rebuilt. Backup = vault + auth.db
   + config.
3. **Git-backed vault, commit-only** — the vault is a local git repository (go-git;
   auto-init, debounced 60s auto-commit, flush on shutdown, `QUIRE_GIT=false` opts
   out). quire never merges, never pushes, never touches remotes: it is the vault's
   time machine, not a sync system. Add a remote yourself and push whenever; quire
   won't interfere. Staying in go-git's commit-only subset keeps git out of the
   Docker image.
4. **`auth: none` bound to loopback instead of a magic cookie** — a real invariant
   beats a decorative secret.
5. **One service layer under REST and MCP** — transports cannot drift on permissions.
6. **Coarse scopes (4)** — a single-user instance doesn't need a 14-scope matrix
   nobody configures correctly.
7. **modernc sqlite over CGO** — trivial cross-compilation and distroless images are
   worth ~2× query cost at personal-vault scale; escape hatch is a build tag.
8. **Content-hash task IDs** — no `🆔` churn in files; rewording recreates a task,
   accepted.
9. **Daily note is a first-class type** — it's the capture spine and Today anchor, with
   behavior no other note has (auto-create, date nav, template).
10. **People/companies/projects capture full metadata from day one but render as
    rollup pages** — the aggregation views deepen over releases; the data (links in
    frontmatter) is right from the first meeting note, so richer pages light up
    retroactively.
11. **Exposure is a sidecar's job, not the app's** — quire embedded tsnet through
    v0.1.1 and split its own surface between tailnet and Funnel visitors. That split
    caused a critical vulnerability (see "Getting to it from outside"), pulled in 196
    Tailscale packages, and bought something a five-line sidecar container gives back.
    The app authenticates every route itself and is safe to expose whole; the tunnel
    is transport.

## v0.1 — in / out

**In:** serve/reindex/doctor/token; vault CRUD + watcher + FTS5 search; notes, daily
notes, meetings, tasks fully; person/project/company as typed stubs with backlink+task
rollups; editor (edit/read/split) with the details above; palette; Today; Inbox; quick
capture; image paste; Mermaid + callouts + highlighting; MCP over bearer tokens;
Docker + compose; responsive mobile; **sharing
links** (the "sitter link"); **recurrence with lead time** (`🔁 every N unit [when
done]`; the due−defer gap is the lead time and carries forward);
**rename-with-link-rewriting** (only links that stop resolving get rewritten; the old
name becomes the alias so prose reads unchanged); **task edit/snooze** (surgical
marker rewrites); **CLI verbs** (`quire task add/search/today` over the HTTP API);
**passkeys + recovery codes** (bootstrap-claimable, then session-gated).

**Out (v0.2+):** import wizard (mostly moot — point the data dir at an existing
vault and reindex), offline service worker, in-app vault history browser (the git
history exists; browsing/restoring is UI work), multi-user (indefinitely deferred).

## Performance budgets (10k docs / 50k tasks)

Cold start <1s; full reindex <10s, never blocks serving; search <50ms server-side;
palette open <100ms; page nav <200ms; editor open (1MB file) <300ms; task toggle and
capture optimistic (0ms perceived).

## Known limitations

- Rewording a task in markdown resets its identity (completion history survives via
  `✅` stamps in the file).
- External renames dangle backlinks until fixed (`quire doctor` lists them).
- No merge on conflict — the user picks; the losing version is preserved as a
  `*.conflict-<timestamp>.md` file, surfaced in the UI.
- `auth: none` trusts the machine boundary entirely (by design; loopback-only).
- Single vault per instance.
