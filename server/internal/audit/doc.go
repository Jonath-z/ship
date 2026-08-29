// Package audit records who changed what and when (spec §53): configuration
// mutations, deploys, rollbacks, server changes, secret reveals, user
// management. Entries are append-only and never updated or deleted.
package audit
