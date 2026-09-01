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

## MCP

Official `modelcontextprotocol/go-sdk`, Streamable HTTP at `/mcp`, bearer-token auth
(OAuth 2.1 + DCR is v0.2 — on a tailnet, tokens cover Claude Code). Tools are thin
wrappers over the same service layer as REST, so permissions cannot drift:

`search`, `get_document`, `create_note`, `update_document` (with `base_sha256`
precondition), `append_to_document` (targeted section append — agents must not rewrite
whole files), `create_person/project/meeting`, `list_tasks`, `create_task` (natural
dates parsed server-side, resolved ISO echoed back; ambiguous name resolution returns
candidates, never guesses), `complete_task`, `today` (the flagship composed call), and
`person_context` (the rollup a person page shows, as JSON — meeting prep in one call).

Agent guardrails: read-only tokens are the default posture; no delete tool; every
API/MCP write is audit-logged (principal, tool, path, when).

## Tailscale (tsnet) — first-class

Setting `ts_hostname` makes quire join the tailnet as its own node (tsnet, state in
`.quire/tsnet/`, auth key needed only until registered). It serves
`https://<hostname>.<tailnet>.ts.net` with Tailscale-managed TLS, and requests are
authenticated by **tailnet identity**: WhoIs on the connection resolves a tailnet peer
to the owner principal — no passkeys or tokens needed on-tailnet. `ts_owner` optionally
pins access to one login. A bearer token presented over the tailnet is still honored
*with its scopes* — a deliberately read-only agent token keeps its reduced blast
radius.

With `ts_funnel: true`, the same `:443` listener also accepts public internet traffic
(Tailscale Funnel). The per-request gate is the whole public story: WhoIs succeeds →
tailnet peer, full app; WhoIs fails → public visitor, `/s/*` share pages only, and
every other path (including `/api/v1/health`) 404s so the internet can't even learn
quire exists. Rejected alternative: a separate funnel port — one listener with an
identity gate has fewer moving parts and can't be misconfigured into exposing the app.

## Sharing

Shares live in auth.db (grants, not derivable from the vault): 16-char random token →
one document, optional expiry, soft revocation, view counts. `/s/<token>` renders a
standalone server-side page (goldmark, raw HTML escaped, wikilinks flattened to styled
text — their targets are private, callout markers become bold titles). Attachments are
served through the share only if the shared document references them; markdown never
serves through the file route. Revoked/expired/nonexistent are indistinguishable 404s.
Share URLs advertise the tailnet/funnel hostname automatically when tsnet is up.

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
- `passkey` — for non-tailnet HTTPS deployments. `go-webauthn/webauthn`; server-side sessions
  (32-byte token, HttpOnly, SameSite=Lax, Secure off-localhost, 30-day sliding).
  Bootstrap: first visit registers the first passkey and prints 8 single-use recovery
  codes (argon2id-hashed). Recovery login forces new passkey registration and burns the
  code. RP ID binds to the hostname — document loudly that passkeys registered at
  `quire.tailXXXX.ts.net` only work there.
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
3. **No git integration in v0.1** — the layout is already git-friendly; the user can
   `git init` the vault today. Later: go-git, commit-only (debounced auto-commit +
   history view), never merge/push — keeps us in go-git's reliable subset and git out
   of the Docker image.
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
11. **Passkeys are v0.1-late, not v0.1-gate** — `none` mode on the tailnet covers the
    founder immediately; auth lands before anyone else touches it.

## v0.1 — in / out

**In:** serve/reindex/doctor/token; vault CRUD + watcher + FTS5 search; notes, daily
notes, meetings, tasks fully; person/project/company as typed stubs with backlink+task
rollups; editor (edit/read/split) with the details above; palette; Today; Inbox; quick
capture; image paste; Mermaid + callouts + highlighting; MCP over bearer tokens;
Docker + compose; responsive mobile; **Tailscale/tsnet with funnel**; **sharing
links** (the "sitter link"); **recurrence with lead time** (`🔁 every N unit [when
done]`; the due−defer gap is the lead time and carries forward);
**rename-with-link-rewriting** (only links that stop resolving get rewritten; the old
name becomes the alias so prose reads unchanged); **task edit/snooze** (surgical
marker rewrites); **CLI verbs** (`quire task add/search/today` over the HTTP API);
**passkeys + recovery codes** (bootstrap-claimable, then session-gated).

**Out (v0.2+):** email digest (needs SMTP decisions), OAuth for MCP, go-git history,
import wizard, photo→task capture gesture, multi-user (indefinitely deferred).

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
