# Ship — V1 Task Backlog

Derived from the Ship product architecture document. Scope is limited to V1 as defined in §56; anything from §57 is parked in the Post-V1 section at the bottom.

**Legend**

- `S` ≈ 1–2 days · `M` ≈ 3–5 days · `L` ≈ 1–2 weeks
- Tasks are grouped into epics. IDs are stable; use them for dependency references.

**Suggested build order:** E0 → E1 → E2 → E3 → E4 → E5 → E6 → E7 → E8 → E9/E10/E11 → E12 → E13 → E14 → E15

---

## E0 — Foundations & repository

### SH-001 — Bootstrap the monorepo skeleton

Set up the `ship/` monorepo exactly as laid out in §40: `apps/web`, `server`, `packages/*`, `infra/*`, `docs`, `scripts`. Wire `pnpm-workspace.yaml` and `go.work` so both toolchains resolve from the root.

- Acceptance: `pnpm install` and `go build ./...` both succeed from a clean clone.
- Acceptance: README documents how to run each workspace locally.
- Size: S

### SH-002 — Local development environment

Docker Compose stack for local dev: PostgreSQL, Redis, API, worker, web with hot reload. Should not require a VPS to develop against.

- Acceptance: `make dev` (or equivalent) brings up all five services and the dashboard loads.
- Acceptance: Seed script creates an admin user and one demo project.
- Depends on: SH-001 · Size: M

### SH-003 — Go service scaffolding (`cmd/api`, `cmd/worker`)

Two binaries sharing `internal/`. HTTP router, structured logging, config loading from env, graceful shutdown, health endpoint on both.

- Acceptance: `GET /healthz` returns build SHA and DB/Redis connectivity status.
- Depends on: SH-001 · Size: M

### SH-004 — Database migration tooling

Pick and wire a migration tool (goose/atlas/dbmate). Migrations live in `server/migrations/`, run automatically on API boot behind a flag.

- Acceptance: Up and down migrations both tested in CI against a real Postgres.
- Depends on: SH-003 · Size: S

### SH-005 — OpenAPI contract and codegen pipeline

Per §48: the Go API is the source of the OpenAPI spec, which generates TypeScript types and the client in `packages/api-client`. Prevents Go/TS model drift.

- Acceptance: `pnpm generate` regenerates `packages/types` and `packages/api-client/src/generated` from the spec.
- Acceptance: CI fails if generated files are stale relative to the spec.
- Depends on: SH-003 · Size: M

### SH-006 — Shared domain package (`internal/domain`)

Central Go types for Project, Environment, Service, Server, Accessory, Domain, Volume, EnvVar, Secret, Deployment, Configuration. No persistence or transport concerns in this package.

- Acceptance: Every feature package imports domain types rather than redefining them.
- Depends on: SH-003 · Size: M

### SH-007 — CI pipeline

Lint, unit test, build for Go and TS; migration test; OpenAPI drift check; container image build for `ship-api`, `ship-worker`, `ship-web`.

- Acceptance: All checks run on PR; images publish on tagged release.
- Depends on: SH-001, SH-005 · Size: M

### SH-008 — Error model and API conventions

One error envelope, consistent status codes, request IDs, pagination convention, and validation error shape used across every endpoint.

- Acceptance: Documented in `docs/api-conventions.md` and enforced by a shared handler wrapper.
- Depends on: SH-003 · Size: S

---

## E1 — Ship installation & control plane packaging

### SH-010 — Containerize the control plane

Build production images for `ship-web`, `ship-api`, `ship-worker` plus pinned `ship-postgres` and `ship-redis`, as described in §39.

- Acceptance: Multi-arch (amd64/arm64) images, non-root users, sub-200MB app images.
- Depends on: SH-007 · Size: M

### SH-011 — Control plane Compose bundle

The `infra/compose/` stack that the installer deploys: five services, internal network, named volumes for Postgres data and Ship's workspace directory, restart policies.

- Acceptance: `docker compose up` on a fresh VPS yields a working control plane.
- Depends on: SH-010 · Size: M

### SH-012 — One-command installer script

`curl -fsSL https://get.ship.dev | bash` (§1). Detects OS/arch, installs Docker if missing, generates secrets, writes `.env`, pulls images, starts the stack, prints the dashboard URL and first-run token.

