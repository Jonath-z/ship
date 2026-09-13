# Ship — Product Architecture and Gap Plan

Status snapshot taken 2026-09-13 against the V1 backlog in `task.md`.

Ship is a self-hosted visual control plane for Kamal. Ship owns desired
infrastructure state; Kamal is the deployment engine beneath it. Application
repositories are never modified and never need a `config/deploy.yml`.

## System architecture

```text
                                   OPERATOR'S VPS (control plane — 5 containers)
                ┌──────────────────────────────────────────────────────────────────────┐
                │                                                                      │
Browser ────────┼─►  ship-web :3000 (public)       ship CLI (host binary)              │
                │      Next.js App Router            status · logs · upgrade ·         │
                │      features/ + @ship/api-client  backup · restore · public-url     │
                │           │                                                          │
                │           │ typed client (openapi-fetch) + react-query, CSRF         │
                │           ▼                                                          │
                │    ship-api :8080 (loopback only)                                    │
                │    ┌────────────────────────────────────────────────┐                │
                │    │ auth (Argon2id) · access (RBAC) · audit · setup│                │
                │    │ CRUD: projects → environments → services,      │                │
                │    │   accessories, domains, volumes, dependencies, │                │
                │    │   env vars / secrets, servers, ssh keys        │                │
                │    │ configuration engine:                          │                │
                │    │   compile → validate → version → diff → render │                │
                │    │ deployments: POST /deployments → enqueue       │                │
                │    │ SSE: /deployments/:id/stream (replay + live)   │                │
                │    └───────┬───────────────────────────┬────────────┘                │
                │            │ desired state             │ jobs (queue)                │
                │            ▼                           ▼    + pub/sub ◄─┐            │
                │    ship-postgres (internal)     ship-redis (internal)   │            │
                │    all desired state, config    queue · locks ·         │            │
                │    versions, deployments,       sessions · events       │ events     │
                │    logs, vault, audit                  │                │            │
                │                                        │ BLMOVE        │            │
                │                                        ▼               │            │
                │    ship-worker :8081 (health)  ─────────────────────────            │
                │    ┌────────────────────────────────────────────────┐               │
                │    │ deployment pipeline (state machine:            │               │
                │    │  QUEUED→VALIDATING→…→VERIFYING→SUCCESS/FAILED) │               │
                │    │ validate → render → materialize workspace      │               │
                │    │   /data/ship/projects/<proj>/<env>/<deploy>/   │               │
                │    │ internal/kamal: executor runs Kamal 2.12       │               │
                │    │   (bundled in image) · output parser · rollback│               │
                │    └───────────────┬────────────────────────────────┘               │
                └────────────────────┼─────────────────────────────────────────────---┘
                                     │ SSH (key auth, TOFU host keys,
                                     │      fixed allowlist of 9 operations)
                    ┌────────────────┼────────────────┐
                    ▼                ▼                ▼
              TARGET VPS #1    TARGET VPS #2    TARGET VPS #3     (agentless)
              Docker           Docker           Docker
              kamal-proxy      app containers   accessories
              app containers   (role: worker)   (Postgres/Redis)
              (role: web)                       + data volumes
```

### Core data flow

1. The dashboard mutates the model through the typed API client — never YAML.
2. The API persists desired state in PostgreSQL (projects, environments,
   services, accessories, servers, groups, domains, volumes, variables,
   encrypted secrets, dependencies).
3. `internal/configuration` compiles the normalized records into a
   deterministic desired-state document, validates it (blocking and warning
   rules), snapshots it as an immutable version, diffs versions, and renders
   per-service Kamal YAML.
4. `POST /deployments` enqueues a `deploy_service` job in Redis and returns
   immediately; no HTTP request ever waits on Kamal.
5. The worker holds a per-environment lock, drives the deployment state
   machine, materializes a workspace (config + 0600 secrets + SSH key), and
   runs Kamal through `internal/kamal` — the only package allowed to invoke it.
6. Kamal connects to the target servers over SSH and reconciles containers;
   kamal-proxy gates traffic on container health.
7. Output lines and status transitions are persisted (`deployment_logs`) and
   published to Redis pub/sub; the API relays them to browsers over SSE with
   cursor-based resume.
8. Rollback re-applies a prior deployment's image and configuration version as
   a new deployment linked to its source.

### Architectural invariants (enforced in review)

1. Ship owns desired state; Kamal turns it into running infrastructure.
2. The UI mutates the model; only `internal/configuration` renders YAML.
3. No Kamal commands exist outside `internal/kamal`.
4. `packages/api-client` is transport only.
5. Desired state lives in PostgreSQL, never Redis.
6. Ship never modifies an application repository.
7. Deployments are asynchronous; HTTP never waits on Kamal.
8. All SSH commands come from a fixed allowlist — no free-text shell anywhere.

