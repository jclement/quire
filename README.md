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
`tmp/vault/`. `mise run dev:reset` wipes local state.

## Running for real

See [docker-compose.example.yml](docker-compose.example.yml). Create API tokens with:

```sh
docker compose exec quire quire token create claude read write tasks
```

Point Claude Code (or any MCP client) at `https://<host>/mcp` with that bearer token.

A complete backup is `data/vault/` + `data/.quire/auth.db` + `data/.quire/config.yaml`.
The index is disposable: `quire reindex` rebuilds it from markdown. `quire doctor`
lists dangling wikilinks.

### Tailscale (first-class)

Set `QUIRE_TS_HOSTNAME` and quire joins your tailnet as its own node via tsnet — no
sidecar, no port publishing:

```yaml
environment:
  QUIRE_TS_HOSTNAME: quire
  QUIRE_TS_AUTHKEY: tskey-auth-…   # first boot only; state persists in /data/.quire/tsnet
  QUIRE_TS_FUNNEL: "true"          # expose ONLY /s/* share pages to the public internet
```

quire then serves `https://quire.<tailnet>.ts.net` with automatic TLS, and requests are
authenticated by **tailnet identity** (WhoIs) — no login at all on-tailnet. Optional
`QUIRE_TS_OWNER=you@example.com` restricts access to one tailnet login. Bearer tokens
still work over the tailnet and keep their scopes (a read-only agent token stays
read-only). With funnel on, the public internet sees exactly one surface: share pages.
Everything else 404s.

### Sharing

Any document can get a revocable share link (`/s/<token>`): optional expiry, view
counts, referenced attachments included, rendered as a clean standalone page. With
funnel enabled the link works from anywhere; revoking it kills it instantly.

### Auth modes

- `none` — loopback only (enforced at startup). Dev and "just run it on my laptop".
- `token-only` — bearer tokens (`quire token create`). Headless/agents.
- `passkey` — WebAuthn: first visit registers the first passkey and issues 8 single-use
  recovery codes; sessions are server-side cookies; more passkeys manageable once
  logged in. `QUIRE_BASE_URL`'s hostname is the RP ID — passkeys only work at that
  exact hostname. On a tailnet you rarely need this: tailnet identity already
  authenticates you.

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
| `QUIRE_TS_HOSTNAME` | _(off)_ | Set to join the tailnet as this hostname (tsnet) |
| `QUIRE_TS_AUTHKEY` | | Tailscale auth key — needed on first boot only |
| `QUIRE_TS_FUNNEL` | `false` | Publish `/s/*` share pages via Tailscale Funnel |
| `QUIRE_TS_OWNER` | _(any member)_ | Restrict tailnet access to this login |
| `QUIRE_URL` / `QUIRE_TOKEN` | | CLI verbs: which quire to talk to, and as whom |

## Agents (MCP)

quire is agent-operable by design: a Streamable-HTTP MCP server at `/mcp` exposes the
same service layer as the UI — `search`, `get_document`, `create_document`,
`update_document` (hash-guarded), `append_to_document`, `list_tasks`, `create_task`,
`complete_task`, `today`, and `person_context` (meeting prep in one call). There is
deliberately no delete tool. Add to Claude Code:

```sh
claude mcp add quire --transport http http://localhost:8321/mcp   # dev, no auth
claude mcp add quire --transport http https://<host>/mcp --header "Authorization: Bearer sk_…"
```

## Tasks

| Task | Does |
|---|---|
| `mise run setup` | Zero → runnable: Go deps, frontend deps, dev vault |
| `mise run dev` | Dev instance with HMR (no auth, localhost) |
| `mise run test` | Go (`-race`) + frontend tests |
| `mise run lint` | vet + format-check + typecheck (never mutates) |
| `mise run fmt` | Format everything |
| `mise run check` | lint + test — the done gate |
| `mise run build` | Frontend build → embedded → `bin/quire` |
| `mise run dev:reset` | Wipe local dev state |

## License

MIT © 2026 Jeff Clement
