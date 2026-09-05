# Folder guide

What belongs in each directory, what must never go there, and why.

---

## Root

| File | Purpose |
|---|---|
| `pnpm-workspace.yaml` | Declares `apps/*` and `packages/*` as workspaces |
| `go.mod` | Root Go module; this keeps `go build ./...` valid from the repository root |
| `go.work` | Pins the root Go module as the active workspace |
| `package.json` | Root scripts only (`dev`, `build`, `lint`, `generate`) — no app dependencies |
| `Makefile` | The commands you actually type daily |
| `.env.example` | Every variable Ship reads, documented. Copy to `.env` |
| `.gitignore` | Critically: ignores `data/`, `workspaces/`, `*.pem`, `*.key` |

**Never commit:** `.env`, SSH private keys, anything under `data/`.

---

## `apps/web` — the dashboard

Next.js App Router, React, TypeScript, Tailwind, React Flow, Monaco.

```
apps/web/
├── app/          routes and layouts only, kept thin
│   ├── login/  dashboard/  projects/  servers/
│   ├── deployments/  logs/  settings/
├── components/   generic primitives with no domain knowledge
├── features/     domain UI — the important one
│   ├── projects/         project + environment management
│   ├── infrastructure/   React Flow canvas, node panel, config viewer
│   ├── deployments/      history, live progress, diff, rollback
│   ├── servers/          server list, detail, add-server wizard
│   └── logs/             log viewer and source selection
├── hooks/        cross-feature hooks: auth, SSE, permissions
├── lib/          client construction, formatters, constants
└── styles/       Tailwind entry and design tokens
```

**The rule that matters:** domain UI lives in `features/`, not in one sprawling
`components/` directory. If a component is named after a Ship entity, it is a
feature. If it could ship in any product, it is a component.

**`features/infrastructure` is the heart of the UI.** React Flow renders the
topology; clicking a node opens a configuration panel; dragging changes *desired
configuration only*. Nothing touches a server until Deploy is pressed.

**Never here:** YAML generation, Kamal knowledge, SSH, raw `fetch` calls to the API.

---

## `server` — the Go backend

```
server/
├── cmd/
│   ├── api/      HTTP API. Serves the UI's requests. Enqueues work.
│   ├── worker/   Consumes jobs. The ONLY binary that runs Kamal or opens SSH.
│   └── ship/     host CLI installed on the VPS (status, logs, upgrade, backup)
├── internal/
│   ├── domain/           shared entity types, zero dependencies
│   ├── projects/         ┐
│   ├── environments/     │  feature packages — one per domain area
│   ├── services/         │  each: service.go, repository.go, routes.go
│   ├── accessories/      │
│   ├── domains/          │
│   ├── volumes/          ┘
│   ├── configuration/    the desired-state engine — most important package
│   ├── monitoring/       metrics, health, drift detection
│   ├── audit/            append-only record of who did what
│   ├── kamal/            the ONLY package that knows Kamal exists
│   ├── ssh/              transport to remote hosts
│   └── platform/         config, database, redis, jobs, crypto, httpx
└── migrations/           GORM schema structs used for migration up and down
```

Packages are created together with their feature, not ahead of it. `servers/`,
`deployments/`, `logs/`, and `docker/` from the spec arrive with the tasks that
implement them.

### Why `api` and `worker` are separate binaries

A deployment takes minutes. An HTTP request must not wait on it (spec §24).
`POST /deployments` writes a row, enqueues a job, and returns an id immediately.
The worker picks it up. This split also means only the worker image needs the
Kamal runtime and Docker CLI — the API image stays small and shell-free.

### `internal/domain`

Types every other package agrees on. No SQL, no HTTP, no SSH, no Kamal. If two
feature packages need a type, it moves here. If only one does, it stays put.

### `internal/configuration` — the bridge

This is where the product lives.

```
model.go       DesiredState: what should be running (spec §54)
compiler.go    DB rows -> DesiredState, deterministic ordering
validator.go   port conflicts, orphan volumes, dependency cycles, missing secrets
versioning.go  immutable snapshots — v14, v15, v16
diff.go        structural comparison for the pre-deploy confirmation
renderer.go    DesiredState -> Kamal configuration
```

Two invariants: the UI never generates YAML, and the same DB state always
compiles to byte-identical output. If either breaks, diffs and drift detection
become untrustworthy.

### `internal/kamal` — the seam

Everything above depends on the `DeploymentEngine` interface, never on Kamal
itself. This is what makes the engine mockable in tests and replaceable later.

```
client.go  renderer.go  executor.go  deployment.go  logs.go  rollback.go
```

If you find yourself writing `exec.Command("kamal", ...)` anywhere else, stop.

### `internal/ssh`

Connections, command execution, file transfer, streaming, lifecycle. Ship is
agentless — nothing is installed on managed VPSs beyond Docker.

**Security rule:** commands come from a fixed allowlist of templated operations.
There is no route from the UI to a free-text shell, and V1 must not add one.

### `internal/platform`

Infrastructure concerns with no domain meaning: `config`, `database`, `redis`,
`jobs`, `crypto`, `httpx`. Feature packages import from here; nothing here
imports a feature package.

---

## `packages` — shared TypeScript

```
packages/
└── api-client/   typed HTTP client + generated types, from OpenAPI
```

`api-client` is transport and nothing else: `Next.js -> @ship/api-client -> HTTP -> Go API`.
No Kamal logic, no infrastructure execution. It also exports the generated
`components`/`paths` types, so it is the single schema package.

`api-client/src/generated` is **generated** — never hand-edited.
Run `pnpm generate`. CI fails if it drifts from the spec.

---

## `infra` — how Ship ships itself

```
infra/
├── compose/
│   ├── docker-compose.dev.yml   five-service local stack with source reload
│   └── docker-compose.yml       the five-container control plane
├── docker/                      api, worker, and web Dockerfiles
└── installer/                   install.sh behind get.ship.dev
```

E0 provides buildable API, worker, and web images. SH-010 extends the worker
image with the pinned Kamal runtime and Docker CLI; those tools must never be
added to the API image. The installer must be idempotent — re-running it can
never destroy data.

---

## `docs` and `scripts`

`docs/` holds architecture, API conventions, the security model, the
configuration-engine explainer, and the Ship-to-Kamal concept mapping.

`scripts/` holds codegen (`generate-client.sh`), seeding, and maintenance. Anything
you'd otherwise paste into a terminal twice belongs here.

---

## Dependency direction

```
apps/web  ->  packages/api-client  ->  HTTP  ->  cmd/api
                                                    |
                                          feature packages
                                                    |
                                          internal/configuration
                                                    |
                                            internal/kamal
                                                    |
                                       internal/ssh + internal/docker
```

Arrows point one way. `internal/domain` and `internal/platform` sit underneath
everything and depend on nothing above them. A cycle here means a design mistake.
