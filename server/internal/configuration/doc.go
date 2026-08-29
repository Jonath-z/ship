// Package configuration is the bridge between Ship's product model and Kamal.
//
// This is the most important package in the codebase (spec §18, §43). Nothing
// outside it may construct Kamal YAML, and the web UI never generates YAML at
// all — it mutates the desired-state model, and this package renders it.
//
// Pipeline:
//
//	database records
//	    -> compiler.go   builds the desired-state document
//	    -> validator.go  rejects impossible states before anything runs
//	    -> versioning.go snapshots it as an immutable version (v14, v15, ...)
//	    -> diff.go       compares two versions for the pre-deploy confirmation
//	    -> renderer.go   emits Kamal configuration into a deploy workspace
//
// File responsibilities:
//
//	model.go       the desired infrastructure state (§54)
//	compiler.go    normalized DB rows -> desired state, deterministic ordering
//	validator.go   port conflicts, orphaned volumes, dependency cycles, ...
//	versioning.go  immutable snapshots, one per meaningful mutation
//	diff.go        structural diff, secret values never included
//	renderer.go    desired state -> Kamal config
package configuration
