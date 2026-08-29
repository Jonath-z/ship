// Package servers owns Server registration, connection testing, preparation,
// and role membership (spec §11, §12, §38).
//
// It decides *what* to do with a server. It does not open SSH connections
// itself — that is internal/ssh.
package servers
