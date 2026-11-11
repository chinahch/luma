#!/bin/sh
set -eu
if [ "$#" -ne 1 ] || [ ! -r "$1" ]; then echo 'Usage: sh scripts/restore.sh BACKUP.dump (new empty database only)' >&2; exit 1; fi
count="$(docker compose exec -T db psql -U luma -d luma -Atc "SELECT count(*) FROM pg_tables WHERE schemaname='public'")"
if [ "$count" != 0 ]; then echo 'Restore refused: database is not empty. Restore into a new instance.' >&2; exit 1; fi
docker compose exec -T db pg_restore -U luma -d luma --single-transaction --exit-on-error --no-owner --no-privileges < "$1"
echo 'Database restored. Start API/web at the matching application version, then verify accounts and accounting.'
