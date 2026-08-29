// Package ssh is the transport layer for everything Ship does on a remote host
// (spec §45): connecting, executing commands, transferring files, streaming
// output, and managing connection lifecycle.
//
// Ship is agentless by design — nothing is installed on managed VPSs beyond
// Docker itself (§38).
//
// Security rule (SH-025): commands are built from a fixed allowlist of
// templated operations. There is no path from the UI to a free-text shell.
//
//	client.go   connection pooling, auth, host key verification
//	keys.go     keypair generation, import, encrypted storage
//	command.go  templated command construction and execution
//	session.go  long-lived sessions and streaming
package ssh