- Acceptance: Works on fresh Ubuntu 22.04/24.04 and Debian 12 VPSs.
- Acceptance: Re-running the script is idempotent and does not destroy data.
- Depends on: SH-011 · Size: L

### SH-013 — Direct control-plane web access

Publish `ship-web` directly on port 3000. Do not install or manage Caddy, Nginx, or TLS as part of Ship; operators may use their existing ingress separately.

- Acceptance: A fresh install is reachable at `http://SERVER_IP:3000`; API, PostgreSQL, and Redis are not publicly exposed.
- Depends on: SH-011 · Size: M

### SH-014 — First-run setup wizard

Post-install flow: create the admin account, set the Ship hostname, optionally add the first server, optionally connect GitHub.

- Acceptance: Setup is single-use — the endpoint is disabled once an admin exists.
- Depends on: SH-011, SH-020 · Size: M

### SH-015 — Upgrade and backup commands

`ship upgrade` (pull new images, run migrations, restart) and `ship backup` / `ship restore` for the control-plane Postgres and encryption keys.

- Acceptance: Upgrade across one minor version preserves all data.
- Acceptance: Restore on a fresh host reproduces the previous control plane.
- Depends on: SH-012 · Size: M

---

## E2 — Authentication, authorization, security

### SH-020 — Local accounts and sessions

MVP auth per §52: email + password, Argon2id hashing, server-side sessions in secure HTTP-only cookies.

- Acceptance: Login, logout, password change, session expiry all covered by tests.
- Depends on: SH-004 · Size: M

### SH-021 — CSRF, rate limiting, and security headers

Per §53: CSRF tokens on state-changing requests, per-IP and per-account rate limits on auth and deployment endpoints, HSTS/CSP/X-Frame-Options.

- Acceptance: Automated tests confirm rejection of cross-origin mutations and lockout after N failed logins.
- Depends on: SH-020 · Size: M

### SH-022 — Secret encryption at rest

Envelope encryption for application secrets, SSH private keys, and Git tokens (§16, §53). Key material lives outside the database; support key rotation.

- Acceptance: Secrets are unreadable from a raw DB dump.
- Acceptance: Rotation re-encrypts all ciphertext without downtime.
- Depends on: SH-004 · Size: L

### SH-023 — Access control model

Roles scoped to the install: owner, admin, deployer, viewer. Deployment and secret-reveal actions restricted to appropriate roles.

- Acceptance: Every API route declares a required permission; unauthorized calls return 403.
- Depends on: SH-020 · Size: M

### SH-024 — Audit log

Append-only record of who changed what and when: config mutations, deploys, rollbacks, server add/remove, secret reads, user management (§49, §53).

- Acceptance: Audit entries are immutable and viewable in Settings with filters.
- Depends on: SH-023 · Size: M

### SH-025 — Prevent arbitrary host shell access from the UI

Explicit guardrail from §53. All SSH-executed commands come from a fixed allowlist of templated operations; no free-text command endpoint ships in V1.

- Acceptance: no endpoint accepts raw shell input.
- Depends on: SH-040 · Size: S

---

## E3 — Core domain model & CRUD

### SH-030 — Projects

Schema, repository layer, and REST endpoints for Project (§8): id, name, slug, timestamps. Slugs unique per install.

- Acceptance: Full CRUD with validation; deleting a project cascades to environments behind an explicit confirmation.
- Depends on: SH-006, SH-008 · Size: S

### SH-031 — Environments

Environment belongs to a project (§9); slug unique within project. Each environment owns an independent infrastructure configuration.

- Acceptance: CRUD plus environment cloning (copy config, exclude secrets by default).
- Depends on: SH-030 · Size: M

### SH-032 — Services

The primary deployable unit (§10): repository, branch, image, port, command, role, type. Belongs to an environment.

- Acceptance: CRUD; validation rejects a service with neither a repository nor an image.
- Depends on: SH-031 · Size: M

### SH-033 — Accessories

Supporting services (§13) — PostgreSQL and Redis for V1. Fields: type, image, target server, port, volumes, environment.

- Acceptance: Creating a Postgres accessory auto-suggests a data volume and generates a connection-string secret.
- Depends on: SH-031, SH-035 · Size: M

### SH-034 — Domains and SSL flags

Domain attached to a service (§14): hostname, sslEnabled. Hostname uniqueness enforced per environment.

