# API conventions

The Ship API uses Gin and JSON over HTTP. Routes are registered directly on the
Gin engine. New endpoints use the response helpers in
`server/internal/platform/httpx`; handlers must not invent response envelopes.

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
Validation failures use `400`; unauthenticated and unauthorized requests use
`401` and `403`; missing resources use `404`; conflicts use `409`; rate limits
use `429`; unexpected failures use `500`.

Every response includes `X-Request-ID`. A valid caller-provided request ID is
preserved; otherwise the API generates one. Structured logs include the same ID.

## Pagination

List endpoints use cursor pagination:

```text
GET /api/v1/projects?limit=20&cursor=<opaque-value>
```

`limit` defaults to 20 and must be between 1 and 100. List responses use:

```json
{ "data": [], "page": { "nextCursor": null } }
```

Cursors are opaque to clients and must not be constructed or parsed outside the
API client.

## Contract

The Go-owned OpenAPI document is
`server/internal/platform/httpx/openapi.yaml`. The API exposes it at
`GET /openapi.yaml`; `pnpm generate` updates `@ship/types` and the generated
schema used by `@ship/api-client`. Generated files are checked for drift in CI.
