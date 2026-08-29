// Package crypto implements envelope encryption for secrets, SSH private keys,
// and Git tokens (spec §16, §53), plus key rotation. Key material lives outside
// the database so a raw dump reveals nothing.
package crypto
