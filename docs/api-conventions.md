# API conventions

The Ship API uses Gin and JSON over HTTP. Routes are registered through the
permission-aware router in `server/internal/platform/httpx`; every route must
declare an access permission. New endpoints use the shared response helpers and
must not invent response envelopes.

The public browser surface is the Next.js application. Explicit handlers under
`/api` forward allowlisted requests to the private Gin API. Gin paths are
unversioned in V1: the browser calls `/api/projects`, while the internal Gin
resource path is `/projects`.

## Errors

Every error has one envelope:

```json
{
  "error": {
    "code": "validation_error",
    "message": "request validation failed",
    "requestId": "8ea8b98049af4da2a763df5ed6ada510",
    "details": [
      { "field": "name", "code": "required", "message": "is required" }
    ]
  }
}
```

Stable, machine-readable `code` values use snake case. `message` is safe to show
to a person. Internal errors are logged but never copied into a response.
Malformed JSON uses `400`; field validation uses `422`; unauthenticated and
unauthorized requests use `401` and `403`; missing resources use `404`;
conflicts use `409`; rate limits use `429`; unexpected failures use `500`.

Every response includes `X-Request-ID`. A valid caller-provided request ID is
preserved; otherwise the API generates one. Structured logs include the same ID.

## Pagination

List endpoints use cursor pagination:

```text
GET /projects?limit=20&cursor=<opaque-value>
```

`limit` defaults to 20 and must be between 1 and 100. List responses use:

```json
{ "items": [], "nextCursor": "opaque-or-omitted" }
```

Cursors are opaque to clients and must not be constructed or parsed outside the
API client.

## Mutations

Partial updates use `PATCH`. Mutations require the exact configured origin and
the session-bound CSRF token. A destructive confirmation must be enforced by
the API contract as well as the UI. Sensitive reveal actions use a
CSRF-protected `POST`, never `GET`.

## Contract

The Go-owned OpenAPI document is
`server/internal/platform/httpx/openapi.yaml`. The API exposes it at
`GET /openapi.yaml`; `pnpm generate` updates the generated schema used by
`@ship/api-client`. The generated file is checked for drift in CI.
