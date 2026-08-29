// Package httpx holds shared HTTP concerns: the error envelope, request IDs,
// pagination, auth middleware, CSRF, rate limiting, and security headers.
// Every handler wraps through here so conventions stay consistent.
package httpx
