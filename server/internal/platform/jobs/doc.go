// Package jobs is the Redis-backed queue and worker runtime (spec §24, §51):
// enqueue, dequeue, retries with backoff, dead-lettering, and per-environment
// locks so two deployments to the same environment never overlap.
//
// Rule: an HTTP request never waits on a job. POST /deployments returns an id.
package jobs
