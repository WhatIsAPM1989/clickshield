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
objects="${archive%.dump}.replay.tar.gz"
resume_worker=false
cleanup() {
  rm -f "$temporary" "$objects.partial"
  if "$resume_worker"; then
    docker compose --env-file "$env_file" -f "$compose_file" up -d --no-deps worker
  fi
}
trap cleanup EXIT
chmod 600 "$temporary"
if [[ -n "${CLICKSHIELD_REPLAY_VOLUME:-}" ]]; then
  if [[ -n "$(docker compose --env-file "$env_file" -f "$compose_file" ps --status running -q worker)" ]]; then
    resume_worker=true
    docker compose --env-file "$env_file" -f "$compose_file" stop -t 15 worker
  fi
fi
docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres pg_dump -U clickshield -d clickshield -Fc >"$temporary"
docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres pg_restore -l <"$temporary" >/dev/null
if [[ -n "${CLICKSHIELD_REPLAY_VOLUME:-}" ]]; then
  docker run --rm --mount "type=volume,src=$CLICKSHIELD_REPLAY_VOLUME,dst=/data,readonly" \
    --mount "type=bind,src=$backup_dir,dst=/backup" alpine:3.22 \
    sh -c 'umask 077; tar -czf "/backup/$1.partial" -C /data .' sh "$(basename "$objects")"
  tar -tzf "$objects.partial" >/dev/null
  mv "$objects.partial" "$objects"
fi
mv "$temporary" "$archive"
cleanup
trap - EXIT
find "$backup_dir" -maxdepth 1 -type f \( -name 'clickshield-*.dump' -o -name 'clickshield-*.replay.tar.gz' \) -mtime +13 -delete
echo "Created and checked $archive"
if [[ -f "$objects" ]]; then echo "Created and checked $objects"; fi
