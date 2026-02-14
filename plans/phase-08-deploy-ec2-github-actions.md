# Phase 8: Deploy to EC2 using GitHub Actions CI/CD

**Goal**: Automate build, test, and deployment of the Avalon backend to an AWS EC2 instance via GitHub Actions. Every push runs CI; deployments run on push to `main` (or manual dispatch).

---

## Summary

- **CI**: On every push and PR — checkout, set up Go 1.24, build `./cmd/server`, run `go test ./...`. No secrets required.
- **CD**: On push to `main` (after CI passes) — build Linux amd64 binary, copy `avalon-server` and `migrations/` to EC2 via SSH, restart systemd service `avalon`. Migrations run on app startup (no separate step).
- **Secrets**: Stored in GitHub repo secrets; production app secrets (`DATABASE_URL`, `WEBSOCKET_TOKEN_SECRET`) live only on EC2 in an env file.

---

## GitHub Actions workflow

- **File**: `.github/workflows/deploy.yml`
- **Triggers**: `push` to `main`, `pull_request` to `main`, `workflow_dispatch`.
- **Jobs**:
  - **ci**: Build and test on all events. No deployment.
  - **deploy**: Runs only on `push` to `main` (not on PR). Depends on `ci`. Builds Linux binary, uploads artifact (binary + migrations), downloads it, configures SSH, copies files to EC2, restarts `avalon` service, optional smoke check of `/healthz`.

---

## GitHub repository secrets

Configure these in the repo: **Settings → Secrets and variables → Actions**.

| Secret          | Description                                              |
|-----------------|----------------------------------------------------------|
| `EC2_HOST`      | EC2 instance hostname or IP (e.g. `ec2-1-2-3-4.compute.amazonaws.com`) |
| `EC2_SSH_KEY`   | Private key contents for SSH (e.g. PEM for `ec2-user`)   |
| `DEPLOY_USER`   | SSH user (e.g. `ec2-user` for Amazon Linux)             |
| `DEPLOY_PATH`   | App directory on EC2 (e.g. `/home/ec2-user/avalon`)      |

Do **not** store `DATABASE_URL` or `WEBSOCKET_TOKEN_SECRET` in GitHub; keep them only on EC2 (see EC2 setup below).

---

## EC2 server setup (one-time manual)

1. **Instance**: Amazon Linux 2 or 2023. Ensure SSH (port 22) is open from GitHub Actions (e.g. GitHub’s IP ranges or a self-hosted runner in your VPC). If the instance has a public IP, you can use that for `EC2_HOST`; otherwise use a private IP with a runner in the VPC.

2. **App directory**: Create e.g. `/home/ec2-user/avalon`. The workflow will copy `avalon-server` and `migrations/` here.

3. **Environment file**: Create `/home/ec2-user/avalon/.env` with:
   - `DATABASE_URL` (required) — PostgreSQL connection string.
   - `WEBSOCKET_TOKEN_SECRET` (required in production) — secret for signing WebSocket tokens.
   - Optional: `AVALON_HTTP_ADDR=:8080`, `MIGRATIONS_DIR=migrations`.
   - Restrict permissions: `chmod 600 /home/ec2-user/avalon/.env`. Do not commit this file.

4. **Systemd unit**: Create `/etc/systemd/system/avalon.service`:

   ```ini
   [Unit]
   Description=Avalon backend
   After=network.target

   [Service]
   Type=simple
   User=ec2-user
   WorkingDirectory=/home/ec2-user/avalon
   EnvironmentFile=/home/ec2-user/avalon/.env
   ExecStart=/home/ec2-user/avalon/avalon-server
   Restart=on-failure
   RestartSec=5

   [Install]
   WantedBy=multi-user.target
   ```

   Then:
   - `sudo systemctl daemon-reload`
   - `sudo systemctl enable avalon`
   - `sudo systemctl start avalon`

5. **SSH key**: Ensure the key whose private part is stored in `EC2_SSH_KEY` is authorized for `DEPLOY_USER` on the instance (e.g. in `~/.ssh/authorized_keys`). The deploy job uses this key to `scp` and `ssh`; it must be able to run `sudo systemctl restart avalon` (so `DEPLOY_USER` may need passwordless sudo for that command, or use a dedicated deploy user with sudo).

6. **PostgreSQL**: Must be reachable from EC2 (e.g. RDS in the same VPC, or a Postgres instance on the same or another host). Set `DATABASE_URL` in `.env` accordingly.

---

## Deployment flow (what the workflow does)

1. **Build**: In the deploy job, build Linux amd64 binary: `GOOS=linux GOARCH=amd64 go build -o avalon-server ./cmd/server`.
2. **Copy**: Upload binary and `migrations/` as an artifact; download in the same job; `scp` to `$DEPLOY_USER@$EC2_HOST:$DEPLOY_PATH/`.
3. **Migrations**: The app runs migrations on startup (`cmd/server/main.go`); no separate migrate step. Restarting the service is enough.
4. **Restart**: `ssh ... 'sudo systemctl restart avalon'`.
5. **Smoke check**: Optional `curl -f http://$EC2_HOST:8080/healthz` (may be skipped if the host is not publicly reachable on 8080).

---

## Optional follow-ups (Phase 8b and beyond)

- **Docker on EC2**: Add a Dockerfile (multi-stage build), push image to ECR or GHCR, and have EC2 pull and run the container instead of a bare binary. Requires Docker on EC2 and a way to pass env (e.g. env file or AWS Secrets Manager).
- **SSM instead of SSH**: Use AWS Systems Manager Session Manager and S3 (or artifact bucket) to copy the binary and run commands, so no SSH key in GitHub. EC2 needs an IAM role and SSM agent.
- **Secrets on EC2**: Use AWS Secrets Manager or SSM Parameter Store with an IAM role so the app fetches `DATABASE_URL` and `WEBSOCKET_TOKEN_SECRET` at startup instead of a static `.env` file.
- **Blue/green or rollback**: Keep the previous binary; after deploy, hit `/healthz` and roll back + restart if it fails.

---

## References

- `.github/workflows/deploy.yml` — CI and CD workflow.
- `cmd/server/main.go` — entrypoint; reads env and runs migrations on startup.
- `plans/README.md` — phase index.

---

## Acceptance criteria

- [ ] Pushing to `main` runs tests and, on success, deploys the Linux binary to EC2 and restarts the service.
- [ ] Server runs with existing behavior; `GET /healthz` returns 200 after deploy.
- [ ] Production secrets (`DATABASE_URL`, `WEBSOCKET_TOKEN_SECRET`) are not stored in GitHub; they live only on EC2 (e.g. in `.env` or env file for systemd).