- Acceptance: Validation rejects malformed hostnames and duplicates.
- Note: Ship follows Kamal here — the operator points DNS at their servers themselves; Ship stores the hostname and does no DNS management or verification.
- Depends on: SH-032 · Size: S

### SH-035 — Volumes

Persistent storage bound to a service or accessory (§15): name, source, destination.

- Acceptance: Destination paths validated as absolute; source names unique per environment.
- Depends on: SH-032 · Size: S

### SH-036 — Environment variables and secrets

Two-tier config (§16): plaintext variables and encrypted secrets, both scoped to environment with optional per-service overrides.

- Acceptance: Secret values are write-only through the API except for an explicit, audited reveal action.
- Acceptance: Bulk import from a pasted `.env` block.
- Depends on: SH-022, SH-031 · Size: M

### SH-037 — Service dependencies

`ServiceDependency` edges (§17) between services and accessories, typed. Feeds the topology graph and validation.

- Acceptance: Cycles are detected and rejected.
- Acceptance: Deleting a target warns about dependent services.
- Depends on: SH-032, SH-033 · Size: M

### SH-038 — Full database schema and migrations

Materialize the table list in §49 with correct foreign keys, indexes, and constraints. Confirm Redis holds only jobs, locks, cache, and transient events — never desired state.

- Acceptance: ER diagram committed to `docs/`; migration applies cleanly from empty.
- Depends on: SH-030 … SH-037 · Size: M

---

## E4 — Servers, SSH, and Docker access

### SH-040 — SSH client package (`internal/ssh`)

Per §45: connection pooling, key-based auth, command execution, file transfer, streaming output, session lifecycle, timeouts.

- Acceptance: Streaming stdout/stderr surfaces line-by-line with backpressure handling.
- Acceptance: Host key verification is enforced after first trusted connection.
- Depends on: SH-003 · Size: L

### SH-041 — SSH key management

Generate or import keypairs, store private keys encrypted, expose the public key for the operator to install on their VPS.

- Acceptance: Keys are never returned in plaintext by the API; public key copy-to-clipboard in UI.
- Depends on: SH-022, SH-040 · Size: M

### SH-042 — Server registration

Add a server (§38): name, IP/hostname, SSH user, SSH key. Persist per §11 including architecture, OS, status, resources.

- Acceptance: CRUD; removing a server is blocked while services are assigned to it.
- Depends on: SH-041 · Size: M

### SH-043 — Server connection test and inspection

The check sequence from §38: SSH reachable → Docker present → OS → architecture → resources. Report each step individually.

- Acceptance: UI shows per-check pass/fail with actionable error text.
- Acceptance: Re-runnable on demand and on a schedule.
- Depends on: SH-042 · Size: M

### SH-044 — Server preparation

Install or verify Docker and any prerequisites Kamal needs on a newly added VPS, over SSH, with a visible log stream. Agentless by design (§38).

- Acceptance: A bare Ubuntu VPS becomes deploy-ready without the operator running manual commands.
- Depends on: SH-043 · Size: L

### SH-045 — Server roles / groups

Named role groups such as `web`, `worker`, `database` (§12, §37), each mapping to a set of servers. Services target a role rather than individual hosts.

- Acceptance: Assigning a role to a service resolves to all member servers at render time.
- Acceptance: Removing the last server from a role that is in use fails validation.
- Depends on: SH-042, SH-032 · Size: M

### SH-046 — Docker client package (`internal/docker`)

Per §46: containers, images, stats, logs — executed over the SSH transport against remote hosts.

- Acceptance: Container list, stats snapshot, and log tail all work against a remote VPS.
- Depends on: SH-040 · Size: M

---

## E5 — Configuration engine

> §18 and §43 — the most important component in the product. The UI never generates YAML directly.

### SH-050 — Desired-state model (`model.go`)

The canonical in-memory representation of an environment's infrastructure (§54): services, accessories, servers, roles, domains, volumes, env, dependencies. Serializable, versionable, and independent of Kamal.

- Acceptance: A full environment round-trips model → JSON → model with no loss.
- Depends on: SH-038 · Size: L

### SH-051 — Configuration compiler (`compiler.go`)

Assembles the desired-state document from the normalized database records, resolving roles to servers, secrets to references, and dependencies to ordering hints.

- Acceptance: Compiling the same DB state twice produces byte-identical output (deterministic ordering).
- Depends on: SH-050 · Size: M

