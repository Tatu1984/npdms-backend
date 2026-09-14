#!/usr/bin/env bash
#
# Back up an NPDMS edge server.
#
# An edge server has no replica. If the box is lost, everything on it is lost,
# so this should run on a schedule and the output should leave the machine.
#
#   ./scripts/backup.sh /var/backups/npdms
#   DATABASE_URL=... ./scripts/backup.sh /mnt/usb/npdms
#
# Restore:
#   createdb npdms
#   gunzip -c npdms-YYYYMMDD-HHMMSS.sql.gz | psql -d npdms
#   # object store:
#   mc mirror ./minio-YYYYMMDD-HHMMSS/ local/npdms
#
# The dump is taken with --no-owner so it can be restored under a different
# role, which is what happens when a box is rebuilt.
#
set -euo pipefail

DEST="${1:-./backups}"
STAMP="$(date +%Y%m%d-%H%M%S)"
RETAIN_DAYS="${BACKUP_RETAIN_DAYS:-30}"

DB_NAME="${DB_NAME:-npdms}"

info() { printf '\033[0;34m›\033[0m %s\n' "$1"; }
ok()   { printf '\033[0;32m✓\033[0m %s\n' "$1"; }
fail() { printf '\033[0;31m✗\033[0m %s\n' "$1" >&2; exit 1; }

command -v pg_dump >/dev/null || fail "pg_dump not found on PATH"
mkdir -p "$DEST"

# ------------------------------------------------------------------ database --
DUMP="$DEST/npdms-$STAMP.sql.gz"
info "Dumping database"

if [[ -n "${DATABASE_URL:-}" ]]; then
  pg_dump --no-owner --no-privileges --clean --if-exists "$DATABASE_URL" | gzip -9 > "$DUMP"
else
  pg_dump --no-owner --no-privileges --clean --if-exists -d "$DB_NAME" | gzip -9 > "$DUMP"
fi

# A dump that cannot be read back is not a backup. Check the gzip stream and
# confirm the schema is actually in there before reporting success.
gzip -t "$DUMP" || fail "Dump is not a valid gzip stream: $DUMP"
# grep -q would close the pipe on its first match, and under `set -o pipefail`
# gunzip then dies of SIGPIPE and the check reports a false failure. Count
# instead, so the whole stream is consumed.
TABLES_IN_DUMP=$(gunzip -c "$DUMP" | grep -c '^CREATE TABLE' || true)
if [[ "$TABLES_IN_DUMP" -lt 50 ]]; then
  fail "Dump contains only $TABLES_IN_DUMP tables — check the connection settings"
fi

SIZE=$(du -h "$DUMP" | cut -f1)
ok "Database dumped to $DUMP ($SIZE, $TABLES_IN_DUMP tables)"

# -------------------------------------------------------------- object store --
# Evidence files live in MinIO. Skipped when mc is unavailable rather than
# failing the whole run, but the operator is told.
if command -v mc >/dev/null 2>&1 && [[ -n "${MINIO_ENDPOINT:-}" ]]; then
  OBJECTS="$DEST/minio-$STAMP"
  info "Mirroring object store"
  mc alias set npdms-backup \
    "http${MINIO_USE_SSL:+s}://$MINIO_ENDPOINT" \
    "${MINIO_ACCESS_KEY:-}" "${MINIO_SECRET_KEY:-}" >/dev/null 2>&1 || true
  if mc mirror --quiet "npdms-backup/${MINIO_BUCKET:-npdms}" "$OBJECTS"; then
    ok "Object store mirrored to $OBJECTS"
  else
    printf '\033[0;33m!\033[0m Object store mirror failed — evidence files are NOT backed up\n' >&2
  fi
else
  printf '\033[0;33m!\033[0m mc or MINIO_ENDPOINT unset — evidence files are NOT in this backup\n' >&2
fi

# ------------------------------------------------------------------ pruning --
# Only prune once the new backup has been verified above.
if [[ "$RETAIN_DAYS" -gt 0 ]]; then
  PRUNED=$(find "$DEST" -maxdepth 1 -name 'npdms-*.sql.gz' -mtime "+$RETAIN_DAYS" -print -delete | wc -l | tr -d ' ')
  [[ "$PRUNED" -gt 0 ]] && ok "Pruned $PRUNED backup(s) older than $RETAIN_DAYS days"
fi

echo
ok "Backup complete: $DUMP"
echo "  Copy this off the server. A backup on the same box is not a backup."
