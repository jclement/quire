# quire

The best damn notes app: a self-hosted personal knowledge & work hub in one Go binary.
Markdown on disk is the source of truth; quire adds first-class People, Companies,
Projects, Meetings, Daily Notes and real task management on top — fast, keyboard-first,
beautiful on mobile, and operable by AI agents over MCP.

> Markdown owns the knowledge. The application owns the experience.

See [DESIGN.md](DESIGN.md) for architecture and decisions; [idea.md](idea.md) is the
original product spec.

## Quick start (dev)

```sh
git clone https://github.com/jclement/quire
cd quire
mise install
mise run setup
mise run dev        # single-user, no auth, http://localhost:5173 (Vite → Go on :8321)
```

The dev instance runs with `QUIRE_AUTH_MODE=none` against a scratch vault in
`tmp/data/`. `mise run dev:reset` wipes local state.

## Installing the CLI

The same binary is both the server and the terminal client, so on a laptop
you usually just want the client, pointed at a quire running elsewhere:

```sh
brew install jclement/tap/quire
export QUIRE_URL=https://quire.example.com QUIRE_TOKEN=sk_…
quire today
```

Or grab a binary from [releases](https://github.com/jclement/quire/releases).

## Running for real

Three complete, copy-and-run deployments live in [deploy/](deploy/) — pick the one
that matches how you want to reach it, `cp .env.sample .env`, fill in a few values,
`docker compose up -d`:

| Sample | Reachable from |
|---|---|
| [`deploy/tailscale`](deploy/tailscale/) | your tailnet only |
| [`deploy/tailscale-funnel`](deploy/tailscale-funnel/) | your tailnet + the public internet |
| [`deploy/cloudflare-tunnel`](deploy/cloudflare-tunnel/) | the public internet, on your own domain |

Images are published to ghcr:

| Tag | What it is |
|---|---|
| `:edge` | Every push to `main` — what to run if you want today's work |
| `:main-<sha>` | One specific commit, to pin or roll back to |
| `:latest`, `:x.y.z` | Tagged releases (also published as signed binaries) |

Create API tokens in **Settings → API tokens**, or from the CLI:

```sh
docker compose exec quire quire token create claude read write tasks
```

Point Claude Code (or any MCP client) at `https://<host>/mcp` with that bearer token.
Settings also lists every connected OAuth app and every live share link, each with a
one-click revoke — that page is the answer to "what can currently reach my notes?"

A complete backup is `data/vault/` + `data/.quire/auth.db` + `data/.quire/config.yaml`.
The index is disposable: `quire reindex` rebuilds it from markdown. `quire doctor`
lists dangling wikilinks.

### Getting to it from outside

quire speaks plain HTTP on one port and does not terminate TLS. Put it behind
something that does. It has no opinion about which, and needs to know only one
thing: **`QUIRE_BASE_URL` must be the URL you actually reach it at** — share
links, passkey RP ID, and OAuth discovery are all built from it.

Three shapes that work, cheapest first:

**Tailscale sidecar** — private, no public exposure at all; the tailnet supplies
HTTPS and MagicDNS. Add Funnel to the same serve config when you want share links
and claude.ai connectors to work from the internet.

**Cloudflare Tunnel** — a public hostname on your own domain with no inbound ports.

**Your own reverse proxy** — Caddy, nginx, Traefik. Nothing special required; just
forward to `:8321` and set `QUIRE_BASE_URL`.

Whichever you pick, **quire's own auth is what protects it** — the proxy is transport,
not a security boundary. Every `/api/*` and `/mcp` request is authenticated in-process,
so exposing the whole app is a supported deployment, not a workaround. Run it in
`passkey` mode if a human uses it.

### Sharing

Any document can get a revocable share link (`/s/<token>`): optional expiry, view
counts, referenced attachments included, rendered as a clean standalone page. Share
pages are the one route that is deliberately readable without credentials — anyone
holding the link can read that document, so the link is the secret. Revoking kills it
instantly.

### Auth modes

- `none` — loopback only (enforced at startup). Dev and "just run it on my laptop".
- `token-only` — bearer tokens (`quire token create`). Headless/agents.
- `passkey` — WebAuthn: the first registration claims the instance and issues 8
  single-use recovery codes; sessions are server-side cookies; more passkeys
  manageable once logged in. On a non-loopback listener that first registration
  needs the **enrollment code** printed in the server log at startup, so a
  publicly-reachable instance cannot be claimed by whoever finds it first. `QUIRE_BASE_URL`'s hostname is the RP ID — passkeys only work at that
  exact hostname — and it must be a **hostname, not an IP**: WebAuthn binds
  credentials to a domain, so quire refuses to start in passkey mode with an
  IP in `QUIRE_BASE_URL`. This is the mode to use for anything a human logs
  into, and the only mode in which OAuth consent can be approved.

### Bringing an existing vault

Point `QUIRE_DATA_DIR` at a directory whose `vault/` is your existing markdown
and run `quire reindex`. Nothing is rewritten: frontmatter, directory
structure, attachments and wikilinks are preserved as they are.

Type inference reads the top-level directory case-insensitively, so `People/`,
`Projects/`, `Meetings/`, `Companies/` and `Daily/` are recognised alongside
their lowercase forms. A directory named something else — `Meeting Notes/` —
stays a plain note; add `type: meeting` to a document's frontmatter to type it
explicitly, which always wins over the directory.

`[[Page]]`, `[[Page|alias]]` and `[[Page#Heading]]` all resolve to Page.
`quire doctor` lists anything that does not.

### Git-backed vault

The vault is a local git repository by default: auto-initialized, with debounced
auto-commits after edits settle and a flush on shutdown. quire never pushes or merges
— add your own remote and push whenever you like. `QUIRE_GIT=false` opts out.

### Life admin

Tasks recur with real semantics: `- [ ] Renew registration 🔁 every year 🛫 2026-08-10
📅 2026-08-31` — completing it mints next year's line, and the 🛫→📅 gap (your lead
time) carries forward. `🔁 every 3 months when done` repeats from the completion day
instead. Person pages with a `birthday:` field feed a birthdays section on Today.
Mobile capture is one gesture: photo → dated task with the image attached
(`POST /api/v1/capture`).