### SH-052 — Validator (`validator.go`)

Rejects invalid states before any deployment: service with no server, port conflicts on the same host, domain pointing at a nonexistent service, missing required secret, orphaned volume, dependency cycle, accessory on an unreachable server.

- Acceptance: Each rule has a unique code, human-readable message, and a link to the offending entity.
- Acceptance: Validation runs both on save (warn) and pre-deploy (block).
- Depends on: SH-051 · Size: L

### SH-053 — Configuration versioning (`versioning.go`)

Immutable snapshot per meaningful mutation (§20), numbered `v14`, `v15`, `v16`. Records author, timestamp, and change summary.

- Acceptance: Every deployment references exactly one configuration version.
- Acceptance: Old versions are readable indefinitely and never mutated.
- Depends on: SH-051 · Size: M

### SH-054 — Configuration diff (`diff.go`)

Structural diff between two versions, grouped by entity, rendering as in §21 (`API + VPS #2`, `Redis + Redis 7`, `Environment + REDIS_URL`).

- Acceptance: Secret values never appear in a diff — only "added/changed/removed".
- Acceptance: Unchanged entities are reported as "No changes" rather than omitted.
- Depends on: SH-053 · Size: M

### SH-055 — Kamal renderer (`renderer.go`)

Translates desired state into a valid Kamal configuration (§19, §37): service definitions, multi-server roles, proxy/SSL, accessories, volumes, env and secret wiring, registry settings.

- Acceptance: Golden-file tests cover single-server, multi-server-role, and accessory-heavy configurations.
- Acceptance: Rendered output is never written into the user's application repository.
- Depends on: SH-052 · Size: L

### SH-056 — Configuration workspace management

Materialize rendered configuration into Ship-owned storage or an ephemeral deploy workspace (§19): `/data/ship/projects/<project>/<environment>/`.

- Acceptance: Workspaces are created per deployment, permission-restricted, and cleaned up on a retention policy.
- Acceptance: Secrets land in the workspace only as files with 0600 permissions and are removed after the run.
- Depends on: SH-055 · Size: M

### SH-057 — Configuration preview API

Read-only endpoint returning the rendered configuration for the current or any historical version, for display in the Monaco viewer (§32).

- Acceptance: Response redacts secret values by default with an audited reveal option.
- Depends on: SH-055 · Size: S

---

## E6 — Kamal adapter

> §23, §44 — no Kamal shell commands scattered through the codebase.

### SH-060 — `DeploymentEngine` interface

Define the abstraction from §23 (`Validate`, `Render`, `Deploy`, `Rollback`, `Logs`) in `internal/kamal`. The rest of the system depends only on this interface.

- Acceptance: A fake in-memory implementation exists for tests; no package outside `internal/kamal` imports Kamal specifics.
- Depends on: SH-006 · Size: M

### SH-061 — Kamal executor (`executor.go`)

Runs Kamal against a prepared workspace with a controlled environment, captures stdout/stderr, enforces timeouts, and returns structured exit results.

- Acceptance: Long-running invocations stream output incrementally rather than buffering to completion.
- Acceptance: Concurrent deployments to the same environment are serialized by lock.
- Depends on: SH-060, SH-056 · Size: L

### SH-062 — Kamal runtime packaging

Ensure Kamal (and its Ruby/Docker prerequisites) is available inside the `ship-worker` image at a pinned version.

- Acceptance: Version is printed in `/healthz` and asserted at worker startup.
- Depends on: SH-010 · Size: M

### SH-063 — Kamal output parser

Map Kamal's console output to structured events and phase transitions so the UI shows progress rather than raw text (feeds SH-070, SH-080).

- Acceptance: Build, push, deploy, and health-check phases are detected reliably.
- Acceptance: Unrecognized output degrades gracefully to plain log lines.
- Depends on: SH-061 · Size: M

### SH-064 — Kamal error classification

Translate common failures — auth failure, registry rejection, image build error, health check timeout, host unreachable, port in use — into actionable Ship-level messages.

- Acceptance: Each known class has a remediation hint shown in the UI.
- Depends on: SH-063 · Size: M

### SH-065 — Kamal rollback support (`rollback.go`)

Wire the rollback path so a previous deployment's image and configuration version can be re-applied.

- Acceptance: Rollback is a first-class engine operation, not a re-deploy of old code from Git.
- Depends on: SH-061 · Size: M

