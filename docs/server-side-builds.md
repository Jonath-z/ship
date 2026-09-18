# Server-side builds and GitHub auto-deploy

Ship deploys a service one of two ways, decided by how the service is
configured:

- **Image-backed** — the service has an `image`. Ship renders the
  configuration and Kamal pulls that image on the target servers. Nothing is
  built.
- **Repository-backed** — the service has a `repository` (and optionally a
  `branch`) and no image. Ship clones the repository, builds it with the
  Dockerfile at its root, pushes the image to the configured registry, and
  deploys it. The deployment moves through `BUILDING → PUSHING → DEPLOYING`
  and records the built commit SHA.

## What a repository-backed service needs

| Requirement | Where it lives |
| --- | --- |
| HTTPS repository URL, e.g. `github.com/acme/webapp` | Service `repository` field. SSH remotes (`git@…`) are rejected. |
| A `Dockerfile` at the repository root | The application repository. |
| Registry credentials | Environment secrets `KAMAL_REGISTRY_USERNAME` and `KAMAL_REGISTRY_PASSWORD`, plus the `KAMAL_REGISTRY_SERVER` variable for non-Docker-Hub registries. Validation blocks the deploy without them — the build must push somewhere. |
| `GIT_TOKEN` secret (private repositories only) | Environment secrets. A GitHub personal access token with read access to the repository. Public repositories need nothing. |

The image is published as
`<registry>/<project>-<environment>-<service>:<commit sha>` and the deploy is
pinned to that exact tag. Rollbacks reactivate the source deployment's SHA.

Deploy-time credentials (`KAMAL_REGISTRY_*`, `GIT_TOKEN`,
`GITHUB_WEBHOOK_SECRET`) never reach application container env; the renderer
strips them.

The clone is shallow and lives only inside the per-deployment workspace,
which is removed when the run finishes. The token is fed to git through an
askpass helper — it never appears in URLs, process arguments, or deploy logs.

## Auto-deploy on push (GitHub webhook)

Ship exposes `POST /webhooks/github`. The route is public; authentication is
GitHub's HMAC signature, verified per environment. Setup:

1. Generate a random secret (`openssl rand -hex 32`).
2. Store it in Ship as the environment secret `GITHUB_WEBHOOK_SECRET`.
3. In the GitHub repository: **Settings → Webhooks → Add webhook**
   - Payload URL: `http://<your-ship-host>:3000/webhooks/github` (route it
     through your ingress if the API is not directly reachable)
   - Content type: `application/json`
   - Secret: the same value
   - Events: just the push event.

On every push, Ship queues a deployment for each repository-backed service
whose repository matches the pushed GitHub repository and whose branch
matches the pushed branch (a service with no branch follows the repository's
default branch). Tag pushes and branch deletions are ignored. Environments
without a verifying `GITHUB_WEBHOOK_SECRET` are skipped, and unverified
callers always receive the same shape of response.
