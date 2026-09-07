# V1 golden-path checklist

The manual release gate that replaces automated cloud E2E (SH-160/SH-161) in
V1. Run it against a fresh VPS before tagging a release. Every step must pass;
record the release tag and date at the bottom when it does.

Prerequisites: a fresh Ubuntu 22.04/24.04 or Debian 12 VPS (the control
plane), a second VPS to deploy onto (the target), and a public container
image to deploy (for example `traefik/whoami`).

## Install and first run

1. [ ] `curl -fsSL https://get.ship.dev | sudo bash` on the control plane
       completes and prints the dashboard URL and a first-run token.
2. [ ] Re-running the installer prints "Existing installation updated" and
       does not print a new token.
3. [ ] `/setup` creates the owner account with the token and signs in;
       revisiting `/setup` afterwards is rejected.
4. [ ] `ship status` shows five healthy containers.

## Server onboarding

5. [ ] Generate an SSH key in the dashboard; install its public key on the
       target VPS (`root` or a sudo user).
6. [ ] Register the target server; connection checks report SSH ✓ and
       Docker ✗ on a bare host.
7. [ ] "Prepare server" installs Docker; re-checks turn the server
       `connected` with OS, architecture, and resources filled in.
8. [ ] The recorded host key is enforced: re-running checks succeeds without
       prompting; a rebuilt VPS at the same address is rejected until the
       address is re-saved.

## Configure and deploy

9. [ ] Create a project and environment; create a service from a public
       registry image with a port; add the server to the service's role group.
10. [ ] Add an environment variable and a secret; the secret shows masked and
        reveal is audited (visible in Settings → audit log).
11. [ ] The configuration preview shows valid rendered Kamal YAML with the
        secret listed by name only; validation is clean.
12. [ ] Deploy: the confirmation shows the diff, live logs stream during the
        run, and the deployment ends `SUCCESS`.
13. [ ] The app answers on the target server (port or domain, per config).
14. [ ] Deploy again after changing an environment variable; the pre-deploy
        diff shows exactly that change.

## Rollback and failure

15. [ ] Roll back to the first deployment; status flows ROLLING_BACK →
        ROLLED_BACK and the previous behaviour is restored.
16. [ ] Break the service (wrong image tag), deploy, and confirm the run ends
        `FAILED` with the Kamal error visible in the logs; the environment
        deploys again cleanly after fixing the tag.
17. [ ] Restart the worker container mid-deploy; the deployment is marked
        FAILED, not stuck.

## Multi-server role (SH-161 essentials)

18. [ ] Add a second target server to the same role; deploy; both hosts run
        the service and appear in the rendered configuration.

## Operations

19. [ ] `ship backup` produces an archive; `ship restore` on a scratch
        control plane reproduces projects, servers, and secrets.
20. [ ] `ship upgrade vX.Y.Z` to the release candidate preserves all data.

| Release | Date | Operator | Result |
| ------- | ---- | -------- | ------ |
|         |      |          |        |
