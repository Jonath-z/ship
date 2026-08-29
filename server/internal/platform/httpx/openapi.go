package httpx

import _ "embed"

// OpenAPISpec is served by the API and consumed by the TypeScript generator.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
