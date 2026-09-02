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

## Running for real

See [docker-compose.example.yml](docker-compose.example.yml). Images are published
to ghcr:

| Tag | What it is |
|---|---|
| `:edge` | Every push to `main` — what to run if you want today's work |
| `:main-<sha>` | One specific commit, to pin or roll back to |
| `:latest`, `:x.y.z` | Tagged releases (also published as signed binaries) |

Create API tokens with:

```sh
docker compose exec quire quire token create claude read write tasks
```

Point Claude Code (or any MCP client) at `https://<host>/mcp` with that bearer token.

A complete backup is `data/vault/` + `data/.quire/auth.db` + `data/.quire/config.yaml`.
The index is disposable: `quire reindex` rebuilds it from markdown. `quire doctor`
lists dangling wikilinks.

### Getting to it from outside

quire speaks plain HTTP on one port and does not terminate TLS. Put it behind
something that does. It has no opinion about which, and needs to know only one
thing: **`QUIRE_BASE_URL` must be the URL you actually reach it at** — share
links, passkey RP ID, and OAuth discovery are all built from it.

Three shapes that work, cheapest first:

**Tailscale sidecar** — private, no public exposure at all. Run `tailscale/tailscale`
alongside quire in the same network namespace (`network_mode: service:tailscale`) with
`TS_SERVE_CONFIG` pointing at quire's port; the tailnet gives you HTTPS and MagicDNS,
and you reach it at `https://quire.<tailnet>.ts.net`. Flip on Funnel in that same serve
config when you want share links or claude.ai connectors to work from the internet.
`docker-compose.example.yml` has this wired up.

**Cloudflare Tunnel** — a public hostname with no inbound ports. Run `cloudflared`
as a sidecar with a token from the Zero Trust dashboard, route your hostname to
`http://quire:8321`, and set `QUIRE_BASE_URL` to that hostname. This is the one to
pick if you want a URL you can hand to someone.

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
- `passkey` — WebAuthn: first visit registers the first passkey and issues 8 single-use
  recovery codes; sessions are server-side cookies; more passkeys manageable once
  logged in. `QUIRE_BASE_URL`'s hostname is the RP ID — passkeys only work at that
  exact hostname. This is the mode to use for anything a human logs into, and the
  only mode in which OAuth consent can be approved.

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

### Calendar

`/calendar` shows the month at a glance: which days have a daily note, what
documents you touched, meetings held, tasks completed.

### Search

One grammar everywhere (UI, API, MCP, CLI): full-text terms, `type:meeting`,
`tag:x`, and task search with `is:task`, `due:today`, `due:overdue`, `due:week`,
`due:2026-09-15`.

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
| `QUIRE_SMTP_HOST/PORT/USER/PASS/FROM` | | SMTP relay (any provider's SMTP endpoint) |
| `QUIRE_DIGEST_TO` / `QUIRE_DIGEST_TIME` | | Daily digest recipient and local HH:MM |
| `QUIRE_URL` / `QUIRE_TOKEN` | | CLI verbs: which quire to talk to, and as whom |

## Agents (MCP)

quire tells every connecting agent how to behave: built-in working rules
(prefer appends over whole-file writes, how relationships and the task grammar
work, which composed tools to reach for) plus **your own guidance**, written in
Settings and stored as an ordinary vault document (`AGENTS.md`). Edit it in the
app or in vim; the next agent session gets it, no restart.

quire is agent-operable by design: a Streamable-HTTP MCP server at `/mcp` exposes the
same service layer as the UI — `search`, `get_document`, `create_document`,
`update_document` (hash-guarded), `append_to_document`, `list_tasks`, `create_task`,
`complete_task`, `today`, and `person_context` (meeting prep in one call). There is
deliberately no delete tool.

Two credential paths, per the house pattern:

- **Named bearer tokens** (Claude Code, scripts):

  ```sh
  claude mcp add quire --transport http http://localhost:8321/mcp   # dev, no auth
  claude mcp add quire --transport http https://<host>/mcp --header "Authorization: Bearer sk_…"
  ```

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
