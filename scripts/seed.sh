#!/usr/bin/env bash
# Seed one demo project without bypassing the E2 account setup flow.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
database_file="${SHIP_DB_PATH:-$repo_root/data/ship/ship.db}"

command -v sqlite3 >/dev/null || {
  echo "sqlite3 is required to seed the development database" >&2
  exit 1
}
[[ -f "$database_file" ]] || {
  echo "no database at $database_file; start the dev stack first (make up)" >&2
  exit 1
}

sqlite3 "$database_file" <<'SQL'
INSERT INTO projects (id, name, slug)
VALUES ('00000000-0000-4000-8000-000000000002', 'Demo Project', 'demo-project')
ON CONFLICT (slug) DO UPDATE SET name = excluded.name, updated_at = CURRENT_TIMESTAMP;

INSERT INTO environments (id, project_id, name, slug)
VALUES (
  '00000000-0000-4000-8000-000000000003',
  '00000000-0000-4000-8000-000000000002',
  'Production',
  'production'
)
ON CONFLICT (project_id, slug) DO UPDATE SET name = excluded.name, updated_at = CURRENT_TIMESTAMP;
SQL

echo "Seeded Demo Project / Production. Create the owner through /setup."
