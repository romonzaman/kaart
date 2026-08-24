# Deploying Kaart on Debian 12

Kaart is designed to run on your own machine. Putting it on a server works, but
one property changes and it matters more than everything else here.

## Read this first

**`kaartd` has no authentication.** There is no login, no token, no API key.
Every deck, every card and every review is readable and writable by anyone who
can open a TCP connection to it. That is a reasonable design for a process
listening on `127.0.0.1` on your laptop. On a server with a public IP it means
the first scanner that finds the port owns your data.

So the deployment below is built around one rule:

> `kaartd` binds loopback. nginx terminates TLS, authenticates, and proxies.

If you set `KAART_ADDR=0.0.0.0:8080`, you have published an unauthenticated
read-write database to the internet. The systemd unit does not prevent this —
nothing can, once you have edited the config — so it is on you.

---

## What gets installed where

| Path | |
|---|---|
| `/usr/local/bin/kaartd` | the server binary |
| `/etc/kaart/kaart.env` | configuration, `root:kaart` `0640` |
| `/var/lib/kaart/` | the SQLite database, created by `StateDirectory=` |
| `/var/www/kaart/kaartd/` | the exported web app, served by nginx |
| `/etc/nginx/snippets/kaartd.conf` | the location blocks, included by your site |
| `/etc/systemd/system/kaartd.service` | the unit |

---

## The URLs

This deployment serves Kaart under a subpath of an existing host:

| | |
|---|---|
| UI | `https://agent.mkzaman.com/kaartd/` |
| API | `https://agent.mkzaman.com/kaartd/api/` |
| Health | `https://agent.mkzaman.com/kaartd/healthz` |

Both come from one origin, which is the point: the browser makes no
cross-origin request, so there is no CORS preflight — and therefore HTTP Basic
auth can work, since a preflight carries no credentials and nginx would reject
it before it reached the proxy.

The prefix is not hardcoded. `make dist BASE_PATH=/somethingelse` rebuilds for
a different one; `BASE_PATH=/` serves from a domain root. The nginx snippet
would need the same substitution by hand.

## 1. Build the release locally

```bash
make dist
```

That cross-compiles a static `linux/amd64` binary (no cgo, so nothing needs to
be installed on the server), exports the Expo web app, and copies the deploy
files alongside them:

```
dist/
  kaartd            static linux/amd64 binary, stripped, version stamped
  web/kaartd/       the Expo web export, in a directory named for the prefix
  deploy/           unit file, env example, nginx snippet, install.sh
```

Two variables shape the web export, both set by `make web`:

- **`EXPO_BASE_URL=/kaartd`** prefixes the asset URLs Expo writes into
  `index.html` and expo-router's own route matching. Without it the page loads
  from `/kaartd/` but asks for its JavaScript at `/_expo/...` and gets a 404.
- **`EXPO_PUBLIC_API_URL=/kaartd`** prefixes the API client's request paths, so
  they come out as `/kaartd/api/v1/...` — relative, same-origin. Without it the
  bundle calls `http://localhost:8080` from the visitor's own browser.

The export directory is named `kaartd` to match the URL prefix. That is what
lets the nginx snippet match it with `root` instead of `alias`: `alias`
combined with `try_files` misresolves the SPA fallback, so a deep link like
`/kaartd/deck/abc/study` would 404 instead of reaching `index.html`.

## 2. Copy it to the server

```bash
rsync -a --delete dist/ you@your-server:/tmp/kaart-dist/
```

## 3. Install

```bash
ssh you@your-server 'cd /tmp/kaart-dist && sudo ./deploy/install.sh'
```

The script creates the `kaart` system user, installs the binary, seeds
`/etc/kaart/kaart.env` from the example **only if it does not already exist**,
publishes the web bundle, installs and verifies the unit, and starts it. It is
idempotent — re-run it for every upgrade.

## 4. Configure

```bash
sudo nano /etc/kaart/kaart.env
sudo systemctl restart kaartd
```