---

## E7 — Deployment engine, jobs, history

### SH-070 — Deployment state machine

Implement §25: `QUEUED → VALIDATING → BUILDING → PUSHING → DEPLOYING → VERIFYING → SUCCESS`, plus `FAILED`, `ROLLING_BACK`, `ROLLED_BACK`. Illegal transitions rejected at the persistence layer.

- Acceptance: Transition table is unit-tested exhaustively.
- Depends on: SH-063 · Size: M

### SH-071 — Job queue and worker

Redis-backed queue (§24, §51) with at-least-once delivery, retries with backoff, dead-letter handling, and per-environment concurrency locks.

- Acceptance: `POST /deployments` returns a `deployment_id` immediately and never blocks on execution.
- Acceptance: Worker restart mid-deployment marks the run failed rather than leaving it stuck.
- Depends on: SH-003 · Size: L

### SH-072 — Job type implementations

The job set from §51: `provision_server`, `inspect_server`, `deploy_service`, `rollback_deployment`, `health_check`, `collect_metrics`, `sync_repository`.

- Acceptance: Each job is idempotent or explicitly documented as not being so.
- Depends on: SH-071 · Size: L

### SH-073 — Deployment orchestration pipeline

The §22 flow end to end: validate → render → create workspace → run Kamal → stream logs → health checks → success/failure, with cleanup on every exit path.

- Acceptance: A failure at any stage leaves no orphaned workspace or lock.
- Depends on: SH-070, SH-061, SH-056 · Size: L

### SH-074 — Deployment records and history

Persist the §26 model: environment, service, commit SHA, configuration version, status, timings. List view as `#42 a82fd91 ● Success`.

- Acceptance: History is filterable by service and status and paginated.
- Depends on: SH-070 · Size: M

### SH-075 — Health verification step

After deploy, poll the configured health endpoint per service until healthy or timeout; failure moves the deployment to `FAILED` and optionally triggers rollback.

- Acceptance: Health check path, expected status, interval, and timeout are configurable per service.
- Depends on: SH-073 · Size: M

### SH-076 — Rollback flow

User selects a prior deployment and rolls back (§27). Ship resolves the image, commit, configuration version, and affected services and reproduces that state.

- Acceptance: Rollback creates a new deployment record linked to its source deployment.
- Acceptance: Rolling back to a version whose servers no longer exist fails validation with a clear message.
- Depends on: SH-065, SH-074 · Size: L

### SH-077 — Pre-deploy diff and confirmation

Wire SH-054 into the deploy flow so the user sees exactly what will change before confirming (§21).

- Acceptance: Deploy button is disabled while validation errors exist.
- Depends on: SH-054, SH-073 · Size: M

---

## E8 — Events and live streaming

### SH-080 — Domain event model

Emit the §50 events: `DeploymentQueued`, `DeploymentStarted`, `BuildStarted`, `BuildCompleted`, `ServiceHealthy`, `DeploymentCompleted`, `DeploymentFailed`.

- Acceptance: Events are persisted for audit and replay, not only broadcast.
- Depends on: SH-070 · Size: M

### SH-081 — Event fan-out via Redis pub/sub

Worker publishes; API subscribes and relays. Supports multiple browser clients watching the same deployment.

- Acceptance: Two clients on the same deployment receive identical ordered streams.
- Depends on: SH-080 · Size: M

### SH-082 — SSE endpoint for live deployment output

`Worker → event stream → SSE → browser` (§50). Includes reconnect with a cursor so no lines are lost.

- Acceptance: Refreshing mid-deployment resumes without duplicate or missing lines.
- Depends on: SH-081 · Size: M

### SH-083 — Frontend live-log consumer

React hook plus a virtualized log viewer with autoscroll, pause, search, and copy.

- Acceptance: Renders 50k lines without noticeable jank.
- Depends on: SH-082 · Size: M

---

## E9 — Logs

### SH-090 — Deployment logs

Persist full deployment output per §28, linked to the deployment record, with a retention policy.

- Acceptance: Logs for a completed deployment are viewable and downloadable after the fact.
- Depends on: SH-073 · Size: M

### SH-091 — Container logs over SSH

MVP approach from §28: retrieve Docker logs through SSH for a selected service and server, with tail and follow modes.