### Relationships

Entities relate through wikilinks, either in prose (`met [[Sarah Chen]] about
[[Project Apollo]]`) or in frontmatter (`company: "[[Acme]]"`,
`people: ["[[Sarah Chen]]"]`). Both are indexed, so either produces a backlink
on the target, and person/company/project pages assemble themselves from them.

### Editing

A new document opens straight into the editor, in whichever of Edit or Split
you used last (remembered per device, like the theme). `e` edits, ⌘E cycles
read → edit → split, and Escape or Read flushes the save. In Split, the
preview's checkboxes flip the task in the editor buffer, so they save with
whatever else you are typing.

A toolbar sits above the editor: Heading (H1–H3 or plain), Task (make the
line a task), Details (due, defer, priority, waiting, repeat — the emoji
grammar, written for you), Callout, Table (insert; Reformat and Edit as
grid wake up inside one), and Drawing. Everything acts on the cursor line.
The properties strip — area, tags, people, companies — stays while editing:
a change flushes the buffer, rewrites the file, and loads the result back
into the editor, so nothing typed is lost.

### Areas

Optional, Nirvana-style **work / personal / unclassified**, as a frontmatter key:

```yaml
area: work
```

Define areas in **Settings → Areas**, each with a colour. Nothing area-shaped
appears until you have two or more; then a switcher at the top of the sidebar
narrows Browse, Search, Tasks and Today to one area, remembers your choice per
device, and files new documents under it. Every document page gets an area chip
to re-file it in place, and its colour shows as a dot wherever the area does.
Daily notes never have an area — they belong to every one. `area:work` works in
search too, and the CLI/MCP accept an area on listing and creation.

The switcher is a badge in the sidebar — "Area: all", or the coloured dots
and names of what is selected — and its picker takes several areas at once,
so Work and Personal can be on together (the API and MCP take the same thing
as a comma-separated list: `area=work,personal`). A document shows its own
badge, "Area: unassigned" or its area in colour, and clicking it re-files
the document.

### Templates

Templates are ordinary markdown under `templates/`. `templates/meeting.md` shapes
every new meeting (likewise `person`, `project`, `daily`); any other file with
`for: note` in its frontmatter is offered by name in the New dialog. `{{title}}`,
`{{date}}`, `{{time}}` and `{{datetime}}` expand; the template's remaining
frontmatter (say `tags: [decision]`) is copied into the new document.

**Settings → Install starter templates** writes a working set for someone who
runs engineering — meeting, 1:1, decision record, project brief, incident
review, weekly review, and a daily-note shape — without touching any template
you already have.

### Journal

`/journal` is every daily note on one scrolling page, today first, history
loading as you scroll. Checkboxes toggle in place; each day's heading opens it
for editing. The Daily page has a Journal toggle and the sidebar links both.

