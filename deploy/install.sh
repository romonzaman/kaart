#!/usr/bin/env bash
#
# Install or upgrade Kaart on Debian 12. Run as root from an unpacked dist
# directory (see `make dist`):
#
#   sudo ./deploy/install.sh
#
# Idempotent: safe to re-run for every upgrade. An existing
# /etc/kaart/kaart.env is never overwritten, and the previous web bundle is
# kept at /var/www/kaart.old so a bad deploy can be reversed by hand.

set -euo pipefail

BIN_SRC="${BIN_SRC:-kaartd}"
WEB_SRC="${WEB_SRC:-web}"
UNIT_SRC="${UNIT_SRC:-deploy/kaartd.service}"
NGINX_SRC="${NGINX_SRC:-deploy/nginx-kaartd.conf}"
ENV_SRC="${ENV_SRC:-deploy/kaart.env.example}"

BIN_DST=/usr/local/bin/kaartd
# The bundle sits in a subdirectory named for the URL prefix, because the
# nginx snippet matches that prefix with root rather than alias.
WEB_DST=/var/www/kaart
NGINX_DST=/etc/nginx/snippets/kaartd.conf
ENV_DST=/etc/kaart/kaart.env
UNIT_DST=/etc/systemd/system/kaartd.service

say() { printf '\n==> %s\n' "$*"; }
die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || die "must run as root (try: sudo $0)"
[[ -f $BIN_SRC ]] || die "no server binary at $BIN_SRC — run 'make dist' and install from inside dist/"
[[ -f $UNIT_SRC ]] || die "no unit file at $UNIT_SRC"

say "Service account"
if id kaart &>/dev/null; then
    echo "user 'kaart' already exists"
else
    adduser --system --group --no-create-home --home /var/lib/kaart \
            --shell /usr/sbin/nologin kaart
fi

say "Server binary -> $BIN_DST"
# install(1) writes to a temp file and renames, so a running kaartd is not
# corrupted mid-copy; it picks up the new binary on restart below.
install -o root -g root -m 0755 "$BIN_SRC" "$BIN_DST"
"$BIN_DST" --version

say "Configuration -> $ENV_DST"
install -d -o root -g kaart -m 0750 /etc/kaart
if [[ -f $ENV_DST ]]; then
    echo "$ENV_DST exists — left untouched"
    echo "compare against $ENV_SRC for keys added since it was written"
else
    [[ -f $ENV_SRC ]] || die "no $ENV_SRC to seed $ENV_DST from"
    install -o root -g kaart -m 0640 "$ENV_SRC" "$ENV_DST"
    echo "seeded from the example — EDIT IT before the service is useful"
fi

if [[ -d $WEB_SRC ]]; then
    say "Web app -> $WEB_DST"
    rm -rf "$WEB_DST.new"
    mkdir -p "$WEB_DST.new"
    cp -a "$WEB_SRC/." "$WEB_DST.new/"
    chown -R root:root "$WEB_DST.new"
    chmod -R a+rX "$WEB_DST.new"
    if [[ -d $WEB_DST ]]; then
        rm -rf "$WEB_DST.old"
        mv "$WEB_DST" "$WEB_DST.old"
        echo "previous bundle kept at $WEB_DST.old"
    fi
    mv "$WEB_DST.new" "$WEB_DST"
else
    echo "no $WEB_SRC directory — skipping the web app"
fi

if [[ -f $NGINX_SRC ]]; then
    say "nginx snippet -> $NGINX_DST"
    install -d -o root -g root -m 0755 /etc/nginx/snippets
    install -o root -g root -m 0644 "$NGINX_SRC" "$NGINX_DST"
    if grep -rqs "snippets/kaartd.conf" /etc/nginx/sites-enabled/; then
        echo "already included by a site; will reload nginx at the end"
    else
        echo "NOT yet included by any site. Add this inside the server block"
        echo "for your hostname, then reload nginx:"
        echo "    include $NGINX_DST;"
    fi
fi

say "systemd unit -> $UNIT_DST"
install -o root -g root -m 0644 "$UNIT_SRC" "$UNIT_DST"
systemctl daemon-reload
systemd-analyze verify "$UNIT_DST" || die "the unit file did not verify"

say "Starting"
systemctl enable kaartd
systemctl restart kaartd

# Type=exec reports "started" once the binary is exec'd, not once it is
# listening, so a bad listen address would otherwise look like a success here.
addr="$(grep -oP '^\s*KAART_ADDR\s*=\s*\K\S+' "$ENV_DST" | tail -1 | tr -d "\"'" || true)"
addr="${addr:-127.0.0.1:8080}"

for _ in $(seq 1 20); do
    systemctl is-active --quiet kaartd || break
    if curl -fsS --max-time 2 "http://${addr}/healthz" >/dev/null 2>&1; then
        echo "healthz answered on ${addr}"
        break
    fi
    sleep 0.5
done

if ! systemctl is-active --quiet kaartd; then
    journalctl -u kaartd -n 30 --no-pager
    die "kaartd is not running"
fi

if grep -rqs "snippets/kaartd.conf" /etc/nginx/sites-enabled/; then
    say "Reloading nginx"
    nginx -t && systemctl reload nginx
fi

say "Done"
systemctl status kaartd --no-pager --lines=5 || true
cat <<'NEXT'

Next:
  1. Edit /etc/kaart/kaart.env — at minimum KAART_CORS_ORIGINS.
     Then: systemctl restart kaartd
  2. Include the nginx snippet in your server block, if you have not yet:
       include /etc/nginx/snippets/kaartd.conf;
     then: nginx -t && systemctl reload nginx
  3. kaartd has NO authentication. If your host does not already require it,
     uncomment the auth_basic lines in the snippet after running:
       htpasswd -c /etc/nginx/kaart.htpasswd yourname
  4. Logs: journalctl -u kaartd -f
NEXT