- Acceptance: Follow mode streams to the browser via the same SSE channel.
- Acceptance: Multi-server services let the user pick a host or interleave.
- Depends on: SH-046, SH-082 · Size: L

### SH-092 — Server logs

Basic system-level log access (Docker daemon, system journal) for troubleshooting, read-only and allowlisted per SH-025.

- Acceptance: No free-text command path is exposed.
- Depends on: SH-046 · Size: M

---

## E10 — Monitoring, health, drift

### SH-100 — Server metrics collection

Scheduled job collecting CPU, RAM, disk, and network per server (§29) via SSH, stored as time series with short retention.

- Acceptance: Collection interval configurable; failures mark the server degraded rather than crashing the job.
- Depends on: SH-046, SH-072 · Size: M

### SH-101 — Container metrics

Per-container CPU, memory, restart count, and status (§29).

- Acceptance: Restart-count spikes are surfaced on the service overview.
- Depends on: SH-046 · Size: M

### SH-102 — Application health monitoring

Periodic health-endpoint polling per service, recording status code and latency (§29).

- Acceptance: Health state drives the `● Healthy` indicators across the dashboard.
- Depends on: SH-072 · Size: M

### SH-103 — Actual-state observation

Query what is actually running on each server — which services on which hosts, which images (§55).

- Acceptance: Actual state is stored separately from desired state and timestamped.
- Depends on: SH-046 · Size: L

### SH-104 — Drift detection

Compare desired state against observed actual state and report "Infrastructure drift detected" with the specific differences (§55).

- Acceptance: Drift banner appears on the environment overview with a per-entity breakdown.
- Acceptance: Drift never auto-remediates in V1 — it only reports.
- Depends on: SH-103, SH-050 · Size: L

---

## E11 — Git integration

### SH-110 — GitHub connection

OAuth app or GitHub App install (§30) storing an encrypted token; list accessible repositories.

- Acceptance: Token revocation is handled gracefully with a reconnect prompt.
- Depends on: SH-022 · Size: L

### SH-111 — Repository and branch selection

Browse repositories, choose one, choose a branch, resolve the current HEAD SHA.

- Acceptance: Private repositories are listed only when the token grants access.
- Depends on: SH-110 · Size: M

### SH-112 — Repository inspection and stack detection

Detect `Dockerfile`, `package.json`, `go.mod`, `Cargo.toml`, `Gemfile`, `requirements.txt` (§30) to prefill service configuration.

- Acceptance: Detection results prefill port, build context, and start command suggestions, all editable.
- Acceptance: A repository with no Dockerfile produces a clear, non-blocking warning.
- Depends on: SH-111 · Size: M

### SH-113 — Repository → service creation flow

The §30 flow end to end, ending in a created Ship service, with an explicit note that no `config/deploy.yml` is added to the user's repository (§31).

- Acceptance: Service is created with repository, branch, and detected defaults in one wizard.
- Depends on: SH-112, SH-032 · Size: M

### SH-114 — Commit resolution for deployments

Resolve and pin the commit SHA at deploy time so the deployment record is reproducible.

- Acceptance: Deployment detail shows the exact SHA and links to it on GitHub.
- Depends on: SH-111, SH-074 · Size: S

---

## E12 — Web application shell

### SH-120 — Next.js app scaffold

`apps/web` per §41: App Router, TypeScript, Tailwind, `features/` for domain UI, `components/` for primitives only.

- Acceptance: Directory conventions documented; lint rule discourages domain logic in `components/`.
- Depends on: SH-001 · Size: M

### SH-121 — Auth UI and session handling

Login, logout, first-run setup, password change, session expiry handling.

- Acceptance: Protected routes redirect cleanly and preserve the intended destination.
- Depends on: SH-020, SH-120 · Size: M

### SH-122 — App shell and navigation

The §33 layout: left nav (Overview, Applications, Servers, Databases, Deployments, Logs, Metrics, Settings), top bar with project/environment switcher and a global Deploy action.

- Acceptance: Environment switching preserves the current section where meaningful.
- Depends on: SH-121 · Size: M

### SH-123 — Typed API client wiring

Consume `@ship/api-client` (§47) with a query/cache layer. No infrastructure or Kamal logic in the frontend.

- Acceptance: All data fetching goes through the generated client; no ad-hoc `fetch` calls to the API.
- Depends on: SH-005, SH-120 · Size: M

### SH-124 — Design system primitives

