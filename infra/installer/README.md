The one-command installer (spec §1, SH-012):

    curl -fsSL https://get.ship.dev | bash

What `install.sh` does, in order:

1. Detect OS and architecture; refuse politely on anything unsupported.
2. Install Docker if it is missing.
3. Create `/opt/ship`, generate `SHIP_MASTER_KEY` and `SHIP_SESSION_SECRET`, write `.env`.
4. Pull images and start the compose stack.
5. Provision TLS for the chosen hostname, with a self-signed fallback for IP-only installs.
6. Print the dashboard URL and a single-use first-run token.

Requirements:
- Idempotent. Re-running must never destroy data.
- Tested on fresh Ubuntu 22.04, Ubuntu 24.04, and Debian 12.
