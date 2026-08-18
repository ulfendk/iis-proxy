# iis-proxy

Transparently proxies IIS auth.

Specifically: a tiny reverse proxy that sits between an email client (e.g.
Thunderbird) and a Microsoft Exchange server whose front end demands
`DOMAIN\username` + password HTTP authentication on every request. The proxy
holds the credentials and injects them as a `Basic` `Authorization` header on
every request it forwards, so the client never has to deal with the
challenge itself.

## Scope

This proxies **API-style endpoints only** — `/EWS/*` and
`/autodiscover/*` — which is what Thunderbird's native Exchange/EWS account
support actually talks to. It does **not**, and cannot, make the browser OWA
UI (`/owa/`) work: on this server `/owa/` is gated behind an F5 BigIP APM
forms-login portal (it redirects to `/my.policy`) that requires an
interactive login the proxy makes no attempt to solve. `/EWS/*` and
`/autodiscover/*` bypass that portal and challenge with plain HTTP Basic/NTLM
auth directly, which is what makes this approach work at all.

## How it works

Every request to the proxy is forwarded to the configured upstream
(`UPSTREAM_SCHEME://UPSTREAM_HOST`) unchanged, except:

- Whatever `Authorization` header the client sent is discarded and replaced
  with `Basic base64(DOMAIN\username:password)`, built from your configured
  credentials.
- The `Host` header is rewritten to match the upstream, since it needs to
  match what the upstream (and its TLS certificate/SNI) expects.

Responses — including redirects, like the `/owa/` → `/my.policy` one — are
streamed straight back to the client, unmodified and unfollowed.

## Quick start

```sh
cp example.env .env
$EDITOR .env   # fill in LISTEN_BIND_IP, EXCHANGE_DOMAIN/USERNAME/PASSWORD, etc.
docker compose up -d --build
# or: podman compose up -d --build
```

Nothing in `docker-compose.yml` itself needs editing — every value that
varies per-deployment (which LAN interface to bind, the credentials, the
upstream host) is read from environment variables with sane defaults, so the
compose file can be left as-is and configured entirely through `.env` (or,
for Portainer, its stack "Environment variables" UI — see below).

The proxy binds to plain HTTP — it's meant to run on a trusted LAN (or a
Tailscale tailnet, which looks like more LAN traffic to it; no special
handling needed), not to be exposed publicly. `LISTEN_BIND_IP` is what
actually restricts exposure to one interface — set it to your host's LAN IP
(or your `tailscale0` interface's IP) rather than leaving it at the default
`0.0.0.0`, which binds every interface.

## Deploying via Portainer (Git repository stack)

If you add this repo to Portainer as a "Repository" stack, Portainer clones
`docker-compose.yml` read-only on each deploy — there's no `.env` file to
edit (it's gitignored and never committed) and no way to hand-edit the
compose file. That's fine: everything below is designed for exactly that.

1. In the stack's **Environment variables** section, add the same variables
   listed in `example.env` / the reference table below (at minimum
   `EXCHANGE_USERNAME`, `EXCHANGE_PASSWORD`, and `LISTEN_BIND_IP`). Portainer
   feeds these into the same `${VAR}` substitution `docker-compose.yml`
   already uses — no compose edits needed.
2. Mark `EXCHANGE_PASSWORD` (and `EXCHANGE_DOMAIN`/`EXCHANGE_USERNAME` if you
   prefer) as **sensitive** in Portainer's UI if it offers that option, so
   it's masked in the stack's settings view.
3. Deploy. Portainer will build the image from the repo's `Dockerfile`
   itself — no separate registry push needed.
4. Anything not listed as an environment variable in Portainer keeps its
   built-in default (see the reference table) — you only need to set the
   ones you want to change from the default.

## Configuring Thunderbird

1. Add a new Exchange/EWS account as usual.
2. In Account Settings → Server Settings → Advanced, set the EWS URL to:
   ```
   http://<lan-ip>:8080/EWS/Exchange.asmx
   ```
   (and, if the account wizard needs it, an autodiscover URL of
   `http://<lan-ip>:8080/autodiscover/autodiscover.xml`).
3. Set Connection security to **None** — the proxy doesn't terminate TLS
   itself; the LAN/tailnet hop between Thunderbird and the proxy is trusted,
   and the proxy always talks HTTPS to the real server on your behalf.
4. Whatever username/password Thunderbird asks you to enter for the account
   itself doesn't matter — the proxy overrides it on every request.

## Configuration reference

Set these in `.env` (plain `docker compose`) or in Portainer's stack
"Environment variables" (Git-repository stacks) — both feed the same
`${VAR}` placeholders in `docker-compose.yml`. Anything left unset keeps its
default.

| Variable | Default | Notes |
|---|---|---|
| `LISTEN_BIND_IP` | `0.0.0.0` | Host-side interface `docker-compose.yml` publishes the port on. Set to your LAN (or `tailscale0`) IP — leaving it at the default exposes every interface on the host. |
| `LISTEN_PORT` | `8080` | Host-side port. |
| `UPSTREAM_SCHEME` | `https` | |
| `UPSTREAM_HOST` | `mail.example.com` | |
| `EXCHANGE_DOMAIN` | *(empty)* | Omit if your account doesn't need a `DOMAIN\` prefix. |
| `EXCHANGE_USERNAME` | *(required)* | |
| `EXCHANGE_PASSWORD` | *(required)* | |
| `UPSTREAM_INSECURE_SKIP_VERIFY` | `false` | Escape hatch only; the real server uses a standard public CA cert, so this shouldn't normally be needed. |

Two more exist as escape hatches for non-Portainer, plain `docker compose`
use, not exposed through `docker-compose.yml`'s `environment:` block above:

- `LISTEN_ADDR` (default `0.0.0.0:8080`) — the address the process binds to
  *inside* the container's own network namespace. Left at its default in
  normal use; `LISTEN_BIND_IP`/`LISTEN_PORT` above are what actually control
  exposure.
- `EXCHANGE_PASSWORD_FILE` (Docker/Podman secrets convention, e.g.
  `/run/secrets/exchange_password`, read and whitespace-trimmed instead of
  `EXCHANGE_PASSWORD`) — only useful when that file genuinely exists on the
  host running the container, which isn't the case for a Portainer
  Git-repository stack; use Portainer's sensitive-variable option there
  instead. If set but unreadable, the proxy refuses to start rather than
  silently treating the value as unset.

## Verification / troubleshooting

```sh
# Health check — doesn't touch the upstream or need credentials.
curl -v http://<lan-ip>:8080/healthz

# Confirm the proxy ignores whatever auth you send and injects the real
# configured credentials instead.
curl -v http://<lan-ip>:8080/EWS/Exchange.asmx -H "Authorization: Basic AAAAAAAA"

# Confirm autodiscover works the same way.
curl -v http://<lan-ip>:8080/autodiscover/autodiscover.xml

# Confirm the /owa/ redirect passes through unmodified (proves redirects
# aren't being followed or hidden server-side).
curl -v http://<lan-ip>:8080/owa/
```

If EWS/autodiscover requests come back with an auth failure, double-check
`EXCHANGE_DOMAIN`/`EXCHANGE_USERNAME`/`EXCHANGE_PASSWORD` in `.env` — you can
confirm the proxy is actually sending your configured credentials (and not
just ignoring the problem) by temporarily setting an obviously wrong
password, restarting, and checking that the upstream's auth-failure response
changes accordingly.

## Development

```sh
go build ./...
go test ./...
```

No third-party dependencies — just the Go standard library.
