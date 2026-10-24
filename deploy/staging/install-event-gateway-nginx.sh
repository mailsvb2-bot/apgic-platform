#!/usr/bin/env bash
# Version-controlled, fail-closed installer for APGIC's staging HTTPS gateway.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bash "$ROOT/deploy/staging/assert-authorized-host.sh"
SITE=/etc/nginx/sites-available/apgic-staging.conf
SNIPPET=/etc/nginx/snippets/apgic-event-gateway.conf
SOURCE="$ROOT/deploy/staging/nginx-event-gateway-location.conf"
[[ -f "$SITE" && -f "$SOURCE" ]] || { echo "BLOCKED: missing HTTPS site or managed snippet"; exit 1; }
[[ -f /etc/letsencrypt/live/apgic.ru/fullchain.pem ]] || { echo "BLOCKED: trusted TLS certificate missing"; exit 1; }
mkdir -p /etc/nginx/snippets
TMP="$(mktemp -d /tmp/apgic-nginx-install.XXXXXX)"
chmod 700 "$TMP"
cp -p "$SITE" "$TMP/site"
HAD_SNIPPET=0
if [[ -f "$SNIPPET" ]]; then
  cp -p "$SNIPPET" "$TMP/snippet"
  HAD_SNIPPET=1
fi
DONE=0
rollback() {
  if [[ "$DONE" != 1 ]]; then
    cp -p "$TMP/site" "$SITE"
    if [[ "$HAD_SNIPPET" == 1 ]]; then cp -p "$TMP/snippet" "$SNIPPET"; else rm -f "$SNIPPET"; fi
    nginx -t && systemctl reload nginx || true
    echo "ROLLBACK: managed gateway route restored"
  fi
  rm -rf "$TMP"
}
trap rollback EXIT
python3 - "$SITE" "$SNIPPET" "$SOURCE" <<'PY'
import pathlib, re, sys
site, snippet, src = map(pathlib.Path, sys.argv[1:])
s = site.read_text()
# Find the sole HTTPS server by balanced braces, avoiding the Certbot HTTP block.
blocks = []
for m in re.finditer(r"\bserver\s*\{", s):
    depth, end = 1, m.end()
    while end < len(s) and depth:
        if s[end] == "{": depth += 1
        elif s[end] == "}": depth -= 1
        end += 1
    if depth: raise SystemExit("BLOCKED: unbalanced nginx server")
    chunk = s[m.end():end - 1]
    if re.search(r"\blisten\s+443\s+ssl\b", chunk) and re.search(r"\bserver_name\s+apgic\.ru\s+www\.apgic\.ru\s*;", chunk):
        blocks.append((m.end(), end - 1, chunk))
if len(blocks) != 1: raise SystemExit("BLOCKED: expected one canonical 443 server")
start, end, chunk = blocks[0]
directive = "    include /etc/nginx/snippets/apgic-event-gateway.conf;"
if chunk.count("apgic-event-gateway.conf") > 1: raise SystemExit("BLOCKED: duplicate gateway include")
if directive not in chunk:
    # Adopt only known, previously installed local-only inline gateway route.
    match = re.search(r"location\s*=\s*/_internal/apgic-event-gateway\s*\{([^{}]*)\}", chunk)
    if match:
        old = match.group(1)
        for required in ("allow 127.0.0.1;", "allow ::1;", "deny all;", "proxy_pass http://127.0.0.1:43114/;"):
            if required not in old: raise SystemExit("BLOCKED: unknown existing gateway route")
        chunk = chunk[:match.start()] + directive + chunk[match.end():]
    else:
        if "/_internal/apgic-event-gateway" in chunk: raise SystemExit("BLOCKED: unknown route ownership")
        chunk = "\n" + directive + "\n" + chunk
    s = s[:start] + chunk + s[end:]
    site.write_text(s)
snippet.write_text(src.read_text())
print("Managed HTTPS gateway route reconciled")
PY
nginx -t
systemctl reload nginx
echo "=== Confirm active HTTPS route (wait for Nginx generations) ==="
CODE=""
for i in $(seq 1 20); do
  CODE="$(curl --noproxy '*' --resolve apgic.ru:443:127.0.0.1 --max-time 5 -sS -o /dev/null -w '%{http_code}' -X POST https://apgic.ru/_internal/apgic-event-gateway || true)"
  [[ "$CODE" == 401 ]] && break
  sleep 1
done
[[ "$CODE" == 401 ]] || { echo "BLOCKED: gateway HTTPS expected HTTP 401, got $CODE"; exit 1; }
SITE_CODE="$(curl --noproxy '*' --resolve apgic.ru:443:127.0.0.1 --max-time 10 -sS -o /dev/null -w '%{http_code}' https://apgic.ru/)"
[[ "$SITE_CODE" == 200 ]] || { echo "BLOCKED: website HTTP $SITE_CODE"; exit 1; }
DONE=1
echo "MANAGED_EVENT_GATEWAY_HTTPS_PASS: route=401 site=200"
