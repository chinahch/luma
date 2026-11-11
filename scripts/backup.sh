#!/bin/sh
set -eu
umask 077
out="${1:-backups/luma-$(date -u +%Y%m%dT%H%M%SZ).dump}"
mkdir -p "$(dirname "$out")"
if [ -e "$out" ]; then echo 'Refusing to overwrite an existing backup' >&2; exit 1; fi
temp="${out}.partial.$$"
trap 'rm -f "$temp"' EXIT HUP INT TERM
docker compose exec -T db pg_dump -U luma -d luma -Fc > "$temp"
test -s "$temp"
mv "$temp" "$out"
echo "Database backup saved: $out"
echo 'Also securely back up .env, deployed commit/version and TLS configuration separately.'
