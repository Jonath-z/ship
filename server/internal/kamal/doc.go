// Package kamal is the only place in Ship that knows Kamal exists.
//
// The rest of the system depends on the DeploymentEngine interface and does not
// care whether the implementation shells out to the Kamal CLI, calls a Ruby
// library, or talks to something else entirely (spec §23, §44).
//
// Files:
//
//	client.go      construction and configuration of the engine
//	renderer.go    thin wrapper delegating to internal/configuration
//	executor.go    process execution, streaming output, timeouts, locking
//	deployment.go  maps engine output onto the deployment state machine
//	logs.go        log retrieval for a running service
//	rollback.go    re-apply a previous image + configuration version
package kamal