Buttons, inputs, forms, tables, tabs, modals, toasts, status indicators (`● Healthy` / `● Connected` / `● Failed`), empty states, skeletons.

- Acceptance: Shared in `packages/ui`; status colors and semantics are consistent everywhere.
- Depends on: SH-120 · Size: L

---

## E13 — Visual infrastructure editor

### SH-130 — Topology canvas with React Flow

The §36 canvas rendering services, accessories, and their dependency edges for the selected environment.

- Acceptance: Layout is stable across reloads; node positions persist per environment.
- Depends on: SH-037, SH-124 · Size: L

### SH-131 — Node configuration panel

Clicking a node opens a side panel for that service or accessory: servers/role, port, command, health check, domains, volumes, env.

- Acceptance: Edits mutate the configuration model and are validated live (§52).
- Depends on: SH-130 · Size: L

### SH-132 — Edit-configuration-not-infrastructure semantics

Enforce the §36 rule: dragging and editing changes desired configuration only; nothing touches a server until Deploy is pressed.

- Acceptance: An unsaved/undeployed indicator shows pending changes with a link to the diff.
- Depends on: SH-131 · Size: M

### SH-133 — Node status overlay

Overlay live health and container status onto canvas nodes (`● Healthy`, restart counts, drift markers).

- Acceptance: Status updates without a full page refresh.
- Depends on: SH-130, SH-102, SH-104 · Size: M

### SH-134 — Generated configuration viewer (Monaco)

Read-only Monaco pane showing the rendered Kamal configuration (§32), with a version selector and copy/export.

- Acceptance: Secrets are redacted; export produces a file the user can inspect outside Ship.
- Depends on: SH-057 · Size: M

---

## E14 — Screens

### SH-140 — Environment overview

The §33 dashboard body: topology, aggregate health, recent deployments, pending configuration changes, drift banner.

- Acceptance: Loads in under one second on a 20-service environment with cached data.
- Depends on: SH-130 · Size: M

### SH-141 — Application screen

Per §34: status, repository, branch, image, servers, domain, port, health check — with tabs for Overview, Configuration, Deployments, Logs, Metrics.

- Acceptance: Every field maps to a documented configuration property.
- Depends on: SH-131 · Size: L

### SH-142 — Servers list and detail

List view per §11 (`production-api-1 ● Connected · 4 CPU · 8 GB RAM`) and detail per §35: CPU/memory/disk, container list, network, plus Restart service / View logs / Open configuration / Remove server actions.

- Acceptance: Destructive actions require typed confirmation and respect role permissions.
- Depends on: SH-042, SH-046, SH-100 · Size: L

### SH-143 — Add-server wizard

The §38 flow: form (name, IP, SSH user, SSH key) → connection test with per-check results → preparation → "Server ready".

- Acceptance: Failures at any check show the specific fix required and allow retry without re-entering data.
- Depends on: SH-043, SH-044 · Size: M

### SH-144 — Databases / accessories screen

Manage PostgreSQL and Redis accessories: placement, version, volumes, generated connection strings, status.

- Acceptance: Changing an accessory's server warns about data volume implications before proceeding.
- Depends on: SH-033 · Size: M

### SH-145 — Deployments screen

History list, deployment detail with phase timeline, live and archived logs, diff of the configuration version used, and the rollback action.

- Acceptance: An in-progress deployment shows the state machine position in real time.
- Depends on: SH-074, SH-083, SH-076 · Size: L

### SH-146 — Deploy flow UI

Deploy button → validation results → configuration diff (§21) → confirm → live progress.

- Acceptance: Matches the `[ Cancel ] [ Deploy ]` interaction described in the spec.
- Depends on: SH-077 · Size: M

### SH-147 — Environment variables and secrets UI

Manage plaintext variables and secrets with masked display, bulk `.env` paste, per-service overrides, and an audited reveal.

- Acceptance: Secrets are never rendered in the DOM until an explicit reveal is authorized.
- Depends on: SH-036 · Size: M

### SH-148 — Logs screen

Unified access to the three log classes from §28 with a source selector (deployment / container / server) and service and host filters.

- Acceptance: Deep links to a specific service's live logs work from anywhere in the app.
- Depends on: SH-091, SH-090 · Size: M

### SH-149 — Metrics screen

Server and container metrics plus health/latency charts (§29). Deliberately not a full observability platform.

