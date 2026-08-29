Dockerfiles for the three Ship images (SH-010).

- `api.Dockerfile` — static Go binary on distroless. Small, no shell.
- `worker.Dockerfile` — Go binary **plus a pinned Kamal runtime** and Docker CLI.
  This is the only image that needs Ruby. Version is asserted at startup (SH-062).
- `web.Dockerfile` — Next.js standalone output.

All images: multi-arch (amd64 + arm64), non-root user, no secrets baked in.
