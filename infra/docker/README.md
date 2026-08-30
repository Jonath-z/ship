Dockerfiles for the three Ship production images used by the E1 installer.

- `api.Dockerfile` — static Go binary on distroless. Small, no shell.
- `worker.Dockerfile` — Go worker with pinned Kamal, Docker CLI, Git, and SSH;
  this remains the only image allowed to carry deployment tooling.
- `web.Dockerfile` — Next.js standalone output.

All images are multi-arch (amd64 + arm64), run as non-root, expose a container
health check, contain no baked-in secrets, and are kept below 200 MB. Exact
build and data-service versions live in `infra/versions.env`.
