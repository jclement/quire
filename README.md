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

See [docker-compose.example.yml](docker-compose.example.yml). The image defaults to
`token-only` auth (passkey mode is designed but not implemented yet). Create tokens with:

```sh
docker compose exec quire quire token create claude read write tasks
```

Point Claude Code (or any MCP client) at `https://<host>/mcp` with that bearer token.

A complete backup is `data/vault/` + `data/.quire/auth.db` + `data/.quire/config.yaml`.
The index is disposable: `quire reindex` rebuilds it from markdown. `quire doctor`
lists dangling wikilinks.

## Configuration

| Env var | Default | Meaning |
|---|---|---|
| `QUIRE_DATA_DIR` | `./data` | Data directory (vault + app state) |
| `QUIRE_ADDR` | `127.0.0.1:8321` | Listen address |
| `QUIRE_BASE_URL` | `http://localhost:8321` | External URL (WebAuthn RP ID binds to its hostname) |
| `QUIRE_AUTH_MODE` | `none` | `none` (loopback only, enforced) \| `token-only` \| `passkey` (not yet implemented) |
| `QUIRE_LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |

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