- Acceptance: Time ranges of 1h / 24h / 7d supported.
- Depends on: SH-100, SH-101, SH-102 · Size: M

### SH-150 — Settings screen

Users and roles, GitHub connection, SSH keys, Ship hostname, external-ingress guidance, backups, audit log, version and upgrade status.

- Acceptance: Each section is permission-gated per SH-023.
- Depends on: SH-023, SH-024, SH-110 · Size: M

---

## E15 — Release readiness

### SH-160 — End-to-end acceptance test

Automated run of the §4 golden path against real VPSs: fresh VPS → install Ship → open dashboard → connect GitHub → select repo → configure → deploy → reachable HTTPS URL.

- Acceptance: Runs in CI against ephemeral cloud instances on every release candidate.
- Depends on: SH-146 · Size: L

### SH-161 — Multi-VPS acceptance test

Cover §37: API across three servers by role, Postgres pinned to one, worker on another; verify role resolution and correct Kamal output.

- Acceptance: Scaling a role from one to three servers redeploys correctly.
- Depends on: SH-045, SH-160 · Size: M

### SH-162 — Failure and recovery testing

Deliberately break things: unreachable host mid-deploy, failing health check, bad Dockerfile, revoked GitHub token, worker crash. Verify state machine correctness and clear messaging.

- Acceptance: No deployment can be left permanently in a non-terminal state.
- Depends on: SH-073, SH-064 · Size: M

### SH-163 — Documentation

Install guide, first deployment walkthrough, servers and roles, accessories, secrets, rollback, backup/restore, security model, troubleshooting, and how Ship maps to Kamal concepts.

- Acceptance: A new user reaches a deployed app using docs alone, without reading Kamal's documentation.
- Depends on: SH-160 · Size: L

### SH-164 — Positioning and landing content

Codify the §1 and §60 framing: what Ship is, what it is not (§3), and the repository-vs-Ship ownership split (§31).

- Acceptance: Docs and marketing copy consistently state that no `deploy.yml` is required in the user's repository.
- Depends on: SH-163 · Size: S

### SH-165 — Release packaging and versioning

Semantic versioning, changelog, tagged image publishing, and an upgrade compatibility matrix.

- Acceptance: `get.ship.dev` always installs the latest stable release; older versions remain pullable.
- Depends on: SH-015, SH-007 · Size: M

---

## Post-V1 backlog (explicitly excluded from V1 — §57)

Do not start these until the core is stable.

| Item                                                               | Note                                                                                                                                                                                                                                                      |
| ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| AI layer                                                           | Separate architecture on top of the existing config + deployment APIs (§58). Exposed as tools: `add_service()`, `add_server()`, `configure_domain()`, `update_environment()`, `deploy()`, `get_logs()`, `rollback()` — never direct SSH or Docker access. |
| Additional accessories                                             | MySQL, MinIO, RabbitMQ, MongoDB (§13).                                                                                                                                                                                                                    |
| GitHub OAuth / passkeys / 2FA / OIDC / SSO                         | Auth roadmap from §52.                                                                                                                                                                                                                                    |
| Additional Git providers                                           | GitLab, Bitbucket, self-hosted Git.                                                                                                                                                                                                                       |
| Kubernetes                                                         | Out of scope entirely.                                                                                                                                                                                                                                    |
| Autoscaling, multi-region, HA databases, database clustering       | §57.                                                                                                                                                                                                                                                      |
| Cloud provisioning                                                 | Ship does not become a VPS marketplace (§3).                                                                                                                                                                                                              |
| Advanced observability, complex networking, full Terraform support | §57.                                                                                                                                                                                                                                                      |
| Automated drift remediation                                        | V1 only detects and reports (SH-104).                                                                                                                                                                                                                     |

---

## Cross-cutting rules to enforce in review

1. **Ship owns the desired infrastructure state; Kamal turns that state into running infrastructure** (§2). Nothing in the UI or API should bypass the configuration model.
2. **The UI never generates YAML directly** (§18). All rendering goes through the configuration engine.
3. **No Kamal shell commands outside `internal/kamal`** (§23).
4. **No infrastructure execution logic in `packages/api-client`** (§47).
5. **Desired state lives in PostgreSQL, never in Redis** (§49).
6. **The user's application repository is never modified** (§31).
7. **Deployments are always asynchronous** — no HTTP request waits on Kamal (§24).
