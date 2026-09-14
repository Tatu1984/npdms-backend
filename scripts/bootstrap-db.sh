#!/usr/bin/env bash
#
# Bootstrap the NPDMS database on an edge server.
#
# Idempotent: safe to run against an empty database or an existing one. Uses
# nothing but psql and the SQL in this repository, so it works on a box with no
# internet access.
#
#   ./scripts/bootstrap-db.sh                      # uses DATABASE_URL, or local defaults
#   DB_NAME=npdms ./scripts/bootstrap-db.sh
#   ./scripts/bootstrap-db.sh --with-demo-data     # also loads demo officers and cases
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCHEMA="$ROOT/services/db/init/001_schema.sql"
MIGRATIONS="$ROOT/services/api/migrations"

DB_NAME="${DB_NAME:-npdms}"
WITH_DEMO=0
[[ "${1:-}" == "--with-demo-data" ]] && WITH_DEMO=1

# DATABASE_URL wins; otherwise talk to the local cluster as the current user.
if [[ -n "${DATABASE_URL:-}" ]]; then
  PSQL=(psql "$DATABASE_URL")
  ADMIN_PSQL=(psql "${DATABASE_URL%/*}/postgres")
  TARGET="$DATABASE_URL"
else
  PSQL=(psql -d "$DB_NAME")
  ADMIN_PSQL=(psql -d postgres)
  TARGET="local cluster, database $DB_NAME"
fi

info()  { printf '\033[0;34m›\033[0m %s\n' "$1"; }
ok()    { printf '\033[0;32m✓\033[0m %s\n' "$1"; }
warn()  { printf '\033[0;33m!\033[0m %s\n' "$1"; }

command -v psql >/dev/null || { echo "psql not found on PATH" >&2; exit 1; }

info "Target: $TARGET"

# ---------------------------------------------------------------- database ---
if [[ -z "${DATABASE_URL:-}" ]]; then
  if "${ADMIN_PSQL[@]}" -tAc "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" | grep -q 1; then
    ok "Database $DB_NAME already exists"
  else
    "${ADMIN_PSQL[@]}" -q -c "CREATE DATABASE $DB_NAME"
    ok "Created database $DB_NAME"
  fi
fi

# -------------------------------------------------------------- extensions ---
"${PSQL[@]}" -q -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp"' 2>/dev/null || warn "uuid-ossp unavailable"
"${PSQL[@]}" -q -c 'CREATE EXTENSION IF NOT EXISTS "pgcrypto"'  2>/dev/null || warn "pgcrypto unavailable"
# PostGIS is optional: only the traffic hotspot index needs it.
if "${PSQL[@]}" -tAc "SELECT 1 FROM pg_available_extensions WHERE name='postgis'" | grep -q 1; then
  "${PSQL[@]}" -q -c 'CREATE EXTENSION IF NOT EXISTS postgis' 2>/dev/null || true
  ok "PostGIS enabled"
else
  warn "PostGIS not available — traffic hotspot geometry index will be skipped"
fi

# ------------------------------------------------------------- base schema ---
TABLE_COUNT=$("${PSQL[@]}" -tAc "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'")
if [[ "$TABLE_COUNT" -lt 5 ]]; then
  info "Applying base schema"
  "${PSQL[@]}" -q -f "$SCHEMA" 2>&1 | grep -iE '^psql.*ERROR' || true
  ok "Base schema applied"
else
  ok "Base schema already present ($TABLE_COUNT tables)"
fi

# ------------------------------------------------------------ prerequisites ---
# Several migrations attach this trigger before any migration defines it.
"${PSQL[@]}" -q -c "
CREATE OR REPLACE FUNCTION update_updated_at_column() RETURNS TRIGGER AS \$\$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
\$\$ LANGUAGE plpgsql;"

# --------------------------------------------------------------- migrations ---
# The chain has order dependencies it does not declare: 000019 references tables
# created by 000020, and several migrations need columns that 000027 adds. Rather
# than renumber — which would break databases already migrated — the pass is run
# twice. Anything that failed only because its dependency had not been created
# yet succeeds on the second pass, and whatever still fails is reported.
run_migrations() {
  local pass="$1"
  local failed=()
  for file in $(ls "$MIGRATIONS"/*.up.sql | sort); do
    local name errors
    name=$(basename "$file")
    errors=$("${PSQL[@]}" -q -f "$file" 2>&1 | grep -iE '^psql.*ERROR' | grep -viE 'already exists|duplicate' || true)
    if [[ -n "$errors" ]]; then
      if echo "$errors" | grep -qi 'st_makepoint\|st_setsrid'; then
        [[ "$pass" == "2" ]] && warn "$name — PostGIS index skipped (extension not installed)"
      else
        failed+=("$name")
        if [[ "$pass" == "2" ]]; then
          warn "$name"
          echo "$errors" | sed 's/^/    /' | head -3
        fi
      fi
    fi
  done
  MIGRATION_FAILURES=("${failed[@]+"${failed[@]}"}")
}

info "Applying migrations (pass 1)"
run_migrations 1
info "Applying migrations (pass 2 — resolves order dependencies)"
run_migrations 2

if [[ ${#MIGRATION_FAILURES[@]} -gt 0 ]]; then
  warn "${#MIGRATION_FAILURES[@]} migration(s) still reporting errors after both passes"
else
  ok "All migrations applied cleanly"
fi

# ------------------------------------------------------------ verification ---
MISSING=$("${PSQL[@]}" -tAc "
  SELECT string_agg(t, ', ') FROM (
    SELECT unnest(ARRAY['users','firs','cases','evidence','warrants','investigation_workspaces','audit_logs']) AS t
  ) want WHERE t NOT IN (SELECT tablename FROM pg_tables WHERE schemaname='public')")

if [[ -n "$MISSING" ]]; then
  echo "Required tables missing after bootstrap: $MISSING" >&2
  exit 1
fi

FINAL=$("${PSQL[@]}" -tAc "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'")
ok "Schema ready — $FINAL tables"

# -------------------------------------------------------------- demo data ---
if [[ "$WITH_DEMO" == "1" ]]; then
  info "Loading demo data"
  "${PSQL[@]}" -q -f "$ROOT/services/db/init/002_demo_kolkata.sql"
  ok "Demo data loaded"
fi

echo
ok "Bootstrap complete."
echo "  Set DATABASE_URL for the API, e.g.:"
echo "    postgresql://npdms:<password>@localhost:5432/$DB_NAME?sslmode=disable"