### Tables

Hover a rendered table and click **Edit table** to work on it as a grid: type
into cells, add or remove rows and columns, click a column header to cycle its
alignment, and save. Pipes and line breaks typed into a cell are escaped for
you. In the editor, a "Table" panel appears while the cursor is inside one,
with **Reformat table** (⌘⌥T, pads every column to its widest cell), **Edit as
grid**, and Tab moving between cells. Tables inside fenced code are left alone.

### Drawings

Run **Insert drawing** from the palette or the document's ⋯ menu to drop an [Excalidraw](https://excalidraw.com)
sketch into the note at the cursor (or onto the end, from read mode) and start
drawing full-screen. Saving writes two files under `attachments/`: the
`.excalidraw` scene and a `.excalidraw.svg` render. The note embeds the
render as an ordinary image, so share pages, print, PDF export, vim and every
other markdown viewer show the picture; the app also knows the SVG has a
source next to it, so hovering it offers **Edit drawing**. Excalidraw's fonts
are served from the binary — nothing is loaded from a CDN. The CJK face is
left out to save 12MB; CJK text in a drawing falls back to a system font.

### Tags

`#tags` in prose and `tags:` in frontmatter are the same thing. The `tags:`
list is editable as chips under the title: each links to its search and
removes with ×, and "+" offers the vault's existing tags or takes a new one.
`/tags` lists
every tag sized by use; tags in prose, in Browse rows and on the tags page all
link to the `tag:x` search. Purely numeric `#123` is not a tag.

### Calendar

`/calendar` shows the month at a glance: which days have a daily note, what
documents you touched, meetings held, tasks completed.

### Search

One grammar everywhere (UI, API, MCP, CLI): full-text terms, `type:meeting`,
`tag:x`, and task search with `is:task`, `due:today`, `due:overdue`, `due:week`,
`due:2026-09-15`.

**Semantic search** is opt-in: set `QUIRE_OPENAI_API_KEY` and a **Semantic**
toggle appears on the search page (and `?mode=semantic` on the API, plus
`semantic_search` / `related_documents` tools for agents). Notes are chunked
by heading, embedded with `text-embedding-3-small` (512 dimensions, a few
cents per thousand notes), kept up to date as you edit, and ranked by cosine
similarity; each document's rail also lists the notes nearest to it in
meaning under "Similar" (as distinct from "Linked from", which is real
backlinks) — only ones that stand clearly above the note's similarity to
the vault at large, and never for a note with no body yet (hover an entry
for its score). Re-embedding happens per heading section, only for
sections whose text changed, and only after a note has been quiet for 30
seconds, so a writing session costs one pass, not one per autosave. Be clear about what this means: **note text is sent to that
embeddings endpoint.** Nothing is sent with the key unset. Any
OpenAI-compatible server works via `QUIRE_OPENAI_BASE_URL` (Ollama, LiteLLM),
which keeps the text on your own machine. Embeddings live in `index.db` and
are rebuilt after a `quire reindex`.

## CLI

The same binary talks to a running quire (`QUIRE_URL`, `QUIRE_TOKEN` env):

```sh
quire task add "Send Sarah the diagram" --due fri
quire search "type:meeting acme"
quire today
```

## Configuration

| Env var | Default | Meaning |
|---|---|---|
| `QUIRE_DATA_DIR` | `./data` | Data directory (vault + app state) |
| `QUIRE_ADDR` | `127.0.0.1:8321` | Listen address |
| `QUIRE_BASE_URL` | `http://localhost:8321` | External URL (WebAuthn RP ID binds to its hostname) |
| `QUIRE_AUTH_MODE` | `none` | `none` (loopback only, enforced) \| `token-only` \| `passkey` |
| `QUIRE_LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `QUIRE_GIT` | `true` | Git-backed vault (auto-init + debounced auto-commit) |
| `QUIRE_UPDATE_CHECK` | `true` | Ask GitHub once a day whether a newer release exists |
| `QUIRE_TRUSTED_PROXIES` | _(none)_ | Proxy IPs/CIDRs (or `any`) whose `X-Forwarded-For` may be believed — **set this behind a tunnel**, or rate limiting sees one client |
| `QUIRE_SMTP_HOST/PORT/USER/PASS/FROM` | | SMTP relay (any provider's SMTP endpoint) |
| `QUIRE_OPENAI_API_KEY` | _(none)_ | Turns on semantic search — **sends note text to the embeddings endpoint** |
| `QUIRE_OPENAI_BASE_URL` | `https://api.openai.com/v1` | Any OpenAI-compatible embeddings API |
| `QUIRE_EMBEDDING_MODEL` | `text-embedding-3-small` | Embeddings model |
| `QUIRE_EMBEDDING_COOLDOWN` | `30s` | How long a note sits unchanged before its changed sections are re-embedded |
| `QUIRE_DIGEST_TO` / `QUIRE_DIGEST_TIME` | | Daily digest recipient and local HH:MM |
| `QUIRE_URL` / `QUIRE_TOKEN` | | CLI verbs: which quire to talk to, and as whom |

