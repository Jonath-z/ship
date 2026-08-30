The one-command installer (spec §1, SH-012):

    curl -fsSL https://get.ship.dev | sudo bash

To label the installation with a specific hostname:

    curl -fsSL https://get.ship.dev | sudo bash -s -- --hostname ship.example.com

What `install.sh` does, in order:

1. Detect OS and architecture; refuse politely on anything unsupported.
2. Install Docker if it is missing.
3. Create `/opt/ship`, generate `SHIP_MASTER_KEY` and `SHIP_SESSION_SECRET`, write `.env`.
4. Pull images and start the compose stack.
5. Print the `http://SERVER_IP:3000` dashboard URL and a single-use first-run token.

The host command is installed at `/usr/local/bin/ship`:

    ship status
    ship logs
    ship upgrade v0.2.0
    ship backup
    ship restore /path/to/backup.tar.gz
    ship public-url https://ship.example.com
    ship rotate-master-key

Ship's five containers and named volumes live under `/opt/ship`. The installer
does not install a reverse proxy or configure TLS. The dashboard is published
on port 3000; API, PostgreSQL, and Redis access remain private.

The initial HTTP URL is bootstrap-only. After configuring an external HTTPS
proxy or tunnel, `ship public-url` sets the exact browser origin used for
same-origin checks and secure cookies. `ship rotate-master-key` keeps both
encryption keys available while it rewraps every per-record data key, and drops
the old key only after the operation succeeds.

For a local installer smoke test without changing system packages:

    SHIP_INSTALL_SOURCE_DIR="$PWD" \
      SHIP_INSTALL_DIR=/tmp/ship-install \
      SHIP_BIN_DIR=/tmp/ship-bin \
      SHIP_SKIP_SYSTEM_PACKAGES=1 \
      SHIP_SKIP_START=1 \
      SHIP_ALLOW_NON_ROOT=1 \
      SHIP_ALLOW_UNSUPPORTED_OS=1 \
      bash infra/installer/install.sh --hostname 127.0.0.1

Requirements:

- Idempotent. Re-running must never destroy data.
- Tested on fresh Ubuntu 22.04, Ubuntu 24.04, and Debian 12.
