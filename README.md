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
cp .env.example .env
$EDITOR .env   # fill in EXCHANGE_DOMAIN / EXCHANGE_USERNAME / EXCHANGE_PASSWORD
$EDITOR docker-compose.yml   # set the port binding to your host's actual LAN IP
docker compose up -d --build
# or: podman compose up -d --build
```

The proxy binds to plain HTTP — it's meant to run on a trusted LAN (or a
Tailscale tailnet, which looks like more LAN traffic to it; no special
handling needed), not to be exposed publicly. The `ports:` binding in
`docker-compose.yml` is what actually restricts exposure to one interface —
set it to your host's LAN IP (or your `tailscale0` interface's IP) rather
than leaving it open on every interface.

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

All variables go in `.env` (see `.env.example`). Any variable also accepts a
`_FILE` suffix (e.g. `EXCHANGE_PASSWORD_FILE=/run/secrets/exchange_password`)
to read the value from a file instead — the standard Docker/Podman secrets
convention. If a `_FILE` variant is set but unreadable, the proxy refuses to
start rather than silently treating the value as unset.

| Variable | Default | Notes |
|---|---|---|
| `LISTEN_ADDR` | `0.0.0.0:8080` | Inside the container's network namespace; control real exposure via the `ports:` binding in compose instead. |
| `UPSTREAM_SCHEME` | `https` | |
| `UPSTREAM_HOST` | `mail.example.com` | |
| `EXCHANGE_DOMAIN` | *(empty)* | Omit if your account doesn't need a `DOMAIN\` prefix. |
| `EXCHANGE_USERNAME` | *(required)* | |
| `EXCHANGE_PASSWORD` | *(required)* | |
| `UPSTREAM_INSECURE_SKIP_VERIFY` | `false` | Escape hatch only; the real server uses a standard public CA cert, so this shouldn't normally be needed. |

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
