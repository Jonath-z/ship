Dockerfiles for the three Ship images. E0 keeps each image buildable in CI;
SH-010 adds the production packaging details required by the installer.

- `api.Dockerfile` — static Go binary on distroless. Small, no shell.
- `worker.Dockerfile` — static Go worker scaffold. SH-010 adds the pinned Kamal
  runtime and Docker CLI; this remains the only image allowed to carry them.
- `web.Dockerfile` — Next.js standalone output.

All images: multi-arch (amd64 + arm64), non-root user, no secrets baked in.
