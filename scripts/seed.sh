#!/usr/bin/env bash
# Seed a local database with an admin user and one demo project (SH-002).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="$repo_root/infra/compose/docker-compose.dev.yml"

docker compose -f "$compose_file" exec -T postgres psql -v ON_ERROR_STOP=1 -U ship -d ship <<'SQL'
INSERT INTO users (id, email, password_hash, role)
VALUES (
  '00000000-0000-4000-8000-000000000001',
  'admin@ship.local',
  'authentication-is-added-in-SH-020',
  'owner'
)
ON CONFLICT (email) DO UPDATE SET role = EXCLUDED.role, updated_at = now();

INSERT INTO projects (id, name, slug)
VALUES ('00000000-0000-4000-8000-000000000002', 'Demo Project', 'demo-project')
ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, updated_at = now();

INSERT INTO environments (id, project_id, name, slug)
VALUES (
  '00000000-0000-4000-8000-000000000003',
  '00000000-0000-4000-8000-000000000002',
  'Production',
  'production'
)
ON CONFLICT (project_id, slug) DO UPDATE SET name = EXCLUDED.name, updated_at = now();
SQL

echo "Seeded admin@ship.local and Demo Project / Production."