## Agents (MCP)

quire tells every connecting agent how to behave: built-in working rules
(prefer appends over whole-file writes, how relationships and the task grammar
work, which composed tools to reach for) plus **your own guidance**, written in
Settings and stored as an ordinary vault document (`AGENTS.md`). Edit it in the
app or in vim; the next agent session gets it, no restart.

quire is agent-operable by design: a Streamable-HTTP MCP server at `/mcp` exposes the
same service layer as the UI. Sixteen tools, each annotated (read-only / additive /
destructive) so clients know what deserves a confirmation:

| Scope | Tools |
|---|---|
| read | `search` (full-text + `type:` `tag:` `is:task` `due:`), `semantic_search` and `related_documents` (when a key is set), `list_documents`, `get_document`, `get_daily`, `list_tasks`, `list_tags`, `today`, `person_context` |
| write | `create_document`, `append_to_document`, `update_document` (hash-guarded), `link_entity`, `set_frontmatter` |
| tasks | `create_task`, `complete_task`, `edit_task` (snooze / reprioritise) |

There is deliberately no delete tool. **Every mutating tool call and every REST
write by a token or connected app is recorded** — Settings → Agent activity shows
who did what, where, and whether it succeeded. Your own edits in the browser are
not logged; the question the log answers is "what did the agents do?".

Two credential paths, per the house pattern:

- **Named bearer tokens** (Claude Code, scripts):

  ```sh
  claude mcp add quire --transport http http://localhost:8321/mcp   # dev, no auth
  claude mcp add quire --transport http https://<host>/mcp --header "Authorization: Bearer sk_…"
  ```

  **Scopes decide which tools the agent sees.** A `read` token gets `search`,
  `get_document`, `list_tasks`, `today` and `person_context`; `tasks` adds
  `create_task`/`complete_task`; `write` adds the document-writing tools. The
  toolset is built per request, so `tools/list` is honest and an agent is never
  offered a tool it cannot use.

- **OAuth 2.1 + dynamic client registration** (claude.ai connectors, Claude Desktop):
  paste `https://<host>/mcp` into Settings → Connectors and quire handles the rest —
  RFC 8414 metadata, DCR, PKCE-only code flow, rotating refresh tokens. Consent is
  approved in the browser with your passkey session, so **this path needs
  `QUIRE_AUTH_MODE=passkey`** — in `token-only` mode there is no browser login and
  the consent page cannot be approved by anyone.

## Tasks

| Task | Does |
|---|---|
| `mise run setup` | Zero → runnable: Go deps, frontend deps, dev vault |
| `mise run test:e2e` | Browser end-to-end tests (Playwright) against a real built binary |
| `mise run dev` | Dev instance with HMR (no auth, localhost) — copy `mise.local.toml.sample` to `mise.local.toml` to point it at real data and your tailnet |
| `mise run test` | Go (`-race`) + frontend tests |
| `mise run lint` | vet + format-check + typecheck (never mutates) |
| `mise run fmt` | Format everything |
| `mise run check` | lint + test — the done gate |
| `mise run build` | Frontend build → embedded → `bin/quire` |
| `mise run dev:reset` | Wipe local dev state |

`quire backup [file]` writes a tar.gz of the vault + auth.db (snapshotted safely) +
config; restore by extracting into an empty data dir and running `quire reindex`.

### Email

Mail goes out over SMTP — every provider (Mailgun, SES, Postmark, Resend) exposes an
SMTP endpoint, so switching providers is four env vars. With `QUIRE_DIGEST_TO` and
`QUIRE_DIGEST_TIME=06:30` set, the server emails a morning digest (meetings,
birthdays, overdue, due today, waiting); quiet days send nothing. `quire digest`
sends one on demand (cron-able).

## License

MIT © 2026 Jeff Clement

Bundles [Excalidraw](https://github.com/excalidraw/excalidraw) (MIT) for
drawings.
