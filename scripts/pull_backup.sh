#!/usr/bin/env bash
set -euo pipefail

backup_host="${CLICKSHIELD_BACKUP_HOST:?set CLICKSHIELD_BACKUP_HOST to an SSH host}"
backup_dir="${CLICKSHIELD_BACKUP_DIR:-$HOME/backups/clickshield}"
mkdir -p "$backup_dir"
chmod 700 "$backup_dir"

name="$(ssh -o BatchMode=yes "$backup_host" "find /opt/clickshield/backups -maxdepth 1 -type f -name 'clickshield-*.dump' -printf '%f\\n' | sort | tail -1")"
[[ "$name" =~ ^clickshield-[0-9]{8}T[0-9]{6}Z\.dump$ ]] || { echo 'No valid remote backup found' >&2; exit 1; }

target="$backup_dir/$name"
if [[ ! -f "$target" ]]; then
  rsync -a --chmod=F600 "$backup_host:/opt/clickshield/backups/$name" "$target"
fi
remote_hash="$(ssh -o BatchMode=yes "$backup_host" "sha256sum /opt/clickshield/backups/$name" | cut -d ' ' -f 1)"
local_hash="$(sha256sum "$target" | cut -d ' ' -f 1)"
[[ "$remote_hash" == "$local_hash" ]] || { echo 'Backup checksum mismatch' >&2; exit 1; }
find "$backup_dir" -maxdepth 1 -type f -name 'clickshield-*.dump' -mtime +29 -delete
echo "Verified off-server copy: $target"
