# Ship security model

Reviewed for E2 on 2026-08-30.

Ship controls deployment credentials, Docker, SSH access, and application
secrets. Authentication is therefore safe for production only when the public
URL uses HTTPS. Ship does not install a TLS proxy: operators terminate HTTPS in
their own ingress and configure the exact origin with `SHIP_PUBLIC_URL`.
Direct `http://SERVER_IP:3000` access is an explicitly insecure bootstrap mode.

## Review checklist

- [x] Passwords use uniquely salted Argon2id hashes; plaintext passwords are
      never persisted or logged.
- [x] Browser cookies contain only opaque random session identifiers. Session
      state is held in Redis and expires on both idle and absolute deadlines.
- [x] Production cookies use `Secure`, `HttpOnly`, `SameSite=Strict`, `Path=/`,
      and the `__Host-` prefix.
- [x] Every state-changing authenticated API request requires the exact public
      origin and a token bound to that server-side session.
- [x] Failed logins are limited independently by normalized account and source
      IP. Setup is also rate limited; deployment execution must use the same shared
      Redis limiter when SH-070 adds those endpoints.
- [x] Web and API responses set CSP, frame, content-type, referrer, and browser
      permissions headers. HSTS is emitted only for an HTTPS public URL.
- [x] Every API operation declares `x-ship-permission` in OpenAPI and is
      registered through the permission-aware Gin router.
- [x] Owner, admin, deployer, and viewer grants are centralized. The final
      active owner cannot be disabled or demoted.
- [x] Sensitive values use per-record data keys and AES-256-GCM envelope
      encryption. The master-key keyring is mounted from outside PostgreSQL.
- [x] Online rotation keeps old and new keys available until every data key is
      rewrapped, then removes the old key.
- [x] Audit entries are append-only through the application and never include
      passwords, session/CSRF tokens, master keys, or secret plaintext.
- [x] No API accepts a command, shell script, or free-form command arguments.
      SSH commands are selected from `internal/ssh` typed operations and rendered
      as executable/argument vectors.

## Non-negotiable SSH invariant

SH-040 will implement the SSH transport later. That transport must accept only
the typed `ssh.Command` produced by the allowlist. Adding a raw-shell endpoint,
passing UI text to a shell, or invoking a command outside the allowlist fails
the E2 security contract and must block review.

## Key and backup handling

The production keyring is `/opt/ship/keys/keyring`, readable only by the Ship
service account and root. `ship rotate-master-key` stages both keys, rewraps
online, and removes the old key only after success. Ship backups include the
keyring so they remain restorable; backup archives are mode `0600` and must be
stored as sensitive credentials because an archive contains both encrypted data
and the keys needed to restore it.
