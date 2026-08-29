// Package redis provides the Redis client used for jobs, locks, transient
// state, event propagation, and caching (spec §49).
//
// Rule: desired infrastructure state never goes in Redis.
package redis
