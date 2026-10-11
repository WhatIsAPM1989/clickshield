#!/usr/bin/env bash
set -euo pipefail

app_dir="${CLICKSHIELD_APP_DIR:-/opt/clickshield}"
cd "$app_dir"
compose_file="${CLICKSHIELD_COMPOSE_FILE:-compose.yml}"
env_file="${CLICKSHIELD_ENV_FILE:-.env}"
backup_dir="${CLICKSHIELD_BACKUP_DIR:-$app_dir/backups}"
mkdir -p "$backup_dir"
chmod 700 "$backup_dir"
exec 9>"$backup_dir/.backup.lock"
flock -n 9 || exit 0

archive="$backup_dir/clickshield-$(date -u +%Y%m%dT%H%M%SZ).dump"
temporary="$(mktemp "$backup_dir/.backup.XXXXXXXX")"
trap 'rm -f "$temporary"' EXIT
chmod 600 "$temporary"
docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres pg_dump -U clickshield -d clickshield -Fc >"$temporary"
docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres pg_restore -l <"$temporary" >/dev/null
mv "$temporary" "$archive"
trap - EXIT
find "$backup_dir" -maxdepth 1 -type f -name 'clickshield-*.dump' -mtime +13 -delete
echo "Created and checked $archive"
