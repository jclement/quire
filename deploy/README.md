# Deployment samples

Three complete, copy-and-run deployments. Each directory holds a
`docker-compose.yml` and a `.env.sample` — copy the sample to `.env`, fill in
the handful of values it asks for, and `docker compose up -d`.

| Directory | Reachable from | Use it when |
|---|---|---|
| [`tailscale/`](tailscale/) | your tailnet only | Private notes. Nothing is on the public internet. |
| [`tailscale-funnel/`](tailscale-funnel/) | your tailnet + the public internet | You want share links to work for other people, and claude.ai connectors to reach `/mcp`. |
| [`cloudflare-tunnel/`](cloudflare-tunnel/) | the public internet, on your own domain | You want a memorable URL like `notes.example.com`. |
| [`gatecrash/`](gatecrash/) | the public internet, through your own tunnel server | Same as above, but the entry point is a box you run rather than Cloudflare. |

All three run the same image and differ only in what sits in front of it.

## The one thing that matters in every setup

**`QUIRE_BASE_URL` must be the URL you actually reach quire at.** It is not
cosmetic and quire cannot work it out for itself — it is deliberately never
read from the `Host` header, which a caller controls. Three things are built
from it:

- **share links** — wrong value, wrong links
- **the WebAuthn RP ID** — passkeys are bound to this hostname and stop
  working if it changes, so pick the URL you intend to keep
- **OAuth discovery metadata** — wrong value and connectors fail with no
  useful error

## Claiming a new instance

quire in passkey mode starts out unclaimed, and an unclaimed instance on a
reachable address would otherwise belong to whoever loads it first. So the
first registration needs an **enrollment code**, minted at startup and
printed to the log:

```sh
docker compose logs quire | grep enrollment_code
```

Enter it on the first-run screen along with your passkey. The code changes on
every restart and stops working the moment a passkey exists. (A loopback-only
instance skips this — there is nobody to race.)

Keep the 8 recovery codes it gives you. They are the only way back in if you
lose every passkey.

## Auth modes

- **`passkey`** — what you want for anything a human logs into, and the only
  mode where OAuth consent can be approved (claude.ai connectors need it).
- **`token-only`** — bearer tokens only, no browser login. Fine for a headless
  box driven entirely by agents; OAuth cannot complete in this mode.
- **`none`** — refuses to start unless bound to loopback. Dev only.

## Agent access

Named bearer tokens work in every mode:

```sh
docker compose exec quire quire token create claude read write tasks
claude mcp add quire --transport http https://<host>/mcp \
  --header "Authorization: Bearer sk_…"
```

Scopes are real: a token created with only `read` cannot write.

## Backups

`data/vault/` is the source of truth and is plain markdown — back it up like
any other directory. `data/.quire/auth.db` holds passkeys, tokens and share
grants; losing it means re-registering. `data/.quire/index.db` is disposable
(`quire reindex` rebuilds it).
