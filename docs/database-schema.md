# Database schema

PostgreSQL is Ship's source of truth. The executable schema is defined by the
GORM structs in `server/migrations`; Ship generates migrations from those
structs and does not maintain handwritten migration SQL.

The schema includes the complete V1 table inventory from `docs.md` section 49.
`server_group_memberships` materializes the server-to-role relationship, and
`vault_entries` stores encrypted payloads for secrets and other protected
credentials. Tables needed by later epics are present here as schema only;
their application behavior remains in those epics.

## Desired-state relationships

```mermaid
erDiagram
    direction LR

    PROJECT ||--o{ ENVIRONMENT : contains
    ENVIRONMENT ||--o{ SERVER_GROUP : defines
    SERVER_GROUP ||--o{ SERVER_GROUP_MEMBERSHIP : contains
    SERVER ||--o{ SERVER_GROUP_MEMBERSHIP : joins
    SERVER_GROUP o|..o{ SERVICE : places
    ENVIRONMENT ||--o{ SERVICE : owns
    ENVIRONMENT ||--o{ ACCESSORY : owns
    SERVER o|..o{ ACCESSORY : hosts
    SERVER_GROUP o|..o{ ACCESSORY : hosts
    SERVICE ||--o{ DOMAIN : exposes
    SERVICE o|..o{ VOLUME : owns
    ACCESSORY o|..o{ VOLUME : owns
    ENVIRONMENT ||--o{ ENV_VAR : defines
    SERVICE o|..o{ ENV_VAR : overrides
    ENVIRONMENT ||--o{ SECRET : defines
    SERVICE o|..o{ SECRET : overrides
    SECRET o|..o| VAULT_ENTRY : encrypts_as
    SERVICE ||--o{ SERVICE_DEPENDENCY : originates
    SERVICE o|..o{ SERVICE_DEPENDENCY : targets
    ACCESSORY o|..o{ SERVICE_DEPENDENCY : targets

    PROJECT {
        uuid id PK
        string name
        string slug UK
        datetime created_at
        datetime updated_at
    }
    ENVIRONMENT {
        uuid id PK
        uuid project_id FK
        string name
        string slug UK
    }
    SERVER {
        uuid id PK
        string name UK
        string hostname
        string ip_address
        string status
        json resources
    }
    SERVER_GROUP {
        uuid id PK
        uuid environment_id FK
        string name UK
    }
    SERVER_GROUP_MEMBERSHIP {
        uuid server_group_id PK, FK
        uuid server_id PK, FK
    }
    SERVICE {
        uuid id PK
        uuid environment_id FK
        uuid server_group_id FK "Nullable"
        string name UK
        string repository
        string image
        int port "Nullable"
    }
    ACCESSORY {
        uuid id PK
        uuid environment_id FK
        uuid server_id FK "Nullable"
        uuid server_group_id FK "Nullable"
        string name UK
        string type "postgres | redis"
        string image
    }
    VOLUME {
        uuid id PK
        uuid environment_id FK
        uuid service_id FK "Exclusive owner"
        uuid accessory_id FK "Exclusive owner"
        string source UK
        string destination
    }
    DOMAIN {
        uuid id PK
        uuid environment_id FK
        uuid service_id FK
        string hostname UK
        bool ssl_enabled
    }
    ENV_VAR["Environment Variable"] {
        uuid id PK
        uuid environment_id FK
        uuid service_id FK "Nullable override"
        string name UK
        string value
    }
    SECRET {
        uuid id PK
        uuid environment_id FK
        uuid service_id FK "Nullable override"
        string name UK
    }
    VAULT_ENTRY["Vault Entry"] {
        uuid id PK
        uuid secret_id FK "Nullable for non-secret credentials"
        string scope_type
        uuid scope_id
        bytes ciphertext
        bytes wrapped_dek
    }
    SERVICE_DEPENDENCY["Service Dependency"] {
        uuid id PK
        uuid environment_id FK
        uuid source_service_id FK
        uuid target_service_id FK "Exclusive target"
        uuid target_accessory_id FK "Exclusive target"
        string type
    }
```

An accessory may be unplaced until E4, but it cannot target both a server and a
server group. A volume has exactly one owner, and a dependency has exactly one
target. Service-layer validation will additionally reject cross-environment
references and dependency cycles when those CRUD operations are introduced.

## Configuration and operations relationships

```mermaid
erDiagram
    direction LR

    USER o|..o{ CONFIGURATION_VERSION : authors
    USER o|..o{ BACKUP : creates
    USER o|..o{ AUDIT_LOG : acts_in
    ENVIRONMENT ||--o| CONFIGURATION : tracks
    CONFIGURATION ||--o{ CONFIGURATION_VERSION : versions
    ENVIRONMENT ||--o{ DEPLOYMENT : runs
    SERVICE ||--o{ DEPLOYMENT : deploys
    CONFIGURATION_VERSION ||--o{ DEPLOYMENT : snapshots
    DEPLOYMENT o|..o{ DEPLOYMENT : rolls_back_from
    DEPLOYMENT ||--o{ DEPLOYMENT_LOG : emits
    ENVIRONMENT o|..o{ BACKUP : scopes

    USER {
        uuid id PK
        string email UK
        string role
        datetime disabled_at "Nullable"
    }
    ENVIRONMENT {
        uuid id PK
        uuid project_id FK
        string slug
    }
    SERVICE {
        uuid id PK
        uuid environment_id FK
        string name
    }
    CONFIGURATION {
        uuid id PK
        uuid environment_id FK, UK
        int current_version
    }
    CONFIGURATION_VERSION["Configuration Version"] {
        uuid id PK
        uuid configuration_id FK
        int version UK
        json document
        uuid actor_user_id FK "Nullable"
        datetime created_at
    }
    DEPLOYMENT {
        uuid id PK
        uuid environment_id FK
        uuid service_id FK
        uuid configuration_version_id FK
        uuid source_deployment_id FK "Nullable"
        string status
    }
    DEPLOYMENT_LOG["Deployment Log"] {
        uuid id PK
        uuid deployment_id FK
        int sequence UK
        string stream
        text message
    }
    BACKUP {
        uuid id PK
        string kind
        uuid environment_id FK "Nullable for control-plane backup"
        string status
        string storage_path
    }
    AUDIT_LOG["Audit Log"] {
        uuid id PK
        uuid actor_user_id FK "Nullable"
        string action
        string resource_type
        string outcome
        json metadata
        datetime created_at
    }
```

Configuration versions and audit entries are immutable through GORM. Deleting
a project cascades through its environments and environment-owned records;
global servers and users remain. Redis is limited to jobs, locks, cache,
sessions, rate limits, temporary state, and event propagation—it never stores
this desired state.

## Verification

The database integration test starts from an empty schema, applies every GORM
model, checks the full table inventory, exercises representative constraints
and foreign keys, verifies the project cascade, and rolls the schema back.
Because it drops every Ship table, point it only at a disposable test database.

```sh
SHIP_TEST_DATABASE_URL='postgres://ship:ship@localhost:5432/ship?sslmode=disable' \
  go test -count=1 ./server/internal/platform/database
```