| Key | Default | |
|---|---|---|
| `KAART_DB` | `./kaart.db` | SQLite file. The **directory** must be writable — SQLite puts `-wal` and `-shm` next to it |
| `KAART_ADDR` | `127.0.0.1:8080` | Listen address. Keep it on loopback |
| `KAART_CORS_ORIGINS` | `http://localhost:8081` | Comma-separated. Not needed at all in the same-origin setup below |
| `KAART_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

Precedence is **flag > process environment > env file > default**, so a value
systemd already put in the environment is never overwritten by a stale file, and
`kaartd --addr ...` on the command line still wins over both. A malformed env
file makes the process refuse to start rather than fall back to a default.

The same file works with the binary directly, which is the quickest way to
reproduce the service's configuration when debugging:

```bash
sudo -u kaart kaartd --env-file /etc/kaart/kaart.env --migrate-only
```

## 5. Wire up nginx

`install.sh` puts the location blocks at `/etc/nginx/snippets/kaartd.conf`. They
are a fragment, not a server block, because `agent.mkzaman.com` already exists
and serves other things. Include them in that server block:

```nginx
server {
    server_name agent.mkzaman.com;
    # ... your existing TLS config, other locations ...

    include /etc/nginx/snippets/kaartd.conf;
}
```

```bash
sudo nginx -t && sudo systemctl reload nginx
```

What the snippet does:

| Location | |
|---|---|
| `= /kaartd` | 301 to `/kaartd/`, so the URL works without the trailing slash |
| `/kaartd/api/` | proxies to `127.0.0.1:8080/api/` — the trailing slash on `proxy_pass` is what strips the prefix |
| `= /kaartd/healthz` | proxies to `/healthz`, `auth_basic off` so a monitor can reach it |
| `/kaartd/_expo/`, `/kaartd/assets/` | hashed filenames, cached for a year |
| `/kaartd/` | static files, falling back to `index.html` for client-side routes |

### Authentication

**kaartd has none.** If `agent.mkzaman.com` does not already require auth, the
API at `/kaartd/api/` is world-readable and world-writable the moment this is
live. The snippet ships with Basic auth commented out — enabling it before the
password file exists makes nginx 500 on every request — so turn it on
deliberately:

```bash
sudo apt install apache2-utils
sudo htpasswd -c /etc/nginx/kaart.htpasswd yourname
sudo sed -i 's/^#auth_basic/auth_basic/' /etc/nginx/snippets/kaartd.conf
sudo nginx -t && sudo systemctl reload nginx
```

The app needs no change for this: same-origin `fetch` sends credentials by
default, so the browser prompts once and attaches the header to every
`/kaartd/api/` request after that.

## 6. Close the port

The reverse proxy only helps if `8080` is not reachable from outside. With
`KAART_ADDR` on loopback it already is not, but a firewall makes it explicit:

```bash
sudo apt install ufw
sudo ufw allow OpenSSH
sudo ufw allow 'Nginx Full'
sudo ufw enable
```

Verify from your laptop, not from the server:

```bash
curl -sS --max-time 5 http://agent.mkzaman.com:8080/healthz   # must fail
curl -sS https://agent.mkzaman.com/kaartd/healthz             # must return ok
```

---

## Operating it

```bash
systemctl status kaartd
journalctl -u kaartd -f              # logs are JSON, one line per request
journalctl -u kaartd -p err -n 50    # errors only
systemctl restart kaartd
```

### Backups

The database is one file, and its two siblings are part of it. Do not copy
`kaart.db` on its own while the server is running — you will get a torn read.
Use SQLite's own backup, which is consistent against a live writer:

```bash
sudo apt install sqlite3
sudo -u kaart sqlite3 /var/lib/kaart/kaart.db \
  ".backup '/var/backups/kaart-$(date +%F).db'"
```

A nightly timer is the usual next step. The review log is append-only, so a
restored backup loses reviews since the snapshot but never corrupts history.

### Upgrading

```bash
make dist
rsync -a --delete dist/ you@your-server:/tmp/kaart-dist/
ssh you@your-server 'cd /tmp/kaart-dist && sudo ./deploy/install.sh'
```

Migrations run as `ExecStartPre` before the server binds, so a schema failure
stops the unit instead of producing a crash loop. The previous web bundle is
kept at `/var/www/kaart.old`.

**Take a backup before upgrading.** Migrations are forward-only; there is no
down path.

---

## Troubleshooting

**The unit will not start.** `journalctl -u kaartd -n 50 --no-pager`. The usual
causes are a typo in `kaart.env` (the process reports the file and line and
exits 1) and a `KAART_DB` outside `/var/lib/kaart` — `ProtectSystem=strict`
makes the rest of the filesystem read-only, so any other path fails to open.

**`healthz` works but the app shows network errors.** The bundle was exported
without `EXPO_PUBLIC_API_URL`, so it is calling `http://localhost:8080` from the
visitor's browser. Rebuild with `make dist`, which sets it.

**The page loads blank and the console 404s on a `.js` file.** The bundle was
exported without `EXPO_BASE_URL`, so `index.html` asks for `/_expo/...` instead
of `/kaartd/_expo/...`. Same fix: rebuild with `make dist`.

**A deep link like `/kaartd/deck/abc/study` 404s but `/kaartd/` works.** The SPA
fallback is not firing. Check that the snippet uses `root /var/www/kaart;` and
that the files really are at `/var/www/kaart/kaartd/` — with `alias` in place of
`root`, `try_files` resolves the fallback against the wrong directory.

**Every request returns 500 after enabling auth.** `auth_basic_user_file` points
at a file that does not exist. Create it with `htpasswd -c`.

**CORS errors in the console.** Something is serving the app from a different
origin than the API. In the setup above there should be no cross-origin request
at all — the UI and the API share `https://agent.mkzaman.com`. If you split them
deliberately, put the app's exact origin (scheme, host and port) in
`KAART_CORS_ORIGINS`, and expect Basic auth to break the preflight.

**Reviews are slow or the log shows `database is locked`.** SQLite allows one
writer. That is ample for one person, but the design assumes one person; this
is the symptom you would see if several were sharing an instance.
