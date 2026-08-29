# Ship

Your servers. Your apps. One simple control plane.

Ship is a self-hosted visual control plane for Kamal. Install it on a VPS with one
command, connect your repositories and servers, then configure, deploy, and operate
your applications from a UI — without ever writing a Kamal configuration yourself.

**Ship owns the infrastructure configuration. Kamal is the deployment engine underneath.**

Your application repository stays clean. No `config/deploy.yml` required.

## Prerequisites

- Go 1.23+
- Node 20+ and pnpm 9+
- Docker with Compose v2

## Getting started

```bash
git clone <your-repo> ship && cd ship
cp .env.example .env
openssl rand -base64 32   # paste into SHIP_MASTER_KEY
openssl rand -base64 32   # paste into SHIP_SESSION_SECRET

pnpm install
make up                   # postgres + redis
make migrate
```

Then in three terminals:

```bash
make api      # :8080
make worker
make web      # :3000
```

## Layout

```
apps/web/     Next.js dashboard
server/       Go API + worker
packages/     Shared TS: types, api-client, ui
infra/        Compose stacks, Dockerfiles, installer
docs/         Architecture and operational docs
scripts/      Codegen, seeding, maintenance
```

See `docs/folder-guide.md` for what belongs in each directory and why.

## Non-negotiable rules

1. Ship owns desired state; Kamal turns it into running infrastructure.
2. The UI never generates YAML — it mutates the model, `internal/configuration` renders.
3. No Kamal commands outside `internal/kamal`.
4. No infrastructure logic in `packages/api-client`.
5. Desired state lives in PostgreSQL, never in Redis.
6. The user's application repository is never modified.
7. Deployments are always asynchronous. No HTTP request waits on Kamal.