### Repository layout

| Path                  | Role                                                        |
| --------------------- | ----------------------------------------------------------- |
| `apps/web`            | Next.js dashboard (App Router, features/ + components/)     |
| `server/cmd/api`      | HTTP API, OpenAPI contract, route registration              |
| `server/cmd/worker`   | queue consumer, deployment pipeline, Kamal runtime          |
| `server/cmd/ship`     | host CLI (status, logs, upgrade, backup, restore, public-url) |
| `server/internal/*`   | domain packages; `platform/*` for cross-cutting infrastructure |
| `packages/api-client` | generated OpenAPI types + typed fetch client                |
| `infra/`              | Compose bundles, Dockerfiles, installer, pinned versions    |
| `scripts/`            | codegen, seeding, image builds, release packaging           |

## Implementation status vs the V1 backlog

Complete: E0 foundations, E1 install/packaging, E2 auth/security, E3 domain
CRUD, E4 servers/SSH/Docker, E5 configuration engine, E6 Kamal adapter,
E7 deployment engine, E8 events/SSE, E12 app shell — plus the host CLI, CI,
and release pipeline.

Deferred by explicit scope decision (2026-09-05, see `task.md`): E10
monitoring/drift, E11 Git integration, E13 visual canvas, SH-064 error
classification, SH-075 health polling, SH-092 server journal logs, SH-149
metrics screen.

## Gap plan

### 1. Frontend gaps — addressed in this change

| Gap | Backlog | Plan |
| --- | --- | --- |
| Environment overview showed control-plane health instead of the environment | SH-140 | New `features/overview/EnvironmentOverview.tsx`: application/database/server counts, validation state, pending configuration diff, recent deployments, compact control-plane health strip (full system view stays at `/dashboard`). |
| Service detail lacked Deployments and Logs tabs | SH-141 | `ServiceDeploymentsTab` (history filtered to the service, cursor paging) and `ServiceLogsTab` (container logs over SSH for the servers in the service's group). |
| No standalone Logs screen; `app/logs` and `features/logs` were empty | SH-148 | `features/logs/LogsScreen.tsx` at `/p/:project/e/:env/logs` with a source selector — deployment logs (live SSE via `LogTerminal`) or container logs (server → container → tail). Added to the left nav. |
| Container-log viewing existed only as a modal on the server detail screen | SH-091 (UI) | Shared `ContainerLogViewer` with tail-size selector and auto-refresh polling ("follow" until the backend gains a streaming follow mode). |

### 2. Documentation (SH-163) — the main release blocker

Write operator-facing docs under `docs/`, in this order:

1. `install.md` — one-command install, prerequisites, what gets installed,
   first-run token, `ship public-url`, troubleshooting install failures.
2. `first-deployment.md` — golden-path walkthrough: add SSH key → register
   server → checks → create service from a registry image → variables/secrets
   → deploy → watch logs → rollback.
3. `servers-and-roles.md` — server groups, multi-server placement, what the
   renderer does with roles.
4. `accessories.md` — Postgres/Redis placement, volumes, generated connection
   secrets.
5. `secrets.md` — two-tier config, bulk import, audited reveal, rotation.
6. `backup-restore.md` and `upgrade.md` — `ship backup/restore/upgrade`
   semantics, what archives contain, key material handling.
7. `kamal-mapping.md` — how Ship entities translate to Kamal configuration.
8. `troubleshooting.md` — failed checks, failed deployments, reading raw
   Kamal output.

Acceptance: a new user reaches a deployed app using docs alone.

### 3. Release-gating tests (SH-161/SH-162 remainder)

1. Script the failure/recovery scenarios that stayed in V1 scope
   (unreachable host mid-deploy, worker restart mid-deploy, exhausted
   retries) as an integration job in CI — the state machine assertions
   already exist; wire them against a Compose stack.
2. Add unit tests for `internal/platform/jobs` (queue delivery, backoff,
   lock expiry) — currently only covered indirectly.
3. Keep the manual golden-path checklist as the release gate for real-VPS
   runs; automate multi-VPS role resolution (SH-161) when ephemeral-VPS CI
   is worth its cost.

### 4. Backend polish (small, V1-friendly)

1. Wire the add-server wizard's Prepare step to the `POST /servers/:id/prepare`
   report end-to-end (SH-044 acceptance).
2. Virtualize the SSE log viewer if 50k-line deployments show jank (SH-083
   acceptance).

### 5. V1.1 (in priority order, per `task.md`)

1. E11 Git integration — GitHub connect, repo/branch selection, stack
   detection, server-side builds; the flagship V1.1 feature.
2. E10 monitoring — metrics collection, actual-state observation, then drift
   detection (report-only).
3. E13 visual canvas over the existing CRUD APIs; Monaco config viewer.
4. SH-064 Kamal error classification; SH-092 server journal logs.
